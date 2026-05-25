// Package handler_test Handler层覆盖率补充测试
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/handler"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

func createTestMFAHandlerForCoverage() (*handler.MFAHandler, *mock.Store) {
	m := mock.New()
	hmacKey := []byte("test-hmac-key-32-bytes-long-xxxx")
	mock.SetMockHMACKey(hmacKey)
	mfaSvc := service.NewMFAService(m)
	mfaSvc.SetHMACKey(hmacKey)
	return handler.NewMFAHandler(mfaSvc), m
}

func ctxWithUserIDForCoverage(userID string) context.Context {
	return context.WithValue(context.Background(), middleware.UserIDKey, userID)
}

func TestMFAHandler_HandleGenerateRecoveryCodes(t *testing.T) {
	t.Run("成功生成恢复码", func(t *testing.T) {
		h, _ := createTestMFAHandlerForCoverage()

		body := `{"count": 5}`
		req := httptest.NewRequest("POST", "/api/v1/mfa/recovery-codes/generate", bytes.NewBufferString(body))
		req = req.WithContext(ctxWithUserIDForCoverage("user-1"))
		w := httptest.NewRecorder()

		h.HandleGenerateRecoveryCodes(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		data, ok := resp["data"].(map[string]interface{})
		require.True(t, ok)

		codes, ok := data["codes"].([]interface{})
		require.True(t, ok)
		assert.Len(t, codes, 5)
	})

	t.Run("使用默认数量", func(t *testing.T) {
		h, _ := createTestMFAHandlerForCoverage()

		body := `{}`
		req := httptest.NewRequest("POST", "/api/v1/mfa/recovery-codes/generate", bytes.NewBufferString(body))
		req = req.WithContext(ctxWithUserIDForCoverage("user-1"))
		w := httptest.NewRecorder()

		h.HandleGenerateRecoveryCodes(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("无用户ID返回401", func(t *testing.T) {
		h, _ := createTestMFAHandlerForCoverage()

		body := `{}`
		req := httptest.NewRequest("POST", "/api/v1/mfa/recovery-codes/generate", bytes.NewBufferString(body))
		w := httptest.NewRecorder()

		h.HandleGenerateRecoveryCodes(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("无效请求体", func(t *testing.T) {
		h, _ := createTestMFAHandlerForCoverage()

		body := `{invalid json`
		req := httptest.NewRequest("POST", "/api/v1/mfa/recovery-codes/generate", bytes.NewBufferString(body))
		req = req.WithContext(ctxWithUserIDForCoverage("user-1"))
		w := httptest.NewRecorder()

		h.HandleGenerateRecoveryCodes(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestMFAHandler_HandleVerifyRecoveryCode(t *testing.T) {
	t.Run("验证成功", func(t *testing.T) {
		h, _ := createTestMFAHandlerForCoverage()

		genBody := `{"count": 8}`
		genReq := httptest.NewRequest("POST", "/generate", bytes.NewBufferString(genBody))
		genReq = genReq.WithContext(ctxWithUserIDForCoverage("user-1"))
		genW := httptest.NewRecorder()
		h.HandleGenerateRecoveryCodes(genW, genReq)

		var genResp map[string]interface{}
		err := json.Unmarshal(genW.Body.Bytes(), &genResp)
		require.NoError(t, err)

		data := genResp["data"].(map[string]interface{})
		codes := data["codes"].([]interface{})
		firstCode := codes[0].(string)

		verifyBody := `{"code": "` + firstCode + `"}`
		req := httptest.NewRequest("POST", "/api/v1/mfa/recovery-codes/verify", bytes.NewBufferString(verifyBody))
		req = req.WithContext(ctxWithUserIDForCoverage("user-1"))
		w := httptest.NewRecorder()

		h.HandleVerifyRecoveryCode(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("无效恢复码", func(t *testing.T) {
		h, _ := createTestMFAHandlerForCoverage()

		body := `{"code": "INVALID-CODE-XXXX-XXXX"}`
		req := httptest.NewRequest("POST", "/api/v1/mfa/recovery-codes/verify", bytes.NewBufferString(body))
		req = req.WithContext(ctxWithUserIDForCoverage("user-1"))
		w := httptest.NewRecorder()

		h.HandleVerifyRecoveryCode(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空恢复码", func(t *testing.T) {
		h, _ := createTestMFAHandlerForCoverage()

		body := `{"code": ""}`
		req := httptest.NewRequest("POST", "/api/v1/mfa/recovery-codes/verify", bytes.NewBufferString(body))
		req = req.WithContext(ctxWithUserIDForCoverage("user-1"))
		w := httptest.NewRecorder()

		h.HandleVerifyRecoveryCode(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("无用户ID返回401", func(t *testing.T) {
		h, _ := createTestMFAHandlerForCoverage()

		body := `{"code": "some-code"}`
		req := httptest.NewRequest("POST", "/api/v1/mfa/recovery-codes/verify", bytes.NewBufferString(body))
		w := httptest.NewRecorder()

		h.HandleVerifyRecoveryCode(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestMFAHandler_HandleGetRecoveryCodeStatus(t *testing.T) {
	t.Run("返回剩余数量", func(t *testing.T) {
		h, _ := createTestMFAHandlerForCoverage()

		genBody := `{"count": 8}`
		genReq := httptest.NewRequest("POST", "/generate", bytes.NewBufferString(genBody))
		genReq = genReq.WithContext(ctxWithUserIDForCoverage("user-1"))
		genW := httptest.NewRecorder()
		h.HandleGenerateRecoveryCodes(genW, genReq)

		req := httptest.NewRequest("GET", "/api/v1/mfa/recovery-codes/status", nil)
		req = req.WithContext(ctxWithUserIDForCoverage("user-1"))
		w := httptest.NewRecorder()

		h.HandleGetRecoveryCodeStatus(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		data := resp["data"].(map[string]interface{})
		remaining := data["remaining"].(float64)
		assert.Equal(t, float64(8), remaining)
	})

	t.Run("无恢复码返回0", func(t *testing.T) {
		h, _ := createTestMFAHandlerForCoverage()

		req := httptest.NewRequest("GET", "/api/v1/mfa/recovery-codes/status", nil)
		req = req.WithContext(ctxWithUserIDForCoverage("user-2"))
		w := httptest.NewRecorder()

		h.HandleGetRecoveryCodeStatus(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("无用户ID返回401", func(t *testing.T) {
		h, _ := createTestMFAHandlerForCoverage()

		req := httptest.NewRequest("GET", "/api/v1/mfa/recovery-codes/status", nil)
		w := httptest.NewRecorder()

		h.HandleGetRecoveryCodeStatus(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestTokenHandler_HandleLogoutAll(t *testing.T) {
	t.Run("成功登出所有设备", func(t *testing.T) {
		h, _ := createTestTokenHandler(t)

		req := httptest.NewRequest("POST", "/api/v1/logout-all", nil)
		req = req.WithContext(ctxWithUserIDForCoverage("user-1"))
		w := httptest.NewRecorder()

		h.HandleLogoutAll(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		msg := resp["message"].(string)
		assert.Equal(t, "已登出所有设备", msg)
	})

	t.Run("无用户ID返回401", func(t *testing.T) {
		h, _ := createTestTokenHandler(t)

		req := httptest.NewRequest("POST", "/api/v1/logout-all", nil)
		w := httptest.NewRecorder()

		h.HandleLogoutAll(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
