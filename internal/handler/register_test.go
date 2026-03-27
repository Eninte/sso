// Package handler_test 注册处理器测试
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// RegisterHandler 扩展测试
// ============================================================================

func TestRegisterHandler_Handle_ValidationErrors(t *testing.T) {
	registerHandler, _ := createTestRegisterHandler(t)

	tests := []struct {
		name           string
		body           map[string]string
		expectedStatus int
	}{
		{
			name:           "空邮箱",
			body:           map[string]string{"email": "", "password": "Password123!"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "无效邮箱格式",
			body:           map[string]string{"email": "invalid-email", "password": "Password123!"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "密码太短",
			body:           map[string]string{"email": "test@example.com", "password": "short"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "密码无大写",
			body:           map[string]string{"email": "test@example.com", "password": "password123!"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "密码无小写",
			body:           map[string]string{"email": "test@example.com", "password": "PASSWORD123!"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "密码无数字",
			body:           map[string]string{"email": "test@example.com", "password": "Password!"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "密码无特殊字符",
			body:           map[string]string{"email": "test@example.com", "password": "Password123"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "空密码",
			body:           map[string]string{"email": "test@example.com", "password": ""},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			registerHandler.Handle(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestRegisterHandler_Handle_InvalidJSON(t *testing.T) {
	registerHandler, _ := createTestRegisterHandler(t)

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		registerHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空请求体", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/register", nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		registerHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestRegisterHandler_Handle_DuplicateEmail(t *testing.T) {
	registerHandler, store := createTestRegisterHandler(t)

	// 先注册一个用户
	store.Reset()
	body := map[string]string{
		"email":    "duplicate@example.com",
		"password": "Password123!",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	registerHandler.Handle(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// 尝试用相同邮箱注册
	req = httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	registerHandler.Handle(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestRegisterHandler_Handle_Success(t *testing.T) {
	registerHandler, _ := createTestRegisterHandler(t)

	body := map[string]string{
		"email":    "newuser@example.com",
		"password": "Password123!",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	registerHandler.Handle(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	// 验证响应包含data字段
	data, ok := resp["data"].(map[string]interface{})
	require.True(t, ok, "response should contain data field")
	assert.Equal(t, "newuser@example.com", data["email"])
	assert.NotEmpty(t, data["user_id"])
}
