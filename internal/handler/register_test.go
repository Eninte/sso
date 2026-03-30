// Package handler_test 注册处理器扩展测试
// 仅包含 handler_test.go 中未覆盖的表驱动验证测试
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// RegisterHandler 验证错误表驱动测试
// handler_test.go 中已有单个验证子测试，此文件提供更全面的表驱动覆盖
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
