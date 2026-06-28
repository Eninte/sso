// Package middleware_test HTTP中间件补充测试
package middleware_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/sso/internal/middleware"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// BasicAuth 测试
// ============================================================================

func TestBasicAuth_EmptyCredentials(t *testing.T) {
	handler := middleware.BasicAuth("", "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// 空凭据时直接通过
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBasicAuth_NoAuthHeader(t *testing.T) {
	handler := middleware.BasicAuth("admin", "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Basic")
}

func TestBasicAuth_InvalidAuthPrefix(t *testing.T) {
	handler := middleware.BasicAuth("admin", "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer token123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBasicAuth_InvalidBase64(t *testing.T) {
	handler := middleware.BasicAuth("admin", "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic invalid!!!")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBasicAuth_InvalidCredentialFormat(t *testing.T) {
	handler := middleware.BasicAuth("admin", "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	encoded := base64.StdEncoding.EncodeToString([]byte("no-colon-separator"))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic "+encoded)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBasicAuth_WrongCredentials(t *testing.T) {
	handler := middleware.BasicAuth("admin", "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	encoded := base64.StdEncoding.EncodeToString([]byte("wrong:credentials"))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic "+encoded)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBasicAuth_CorrectCredentials(t *testing.T) {
	handler := middleware.BasicAuth("admin", "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	encoded := base64.StdEncoding.EncodeToString([]byte("admin:secret"))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic "+encoded)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ============================================================================
// GetCSPNonce 测试
// ============================================================================

func TestGetCSPNonce_WithContext(t *testing.T) {
	// SecurityHeaders 中间件会将 nonce 添加到上下文
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := middleware.GetCSPNonce(r.Context())
		assert.NotEmpty(t, nonce)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
}

func TestGetCSPNonce_WithoutContext(t *testing.T) {
	// 没有nonce的上下文应返回空字符串
	nonce := middleware.GetCSPNonce(context.Background())
	assert.Empty(t, nonce)
}

// ============================================================================
// RateLimiter.Stop 测试
// ============================================================================

func TestRateLimiter_Stop(t *testing.T) {
	rl := middleware.NewRateLimiter(100, 60)
	rl.Stop()
	// 不应该panic
}
