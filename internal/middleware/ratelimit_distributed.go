// Package middleware 分布式限流中间件
// 使用Redis实现分布式限流，支持多实例共享限流状态
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ============================================================================
// DistributedRateLimiter 分布式限流器
// ============================================================================

// rateLimitScript 使用Lua脚本实现原子的滑动窗口限流
// 返回 {allowed(0/1), remaining}
var rateLimitScript = redis.NewScript(`
local key = KEYS[1]
local now = ARGV[1]
local window_start = ARGV[2]
local limit = tonumber(ARGV[3])
local window_ttl_ms = tonumber(ARGV[4])
local member = ARGV[5]

-- 移除窗口外的记录
redis.call('ZREMRANGEBYSCORE', key, '0', window_start)

-- 获取当前窗口内的请求数
local count = redis.call('ZCARD', key)

-- 检查是否超过限制
if count >= limit then
    return {0, 0}
end

-- 添加当前请求记录
redis.call('ZADD', key, now, member)
redis.call('PEXPIRE', key, window_ttl_ms)

return {1, limit - count - 1}
`)

// DistributedRateLimiter 基于Redis的分布式限流器
// 使用滑动窗口算法实现精确限流
type DistributedRateLimiter struct {
	redisClient *redis.Client
	limit       int           // 每个时间窗口的请求数
	window      time.Duration // 时间窗口
	keyPrefix   string        // Redis键前缀
	metricFunc  func()        // 限流触发回调（用于指标计数）
	errorFunc   func()        // 阶段 4：fail-open 错误回调（用于指标计数）

	// T10（M4）：Redis 故障降级（方案 B）
	// 降级时当次请求改用进程内内存限流器（同限额同窗口），而非 fail-open 放行
	fallback        *RateLimiter // 内存降级限流器（WithMemoryFallback 启用）
	fallbackLogMu   sync.Mutex   // 降级日志节流锁
	lastFallbackLog time.Time    // 上次降级日志时间（每分钟最多一条）
}

// fallbackLogInterval 降级日志节流间隔（避免 Redis 故障期间刷日志）
const fallbackLogInterval = time.Minute

// NewDistributedRateLimiter 创建分布式限流器
// redisClient: Redis客户端
// limit: 每个时间窗口允许的最大请求数
// window: 时间窗口长度
// keyPrefix: Redis键前缀，用于区分不同的限流规则
func NewDistributedRateLimiter(redisClient *redis.Client, limit int, window time.Duration, keyPrefix string) *DistributedRateLimiter {
	return &DistributedRateLimiter{
		redisClient: redisClient,
		limit:       limit,
		window:      window,
		keyPrefix:   keyPrefix,
	}
}

// WithMetrics 设置限流触发回调（与本地限流器接口一致）
// 当请求被限流时调用此函数
func (drl *DistributedRateLimiter) WithMetrics(metricFunc func()) *DistributedRateLimiter {
	drl.metricFunc = metricFunc
	return drl
}

// WithErrorCallback 设置 fail-open 时的错误回调（用于指标计数）
// 阶段 4 安全增强：fail-open 时必须记录日志和指标，禁止忽略错误（AGENTS.md 8.4）
func (drl *DistributedRateLimiter) WithErrorCallback(fn func()) *DistributedRateLimiter {
	drl.errorFunc = fn
	return drl
}

// WithMemoryFallback 启用内存降级（T10 方案 B）
// Redis 故障时当次请求降级到进程内内存限流器（同限额同窗口），
// 降级期间限额仍然生效（可用性与安全的折中），而非 fail-open 无限放行
func (drl *DistributedRateLimiter) WithMemoryFallback() *DistributedRateLimiter {
	drl.fallback = NewRateLimiter(drl.limit, drl.window)
	return drl
}

// logFallbackThrottled 记录降级日志（Error 级，每分钟最多一条，避免刷日志）
func (drl *DistributedRateLimiter) logFallbackThrottled(err error, clientIP string) {
	drl.fallbackLogMu.Lock()
	defer drl.fallbackLogMu.Unlock()
	if time.Since(drl.lastFallbackLog) < fallbackLogInterval {
		return
	}
	drl.lastFallbackLog = time.Now()
	slog.Error("限流 Redis 错误，降级为进程内内存限流",
		"error", err,
		"client_ip", clientIP,
		"key_prefix", drl.keyPrefix)
}

