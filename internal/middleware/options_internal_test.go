// Package middleware 内部测试（覆盖 options 构造函数与 Stop 方法）
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/model"
	mockstore "github.com/example/sso/internal/store/mock"
)

// 注意：crypto.GenerateRSAKeyPair 返回 (*rsa.PrivateKey, error)

// ============================================================================
// AuthMiddlewareWithStore / WithCache / WithMetrics 测试
// 覆盖原本 0% 的三个带依赖注入的认证中间件构造函数
// ============================================================================

// createInternalJWTService 创建测试用 JWT 服务（内部测试辅助）
func createInternalJWTService(t *testing.T) *crypto.JWTService {
	t.Helper()
	privateKey, err := crypto.GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	return crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)
}

// storeTokenToMock 向 mock store 注入 token 记录
func storeTokenToMock(t *testing.T, s *mockstore.Store, accessToken string, revoked bool) {
	t.Helper()
	var revokedAt *time.Time
	if revoked {
		now := time.Now()
		revokedAt = &now
	}
	require.NoError(t, s.StoreToken(context.Background(), &model.Token{
		AccessToken: accessToken,
		RevokedAt:   revokedAt,
	}))
}

// makeRequest 构造带 Bearer token 的请求
func makeRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// TestAuthMiddlewareWithStore 覆盖 AuthMiddlewareWithStore
// 验证：有效 token + 未撤销 → 通过；token 已撤销 → 401
func TestAuthMiddlewareWithStore(t *testing.T) {
	jwtSvc := createInternalJWTService(t)
	s := mockstore.New()

	// 生成两个 token：一个未撤销，一个已撤销
	validToken, err := jwtSvc.GenerateAccessToken("user-1", "u1@example.com", "user", nil)
	require.NoError(t, err)
	revokedToken, err := jwtSvc.GenerateAccessToken("user-2", "u2@example.com", "user", nil)
	require.NoError(t, err)

	storeTokenToMock(t, s, validToken, false)
	storeTokenToMock(t, s, revokedToken, true)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AuthMiddlewareWithStore(jwtSvc, s)(handler)

	t.Run("未撤销token_通过", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, makeRequest(validToken))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("已撤销token_401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, makeRequest(revokedToken))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("DB中不存在的token_视为已撤销_401", func(t *testing.T) {
		// store.GetTokenByAccessToken 返回 ErrNotFound → isBlacklisted 返回 true
		unknownToken, err := jwtSvc.GenerateAccessToken("user-x", "x@example.com", "user", nil)
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, makeRequest(unknownToken))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("DB错误时_fail-closed_401", func(t *testing.T) {
		// DB 错误（非 ErrNotFound）时应 fail-closed，拒绝请求
		s.GetTokenByAccessTokenErr = fmt.Errorf("database connection lost")
		defer func() { s.GetTokenByAccessTokenErr = nil }()

		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, makeRequest(validToken))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// TestAuthMiddlewareWithCache 覆盖 AuthMiddlewareWithCache
// 验证：缓存命中时跳过 DB 查询；缓存未命中时回源 DB
func TestAuthMiddlewareWithCache(t *testing.T) {
	jwtSvc := createInternalJWTService(t)
	s := mockstore.New()
	c := cache.NewMemoryCache()
	defer c.Close()

	validToken, err := jwtSvc.GenerateAccessToken("user-1", "u1@example.com", "user", nil)
	require.NoError(t, err)
	storeTokenToMock(t, s, validToken, false)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AuthMiddlewareWithCache(jwtSvc, s, c)(handler)

	t.Run("首次请求_缓存未命中_回源DB_通过", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, makeRequest(validToken))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("二次请求_缓存命中_仍通过", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, makeRequest(validToken))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("撤销后缓存反映新状态", func(t *testing.T) {
		// 手动在缓存中标记为已撤销
		ctx := context.Background()
		require.NoError(t, c.Set(ctx, cache.TokenKey(validToken), true, cache.TokenTTL))

		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, makeRequest(validToken))
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "缓存标记撤销后应拒绝")
	})
}

