package repository

import (
	"database/sql"
	"strings"
	"time"

	"notice-service/internal/model"
)

type TaskLogRepo struct{ db *sql.DB }

func NewTaskLogRepo(db *sql.DB) *TaskLogRepo { return &TaskLogRepo{db: db} }

// taskLogCols 发送日志的通用列清单（不 JOIN 的查询复用：ListByTask/Recent）。
const taskLogCols = "id, task_id, channel_id, subject, content, status, request, response, error_msg, retry_count, trigger_type, trigger_by, trigger_ip, sent_at"

// taskLogColsJoined JOIN tasks 的列清单（Query/GetByID 用，附带任务的当前分类）。
// 分类语义：日志跟随任务的当前分类，不落库；任务行缺失时兜底 default。
const taskLogColsJoined = "tl.id, tl.task_id, tl.channel_id, tl.subject, tl.content, tl.status, tl.request, tl.response, tl.error_msg, tl.retry_count, tl.trigger_type, tl.trigger_by, tl.trigger_ip, tl.sent_at, COALESCE(t.category,'default') AS category"

// taskLogFrom 日志 JOIN 查询的 FROM 子句（Query/GetByID/计数共用）。
const taskLogFrom = " FROM task_logs tl LEFT JOIN tasks t ON t.id = tl.task_id"

