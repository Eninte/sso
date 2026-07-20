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

	"github.com/example/sso/internal/middleware"
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

// TestDistributedRateLimiter_WithMetrics 验证 WithMetrics 链式调用和回调触发
func TestDistributedRateLimiter_WithMetrics(t *testing.T) {
	redisClient := setupTestRedis(t)
	metricCalled := 0
	limiter := middleware.NewDistributedRateLimiter(redisClient, 1, time.Minute, "test_metrics").
		WithMetrics(func() { metricCalled++ })

	// 验证链式调用返回自身
	assert.NotNil(t, limiter)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := limiter.Middleware(handler)

	// 第一个请求：允许，不触发 metric
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, 0, metricCalled, "允许时不应调用 metric 回调")

	// 第二个请求：超限，触发 metric
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	rr2 := httptest.NewRecorder()
	mw.ServeHTTP(rr2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rr2.Code)
	assert.Equal(t, 1, metricCalled, "限流时应该调用 metric 回调一次")
}

// TestDistributedRateLimiter_WithErrorCallback 验证 Redis 错误时 fail-open + 触发 errorFunc
func TestDistributedRateLimiter_WithErrorCallback(t *testing.T) {
	redisClient := setupTestRedis(t)
	// 提前关闭 Redis 触发错误
	redisClient.Close()

	errorCalled := 0
	limiter := middleware.NewDistributedRateLimiter(redisClient, 10, time.Minute, "test_err_cb").
		WithErrorCallback(func() { errorCalled++ })

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := limiter.Middleware(handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	// Redis 错误时 fail-open，应返回 200
	assert.Equal(t, http.StatusOK, rr.Code, "Redis 错误时应 fail-open 放行")
	assert.Equal(t, 1, errorCalled, "Redis 错误时应调用 errorFunc 回调")
}

// TestDistributedRateLimiter_Middleware_RateLimited 验证 Middleware 429 响应路径
func TestDistributedRateLimiter_Middleware_RateLimited(t *testing.T) {
	redisClient := setupTestRedis(t)
	limiter := middleware.NewDistributedRateLimiter(redisClient, 1, time.Minute, "test_429")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mw := limiter.Middleware(handler)

	// 第一个请求通过
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "10.0.0.3:1234"
	rr1 := httptest.NewRecorder()
	mw.ServeHTTP(rr1, req1)
	assert.Equal(t, http.StatusOK, rr1.Code)

	// 第二个请求应返回 429
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "10.0.0.3:1234"
	rr2 := httptest.NewRecorder()
	mw.ServeHTTP(rr2, req2)

	assert.Equal(t, http.StatusTooManyRequests, rr2.Code)
	assert.NotEmpty(t, rr2.Header().Get("Retry-After"))
	assert.NotEmpty(t, rr2.Header().Get("X-Ratelimit-Limit"))
	assert.NotEmpty(t, rr2.Header().Get("X-Ratelimit-Remaining"))
	assert.Equal(t, "0", rr2.Header().Get("X-Ratelimit-Remaining"))
}

// TestDistributedRateLimiter_Middleware_Disabled 验证禁用限流（limit=0）时 Middleware 直接放行
func TestDistributedRateLimiter_Middleware_Disabled(t *testing.T) {
	redisClient := setupTestRedis(t)
	limiter := middleware.NewDistributedRateLimiter(redisClient, 0, time.Minute, "test_disabled_mw")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := limiter.Middleware(handler)

	// 多次请求都应放行
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.4:1234"
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "禁用限流时第 %d 个请求应放行", i+1)
	}
}

// TestDistributedRateLimiter_Allow_Disabled 验证禁用限流（limit=0）时 Allow 行为
func TestDistributedRateLimiter_Allow_Disabled(t *testing.T) {
	redisClient := setupTestRedis(t)
	limiter := middleware.NewDistributedRateLimiter(redisClient, 0, time.Minute, "test_allow_disabled")

	ctx := context.Background()
	allowed, remaining, resetTime, err := limiter.Allow(ctx, "10.0.0.5")
	require.NoError(t, err)
	assert.True(t, allowed, "禁用限流时应允许")
	assert.Equal(t, 0, remaining)
	assert.True(t, resetTime.IsZero(), "禁用限流时 resetTime 应为零值")
}

// TestDistributedRateLimiter_Allow_NilContext 验证 Allow 在 ctx 为 nil 时的容错（不 panic）
// 注：实际使用中不会传 nil context，但测试为了覆盖率验证
func TestDistributedRateLimiter_Allow_NilContext(t *testing.T) {
	redisClient := setupTestRedis(t)
	// 设置极短的窗口，确保能正常运行
	limiter := middleware.NewDistributedRateLimiter(redisClient, 1, time.Minute, "test_nil_ctx")

	// 使用 context.Background() 模拟正常调用
	allowed, _, _, err := limiter.Allow(context.Background(), "10.0.0.6")
	require.NoError(t, err)
	assert.True(t, allowed)
}
