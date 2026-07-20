// Package handler_test Authorize Handler单元测试
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// AuthorizeHandler 测试
// ============================================================================

func createTestAuthorizeHandler(t *testing.T) *handler.AuthorizeHandler {
	storeInst := mock.New()
	tokenSvc := createTestTokenServiceForHandler()
	passwordSvc := crypto.NewPasswordService(4)
	oauthSvc := service.NewOAuthService(storeInst, tokenSvc, service.WithOAuthPassword(passwordSvc))
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

		// 无效客户端应返回400 Bad Request
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestAuthorizeHandler_HandleDeny 覆盖 HandleDeny 全部分支
func TestAuthorizeHandler_HandleDeny(t *testing.T) {
	h := createTestAuthorizeHandler(t)

	t.Run("未认证返回401", func(t *testing.T) {
		body := map[string]string{
			"consent_token": "token-abc",
			"state":         "1234567890abcdef",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/authorize/deny", bytes.NewReader(bodyBytes))
		// 不设置 userID
		w := httptest.NewRecorder()

		h.HandleDeny(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("无效JSON返回400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/authorize/deny", bytes.NewReader([]byte("invalid json")))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		h.HandleDeny(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("合法请求返回403 access_denied", func(t *testing.T) {
		body := map[string]string{
			"consent_token": "token-abc",
			"state":         "1234567890abcdef",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/authorize/deny", bytes.NewReader(bodyBytes))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		h.HandleDeny(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "access_denied", resp["error"])
		assert.Equal(t, "1234567890abcdef", resp["state"])
		assert.NotEmpty(t, resp["error_description"])
	})
}