// Middleware 分布式限流中间件
// 检查客户端请求频率，超过限制返回429
func (drl *DistributedRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 获取客户端标识
		clientIP := GetClientIP(r)

		// 检查是否超过限制
		allowed, remaining, resetTime, err := drl.Allow(r.Context(), clientIP)
		if err != nil {
			// 指标：每次降级都计数（日志节流，指标不节流）
			if drl.errorFunc != nil {
				drl.errorFunc()
			}
			// T10（方案 B）：配置了内存降级时，当次请求改用进程内内存限流，
			// Redis 故障期间限额仍然生效
			if drl.fallback != nil {
				drl.logFallbackThrottled(err, clientIP)
				if !drl.fallback.Allow(clientIP) {
					w.Header().Set("Retry-After", strconv.Itoa(int(drl.window.Seconds())))
					if drl.metricFunc != nil {
						drl.metricFunc()
					}
					writeError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			// 阶段 4 安全增强：fail-open 时必须记录日志和指标
			// 满足 AGENTS.md 第 8.4 节"禁止忽略错误"规则
			slog.Error("限流 Redis 错误，fail-open 放行",
				"error", err,
				"client_ip", clientIP,
				"key_prefix", drl.keyPrefix)
			next.ServeHTTP(w, r)
			return
		}

		// 设置限流响应头
		w.Header().Set("X-Ratelimit-Limit", strconv.Itoa(drl.limit))
		w.Header().Set("X-Ratelimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-Ratelimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(drl.window.Seconds())))
			// 阶段 4：限流触发时记录指标
			if drl.metricFunc != nil {
				drl.metricFunc()
			}
			writeError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			return
		}

		// 继续处理请求
		next.ServeHTTP(w, r)
	})
}

// Allow 检查是否允许请求（使用Lua脚本实现原子操作）
// 返回: allowed, remaining, resetTime, error
func (drl *DistributedRateLimiter) Allow(ctx context.Context, clientIP string) (bool, int, time.Time, error) {
	// limit <= 0 表示禁用限流
	if drl.limit <= 0 {
		return true, drl.limit, time.Time{}, nil
	}

	now := time.Now()
	key := drl.buildKey(clientIP)
	windowStart := now.Add(-drl.window)

	// 生成唯一 member，避免同纳秒请求的 ZAdd 覆盖
	// #nosec G404 -- member 仅用作 Redis ZSet 的唯一标识，不涉及密码学安全场景
	member := fmt.Sprintf("%d:%d", now.UnixNano(), rand.Int63())

	// 使用 Lua 脚本原子执行：清理过期记录 → 检查计数 → 添加记录
	// TTL 使用毫秒（PEXPIRE），支持亚秒级窗口
	ttlMs := int64(drl.window*2) / int64(time.Millisecond)
	if ttlMs < 1 {
		ttlMs = 1
	}
	result, err := rateLimitScript.Run(ctx, drl.redisClient,
		[]string{key},
		strconv.FormatInt(now.UnixNano(), 10),
		strconv.FormatInt(windowStart.UnixNano(), 10),
		drl.limit,
		ttlMs,
		member,
	).Result()
	if err != nil {
		// Redis错误：返回错误由 Middleware 决策（T10：内存降级或 fail-open），
		// 避免 Allow 被外部调用时重复记录日志和指标
		return true, 0, time.Time{}, err
	}

	// 解析 Lua 返回的 {allowed, remaining}
	vals, ok := result.([]interface{})
	if !ok || len(vals) < 2 {
		return true, 0, time.Time{}, nil
	}

	allowed, _ := vals[0].(int64)
	remaining, _ := vals[1].(int64)
	resetTime := now.Add(drl.window)

	return allowed == 1, int(remaining), resetTime, nil
}

// buildKey 构建Redis键
func (drl *DistributedRateLimiter) buildKey(clientIP string) string {
	return drl.keyPrefix + ":" + clientIP
}

// Stop 停止分布式限流器（含内存降级限流器的后台清理 goroutine）
func (drl *DistributedRateLimiter) Stop() {
	if drl.fallback != nil {
		drl.fallback.Stop()
	}
}
