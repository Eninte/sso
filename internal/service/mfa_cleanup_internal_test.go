// Package service MFA 清理逻辑内部测试（同包以访问未导出方法）
package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/example/sso/internal/store/mock"
)

// TestMFAService_cleanupTOTPUsage 测试 TOTP 使用记录清理
func TestMFAService_cleanupTOTPUsage(t *testing.T) {
	store := mock.New()
	svc := NewMFAService(store)
	defer svc.Close()

	// 注入两条记录：一条过期，一条未过期
	svc.recordTOTPUsage("user-expired", "123456", 100)
	// 手动将过期记录回拨到 91 秒前
	svc.totpMu.Lock()
	svc.totpUsage["user-expired"].usedAt = time.Now().Add(-91 * time.Second)
	svc.totpMu.Unlock()

	svc.recordTOTPUsage("user-fresh", "654321", 101)

	// 执行清理
	svc.cleanupTOTPUsage()

	svc.totpMu.Lock()
	defer svc.totpMu.Unlock()
	assert.NotContains(t, svc.totpUsage, "user-expired", "超过90秒的记录应被清理")
	assert.Contains(t, svc.totpUsage, "user-fresh", "未过期的记录应保留")
}

// TestMFAService_cleanupRecoveryAttempts 测试恢复码尝试记录清理
func TestMFAService_cleanupRecoveryAttempts(t *testing.T) {
	store := mock.New()
	svc := NewMFAService(store)
	defer svc.Close()

	// 注入一条过期失败记录
	svc.recoveryMu.Lock()
	svc.recoveryAttempts["user-locked"] = &recoveryAttempt{
		count:     3,
		lastFail:  time.Now().Add(-31 * time.Minute), // 超过30分钟
		lockUntil: time.Now().Add(-1 * time.Minute),
	}
	svc.recoveryAttempts["user-recent"] = &recoveryAttempt{
		count:    1,
		lastFail: time.Now().Add(-5 * time.Minute), // 未过期
	}
	svc.recoveryMu.Unlock()

	svc.cleanupRecoveryAttempts()

	svc.recoveryMu.Lock()
	defer svc.recoveryMu.Unlock()
	assert.NotContains(t, svc.recoveryAttempts, "user-locked", "超过30分钟的记录应被清理")
	assert.Contains(t, svc.recoveryAttempts, "user-recent", "未过期的记录应保留")
}

// TestMFAService_Close 测试 Close 停止后台 goroutine
// 通过二次 Close 应 panic 来验证 stopChan 被关闭
func TestMFAService_Close(t *testing.T) {
	store := mock.New()
	svc := NewMFAService(store)

	// 正常关闭不应 panic
	assert.NotPanics(t, func() { svc.Close() })

	// 二次关闭应 panic（close of closed channel）
	assert.Panics(t, func() { svc.Close() })
}

// TestMFAService_recordTOTPUsage 测试 TOTP 使用记录写入
func TestMFAService_recordTOTPUsage(t *testing.T) {
	store := mock.New()
	svc := NewMFAService(store)
	defer svc.Close()

	svc.recordTOTPUsage("user-1", "111111", 200)

	svc.totpMu.Lock()
	defer svc.totpMu.Unlock()
	rec, ok := svc.totpUsage["user-1"]
	assert.True(t, ok, "记录应被写入")
	assert.Equal(t, "111111", rec.code)
	assert.Equal(t, uint64(200), rec.timeStep)
	assert.WithinDuration(t, time.Now(), rec.usedAt, 2*time.Second)
}

// TestMFAService_isTOTPUsed 测试 TOTP 重放检测
func TestMFAService_isTOTPUsed(t *testing.T) {
	store := mock.New()
	svc := NewMFAService(store)
	defer svc.Close()

	// 未记录前应返回 false
	assert.False(t, svc.isTOTPUsed("user-1", "111111", 200))

	// 记录后相同 (userID, code, timeStep) 应返回 true
	svc.recordTOTPUsage("user-1", "111111", 200)
	assert.True(t, svc.isTOTPUsed("user-1", "111111", 200))

	// 不同 code 应返回 false
	assert.False(t, svc.isTOTPUsed("user-1", "222222", 200))

	// 不同 timeStep 应返回 false
	assert.False(t, svc.isTOTPUsed("user-1", "111111", 201))

	// 不同 user 应返回 false
	assert.False(t, svc.isTOTPUsed("user-2", "111111", 200))
}

// TestMFAService_clearRecoveryAttempts 测试清除指定用户的恢复码尝试记录
func TestMFAService_clearRecoveryAttempts(t *testing.T) {
	store := mock.New()
	svc := NewMFAService(store)
	defer svc.Close()

	svc.recoveryMu.Lock()
	svc.recoveryAttempts["user-1"] = &recoveryAttempt{count: 2, lastFail: time.Now()}
	svc.recoveryAttempts["user-2"] = &recoveryAttempt{count: 1, lastFail: time.Now()}
	svc.recoveryMu.Unlock()

	svc.clearRecoveryAttempts("user-1")

	svc.recoveryMu.Lock()
	defer svc.recoveryMu.Unlock()
	assert.NotContains(t, svc.recoveryAttempts, "user-1")
	assert.Contains(t, svc.recoveryAttempts, "user-2")
}
