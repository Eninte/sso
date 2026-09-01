// Package middleware_test AuthMiddlewareOptional（浏览器 OIDC 可选认证）单元测试
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store/mock"
)

// seedOptionalAuthToken 向 mock store 注入一个未撤销的 token 记录
func seedOptionalAuthToken(store *mock.Store, accessToken string, revoked bool) {
	token := &model.Token{
		ID:          "id-" + accessToken,
		AccessToken: accessToken,
		UserID:      "user-123",
		ExpiresAt:   time.Now().Add(15 * time.Minute),
		CreatedAt:   time.Now(),
	}
	if revoked {
		now := time.Now()
		token.RevokedAt = &now
	}
	store.AddToken(token)
}

// optionalAuthProbe 返回一个记录认证状态的 handler
func optionalAuthProbe(w http.ResponseWriter, r *http.Request) {
	if middleware.GetUserIDFromContext(r.Context()) != "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusTeapot) // 418：放行但未认证
}

func TestAuthMiddlewareOptional(t *testing.T) {
	jwtSvc := createTestJWTService(t)

	newMiddleware := func(store *mock.Store) (func(http.Handler) http.Handler, *int) {
		invalidCalls := 0
		mw := middleware.AuthMiddlewareOptional(jwtSvc, store, cache.NewMemoryCache(), func() {
			invalidCalls++
		})
		return mw, &invalidCalls
	}

	t.Run("无任何凭据：放行且不认证", func(t *testing.T) {
		mw, invalidCalls := newMiddleware(mock.New())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/authorize", nil)
		rec := httptest.NewRecorder()

		mw(http.HandlerFunc(optionalAuthProbe)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTeapot, rec.Code)
		assert.Equal(t, 0, *invalidCalls)
	})

	t.Run("Bearer有效Token：认证通过", func(t *testing.T) {
		store := mock.New()
		token, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
		require.NoError(t, err)
		seedOptionalAuthToken(store, token, false)

		mw, _ := newMiddleware(store)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/authorize", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		mw(http.HandlerFunc(optionalAuthProbe)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("会话Cookie有效Token：认证通过", func(t *testing.T) {
		store := mock.New()
		token, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
		require.NoError(t, err)
		seedOptionalAuthToken(store, token, false)

		mw, _ := newMiddleware(store)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/authorize", nil)
		req.AddCookie(&http.Cookie{Name: middleware.BrowserSessionCookie, Value: token})
		rec := httptest.NewRecorder()

		mw(http.HandlerFunc(optionalAuthProbe)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("Bearer优先于Cookie", func(t *testing.T) {
		store := mock.New()
		headerToken, err := jwtSvc.GenerateAccessToken("user-header", "h@example.com", "user", nil)
		require.NoError(t, err)
		cookieToken, err := jwtSvc.GenerateAccessToken("user-cookie", "c@example.com", "user", nil)
		require.NoError(t, err)
		seedOptionalAuthToken(store, headerToken, false)
		seedOptionalAuthToken(store, cookieToken, false)

		mw, _ := newMiddleware(store)
		var gotUserID string
		probe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUserID = middleware.GetUserIDFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/authorize", nil)
		req.Header.Set("Authorization", "Bearer "+headerToken)
		req.AddCookie(&http.Cookie{Name: middleware.BrowserSessionCookie, Value: cookieToken})
		rec := httptest.NewRecorder()

		mw(probe).ServeHTTP(rec, req)

		assert.Equal(t, "user-header", gotUserID)
	})

	t.Run("无效Token：放行但未认证并计数", func(t *testing.T) {
		mw, invalidCalls := newMiddleware(mock.New())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/authorize", nil)
		req.Header.Set("Authorization", "Bearer invalid.token.here")
		rec := httptest.NewRecorder()

		mw(http.HandlerFunc(optionalAuthProbe)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTeapot, rec.Code)
		assert.Equal(t, 1, *invalidCalls)
	})

	t.Run("已撤销Token：放行但未认证", func(t *testing.T) {
		store := mock.New()
		token, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", nil)
		require.NoError(t, err)
		seedOptionalAuthToken(store, token, true)

		mw, invalidCalls := newMiddleware(store)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/authorize", nil)
		req.AddCookie(&http.Cookie{Name: middleware.BrowserSessionCookie, Value: token})
		rec := httptest.NewRecorder()

		mw(http.HandlerFunc(optionalAuthProbe)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTeapot, rec.Code)
		assert.Equal(t, 1, *invalidCalls)
	})

	t.Run("Token不存在于Store：按未认证放行", func(t *testing.T) {
		token, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", nil)
		require.NoError(t, err)

		mw, _ := newMiddleware(mock.New())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/authorize", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		mw(http.HandlerFunc(optionalAuthProbe)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTeapot, rec.Code)
	})
}
