// Package sdk_test SSO SDK单元测试
package sdk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/example/sso/sdks/golang"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// mockServer 创建模拟SSO服务
func mockServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// jsonResponse 写入JSON响应
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// ============================================================================
// Client 创建测试
// ============================================================================

func TestNewClient(t *testing.T) {
	c := sdk.NewClient("http://localhost:9090")
	assert.NotNil(t, c)
	assert.Equal(t, "http://localhost:9090", c.BaseURL())
}

func TestNewClient_TrailingSlash(t *testing.T) {
	c := sdk.NewClient("http://localhost:9090/")
	assert.Equal(t, "http://localhost:9090", c.BaseURL())
}

func TestNewClient_WithTimeout(t *testing.T) {
	c := sdk.NewClient("http://localhost:9090", sdk.WithTimeout(5))
	assert.NotNil(t, c)
}

// ============================================================================
// Register 测试
// ============================================================================

func TestClient_Register(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/register", r.URL.Path)

		jsonResponse(w, http.StatusCreated, sdk.RegisterResponse{
			Message: "注册成功",
			Data: &sdk.RegisterData{
				UserID: "user-123",
				Email:  "test@example.com",
			},
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL)
	resp, err := client.Register(context.Background(), "test@example.com", "P@ssw0rd1")

	require.NoError(t, err)
	assert.Equal(t, "注册成功", resp.Message)
	assert.Equal(t, "user-123", resp.Data.UserID)
	assert.Equal(t, "test@example.com", resp.Data.Email)
}

func TestClient_Register_EmailExists(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		// 服务端 writeValidationError 返回 {"error": "<message>"} 格式（无 code 字段）
		jsonResponse(w, http.StatusConflict, map[string]string{
			"error": "邮箱已存在",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL)
	_, err := client.Register(context.Background(), "exist@example.com", "P@ssw0rd1")

	require.Error(t, err)
	ssoErr, ok := err.(*sdk.Error)
	require.True(t, ok)
	assert.True(t, ssoErr.IsConflict())
	// error 字段为消息文本，code 应为空
	assert.Equal(t, "邮箱已存在", ssoErr.Message)
}

// ============================================================================
// Login 测试
// ============================================================================

func TestClient_Login(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/login", r.URL.Path)

		jsonResponse(w, http.StatusOK, sdk.TokenResponse{
			AccessToken:  "access-123",
			RefreshToken: "refresh-456",
			TokenType:    "Bearer",
			ExpiresIn:    900,
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL)
	resp, err := client.Login(context.Background(), "test@example.com", "P@ssw0rd1")

	require.NoError(t, err)
	assert.Equal(t, "access-123", resp.AccessToken)
	assert.Equal(t, "refresh-456", resp.RefreshToken)
	assert.Equal(t, 900, resp.ExpiresIn)

	// 验证Token已自动保存
	assert.Equal(t, "access-123", client.AccessToken())
}

func TestClient_Login_InvalidCredentials(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusUnauthorized, map[string]string{
			"code":    "INVALID_CREDENTIALS",
			"message": "邮箱或密码错误",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL)
	_, err := client.Login(context.Background(), "test@example.com", "wrong")

	require.Error(t, err)
	ssoErr, ok := err.(*sdk.Error)
	require.True(t, ok)
	assert.True(t, ssoErr.IsUnauthorized())
}

// ============================================================================
// UserInfo 测试
// ============================================================================

func TestClient_UserInfo(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/v1/userinfo", r.URL.Path)
		assert.Equal(t, "Bearer access-123", r.Header.Get("Authorization"))

		jsonResponse(w, http.StatusOK, sdk.UserInfo{
			Sub:   "user-123",
			Email: "test@example.com",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL, sdk.WithAccessToken("access-123"))
	info, err := client.UserInfo(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "user-123", info.Sub)
	assert.Equal(t, "test@example.com", info.Email)
}

func TestClient_UserInfo_NoToken(t *testing.T) {
	client := sdk.NewClient("http://localhost:9090")
	_, err := client.UserInfo(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no access token")
}

// ============================================================================
// RevokeToken 测试
// ============================================================================

func TestClient_RevokeToken(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/token/revoke", r.URL.Path)

		jsonResponse(w, http.StatusOK, sdk.MessageResponse{
			Message: "Token已撤销",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL, sdk.WithAccessToken("access-123"))
	err := client.RevokeToken(context.Background())

	require.NoError(t, err)
	assert.Empty(t, client.AccessToken())
}

// ============================================================================
// RefreshToken 测试
// ============================================================================

func TestClient_RefreshToken(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/token", r.URL.Path)

		jsonResponse(w, http.StatusOK, sdk.TokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			TokenType:    "Bearer",
			ExpiresIn:    900,
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL,
		sdk.WithAccessToken("old-access"),
		sdk.WithRefreshToken("refresh-456"),
	)

	resp, err := client.RefreshToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "new-access", resp.AccessToken)
	assert.Equal(t, "new-access", client.AccessToken())
}

func TestClient_RefreshToken_NoRefreshToken(t *testing.T) {
	client := sdk.NewClient("http://localhost:9090")
	_, err := client.RefreshToken(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no refresh token")
}

// ============================================================================
// ExchangeCode 测试
// ============================================================================

func TestClient_ExchangeCode(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/token", r.URL.Path)

		// 服务端使用 handlerutil.WriteJSONSuccess 包裹响应，格式为 {"data": {...}}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{
				"access_token":  "access-oauth-123",
				"refresh_token": "refresh-oauth-456",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"scope":         "openid profile",
			},
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL)
	resp, err := client.ExchangeCode(context.Background(),
		"auth-code-abc",
		"client-123",
		"client-secret-xyz",
		"http://localhost:8080/callback",
		"code-verifier-123",
	)

	require.NoError(t, err)
	assert.Equal(t, "access-oauth-123", resp.AccessToken)
	assert.Equal(t, "refresh-oauth-456", resp.RefreshToken)
	assert.Equal(t, "Bearer", resp.TokenType)
	assert.Equal(t, 3600, resp.ExpiresIn)
	assert.Equal(t, "openid profile", resp.Scope)
}

// ============================================================================
// ForgotPassword / ResetPassword 测试
// ============================================================================

func TestClient_ForgotPassword(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/forgot-password", r.URL.Path)
		jsonResponse(w, http.StatusOK, sdk.MessageResponse{
			Message: "如果该邮箱已注册，重置邮件已发送",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL)
	resp, err := client.ForgotPassword(context.Background(), "test@example.com")

	require.NoError(t, err)
	assert.Contains(t, resp.Message, "重置邮件")
}

func TestClient_ResetPassword(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/reset-password", r.URL.Path)
		jsonResponse(w, http.StatusOK, sdk.MessageResponse{
			Message: "密码重置成功",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL)
	resp, err := client.ResetPassword(context.Background(), "token-123", "user-456", "NewP@ss1")

	require.NoError(t, err)
	assert.Equal(t, "密码重置成功", resp.Message)
}

// ============================================================================
// ChangePassword 测试
// ============================================================================

func TestClient_ChangePassword(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/change-password", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Authorization"))

		jsonResponse(w, http.StatusOK, sdk.MessageResponse{
			Message: "密码修改成功",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL, sdk.WithAccessToken("access-123"))
	resp, err := client.ChangePassword(context.Background(), "OldP@ss1", "NewP@ss1")

	require.NoError(t, err)
	assert.Equal(t, "密码修改成功", resp.Message)
}

// ============================================================================
// MFA 测试
// ============================================================================

func TestClient_MFASetup(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/mfa/setup", r.URL.Path)
		jsonResponse(w, http.StatusOK, sdk.MFASetupResponse{
			Secret:      "JBSWY3DPEHPK3PXP",
			QRCodeURL:   "otpauth://totp/SSO:test@example.com?secret=JBSWY3DPEHPK3PXP",
			ManualEntry: "JBSWY3DPEHPK3PXP",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL, sdk.WithAccessToken("access-123"))
	resp, err := client.MFASetup(context.Background())

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Secret)
	assert.NotEmpty(t, resp.QRCodeURL)
}

func TestClient_MFAVerify(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/mfa/verify", r.URL.Path)
		jsonResponse(w, http.StatusOK, sdk.MessageResponse{
			Message: "MFA已启用",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL, sdk.WithAccessToken("access-123"))
	resp, err := client.MFAVerify(context.Background(), "123456")

	require.NoError(t, err)
	assert.Equal(t, "MFA已启用", resp.Message)
}

func TestClient_MFAStatus(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/mfa/status", r.URL.Path)
		jsonResponse(w, http.StatusOK, sdk.MFAStatusResponse{
			Enabled: true,
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL, sdk.WithAccessToken("access-123"))
	resp, err := client.MFAStatus(context.Background())

	require.NoError(t, err)
	assert.True(t, resp.Enabled)
}

// ============================================================================
// Admin 测试
// ============================================================================

func TestClient_AdminHealth(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/admin/health", r.URL.Path)
		jsonResponse(w, http.StatusOK, sdk.HealthResponse{
			Status:    "ok",
			Database:  "connected",
			Version:   "1.0.0",
			BuildTime: "2026-07-15T00:00:00Z",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL, sdk.WithAccessToken("admin-token"))
	resp, err := client.AdminHealth(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "connected", resp.Database)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.Equal(t, "2026-07-15T00:00:00Z", resp.BuildTime)
}

func TestClient_ListUsers(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/admin/users", r.URL.Path)
		assert.Equal(t, "1", r.URL.Query().Get("page"))
		assert.Equal(t, "10", r.URL.Query().Get("pageSize"))

		jsonResponse(w, http.StatusOK, sdk.UserListResponse{
			Users: []sdk.UserItem{
				{ID: "user-1", Email: "user1@example.com", Status: "active"},
				{ID: "user-2", Email: "user2@example.com", Status: "active"},
			},
			Total:    2,
			Page:     1,
			PageSize: 10,
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL, sdk.WithAccessToken("admin-token"))
	resp, err := client.ListUsers(context.Background(), 1, 10)

	require.NoError(t, err)
	assert.Len(t, resp.Users, 2)
	assert.Equal(t, 2, resp.Total)
}

func TestClient_DisableUser(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		// 路径参数形式：/api/v1/admin/users/{id}/disable
		assert.Equal(t, "/api/v1/admin/users/user-123/disable", r.URL.Path)
		jsonResponse(w, http.StatusOK, sdk.MessageResponse{
			Message: "用户已禁用",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL, sdk.WithAccessToken("admin-token"))
	resp, err := client.DisableUser(context.Background(), "user-123")

	require.NoError(t, err)
	assert.Equal(t, "用户已禁用", resp.Message)
}

func TestClient_EnableUser(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		// 路径参数形式：/api/v1/admin/users/{id}/enable
		assert.Equal(t, "/api/v1/admin/users/user-456/enable", r.URL.Path)
		jsonResponse(w, http.StatusOK, sdk.MessageResponse{
			Message: "用户已启用",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL, sdk.WithAccessToken("admin-token"))
	resp, err := client.EnableUser(context.Background(), "user-456")

	require.NoError(t, err)
	assert.Equal(t, "用户已启用", resp.Message)
}

func TestClient_GetUser(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		// 路径参数形式：/api/v1/admin/users/{id}
		assert.Equal(t, "/api/v1/admin/users/user-789", r.URL.Path)
		jsonResponse(w, http.StatusOK, sdk.UserItem{
			ID:     "user-789",
			Email:  "test@example.com",
			Status: "active",
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL, sdk.WithAccessToken("admin-token"))
	resp, err := client.GetUser(context.Background(), "user-789")

	require.NoError(t, err)
	assert.Equal(t, "user-789", resp.ID)
	assert.Equal(t, "test@example.com", resp.Email)
}

// ============================================================================
// Discovery / JWKS 测试
// ============================================================================

func TestClient_Discovery(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/.well-known/openid-configuration", r.URL.Path)

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"issuer":                           "http://test",
			"token_endpoint":                   "http://test/api/v1/token",
			"jwks_uri":                         "http://test/.well-known/jwks.json",
			"grant_types_supported":            []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported": []string{"S256"},
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL)
	resp, err := client.Discovery(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "http://test", resp.Issuer)
	assert.Contains(t, resp.GrantTypesSupported, "authorization_code")
}

func TestClient_JWKS(t *testing.T) {
	server := mockServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/.well-known/jwks.json", r.URL.Path)

		jsonResponse(w, http.StatusOK, sdk.JWKSResponse{
			Keys: []sdk.JWK{
				{Kty: "RSA", Use: "sig", Kid: "key-1", N: "abc", E: "def"},
			},
		})
	})
	defer server.Close()

	client := sdk.NewClient(server.URL)
	resp, err := client.JWKS(context.Background())

	require.NoError(t, err)
	assert.Len(t, resp.Keys, 1)
	assert.Equal(t, "RSA", resp.Keys[0].Kty)
}

// ============================================================================
// Error 方法测试
// ============================================================================

func TestError_Methods(t *testing.T) {
	err := &sdk.Error{
		HTTPStatus: 404,
		Code:       sdk.ErrCodeNotFound,
		Message:    "not found",
	}

	assert.True(t, err.IsNotFound())
	assert.False(t, err.IsUnauthorized())
	assert.False(t, err.IsForbidden())
	assert.False(t, err.IsConflict())
	assert.False(t, err.IsRateLimited())
	assert.Contains(t, err.Error(), "NOT_FOUND")
	assert.Contains(t, err.Error(), "404")
}

func TestError_AllMethods(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		checkFunc func(*sdk.Error) bool
	}{
		{"401", 401, (*sdk.Error).IsUnauthorized},
		{"403", 403, (*sdk.Error).IsForbidden},
		{"409", 409, (*sdk.Error).IsConflict},
		{"429", 429, (*sdk.Error).IsRateLimited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &sdk.Error{HTTPStatus: tt.status}
			assert.True(t, tt.checkFunc(err))
		})
	}
}

// ============================================================================
// SetTokens 测试
// ============================================================================

func TestClient_SetTokens(t *testing.T) {
	client := sdk.NewClient("http://localhost:9090")
	client.SetTokens("access", "refresh", 900)

	assert.Equal(t, "access", client.AccessToken())
}
