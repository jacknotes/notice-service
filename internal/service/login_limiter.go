package service

import (
	"errors"
	"sync"
	"time"
)

// loginLimiter 登录失败限流：同一用户名连续失败达上限后锁定一段时间。
// 内存态实现（单实例有效）；多实例场景各实例独立计数，属于可接受的 v1 缓解。
type loginLimiter struct {
	mu         sync.Mutex
	failures   map[string]int
	lockedAt   map[string]time.Time
	maxFails   int
	lockWindow time.Duration
}

func newLoginLimiter(maxFails int, lockWindow time.Duration) *loginLimiter {
	return &loginLimiter{
		failures:   map[string]int{},
		lockedAt:   map[string]time.Time{},
		maxFails:   maxFails,
		lockWindow: lockWindow,
	}
}

// checkLocked 若用户名当前处于锁定则返回错误；锁定过期则清除记录。
func (l *loginLimiter) checkLocked(username string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if until, ok := l.lockedAt[username]; ok {
		if time.Now().Before(until) {
			return errors.New("登录失败次数过多，请稍后再试")
		}
		delete(l.lockedAt, username)
		delete(l.failures, username)
	}
	return nil
}

// recordFailure 记录一次失败；连续失败达到上限则锁定 lockWindow。
func (l *loginLimiter) recordFailure(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[username]++
	if l.failures[username] >= l.maxFails {
		l.lockedAt[username] = time.Now().Add(l.lockWindow)
	}
}

// reset 登录成功后清除计数与锁定。
func (l *loginLimiter) reset(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, username)
	delete(l.lockedAt, username)
}
