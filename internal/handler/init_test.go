package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
	"github.com/example/sso/internal/util/auditutil"
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

			handler := NewInitHandler(store, nil, nil, nil, "1.0.0", "2024-01-01", true)

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
		handler := NewInitHandler(store, nil, nil, nil, "1.0.0", "2024-01-01", true)

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

		handler := NewInitHandler(store, nil, nil, nil, "1.0.0", "2024-01-01", true)

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
			handler := NewInitHandler(store, passwordSvc, nil, nil, "1.0.0", "2024-01-01", true)

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
			handler := NewInitHandler(store, passwordSvc, nil, nil, "1.0.0", "2024-01-01", true)

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

// ============================================================================
// 反向代理绕过防御测试
// ============================================================================

// TestInitHandler_ReverseProxyBypass_Rejected 验证当 INIT_ENABLED=false 时，
// 即使请求伪造 RemoteAddr=127.0.0.1（模拟反向代理在本机转发的场景，
// isLocalRequest 检查会被绕过），所有初始化接口仍返回 404。
//
// 这是审计报告中"严重问题 1"的核心防御层：
//   - 第一层：独立 loopback HTTP 服务器（网络层隔离，反向代理绕过）
//   - 第二层：isLocalRequest 检查（防御 loopback 上的转发）
//   - 第三层：INIT_ENABLED=false 时所有接口返回 404（兜底防御）
//
// 本测试验证第三层：即使前两层被绕过，初始化完成后仍可永久关闭接口
func TestInitHandler_ReverseProxyBypass_Rejected(t *testing.T) {
	store := mock.New()
	passwordSvc := crypto.NewPasswordService(crypto.NormalizeBcryptCost(4))
	// initEnabled=false 模拟初始化完成后的状态
	handler := NewInitHandler(store, passwordSvc, nil, nil, "1.0.0", "2024-01-01", false)

	t.Run("HandleInitPage-返回404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/init", nil)
		// 模拟反向代理本机转发：RemoteAddr 看起来是 127.0.0.1（绕过 isLocalRequest）
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()

		handler.HandleInitPage(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code, "initEnabled=false 时应返回 404")
	})

	t.Run("HandleSystemStatus-返回404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/init/status", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()

		handler.HandleSystemStatus(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code, "initEnabled=false 时应返回 404")
	})

	t.Run("HandleCreateAdmin-返回404", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":    "attacker@example.com",
			"password": "Password123!",
		})
		req := httptest.NewRequest("POST", "/api/v1/init/admin", bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleCreateAdmin(w, req)

		// 攻击者无法抢先创建管理员
		assert.Equal(t, http.StatusNotFound, w.Code, "initEnabled=false 时应返回 404")
	})

	t.Run("HandleCreateClient-返回404", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"name":         "Attacker App",
			"redirect_uri": "https://evil.com/callback",
		})
		req := httptest.NewRequest("POST", "/api/v1/init/client", bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleCreateClient(w, req)

		// 即使已存在管理员，攻击者也无法获取 OAuth Client Secret
		assert.Equal(t, http.StatusNotFound, w.Code, "initEnabled=false 时应返回 404")
	})

	t.Run("非本地请求-返回404优先于403", func(t *testing.T) {
		// 验证 initEnabled=false 检查发生在 isLocalRequest 检查之前
		// 即使 RemoteAddr 是公网地址（非 127.0.0.1），也只返回 404 而不是 403
		// 这样不会泄露"初始化面板存在"的信息
		req := httptest.NewRequest("GET", "/init", nil)
		req.RemoteAddr = "203.0.113.1:12345" // 公网地址
		w := httptest.NewRecorder()

		handler.HandleInitPage(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code, "initEnabled=false 时应返回 404 而非 403")
	})
}

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
