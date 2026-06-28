// Package service 邮件发送限流服务
package service

import (
	"context"
	"fmt"
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
type EmailRateLimiter struct {
	cache cache.Cache
}

// NewEmailRateLimiter 创建邮件限流器
func NewEmailRateLimiter(cache cache.Cache) *EmailRateLimiter {
	return &EmailRateLimiter{
		cache: cache,
	}
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
		// 缓存错误不应阻止邮件发送
		return true, EmailRateLimitMax, nil
	}

	// 如果是第一次计数，设置过期时间
	if count == 1 {
		if err := r.cache.SetTTL(ctx, key, EmailRateLimitWindow); err != nil {
			// TTL设置失败，记录但不阻止
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

// GetRemaining 获取剩余发送次数
func (r *EmailRateLimiter) GetRemaining(ctx context.Context, email string) (int, error) {
	if r.cache == nil {
		return EmailRateLimitMax, nil
	}

	key := emailRateLimitKeyPrefix + email

	var count int
	err := r.cache.Get(ctx, key, &count)
	if err != nil {
		// 键不存在或其他错误，返回最大值
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
