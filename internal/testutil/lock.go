// Package testutil 提供**仅测试使用**的辅助工具（不参与任何业务逻辑，不被
// cmd/server 或任何生产代码引用）。
//
// Acquire 用 MySQL 命名锁把「共用同一个测试库」的包串行化：
// cmd/server、internal/database、internal/handler、internal/integration、
// internal/repository、internal/scheduler、internal/service 的测试都把数据写进
// 同一个 notice_service_test 库（DSN 硬编码在各包 *_test.go 中），而
// `go test ./internal/...` 默认**并行执行不同包** → 互相删除对方的
// users/admin/channels/send_jobs 行，产生与业务代码无关的随机失败。
//
// 该不稳定在 v2.0.0..HEAD 的基线 HEAD 上同样 100% 复现（对照实验见
// docs/fixes/security-fixes.md「验证记录」），因此用测试期串行锁消除。
//
// 用法（各包 TestMain）：
//
//	func TestMain(m *testing.M) {
//		release := testutil.Acquire()
//		code := m.Run()
//		release()
//		os.Exit(code)
//	}
//
// 锁不可用（如本地无 MySQL）时直接放行，让各用例自身给出明确的连接错误，
// 而不是把「环境缺失」伪装成锁超时。
package testutil

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// DSN 测试库连接串（与各包 *_test.go 中的硬编码保持一致）。
const DSN = "notice:notice123@tcp(127.0.0.1:3306)/notice_service_test?parseTime=true&charset=utf8mb4&loc=Local"

// lockName 命名锁名称：所有共用该库的包使用同一个名字。
const lockName = "notice_service_test_serial"

// lockWaitSeconds GET_LOCK 等待上限（秒）：覆盖其它包的整轮测试（当前全量约 20–30s）。
const lockWaitSeconds = 600

// Acquire 取跨包 MySQL 命名锁，返回释放函数（取锁失败时返回空操作）。
// GET_LOCK 是**连接级**锁，必须用同一条 *sql.Conn 执行 RELEASE_LOCK，
// 因此这里固定 MaxOpenConns=1 并复用同一连接。
func Acquire() func() {
	db, err := sql.Open("mysql", DSN)
	if err != nil {
		return func() {}
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return func() {}
	}
	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, lockWaitSeconds).Scan(&got); err != nil ||
		!got.Valid || got.Int64 != 1 {
		_ = conn.Close()
		_ = db.Close()
		return func() {}
	}
	return func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockName)
		_ = conn.Close()
		_ = db.Close()
	}
}
