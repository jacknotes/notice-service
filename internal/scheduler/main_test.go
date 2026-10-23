package scheduler

import (
	"os"
	"testing"

	"notice-service/internal/testutil"
)

// TestMain 串行化「共用同一个 notice_service_test 库」的测试包（详见
// internal/testutil 的说明）：`go test ./internal/...` 默认并行跑各包，
// 而这些包会互相删除对方的 users/channels/send_jobs 行。
func TestMain(m *testing.M) {
	release := testutil.Acquire()
	code := m.Run()
	release()
	os.Exit(code)
}