// TestAuthMiddlewareWithMetrics 覆盖 AuthMiddlewareWithMetrics
// 验证：invalidTokenFunc 在 token 无效时被调用
func TestAuthMiddlewareWithMetrics(t *testing.T) {
	jwtSvc := createInternalJWTService(t)
	s := mockstore.New()
	c := cache.NewMemoryCache()
	defer c.Close()

	invalidCalled := 0
	mw := AuthMiddlewareWithMetrics(jwtSvc, s, c, func() {
		invalidCalled++
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("缺少Authorization头_触发invalidTokenFunc", func(t *testing.T) {
		invalidCalled = 0
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, makeRequest(""))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, 1, invalidCalled, "无效 token 应触发指标回调")
	})

	t.Run("无效token格式_触发invalidTokenFunc", func(t *testing.T) {
		invalidCalled = 0
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "InvalidFormat")
		mw.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, 1, invalidCalled)
	})

	t.Run("有效token_不触发invalidTokenFunc", func(t *testing.T) {
		invalidCalled = 0
		validToken, err := jwtSvc.GenerateAccessToken("u", "u@example.com", "user", nil)
		require.NoError(t, err)
		storeTokenToMock(t, s, validToken, false)

		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, makeRequest(validToken))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, 0, invalidCalled, "有效 token 不应触发指标回调")
	})
}

// ============================================================================
// RateLimiter.WithMetrics 测试
// 覆盖原本 0% 的指标回调设置方法
// ============================================================================

func TestRateLimiter_WithMetrics(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	defer rl.Stop()

	t.Run("返回同一实例_链式调用", func(t *testing.T) {
		returned := rl.WithMetrics(func() {})
		assert.Same(t, rl, returned, "WithMetrics 应返回原实例")
	})

	t.Run("限流触发时调用metricFunc", func(t *testing.T) {
		triggered := 0
		rl2 := NewRateLimiter(1, time.Minute)
		defer rl2.Stop()
		rl2.WithMetrics(func() { triggered++ })

		handler := rl2.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// 第1次请求允许，第2次被限流
		req1 := httptest.NewRequest(http.MethodGet, "/", nil)
		req1.RemoteAddr = "1.1.1.1:1234"
		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		req2.RemoteAddr = "1.1.1.1:1234"

		handler.ServeHTTP(httptest.NewRecorder(), req1)
		handler.ServeHTTP(httptest.NewRecorder(), req2)

		assert.Equal(t, 1, triggered, "第2次请求被限流应触发 metricFunc")
	})

	t.Run("metricFunc为nil时不panic", func(t *testing.T) {
		// 不设置 WithMetrics，metricFunc 保持 nil
		rl3 := NewRateLimiter(1, time.Minute)
		defer rl3.Stop()

		handler := rl3.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "2.2.2.2:1234"
		assert.NotPanics(t, func() {
			handler.ServeHTTP(httptest.NewRecorder(), req)
		})
	})
}

// ============================================================================
// DistributedRateLimiter.Stop 测试
// 覆盖原本 0% 的空操作 Stop 方法（接口一致性）
// ============================================================================

func TestDistributedRateLimiter_Stop(t *testing.T) {
	t.Run("Stop为空操作_不panic", func(t *testing.T) {
		drl := &DistributedRateLimiter{
			redisClient: nil, // Stop 不访问 redisClient
			limit:       10,
			window:      time.Minute,
			keyPrefix:   "test",
		}
		assert.NotPanics(t, func() {
			drl.Stop()
		}, "Stop 应为空操作，不访问任何资源")
	})

	t.Run("Stop可多次调用_幂等", func(t *testing.T) {
		drl := &DistributedRateLimiter{}
		assert.NotPanics(t, func() {
			drl.Stop()
			drl.Stop()
			drl.Stop()
		}, "空操作 Stop 应可多次幂等调用")
	})
}
