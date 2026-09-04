// Package integration 通过本地模拟各厂商端点，端到端验证每个通知渠道的
// 真实发送管线（AES 解密配置 → 模板渲染 → SMTP/HTTP 发送）与载荷格式。
// 不依赖外部网络；将渠道配置指向本地 sink 即可验证载荷是否与厂商规范一致。
package integration

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"notice-service/internal/channel"
	"notice-service/internal/crypto"
	"notice-service/internal/database"
	"notice-service/internal/model"
	"notice-service/internal/service"
)

const testDSN = "notice:notice123@tcp(127.0.0.1:3306)/notice_service_test?parseTime=true&charset=utf8mb4&loc=Local"

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", testDSN)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("connect local mariadb failed: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	uname := fmt.Sprintf("itg_%d", time.Now().UnixNano())
	res, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, 'h', 'user')", uname)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM users WHERE id=?", id) })
	return id
}

func key32() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

type noopSched struct{}

func (noopSched) RegisterTask(int64, string) {}
func (noopSched) UnregisterTask(int64)       {}

// ---- 本地 SMTP sink（127.0.0.1，纯文本，无需 TLS —— PlainAuth 对 localhost 放行）----

type smtpSink struct {
	ln   net.Listener
	mu   sync.Mutex
	msgs []string
}

func newSMTPSink(t *testing.T) *smtpSink {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &smtpSink{ln: ln}
	go s.accept()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *smtpSink) addr() string { return s.ln.Addr().String() }

func (s *smtpSink) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *smtpSink) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	write := func(code int, msg string) {
		fmt.Fprintf(w, "%d %s\r\n", code, msg)
		_ = w.Flush()
	}
	write(220, "sink ESMTP ready")
	var data []byte
	inData := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				inData = false
				s.mu.Lock()
				s.msgs = append(s.msgs, string(data))
				s.mu.Unlock()
				data = nil
				write(250, "OK")
			} else {
				data = append(data, line...)
				data = append(data, '\n')
			}
			continue
		}
		fields := strings.SplitN(line, " ", 2)
		cmd := strings.ToUpper(fields[0])
		switch cmd {
		case "EHLO":
			// 多行响应：首行/中间行用 "250-"（连字符表示还有后续），末行用 "250 "
			fmt.Fprint(w, "250-sink\r\n")
			fmt.Fprint(w, "250-AUTH PLAIN\r\n")
			fmt.Fprint(w, "250 SIZE 10485760\r\n")
			_ = w.Flush()
		case "HELO":
			write(250, "sink")
		case "AUTH":
			write(235, "2.7.0 Authentication successful")
		case "MAIL", "RCPT":
			write(250, "OK")
		case "DATA":
			write(354, "End data with <CR><LF>.<CR><LF>")
			inData = true
		case "QUIT":
			write(221, "Bye")
			return
		default:
			write(250, "OK")
		}
	}
}

func (s *smtpSink) lastMessage() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.msgs) == 0 {
		return ""
	}
	return s.msgs[len(s.msgs)-1]
}

// ---- 通用 webhook sink：捕获请求，可自定义响应 ----

type hookSink struct {
	mu     sync.Mutex
	bodies []string
	urls   []string
	forms  []url.Values
	resp   map[string]interface{} // 返回的 JSON
	srv    *httptest.Server
}

func newHookSink(t *testing.T, resp map[string]interface{}) *hookSink {
	t.Helper()
	h := &hookSink{resp: resp}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		if ct := r.Header.Get("Content-Type"); strings.Contains(ct, "application/x-www-form-urlencoded") {
			_ = r.ParseForm() // 必须先解析再读 body，否则 form 为空
			h.forms = append(h.forms, r.PostForm)
			h.bodies = append(h.bodies, "form")
		} else {
			b, _ := io.ReadAll(r.Body)
			h.bodies = append(h.bodies, string(b))
		}
		h.urls = append(h.urls, r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.resp)
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *hookSink) url() string { return h.srv.URL }

func (h *hookSink) lastBody() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.bodies) == 0 {
		return ""
	}
	return h.bodies[len(h.bodies)-1]
}

func (h *hookSink) lastURL() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.urls) == 0 {
		return ""
	}
	return h.urls[len(h.urls)-1]
}

func (h *hookSink) lastForm() url.Values {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.forms) == 0 {
		return nil
	}
	return h.forms[len(h.forms)-1]
}

// ---- 用例骨架：建渠道/模板/任务并通过真实发送管线触发 ----

