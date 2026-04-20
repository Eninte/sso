package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/your-org/sso/internal/crypto"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
	"github.com/your-org/sso/internal/util/auditutil"
)

var testTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func TestInitHandler_HandleInitPage(t *testing.T) {
	tests := []struct {
		name         string
		adminExists  bool
		expectedCode int
		expectedBody string
	}{
		{
			name:         "管理员不存在-返回初始化页面",
			adminExists:  false,
			expectedCode: http.StatusOK,
			expectedBody: "SSO 部署初始化",
		},
		{
			name:         "管理员已存在-返回403",
			adminExists:  true,
			expectedCode: http.StatusForbidden,
			expectedBody: "初始化已完成",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := mock.New()

			if tt.adminExists {
				user := &model.User{
					ID:           "admin-1",
					Email:        "admin@example.com",
					PasswordHash: "hash",
					Role:         model.UserRoleAdmin,
					Status:       model.UserStatusActive,
					CreatedAt:    testTime,
					UpdatedAt:    testTime,
				}
				_ = store.Create(context.Background(), user)
			}

			handler := NewInitHandler(store, nil, nil, nil, "1.0.0", "2024-01-01")

			req := httptest.NewRequest("GET", "/init", nil)
			req.RemoteAddr = "127.0.0.1:12345" // 模拟本地请求
			w := httptest.NewRecorder()

			httpHandler := middleware.SecurityHeaders(http.HandlerFunc(handler.HandleInitPage))
			httpHandler.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("期望状态码 %d, 实际 %d", tt.expectedCode, w.Code)
			}

			if !bytes.Contains(w.Body.Bytes(), []byte(tt.expectedBody)) {
				t.Errorf("期望响应包含 %q, 实际 %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}

func TestInitHandler_HandleSystemStatus(t *testing.T) {
	t.Run("管理员不存在-返回系统状态", func(t *testing.T) {
		store := mock.New()
		handler := NewInitHandler(store, nil, nil, nil, "1.0.0", "2024-01-01")

		req := httptest.NewRequest("GET", "/api/v1/init/status", nil)
		req.RemoteAddr = "127.0.0.1:12345" // 模拟本地请求
		w := httptest.NewRecorder()

		handler.HandleSystemStatus(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("期望状态码 %d, 实际 %d", http.StatusOK, w.Code)
		}
	})

	t.Run("管理员已存在-返回404", func(t *testing.T) {
		store := mock.New()
		user := &model.User{
			ID:           "admin-1",
			Email:        "admin@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleAdmin,
			Status:       model.UserStatusActive,
			CreatedAt:    testTime,
			UpdatedAt:    testTime,
		}
		_ = store.Create(context.Background(), user)

		handler := NewInitHandler(store, nil, nil, nil, "1.0.0", "2024-01-01")

		req := httptest.NewRequest("GET", "/api/v1/init/status", nil)
		req.RemoteAddr = "127.0.0.1:12345" // 模拟本地请求
		w := httptest.NewRecorder()

		handler.HandleSystemStatus(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("期望状态码 %d, 实际 %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestInitHandler_HandleCreateAdmin(t *testing.T) {
	tests := []struct {
		name         string
		adminExists  bool
		requestBody  interface{}
		expectedCode int
	}{
		{
			name:         "管理员已存在-返回403",
			adminExists:  true,
			requestBody:  map[string]string{"email": "admin2@example.com", "password": "Password123!"},
			expectedCode: http.StatusForbidden,
		},
		{
			name:         "无效的JSON-返回400",
			adminExists:  false,
			requestBody:  "invalid json",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "邮箱为空-返回400",
			adminExists:  false,
			requestBody:  map[string]string{"email": "", "password": "Password123!"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "密码为空-返回400",
			adminExists:  false,
			requestBody:  map[string]string{"email": "admin@example.com", "password": ""},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "密码不符合要求-返回400",
			adminExists:  false,
			requestBody:  map[string]string{"email": "admin@example.com", "password": "weak"},
			expectedCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := mock.New()

			if tt.adminExists {
				user := &model.User{
					ID:           "admin-1",
					Email:        "admin@example.com",
					PasswordHash: "hash",
					Role:         model.UserRoleAdmin,
					Status:       model.UserStatusActive,
					CreatedAt:    testTime,
					UpdatedAt:    testTime,
				}
				_ = store.Create(context.Background(), user)
			}

			passwordSvc := crypto.NewPasswordService(crypto.NormalizeBcryptCost(4))
			handler := NewInitHandler(store, passwordSvc, nil, nil, "1.0.0", "2024-01-01")

			var body bytes.Buffer
			switch v := tt.requestBody.(type) {
			case string:
				body.WriteString(v)
			case map[string]string:
				json.NewEncoder(&body).Encode(v)
			}

			req := httptest.NewRequest("POST", "/api/v1/init/admin", &body)
			req.RemoteAddr = "127.0.0.1:12345" // 模拟本地请求
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleCreateAdmin(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("期望状态码 %d, 实际 %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestInitHandler_HandleCreateClient(t *testing.T) {
	tests := []struct {
		name         string
		adminExists  bool
		requestBody  interface{}
		expectedCode int
	}{
		{
			name:         "管理员不存在-返回403",
			adminExists:  false,
			requestBody:  map[string]string{"name": "Test App", "redirect_uri": "http://localhost:3000/callback"},
			expectedCode: http.StatusForbidden,
		},
		{
			name:         "无效的JSON-返回400",
			adminExists:  true,
			requestBody:  "invalid json",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "客户端名称为空-返回400",
			adminExists:  true,
			requestBody:  map[string]string{"name": "", "redirect_uri": "http://localhost:3000/callback"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "成功创建客户端",
			adminExists:  true,
			requestBody:  map[string]string{"name": "Test App", "redirect_uri": "http://localhost:3000/callback"},
			expectedCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := mock.New()

			if tt.adminExists {
				user := &model.User{
					ID:           "admin-1",
					Email:        "admin@example.com",
					PasswordHash: "hash",
					Role:         model.UserRoleAdmin,
					Status:       model.UserStatusActive,
					CreatedAt:    testTime,
					UpdatedAt:    testTime,
				}
				_ = store.Create(context.Background(), user)
			}

			passwordSvc := crypto.NewPasswordService(crypto.NormalizeBcryptCost(4))
			handler := NewInitHandler(store, passwordSvc, nil, nil, "1.0.0", "2024-01-01")

			var body bytes.Buffer
			switch v := tt.requestBody.(type) {
			case string:
				body.WriteString(v)
			case map[string]string:
				json.NewEncoder(&body).Encode(v)
			}

			req := httptest.NewRequest("POST", "/api/v1/init/client", &body)
			req.RemoteAddr = "127.0.0.1:12345" // 模拟本地请求
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleCreateClient(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("期望状态码 %d, 实际 %d", tt.expectedCode, w.Code)
			}
		})
	}
}

var _ auditutil.AuditService = (*mockAuditService)(nil)

type mockAuditService struct{}

func (m *mockAuditService) Log(ctx context.Context, log *model.AuditLog) {}

var _ service.InitServiceInterface = (*mockInitService)(nil)

type mockInitService struct {
	adminExists bool
}

func (m *mockInitService) AdminExists(ctx context.Context) (bool, error) {
	return m.adminExists, nil
}

func (m *mockInitService) CreateAdmin(ctx context.Context, email, password string) (*model.User, error) {
	return nil, apperrors.ErrInternal
}

func (m *mockInitService) CreateOAuthClient(ctx context.Context, name, redirectURI string) (*model.Client, string, error) {
	return nil, "", apperrors.ErrInternal
}
