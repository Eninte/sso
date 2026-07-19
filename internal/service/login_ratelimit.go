// Package service 登录频率限制服务
// 基于IP地址限制登录尝试频率，防止撞库和账户锁定DoS攻击
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/example/sso/internal/cache"
)

// ============================================================================
// 登录限流配置
// ============================================================================

const (
	// LoginRateLimitWindow 登录限流时间窗口（10分钟）
	LoginRateLimitWindow = 10 * time.Minute

	// LoginRateLimitMax 每个时间窗口内每个IP的最大登录尝试次数
	// 设置为账户锁定阈值的4倍（5×4=20），允许合理范围内的重试
	LoginRateLimitMax = 20

	// loginRateLimitKeyPrefix Redis键前缀
	loginRateLimitKeyPrefix = "login:ratelimit:"
)

// ============================================================================
// LoginRateLimiter 登录频率限制器
// ============================================================================

// LoginRateLimiter 基于IP的登录频率限制器
// 使用Redis实现分布式限流，防止撞库攻击和账户锁定DoS
type LoginRateLimiter struct {
	cache  cache.Cache
	limit  int
	window time.Duration
}

// NewLoginRateLimiter 创建登录频率限制器
func NewLoginRateLimiter(cache cache.Cache) *LoginRateLimiter {
	return &LoginRateLimiter{
		cache:  cache,
		limit:  LoginRateLimitMax,
		window: LoginRateLimitWindow,
	}
}

// CheckAndRecord 检查并记录登录尝试
// 返回：是否允许尝试、剩余次数、错误
// 调用方应在每次登录尝试（无论成功失败）前调用此方法
func (r *LoginRateLimiter) CheckAndRecord(ctx context.Context, clientIP string) (bool, int, error) {
	if r.cache == nil || clientIP == "" {
		return true, r.limit, nil
	}

	key := loginRateLimitKeyPrefix + clientIP

	// 原子递增尝试计数
	count, err := r.cache.Increment(ctx, key)
	if err != nil {
		// 缓存错误不应阻止登录
		return true, r.limit, nil //nolint:nilerr // 限流降级：缓存故障时放行，避免影响核心登录流程
	}

	// 首次计数，设置过期时间
	if count == 1 {
		if err := r.cache.SetTTL(ctx, key, r.window); err != nil {
			return true, r.limit - 1, nil //nolint:nilerr // TTL失败不阻断，计数已递增故剩余次数 -1
		}
	}

	// 检查是否超过限制
	if count > r.limit {
		return false, 0, nil
	}

	remaining := r.limit - count
	if remaining < 0 {
		remaining = 0
	}

	return true, remaining, nil
}

// FormatLoginRateLimitError 格式化限流错误消息
func FormatLoginRateLimitError(ttl time.Duration) string {
	minutes := int(ttl.Minutes())
	if minutes < 1 {
		return "登录尝试过于频繁，请稍后再试"
	}
	return fmt.Sprintf("登录尝试过于频繁，请在 %d 分钟后再试", minutes)
}
