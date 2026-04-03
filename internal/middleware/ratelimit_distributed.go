// Package middleware 分布式限流中间件
// 使用Redis实现分布式限流，支持多实例共享限流状态
package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// ============================================================================
// DistributedRateLimiter 分布式限流器
// ============================================================================

// DistributedRateLimiter 基于Redis的分布式限流器
// 使用滑动窗口算法实现精确限流
type DistributedRateLimiter struct {
	redisClient *redis.Client
	limit       int           // 每个时间窗口的请求数
	window      time.Duration // 时间窗口
	keyPrefix   string        // Redis键前缀
}

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

// Middleware 分布式限流中间件
// 检查客户端请求频率，超过限制返回429
func (drl *DistributedRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 获取客户端标识
		clientIP := getClientIP(r)

		// 检查是否超过限制
		allowed, remaining, resetTime, err := drl.Allow(r.Context(), clientIP)
		if err != nil {
			// Redis错误时，默认允许请求（fail-open）
			next.ServeHTTP(w, r)
			return
		}

		// 设置限流响应头
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(drl.limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		if !allowed {
			w.Header().Set("Retry-After", drl.window.String())
			writeError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			return
		}

		// 继续处理请求
		next.ServeHTTP(w, r)
	})
}

// Allow 检查是否允许请求
// 返回: allowed, remaining, resetTime, error
func (drl *DistributedRateLimiter) Allow(ctx context.Context, clientIP string) (bool, int, time.Time, error) {
	// limit <= 0 表示禁用限流
	if drl.limit <= 0 {
		return true, drl.limit, time.Time{}, nil
	}

	now := time.Now()
	key := drl.buildKey(clientIP)
	windowStart := now.Add(-drl.window)

	// 使用Redis事务实现原子操作
	pipe := drl.redisClient.TxPipeline()

	// 移除窗口外的记录
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart.UnixNano(), 10))

	// 获取当前窗口内的请求数
	pipe.ZCard(ctx, key)

	// 执行管道命令
	results, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return true, 0, time.Time{}, err
	}

	// 获取当前请求数
	cardResult := results[1].(*redis.IntCmd)
	currentCount := int(cardResult.Val())

	// 检查是否超过限制
	if currentCount >= drl.limit {
		resetTime := now.Add(drl.window)
		return false, 0, resetTime, nil
	}

	// 添加当前请求记录
	pipe = drl.redisClient.TxPipeline()
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: strconv.FormatInt(now.UnixNano(), 10),
	})
	pipe.Expire(ctx, key, drl.window*2)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return true, 0, time.Time{}, err
	}

	remaining := drl.limit - currentCount - 1
	resetTime := now.Add(drl.window)

	return true, remaining, resetTime, nil
}

// buildKey 构建Redis键
func (drl *DistributedRateLimiter) buildKey(clientIP string) string {
	return drl.keyPrefix + ":" + clientIP
}
