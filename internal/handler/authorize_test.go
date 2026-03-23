// Package handler_test Authorize Handler单元测试
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/your-org/sso/internal/handler"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// AuthorizeHandler 测试
// ============================================================================

func createTestAuthorizeHandler(t *testing.T) *handler.AuthorizeHandler {
	storeInst := mock.New()
	oauthSvc := service.NewOAuthService(storeInst)
	return handler.NewAuthorizeHandler(oauthSvc)
}

func TestAuthorizeHandler_HandleAuthorize(t *testing.T) {
	h := createTestAuthorizeHandler(t)

	t.Run("缺少client_id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/authorize?redirect_uri=http://localhost/callback&response_type=code&state=1234567890abcdef", nil)
		w := httptest.NewRecorder()

		h.HandleAuthorize(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少redirect_uri", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/authorize?client_id=test&response_type=code&state=1234567890abcdef", nil)
		w := httptest.NewRecorder()

		h.HandleAuthorize(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("response_type不是code", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/authorize?client_id=test&redirect_uri=http://localhost/callback&response_type=token&state=1234567890abcdef", nil)
		w := httptest.NewRecorder()

		h.HandleAuthorize(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("state为空", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/authorize?client_id=test&redirect_uri=http://localhost/callback&response_type=code&state=", nil)
		w := httptest.NewRecorder()

		h.HandleAuthorize(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("state长度不足16", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/authorize?client_id=test&redirect_uri=http://localhost/callback&response_type=code&state=short", nil)
		w := httptest.NewRecorder()

		h.HandleAuthorize(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("state有效但client无效", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/authorize?client_id=invalid-client&redirect_uri=http://localhost/callback&response_type=code&state=1234567890abcdef", nil)
		w := httptest.NewRecorder()

		h.HandleAuthorize(w, req)

		assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusUnauthorized)
	})
}

func TestAuthorizeHandler_HandleApprove(t *testing.T) {
	h := createTestAuthorizeHandler(t)

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/authorize/approve", bytes.NewReader([]byte("invalid")))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		h.HandleApprove(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("state为空", func(t *testing.T) {
		body := map[string]string{
			"client_id":    "test-client",
			"redirect_uri": "http://localhost/callback",
			"scope":        "openid",
			"state":        "",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/authorize/approve", bytes.NewReader(bodyBytes))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		h.HandleApprove(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("state长度不足16", func(t *testing.T) {
		body := map[string]string{
			"client_id":    "test-client",
			"redirect_uri": "http://localhost/callback",
			"scope":        "openid",
			"state":        "short",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/authorize/approve", bytes.NewReader(bodyBytes))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		h.HandleApprove(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("未认证", func(t *testing.T) {
		body := map[string]string{
			"client_id":    "test-client",
			"redirect_uri": "http://localhost/callback",
			"scope":        "openid",
			"state":        "1234567890abcdef",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/authorize/approve", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()

		h.HandleApprove(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("无效客户端触发OAuth错误", func(t *testing.T) {
		body := map[string]string{
			"client_id":    "nonexistent-client",
			"redirect_uri": "http://localhost/callback",
			"scope":        "openid",
			"state":        "1234567890abcdef",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/authorize/approve", bytes.NewReader(bodyBytes))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		h.HandleApprove(w, req)

		// 触发 writeOAuthError 路径
		assert.True(t, w.Code >= 400)
	})
}