func (r *TaskLogRepo) Create(l *model.TaskLog) error {
	if l.SentAt.IsZero() {
		l.SentAt = time.Now() // 未显式指定时用当前时间，避免零值覆盖 DB 默认
	}
	res, err := r.db.Exec(
		"INSERT INTO task_logs (task_id, channel_id, subject, content, status, request, response, error_msg, retry_count, trigger_type, trigger_by, trigger_ip, sent_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		l.TaskID, l.ChannelID, l.Subject, l.Content, l.Status, l.Request, l.Response, l.ErrorMsg, l.RetryCount, l.TriggerType, l.TriggerBy, l.TriggerIP, l.SentAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	l.ID = id
	return nil
}

func (r *TaskLogRepo) GetByID(id int64) (*model.TaskLog, error) {
	rows, err := r.db.Query("SELECT "+taskLogColsJoined+taskLogFrom+" WHERE tl.id=?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs, err := scanLogsJoined(rows)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, ErrNotFound
	}
	return logs[0], nil
}

func (r *TaskLogRepo) ListByTask(taskID int64) ([]*model.TaskLog, error) {
	rows, err := r.db.Query("SELECT "+taskLogCols+" FROM task_logs WHERE task_id=? ORDER BY id DESC LIMIT 200",
		taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogs(rows)
}

func (r *TaskLogRepo) Recent(limit int) ([]*model.TaskLog, error) {
	rows, err := r.db.Query("SELECT "+taskLogCols+" FROM task_logs ORDER BY id DESC LIMIT ?",
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogs(rows)
}

// scanLogInto 扫描单行；cat 非空时对应 JOIN 查询（列清单末尾多一列 category）。
func scanLogInto(rows *sql.Rows, l *model.TaskLog, cat *string) error {
	var subj, content, req, resp, errMsg, trigBy, trigIP sql.NullString
	dest := []any{&l.ID, &l.TaskID, &l.ChannelID, &subj, &content, &l.Status, &req, &resp, &errMsg, &l.RetryCount, &l.TriggerType, &trigBy, &trigIP, &l.SentAt}
	if cat != nil {
		dest = append(dest, cat)
	}
	if err := rows.Scan(dest...); err != nil {
		return err
	}
	l.Subject = subj.String
	l.Content = content.String
	l.Request = req.String
	l.Response = resp.String
	l.ErrorMsg = errMsg.String
	l.TriggerBy = trigBy.String
	l.TriggerIP = trigIP.String
	return nil
}

func scanLogs(rows *sql.Rows) ([]*model.TaskLog, error) {
	out := []*model.TaskLog{}
	for rows.Next() {
		l := &model.TaskLog{}
		if err := scanLogInto(rows, l, nil); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// scanLogsJoined 带 category 列的扫描（Query/GetByID 的 JOIN 查询用）。
func scanLogsJoined(rows *sql.Rows) ([]*model.TaskLog, error) {
	out := []*model.TaskLog{}
	for rows.Next() {
		l := &model.TaskLog{}
		if err := scanLogInto(rows, l, &l.Category); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// CountByRange 统计时间段内各状态数量（仪表盘用）。
func (r *TaskLogRepo) CountByRange(from, to time.Time) (total, success, failed int, err error) {
	err = r.db.QueryRow(
		"SELECT COUNT(*), COALESCE(SUM(status='success'),0), COALESCE(SUM(status='failed'),0) FROM task_logs WHERE sent_at >= ? AND sent_at < ?",
		from, to).Scan(&total, &success, &failed)
	return
}

// CleanupOlderThan 删除超过保留天数的发送日志（幂等，多实例重复执行无害）。
func (r *TaskLogRepo) CleanupOlderThan(days int) (int64, error) {
	total := int64(0)
	for {
		res, err := r.db.Exec(
			"DELETE FROM task_logs WHERE sent_at < NOW() - INTERVAL ? DAY LIMIT 1000", days)
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
		if n < 1000 {
			return total, nil
		}
	}
}

// LogFilter 日志查询过滤条件（后端分页/筛选下推 DB）。
type LogFilter struct {
	TaskID   int64
	Category string // 任务的当前分类；空串=全部
	Status   string // success | failed | ""（全部）
	From, To time.Time
	Page     int
	PageSize int
	// SortBy / SortOrder 后端排序（SortBy 为白名单内的列名）。
	SortBy    string // id | sent_at | task_id | channel_id | status | retry_count | category
	SortOrder string // asc | desc（默认 desc）
}

// sortColumn 排序白名单：防 SQL 注入，非法值回退 tl.id。
// 返回带表别名的列名（Query 已 JOIN tasks，裸列名 id 等会有歧义）。
func (f LogFilter) sortColumn() string {
	switch f.SortBy {
	case "sent_at":
		return "tl.sent_at"
	case "task_id":
		return "tl.task_id"
	case "channel_id":
		return "tl.channel_id"
	case "status":
		return "tl.status"
	case "retry_count":
		return "tl.retry_count"
	case "category":
		return "t.category"
	}
	return "tl.id" // 默认按 id 倒序（与既有行为一致）
}

// Query 按过滤条件分页查询发送日志，返回总数与当前页数据。
// JOIN tasks 取任务的当前分类：category 筛选/排序/展示均基于它。
func (r *TaskLogRepo) Query(f LogFilter) (total int, logs []*model.TaskLog, err error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	if f.TaskID > 0 {
		where += " AND tl.task_id=?"
		args = append(args, f.TaskID)
	}
	if f.Category != "" {
		where += " AND t.category=?"
		args = append(args, f.Category)
	}
	if f.Status != "" {
		where += " AND tl.status=?"
		args = append(args, f.Status)
	}
	if !f.From.IsZero() {
		where += " AND tl.sent_at >= ?"
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		where += " AND tl.sent_at < ?"
		args = append(args, f.To)
	}
	if err = r.db.QueryRow("SELECT COUNT(*)"+taskLogFrom+" "+where, args...).Scan(&total); err != nil {
		return 0, nil, err
	}
	limit := f.PageSize
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := (f.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}
	order := "DESC"
	if strings.EqualFold(f.SortOrder, "asc") {
		order = "ASC"
	}
	col := f.sortColumn()
	// 固定 id 作为次级排序键，保证同值分页稳定不重不漏。
	queryArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := r.db.Query(
		"SELECT "+taskLogColsJoined+taskLogFrom+" "+where+" ORDER BY "+col+" "+order+", tl.id DESC LIMIT ? OFFSET ?",
		queryArgs...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	logs, err = scanLogsJoined(rows)
	if err != nil {
		return 0, nil, err
	}
	return total, logs, nil
}

// CountByDay 单条 GROUP BY 统计 [from,to) 内每天的总发送数与成功/失败数（仪表盘趋势用）。
// 返回 key = "YYYY-MM-DD"（含年份，避免跨年查询时同月同日的数据被合并）。
func (r *TaskLogRepo) CountByDay(from, to time.Time) (map[string]struct{ Total, Success, Failed int }, error) {
	rows, err := r.db.Query(
		`SELECT DATE_FORMAT(sent_at, '%Y-%m-%d') AS d, COUNT(*), COALESCE(SUM(status='success'),0), COALESCE(SUM(status='failed'),0)
		 FROM task_logs WHERE sent_at >= ? AND sent_at < ?
		 GROUP BY d ORDER BY d`,
		from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{ Total, Success, Failed int }{}
	for rows.Next() {
		var d string
		var total, success, failed int
		if err := rows.Scan(&d, &total, &success, &failed); err != nil {
			return nil, err
		}
		out[d] = struct{ Total, Success, Failed int }{Total: total, Success: success, Failed: failed}
	}
	return out, rows.Err()
}

// CountDistinctByRange 区间内 distinct 任务数 / 渠道数。
func (r *TaskLogRepo) CountDistinctByRange(from, to time.Time) (tasks, channels int, err error) {
	err = r.db.QueryRow(
		`SELECT COUNT(DISTINCT task_id), COUNT(DISTINCT channel_id) FROM task_logs WHERE sent_at >= ? AND sent_at < ?`,
		from, to).Scan(&tasks, &channels)
	return
}

// RowCount 单行分组统计结果（任务/渠道）。
type RowCount struct {
	ID      int64
	Total   int
	Success int
	Failed  int
}

// CountByTask 按任务分组统计区间发送量，按 total 降序取前 limit。
func (r *TaskLogRepo) CountByTask(from, to time.Time, limit int) ([]RowCount, error) {
	rows, err := r.db.Query(
		`SELECT task_id, COUNT(*), COALESCE(SUM(status='success'),0), COALESCE(SUM(status='failed'),0)
		 FROM task_logs WHERE sent_at >= ? AND sent_at < ?
		 GROUP BY task_id ORDER BY COUNT(*) DESC LIMIT ?`,
		from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRowCounts(rows)
}

// CountByChannel 按渠道分组统计区间发送量。
func (r *TaskLogRepo) CountByChannel(from, to time.Time) ([]RowCount, error) {
	rows, err := r.db.Query(
		`SELECT channel_id, COUNT(*), COALESCE(SUM(status='success'),0), COALESCE(SUM(status='failed'),0)
		 FROM task_logs WHERE sent_at >= ? AND sent_at < ?
		 GROUP BY channel_id ORDER BY COUNT(*) DESC`,
		from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRowCounts(rows)
}

func scanRowCounts(rows *sql.Rows) ([]RowCount, error) {
	out := []RowCount{}
	for rows.Next() {
		var rc RowCount
		if err := rows.Scan(&rc.ID, &rc.Total, &rc.Success, &rc.Failed); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

// LogExportRow CSV 导出用扁平行（左连任务/渠道取名称）。
type LogExportRow struct {
	ID          int64
	SentAt      time.Time
	TaskID      int64
	TaskName    string
	Category    string
	ChannelID   int64
	ChannelName string
	Status      string
	Subject     string
	Content     string
	Request     string
	Response    string
	ErrorMsg    string
	RetryCount  int
	TriggerType string
	TriggerBy   string
	TriggerIP   string
}

// ListExportRows 按过滤条件导出全部匹配行（不分页，最多 limit 行；CSV 用）。
func (r *TaskLogRepo) ListExportRows(f LogFilter, limit int) ([]*LogExportRow, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	if f.TaskID > 0 {
		where += " AND tl.task_id=?"
		args = append(args, f.TaskID)
	}
	if f.Status != "" {
		where += " AND tl.status=?"
		args = append(args, f.Status)
	}
	if f.Category != "" {
		where += " AND t.category=?"
		args = append(args, f.Category)
	}
	if !f.From.IsZero() {
		where += " AND tl.sent_at >= ?"
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		where += " AND tl.sent_at < ?"
		args = append(args, f.To)
	}
	if limit <= 0 || limit > 100000 {
		limit = 100000
	}
	query := `SELECT tl.id, tl.sent_at, tl.task_id, COALESCE(t.name,''), COALESCE(t.category,'default'), tl.channel_id, COALESCE(c.name,''),
		tl.status, tl.subject, COALESCE(tl.content,''), COALESCE(tl.request,''), COALESCE(tl.response,''),
		COALESCE(tl.error_msg,''), tl.retry_count, COALESCE(tl.trigger_type,''), COALESCE(tl.trigger_by,''), COALESCE(tl.trigger_ip,'')
		FROM task_logs tl
		LEFT JOIN tasks t ON t.id = tl.task_id
		LEFT JOIN channels c ON c.id = tl.channel_id
		` + where + ` ORDER BY tl.id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*LogExportRow{}
	for rows.Next() {
		row := &LogExportRow{}
		if err := rows.Scan(&row.ID, &row.SentAt, &row.TaskID, &row.TaskName, &row.Category, &row.ChannelID, &row.ChannelName,
			&row.Status, &row.Subject, &row.Content, &row.Request, &row.Response, &row.ErrorMsg, &row.RetryCount,
			&row.TriggerType, &row.TriggerBy, &row.TriggerIP); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
