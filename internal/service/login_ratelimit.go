// Package service 登录频率限制服务
// 基于IP地址限制登录尝试频率，防止撞库和账户锁定DoS攻击
package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
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
//
// T10（M4 方案 B）：Redis 故障时降级为进程内固定窗口内存限流，
// 降级期间限额仍然生效，并输出 Error 日志与指标（禁止静默放行）
type LoginRateLimiter struct {
	cache     cache.Cache
	limit     int
	window    time.Duration
	errorFunc func() // 降级错误回调（用于指标计数）

	// 内存降级：固定窗口计数（与 Redis INCR+TTL 语义一致）
	memoryMu sync.Mutex
	memory   map[string]*rateLimitWindow
}

// rateLimitWindow 内存降级的固定窗口计数
type rateLimitWindow struct {
	count       int
	windowStart time.Time
}

// NewLoginRateLimiter 创建登录频率限制器
func NewLoginRateLimiter(cache cache.Cache) *LoginRateLimiter {
	return &LoginRateLimiter{
		cache:  cache,
		limit:  LoginRateLimitMax,
		window: LoginRateLimitWindow,
		memory: make(map[string]*rateLimitWindow),
	}
}

// WithErrorCallback 设置降级错误回调（T10：Redis 故障时触发指标计数）
func (r *LoginRateLimiter) WithErrorCallback(fn func()) *LoginRateLimiter {
	r.errorFunc = fn
	return r
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
		// T10（方案 B）：Redis 故障降级为进程内内存限流，禁止静默放行
		r.notifyFallback("登录限流Redis写入失败，降级为进程内内存限流", clientIP, err)
		allowed, remaining := r.checkMemory(clientIP)
		return allowed, remaining, nil
	}

	// 首次计数，设置过期时间
	if count == 1 {
		if err := r.cache.SetTTL(ctx, key, r.window); err != nil {
			// TTL失败不阻断，计数已递增故剩余次数 -1
			r.notifyFallback("登录限流Redis设置TTL失败", clientIP, err)
			return true, r.limit - 1, nil
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

// notifyFallback 记录降级日志（Error 级）并触发指标回调
func (r *LoginRateLimiter) notifyFallback(msg, clientIP string, err error) {
	slog.Error(msg, "error", err, "client_ip", clientIP)
	if r.errorFunc != nil {
		r.errorFunc()
	}
}

// checkMemory 进程内固定窗口内存限流（T10 降级路径）
// 窗口与限额同 Redis 路径配置；窗口过期自动重置
func (r *LoginRateLimiter) checkMemory(clientIP string) (bool, int) {
	r.memoryMu.Lock()
	defer r.memoryMu.Unlock()

	now := time.Now()
	w, exists := r.memory[clientIP]
	if !exists || now.Sub(w.windowStart) >= r.window {
		w = &rateLimitWindow{windowStart: now}
		r.memory[clientIP] = w
	}
	w.count++

	if w.count > r.limit {
		return false, 0
	}
	return true, r.limit - w.count
}

// FormatLoginRateLimitError 格式化限流错误消息
func FormatLoginRateLimitError(ttl time.Duration) string {
	minutes := int(ttl.Minutes())
	if minutes < 1 {
		return "登录尝试过于频繁，请稍后再试"
	}
	return fmt.Sprintf("登录尝试过于频繁，请在 %d 分钟后再试", minutes)
}