type fixture struct {
	db      *sql.DB
	ns      *service.NotificationService
	userID  int64
	chID    int64
	tplID   int64
	taskID  int64
	subject string
	content string
}

// buildFixture 创建 AES 加密的渠道、模板与任务，返回可触发 SendTask 的夹具。
// cfg 为渠道明文配置（webhook_url 指向本地 sink 等）。
func buildFixture(t *testing.T, chType string, cfg map[string]string) *fixture {
	t.Helper()
	db := openDB(t)
	ciph, err := crypto.New(key32())
	if err != nil {
		t.Fatal(err)
	}
	uid := seedUser(t, db)

	chSvc := service.NewChannelService(db, ciph)
	ch := &model.Channel{Type: chType, Name: "联调渠道", Config: cfg, Enabled: true}
	if err := chSvc.Create(uid, ch); err != nil {
		t.Fatal(err)
	}

	tplSvc := service.NewTemplateService(db)
	tpl := &model.Template{
		Name:      "会议提醒",
		Subject:   "会议 {{time}}",
		ContentMD: "## 标题\n\n大家好 **{{name}}**，明天 {{time}} 开会",
		Variables: []model.TemplateVar{{Name: "name", Default: "张三"}, {Name: "time", Default: "10:00"}},
		Enabled:   true,
	}
	if err := tplSvc.Create(uid, tpl); err != nil {
		t.Fatal(err)
	}

	taskSvc := service.NewTaskService(db, noopSched{})
	tk := &model.Task{
		Name: "联调任务", ChannelID: ch.ID, TemplateID: tpl.ID,
		TriggerType: "api", Receivers: []string{"zhangsan@x.com"}, Enabled: true,
	}
	if err := taskSvc.Create(uid, tk); err != nil {
		t.Fatal(err)
	}
	// 清理夹具数据：避免遗留的联调任务与发送日志累积在测试库中，
	// 干扰其他包对 task_logs 全量计数的断言（如日志查询分页测试）。
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM task_logs WHERE task_id=?", tk.ID)
		_, _ = db.Exec("DELETE FROM tasks WHERE id=?", tk.ID)
		_, _ = db.Exec("DELETE FROM channels WHERE id=?", ch.ID)
		_, _ = db.Exec("DELETE FROM templates WHERE id=?", tpl.ID)
	})

	ns := service.NewNotificationService(db, ciph)
	return &fixture{
		db: db, ns: ns, userID: uid, chID: ch.ID, tplID: tpl.ID, taskID: tk.ID,
		subject: "会议 10:00", content: "## 标题\n\n大家好 **张三**，明天 10:00 开会",
	}
}

// ---- 各渠道端到端验证 ----

func TestIntegrationEmailSMTP(t *testing.T) {
	sink := newSMTPSink(t)
	// 本地假 SMTP sink 不支持 STARTTLS：仅此测试放开明文认证（生产渠道默认拒绝）。
	fx := buildFixture(t, "email", map[string]string{
		"host": "127.0.0.1", "port": strings.SplitN(sink.addr(), ":", 2)[1],
		"username": "u", "password": "p", "from": "a@x.com", "allow_insecure": "true",
	})
	if err := fx.ns.SendTask(fx.taskID, map[string]string{}, service.Trigger{}); err != nil {
		t.Fatalf("email send: %v", err)
	}
	msg := sink.lastMessage()
	if msg == "" {
		t.Fatal("SMTP sink received no message")
	}
	for _, want := range []string{
		"From: a@x.com",
		"To: zhangsan@x.com",
		"Subject: " + fx.subject,
		"Content-Type: text/html",
		"<h2", // AutoHeadingIDs 会渲染成 <h2 id="标题">…</h2>
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("email message missing %q; got:\n%s", want, msg)
		}
	}
}

func TestIntegrationWecom(t *testing.T) {
	sink := newHookSink(t, map[string]interface{}{"errcode": 0, "errmsg": "ok"})
	fx := buildFixture(t, "wecom", map[string]string{"webhook_url": sink.url()})
	if err := fx.ns.SendTask(fx.taskID, map[string]string{}, service.Trigger{}); err != nil {
		t.Fatalf("wecom send: %v", err)
	}
	var body struct {
		MsgType  string `json:"msgtype"`
		Markdown struct {
			Content string `json:"content"`
		} `json:"markdown"`
	}
	if err := json.Unmarshal([]byte(sink.lastBody()), &body); err != nil {
		t.Fatalf("wecom body: %v", err)
	}
	if body.MsgType != "markdown" {
		t.Errorf("wecom msgtype = %q, want markdown", body.MsgType)
	}
	if !strings.Contains(body.Markdown.Content, fx.subject) || !strings.Contains(body.Markdown.Content, fx.content) {
		t.Errorf("wecom content = %q", body.Markdown.Content)
	}
}

