// Package middleware T10 限流 Redis 故障降级测试（M4 方案 B）
package middleware_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/middleware"
)

// setupBrokenRedisClient 创建指向已关闭 Redis 的客户端（触发操作错误）
func setupBrokenRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t, client.Ping(context.Background()).Err())
	t.Cleanup(func() { client.Close() })
	mr.Close() // 关闭服务端，后续操作必然失败
	return client
}

// countingSlogHandler 统计 Error 级日志条数的测试 handler
type countingSlogHandler struct {
	mu         sync.Mutex
	errorCount int
}

func (h *countingSlogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *countingSlogHandler) WithAttrs(_ []slog.Attr) slog.Handler         { return h }
func (h *countingSlogHandler) WithGroup(_ string) slog.Handler              { return h }
func (h *countingSlogHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelError {
		h.mu.Lock()
		h.errorCount++
		h.mu.Unlock()
	}
	return nil
}

// TestDistributedRateLimiter_MemoryFallback 验证 Redis 故障时降级内存限流且限额仍生效
func TestDistributedRateLimiter_MemoryFallback(t *testing.T) {
	redisClient := setupBrokenRedisClient(t)

	errorCalled := 0
	metricCalled := 0
	limiter := middleware.NewDistributedRateLimiter(redisClient, 3, time.Minute, "test_fallback").
		WithMemoryFallback().
		WithErrorCallback(func() { errorCalled++ }).
		WithMetrics(func() { metricCalled++ })
	defer limiter.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := limiter.Middleware(handler)

	doRequest := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.9.0.1:1234"
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		return rr
	}

	// 降级后前 3 次放行（与 Redis 路径同限额）
	for i := 1; i <= 3; i++ {
		rr := doRequest()
		assert.Equal(t, http.StatusOK, rr.Code, "降级期间第%d次请求应放行", i)
	}

	// 第 4 次超过内存限额，拒绝（不再 fail-open 无限放行）
	rr := doRequest()
	assert.Equal(t, http.StatusTooManyRequests, rr.Code, "降级期间超过限额应返回 429")
	assert.Equal(t, 4, errorCalled, "每次降级请求都应触发错误指标")
	assert.Equal(t, 1, metricCalled, "降级限流触发应调用 metric 回调一次")
}

// TestDistributedRateLimiter_MemoryFallback_LogThrottled 验证降级日志每分钟最多一条
func TestDistributedRateLimiter_MemoryFallback_LogThrottled(t *testing.T) {
	redisClient := setupBrokenRedisClient(t)

	limiter := middleware.NewDistributedRateLimiter(redisClient, 100, time.Minute, "test_throttle").
		WithMemoryFallback()
	defer limiter.Stop()

	handler := &countingSlogHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(prev) })

	mw := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.9.0.2:1234"
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	}

	assert.Equal(t, 1, handler.errorCount, "降级日志应节流为每分钟一条")
}

// TestDistributedRateLimiter_NoFallback_StillFailOpen 验证未启用降级时保持 fail-open
func TestDistributedRateLimiter_NoFallback_StillFailOpen(t *testing.T) {
	redisClient := setupBrokenRedisClient(t)

	limiter := middleware.NewDistributedRateLimiter(redisClient, 1, time.Minute, "test_no_fallback")
	mw := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 限额为 1，但无降级时 Redis 错误仍 fail-open 放行（向后兼容）
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.9.0.3:1234"
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "未启用降级时应保持 fail-open")
	}
}

// TestDistributedRateLimiter_MemoryFallback_PerClientIsolation 验证降级期间按客户端独立计数
func TestDistributedRateLimiter_MemoryFallback_PerClientIsolation(t *testing.T) {
	redisClient := setupBrokenRedisClient(t)

	limiter := middleware.NewDistributedRateLimiter(redisClient, 1, time.Minute, "test_isolation").
		WithMemoryFallback()
	defer limiter.Stop()

	mw := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	doRequest := func(ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip + ":1234"
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		return rr
	}

	assert.Equal(t, http.StatusOK, doRequest("10.9.1.1").Code)
	assert.Equal(t, http.StatusOK, doRequest("10.9.1.2").Code, "其他客户端不受限流影响")
	assert.Equal(t, http.StatusTooManyRequests, doRequest("10.9.1.1").Code, "超限客户端应被拒绝")
}
