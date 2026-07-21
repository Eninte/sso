// Package service T10 服务层限流 Redis 故障降级测试（M4 方案 B）
// 使用 miniredis 构造真实 Redis 缓存后关闭服务端，模拟 Redis 故障
package service

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
)

// setupBrokenRedisCache 创建指向已关闭 Redis 的缓存（触发操作错误）
func setupBrokenRedisCache(t *testing.T) cache.Cache {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	rc, err := cache.NewRedisCacheWithOptions(&redis.Options{Addr: mr.Addr()})
	require.NoError(t, err)
	t.Cleanup(func() { rc.Close() })
	mr.Close() // 关闭服务端，后续操作必然失败
	return rc
}

// TestLoginRateLimiter_RedisDownFallback 验证登录限流 Redis 故障降级（T10）
func TestLoginRateLimiter_RedisDownFallback(t *testing.T) {
	ctx := context.Background()
	brokenCache := setupBrokenRedisCache(t)

	errorCalled := 0
	limiter := NewLoginRateLimiter(brokenCache).WithErrorCallback(func() { errorCalled++ })
	ip := "10.8.0.1"

	// 降级期间限额仍生效：前 LoginRateLimitMax 次放行
	for i := 1; i <= LoginRateLimitMax; i++ {
		allowed, _, err := limiter.CheckAndRecord(ctx, ip)
		assert.NoError(t, err)
		assert.True(t, allowed, "降级期间第%d次尝试应放行", i)
	}

	// 超过限额拒绝（不再静默放行）
	allowed, remaining, err := limiter.CheckAndRecord(ctx, ip)
	assert.NoError(t, err)
	assert.False(t, allowed, "降级期间超过限额应拒绝")
	assert.Equal(t, 0, remaining)
	assert.Equal(t, LoginRateLimitMax+1, errorCalled, "每次降级请求都应触发错误指标")
}

// TestLoginRateLimiter_RedisDownFallback_PerIPIsolation 验证降级期间按 IP 独立计数
func TestLoginRateLimiter_RedisDownFallback_PerIPIsolation(t *testing.T) {
	ctx := context.Background()
	brokenCache := setupBrokenRedisCache(t)

	limiter := NewLoginRateLimiter(brokenCache)

	for i := 0; i < LoginRateLimitMax; i++ {
		limiter.CheckAndRecord(ctx, "10.8.1.1")
	}
	allowed, _, err := limiter.CheckAndRecord(ctx, "10.8.1.2")
	assert.NoError(t, err)
	assert.True(t, allowed, "其他 IP 不受限流影响")
}

// TestEmailRateLimiter_RedisDownFallback 验证邮件限流 Redis 故障降级（T10）
func TestEmailRateLimiter_RedisDownFallback(t *testing.T) {
	ctx := context.Background()
	brokenCache := setupBrokenRedisCache(t)

	errorCalled := 0
	limiter := NewEmailRateLimiter(brokenCache).WithErrorCallback(func() { errorCalled++ })
	email := "fallback@example.com"

	// 降级期间限额仍生效：前 EmailRateLimitMax 次放行
	for i := 1; i <= EmailRateLimitMax; i++ {
		allowed, _, err := limiter.CheckLimit(ctx, email)
		assert.NoError(t, err)
		assert.True(t, allowed, "降级期间第%d次发送应放行", i)
	}

	// 超过限额拒绝（不再静默放行）
	allowed, remaining, err := limiter.CheckLimit(ctx, email)
	assert.NoError(t, err)
	assert.False(t, allowed, "降级期间超过限额应拒绝")
	assert.Equal(t, 0, remaining)
	assert.Equal(t, EmailRateLimitMax+1, errorCalled, "每次降级请求都应触发错误指标")
}

// TestEmailRateLimiter_RedisDownGetRemaining 验证 GetRemaining 读取故障补齐可观测性
func TestEmailRateLimiter_RedisDownGetRemaining(t *testing.T) {
	ctx := context.Background()
	brokenCache := setupBrokenRedisCache(t)

	errorCalled := 0
	limiter := NewEmailRateLimiter(brokenCache).WithErrorCallback(func() { errorCalled++ })

	remaining, err := limiter.GetRemaining(ctx, "readonly@example.com")
	assert.NoError(t, err)
	assert.Equal(t, EmailRateLimitMax, remaining, "读取故障时返回最大剩余次数（向后兼容）")
	assert.Equal(t, 1, errorCalled, "读取故障应触发错误指标")
}