func TestIntegrationDingtalk(t *testing.T) {
	secret := "sec-test-secret"
	sink := newHookSink(t, map[string]interface{}{"errcode": 0})
	fx := buildFixture(t, "dingtalk", map[string]string{"webhook_url": sink.url() + "?access_token=x", "secret": secret})
	if err := fx.ns.SendTask(fx.taskID, map[string]string{}, service.Trigger{}); err != nil {
		t.Fatalf("dingtalk send: %v", err)
	}
	u, err := url.Parse(sink.lastURL())
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	ts := q.Get("timestamp")
	sign := q.Get("sign")
	if ts == "" || sign == "" {
		t.Fatalf("dingtalk URL missing timestamp/sign: %s", sink.lastURL())
	}
	// 校验签名：HMAC-SHA256(timestamp+"\n"+secret)，base64（q.Get 已做 URL 解码）
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "\n" + secret))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if sign != want {
		t.Errorf("dingtalk sign mismatch: got %q want %q", sign, want)
	}
	var body struct {
		MsgType  string `json:"msgtype"`
		Markdown struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		} `json:"markdown"`
	}
	if err := json.Unmarshal([]byte(sink.lastBody()), &body); err != nil {
		t.Fatal(err)
	}
	if body.MsgType != "markdown" || body.Markdown.Title != fx.subject {
		t.Errorf("dingtalk body = %+v", body)
	}
}

func TestIntegrationFeishu(t *testing.T) {
	sink := newHookSink(t, map[string]interface{}{"code": 0, "msg": "success"})
	fx := buildFixture(t, "feishu", map[string]string{"webhook_url": sink.url()})
	if err := fx.ns.SendTask(fx.taskID, map[string]string{}, service.Trigger{}); err != nil {
		t.Fatalf("feishu send: %v", err)
	}
	var body struct {
		MsgType string `json:"msg_type"`
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(sink.lastBody()), &body); err != nil {
		t.Fatal(err)
	}
	if body.MsgType != "text" || !strings.Contains(body.Content.Text, fx.subject) {
		t.Errorf("feishu body = %+v", body)
	}
}

func TestIntegrationPushPlus(t *testing.T) {
	sink := newHookSink(t, map[string]interface{}{"code": 200, "msg": "success"}) // 真实 PushPlus 成功码为 200
	fx := buildFixture(t, "wechat", map[string]string{
		"pushplus_token": "tok123", "pushplus_url": sink.url(),
	})
	if err := fx.ns.SendTask(fx.taskID, map[string]string{}, service.Trigger{}); err != nil {
		t.Fatalf("pushplus send: %v", err)
	}
	form := sink.lastForm()
	if form == nil {
		t.Fatal("pushplus should receive application/x-www-form-urlencoded")
	}
	if form.Get("token") != "tok123" {
		t.Errorf("pushplus token = %q", form.Get("token"))
	}
	if form.Get("title") != fx.subject {
		t.Errorf("pushplus title = %q want %q", form.Get("title"), fx.subject)
	}
}

// TestIntegrationWebhookErrorDetection 验证厂商返回业务错误时渠道识别为失败。
func TestIntegrationWebhookErrorDetection(t *testing.T) {
	sink := newHookSink(t, map[string]interface{}{"errcode": 40035, "errmsg": "bad"})
	fx := buildFixture(t, "wecom", map[string]string{"webhook_url": sink.url()})
	if err := fx.ns.SendTask(fx.taskID, map[string]string{}, service.Trigger{}); err == nil {
		t.Fatal("wecom errcode!=0 should fail the send")
	}
	// 失败应写入 failed 日志
	var status, errMsg string
	if err := fx.db.QueryRow("SELECT status, error_msg FROM task_logs WHERE task_id=? ORDER BY id DESC LIMIT 1", fx.taskID).
		Scan(&status, &errMsg); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || errMsg == "" {
		t.Errorf("expected failed log, got status=%q err=%q", status, errMsg)
	}
}

var _ = channel.NewEmailChannel
