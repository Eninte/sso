// Package service 邮件发送限流服务
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/example/sso/internal/cache"
)

// ============================================================================
// 邮件限流配置
// ============================================================================

const (
	// EmailRateLimitWindow 邮件限流时间窗口（1小时）
	EmailRateLimitWindow = 1 * time.Hour

	// EmailRateLimitMax 每个时间窗口内的最大邮件发送次数
	EmailRateLimitMax = 5

	// emailRateLimitKeyPrefix Redis键前缀
	emailRateLimitKeyPrefix = "email:ratelimit:"
)

// ============================================================================
// EmailRateLimiter 邮件限流器
// ============================================================================

// EmailRateLimiter 邮件发送限流器
// 使用Redis实现分布式限流，防止邮件滥用
//
// T10（M4 方案 B）：Redis 故障时降级为进程内固定窗口内存限流，
// 降级期间限额仍然生效，并输出 Error 日志与指标（禁止静默放行）
type EmailRateLimiter struct {
	cache     cache.Cache
	errorFunc func() // 降级错误回调（用于指标计数）

	// 内存降级：固定窗口计数（与 Redis INCR+TTL 语义一致）
	memoryMu sync.Mutex
	memory   map[string]*rateLimitWindow
}

// NewEmailRateLimiter 创建邮件限流器
func NewEmailRateLimiter(cache cache.Cache) *EmailRateLimiter {
	return &EmailRateLimiter{
		cache:  cache,
		memory: make(map[string]*rateLimitWindow),
	}
}

// WithErrorCallback 设置降级错误回调（T10：Redis 故障时触发指标计数）
func (r *EmailRateLimiter) WithErrorCallback(fn func()) *EmailRateLimiter {
	r.errorFunc = fn
	return r
}

// CheckLimit 检查是否超过限流
// 返回：是否允许发送、剩余次数、错误
func (r *EmailRateLimiter) CheckLimit(ctx context.Context, email string) (bool, int, error) {
	if r.cache == nil {
		// 如果没有配置缓存，允许发送（向后兼容）
		return true, EmailRateLimitMax, nil
	}

	key := emailRateLimitKeyPrefix + email

	// 获取当前计数
	count, err := r.cache.Increment(ctx, key)
	if err != nil {
		// T10（方案 B）：Redis 故障降级为进程内内存限流，禁止静默放行
		r.notifyFallback("邮件限流Redis写入失败，降级为进程内内存限流", email, err)
		allowed, remaining := r.checkMemory(email)
		return allowed, remaining, nil
	}

	// 如果是第一次计数，设置过期时间
	if count == 1 {
		if err := r.cache.SetTTL(ctx, key, EmailRateLimitWindow); err != nil {
			// TTL设置失败，记录但不阻止，计数已递增故剩余次数 -1
			r.notifyFallback("邮件限流Redis设置TTL失败", email, err)
			return true, EmailRateLimitMax - 1, nil
		}
	}

	// 检查是否超过限制
	if count > EmailRateLimitMax {
		return false, 0, nil
	}

	remaining := EmailRateLimitMax - count
	if remaining < 0 {
		remaining = 0
	}

	return true, remaining, nil
}

// notifyFallback 记录降级日志（Error 级）并触发指标回调
func (r *EmailRateLimiter) notifyFallback(msg, email string, err error) {
	slog.Error(msg, "error", err, "email", email)
	if r.errorFunc != nil {
		r.errorFunc()
	}
}

// checkMemory 进程内固定窗口内存限流（T10 降级路径）
// 窗口与限额同 Redis 路径配置；窗口过期自动重置
func (r *EmailRateLimiter) checkMemory(email string) (bool, int) {
	r.memoryMu.Lock()
	defer r.memoryMu.Unlock()

	now := time.Now()
	w, exists := r.memory[email]
	if !exists || now.Sub(w.windowStart) >= EmailRateLimitWindow {
		w = &rateLimitWindow{windowStart: now}
		r.memory[email] = w
	}
	w.count++

	if w.count > EmailRateLimitMax {
		return false, 0
	}
	return true, EmailRateLimitMax - w.count
}

// GetRemaining 获取剩余发送次数
func (r *EmailRateLimiter) GetRemaining(ctx context.Context, email string) (int, error) {
	if r.cache == nil {
		return EmailRateLimitMax, nil
	}

	key := emailRateLimitKeyPrefix + email

	var count int
	err := r.cache.Get(ctx, key, &count)
	if err != nil {
		if !errors.Is(err, cache.ErrCacheMiss) {
			// T10：读取故障补齐可观测性（键不存在视为未限流，不告警）
			r.notifyFallback("邮件限流Redis读取失败", email, err)
		}
		return EmailRateLimitMax, nil
	}

	remaining := EmailRateLimitMax - count
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

// GetTTL 获取限流重置时间
func (r *EmailRateLimiter) GetTTL(ctx context.Context, email string) (time.Duration, error) {
	if r.cache == nil {
		return 0, nil
	}

	key := emailRateLimitKeyPrefix + email
	return r.cache.GetTTL(ctx, key)
}

// Reset 重置限流计数（仅用于测试）
func (r *EmailRateLimiter) Reset(ctx context.Context, email string) error {
	if r.cache == nil {
		return nil
	}

	key := emailRateLimitKeyPrefix + email
	return r.cache.Delete(ctx, key)
}

// FormatRateLimitError 格式化限流错误消息
func FormatRateLimitError(ttl time.Duration) string {
	minutes := int(ttl.Minutes())
	if minutes < 1 {
		return "发送邮件过于频繁，请稍后再试"
	}
	return fmt.Sprintf("发送邮件过于频繁，请在 %d 分钟后再试", minutes)
}
