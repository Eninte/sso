// Package middleware 分布式限流器测试
package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/middleware"
)

// setupTestRedis 创建测试用的Redis客户端
func setupTestRedis(t *testing.T) *redis.Client {
	t.Helper()

	// 创建迷你Redis服务器
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	// 创建Redis客户端
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// 测试连接
	err = client.Ping(context.Background()).Err()
	require.NoError(t, err)

	t.Cleanup(func() { client.Close() })

	return client
}

func TestDistributedRateLimiter_Basic(t *testing.T) {
	redisClient := setupTestRedis(t)
	limiter := middleware.NewDistributedRateLimiter(redisClient, 5, time.Minute, "test")

	t.Run("允许请求在限制内", func(t *testing.T) {
		ctx := context.Background()
		allowed, remaining, _, err := limiter.Allow(ctx, "192.168.1.1")

		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 4, remaining) // 5 - 1 = 4
	})

	t.Run("禁用限流时始终允许", func(t *testing.T) {
		disabledLimiter := middleware.NewDistributedRateLimiter(redisClient, 0, time.Minute, "test_disabled")
		ctx := context.Background()
		allowed, remaining, _, err := disabledLimiter.Allow(ctx, "192.168.1.1")

		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 0, remaining)
	})
}

func TestDistributedRateLimiter_Limit(t *testing.T) {
	redisClient := setupTestRedis(t)
	limiter := middleware.NewDistributedRateLimiter(redisClient, 3, time.Minute, "test_limit")
	ctx := context.Background()

	// 前3个请求应该允许
	for i := 0; i < 3; i++ {
		allowed, _, _, err := limiter.Allow(ctx, "192.168.1.2")
		require.NoError(t, err)
		assert.True(t, allowed, "请求 %d 应该被允许", i+1)
	}

	// 第4个请求应该被拒绝
	allowed, remaining, _, err := limiter.Allow(ctx, "192.168.1.2")
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, 0, remaining)
}

func TestDistributedRateLimiter_Window(t *testing.T) {
	redisClient := setupTestRedis(t)
	window := 100 * time.Millisecond
	limiter := middleware.NewDistributedRateLimiter(redisClient, 2, window, "test_window")
	ctx := context.Background()

	// 前2个请求应该允许
	allowed1, _, _, err1 := limiter.Allow(ctx, "192.168.1.3")
	require.NoError(t, err1)
	assert.True(t, allowed1)

	allowed2, _, _, err2 := limiter.Allow(ctx, "192.168.1.3")
	require.NoError(t, err2)
	assert.True(t, allowed2)

	// 第3个请求应该被拒绝
	allowed3, _, _, err3 := limiter.Allow(ctx, "192.168.1.3")
	require.NoError(t, err3)
	assert.False(t, allowed3)

	// 等待窗口过期
	time.Sleep(window + 50*time.Millisecond)

	// 窗口过期后应该允许新请求
	allowed4, _, _, err4 := limiter.Allow(ctx, "192.168.1.3")
	require.NoError(t, err4)
	assert.True(t, allowed4)
}

func TestDistributedRateLimiter_DifferentClients(t *testing.T) {
	redisClient := setupTestRedis(t)
	limiter := middleware.NewDistributedRateLimiter(redisClient, 2, time.Minute, "test_different")
	ctx := context.Background()

	// 客户端1的2个请求
	allowed1, _, _, err1 := limiter.Allow(ctx, "client1")
	require.NoError(t, err1)
	assert.True(t, allowed1)

	allowed2, _, _, err2 := limiter.Allow(ctx, "client1")
	require.NoError(t, err2)
	assert.True(t, allowed2)

	// 客户端1的第3个请求应该被拒绝
	allowed3, _, _, err3 := limiter.Allow(ctx, "client1")
	require.NoError(t, err3)
	assert.False(t, allowed3)

	// 客户端2的请求应该允许（独立限流）
	allowed4, _, _, err4 := limiter.Allow(ctx, "client2")
	require.NoError(t, err4)
	assert.True(t, allowed4)
}

func TestDistributedRateLimiter_Middleware(t *testing.T) {
	redisClient := setupTestRedis(t)
	limiter := middleware.NewDistributedRateLimiter(redisClient, 2, time.Minute, "test_middleware")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middlewareHandler := limiter.Middleware(handler)

	t.Run("允许请求", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rr := httptest.NewRecorder()

		middlewareHandler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.NotEmpty(t, rr.Header().Get("X-Ratelimit-Limit"))
		assert.NotEmpty(t, rr.Header().Get("X-Ratelimit-Remaining"))
	})
}

func TestDistributedRateLimiter_BuildKey(t *testing.T) {
	redisClient := setupTestRedis(t)
	limiter := middleware.NewDistributedRateLimiter(redisClient, 10, time.Minute, "myprefix")

	// 验证键构建
	// buildKey 是私有方法，通过 Allow 的行为间接测试
	ctx := context.Background()
	allowed, _, _, err := limiter.Allow(ctx, "192.168.1.1")
	require.NoError(t, err)
	assert.True(t, allowed) // 第一个请求应该被允许
	assert.True(t, allowed) // 第一个请求应该被允许

	// 验证Redis中是否有正确的键
	keys, err := redisClient.Keys(ctx, "myprefix:*").Result()
	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Contains(t, keys[0], "192.168.1.1")
}
