// Package handler_test Userinfo, Metrics, Social Handler补充测试
package handler_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/metrics"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// createTestJWTService 创建测试用的JWT服务
func createTestJWTService() *crypto.JWTService {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	return crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)
}

// ============================================================================
// MetricsHandler 测试
// ============================================================================

func TestMetricsHandler_HandleMetrics(t *testing.T) {
	metricsSvc := metrics.NewService()

	t.Run("返回指标数据", func(t *testing.T) {
		h := handler.NewMetricsHandler(metricsSvc)
		metricsSvc.Increment("http_requests_total")

		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()

		h.HandleMetrics(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/plain; version=0.0.4", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "http_requests_total")
	})

	t.Run("无指标时返回空数据", func(t *testing.T) {
		h := handler.NewMetricsHandler(metrics.NewService())

		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()

		h.HandleMetrics(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/plain; version=0.0.4", w.Header().Get("Content-Type"))
	})
}

// ============================================================================
// BasicAuth中间件测试
// ============================================================================

func TestBasicAuth_Middleware(t *testing.T) {
	metricsSvc := metrics.NewService()
	metricsHandler := handler.NewMetricsHandler(metricsSvc)

	t.Run("无认证配置时直接通过", func(t *testing.T) {
		middleware := middleware.BasicAuth("", "")
		handler := middleware(http.HandlerFunc(metricsHandler.HandleMetrics))

		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("配置认证后无凭据返回401", func(t *testing.T) {
		middleware := middleware.BasicAuth("admin", "secret")
		handler := middleware(http.HandlerFunc(metricsHandler.HandleMetrics))

		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Basic")
	})

	t.Run("配置认证后错误凭据返回401", func(t *testing.T) {
		middleware := middleware.BasicAuth("admin", "secret")
		handler := middleware(http.HandlerFunc(metricsHandler.HandleMetrics))

		req := httptest.NewRequest("GET", "/metrics", nil)
		req.SetBasicAuth("admin", "wrong")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("配置认证后正确凭据返回200", func(t *testing.T) {
		middleware := middleware.BasicAuth("admin", "secret")
		handler := middleware(http.HandlerFunc(metricsHandler.HandleMetrics))

		req := httptest.NewRequest("GET", "/metrics", nil)
		req.SetBasicAuth("admin", "secret")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ============================================================================
// SocialLoginHandler 测试
// ============================================================================

func createTestSocialLoginHandler(t *testing.T) *handler.SocialLoginHandler {
	storeInst := mock.New()
	jwtSvc := createTestJWTService()
	socialSvc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "g-id", "g-secret", "gh-id", "gh-secret")
	return handler.NewSocialLoginHandler(socialSvc)
}

func TestSocialLoginHandler_HandleLogin(t *testing.T) {
	h := createTestSocialLoginHandler(t)

	t.Run("Google登录重定向-通过query参数", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth?provider=google&state=random-state-1234567890&redirect_uri=http://localhost/callback", nil)
		w := httptest.NewRecorder()

		h.HandleLogin(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "accounts.google.com")
	})

	t.Run("GitHub登录重定向-通过query参数", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth?provider=github&state=random-state-1234567890&redirect_uri=http://localhost/callback", nil)
		w := httptest.NewRecorder()

		h.HandleLogin(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "github.com")
	})

	t.Run("不支持的提供商", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/unsupported?state=random-state-1234567890", nil)
		w := httptest.NewRecorder()

		h.HandleLogin(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空provider无路径", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth?state=random-state-1234567890", nil)
		w := httptest.NewRecorder()

		h.HandleLogin(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestSocialLoginHandler_HandleCallback(t *testing.T) {
	h := createTestSocialLoginHandler(t)

	t.Run("缺少授权码", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/google/callback", nil)
		w := httptest.NewRecorder()

		h.HandleCallback(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("有授权码但不支持的提供商", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/unsupported/callback?code=abc123&state=test-state", nil)
		w := httptest.NewRecorder()

		h.HandleCallback(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestSocialLoginHandler_HandleProviders(t *testing.T) {
	h := createTestSocialLoginHandler(t)

	t.Run("返回提供商列表", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/providers", nil)
		w := httptest.NewRecorder()

		h.HandleProviders(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var providers []map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &providers)
		require.NoError(t, err)
		assert.Len(t, providers, 2)

		// 验证提供商包含Google和GitHub
		names := make([]string, len(providers))
		for i, p := range providers {
			names[i] = p["name"]
		}
		assert.Contains(t, names, "google")
		assert.Contains(t, names, "github")
	})
}

// ============================================================================
// writeLocalizedError / writeOAuthError 测试
// ============================================================================

func TestHandlerErrorFunctions(t *testing.T) {
	t.Run("writeLocalizedError", func(t *testing.T) {
		// 通过验证失败触发写入错误
		storeInst := mock.New()
		passwordSvc := crypto.NewPasswordService(4)
		jwtSvc := createTestJWTService()
		authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)
		loginHandler := handler.NewLoginHandler(authSvc, testCaptchaSvc)

		// 无效JSON触发writeError
		req := httptest.NewRequest("POST", "/login", nil)
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"无效JSON请求应返回400 Bad Request")
	})

	t.Run("writeOAuthError - ErrInvalidClient", func(t *testing.T) {
		storeInst := mock.New()
		tokenSvc := service.NewTokenService(createTestJWTService(), storeInst)
		passwordSvc := crypto.NewPasswordService(4)
		oauthSvc := service.NewOAuthService(storeInst, tokenSvc, service.WithOAuthPassword(passwordSvc))
		h := handler.NewAuthorizeHandler(oauthSvc)

		req := httptest.NewRequest("GET", "/authorize?client_id=invalid&redirect_uri=http://localhost", nil)
		w := httptest.NewRecorder()
		h.HandleAuthorize(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("writeValidationError - validation error triggers 400", func(t *testing.T) {
		storeInst := mock.New()
		passwordSvc := crypto.NewPasswordService(4)
		jwtSvc := createTestJWTService()
		authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)
		loginHandler := handler.NewLoginHandler(authSvc, testCaptchaSvc)

		// 空密码触发密码验证错误
		req := httptest.NewRequest("POST", "/login", bytes.NewReader([]byte(`{"email":"test@example.com","password":""}`)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code,
			"空密码应返回400 Bad Request")
	})

	// 测试 helpers.go 中的工具函数
	t.Run("decodeJSON - 正常解析", func(t *testing.T) {
		storeInst := mock.New()
		passwordSvc := crypto.NewPasswordService(4)
		jwtSvc := createTestJWTService()
		authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)
		loginHandler := handler.NewLoginHandler(authSvc, testCaptchaSvc)

		// 正常JSON
		req := httptest.NewRequest("POST", "/login", bytes.NewReader([]byte(`{"email":"test@example.com","password":"Pass123!"}`)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		// 有效JSON不应返回解析错误，应返回400或401
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnauthorized}, w.Code)
	})
}
