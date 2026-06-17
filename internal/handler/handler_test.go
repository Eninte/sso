// Package handler_test Handler层单元测试
package handler_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/handler"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestJWTService 创建测试用的JWT服务
func createTestJWTServiceForHandlerTest() *crypto.JWTService {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	return crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)
}

// createTestTokenServiceForHandler 创建测试用的Token服务
func createTestTokenServiceForHandler() *service.TokenService {
	return service.NewTokenService(createTestJWTServiceForHandlerTest(), mock.New())
}

// createTestLoginHandler 创建测试用的登录处理器
func createTestLoginHandler(t *testing.T) (*handler.LoginHandler, *mock.Store) {
	// 创建Mock存储
	store := mock.New()

	// 创建密码服务
	passwordSvc := crypto.NewPasswordService(4)

	// 创建JWT服务
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 创建认证服务
	authSvc := service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute)

	// 创建登录处理器
	loginHandler := handler.NewLoginHandler(authSvc)

	return loginHandler, store
}

// createTestRegisterHandler 创建测试用的注册处理器
func createTestRegisterHandler(t *testing.T) (*handler.RegisterHandler, *mock.Store) {
	// 创建Mock存储
	store := mock.New()

	// 创建密码服务
	passwordSvc := crypto.NewPasswordService(4)

	// 创建JWT服务
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 创建认证服务
	authSvc := service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute)

	// 创建注册处理器
	registerHandler := handler.NewRegisterHandler(authSvc)

	return registerHandler, store
}

// ============================================================================
// LoginHandler 测试
// ============================================================================

func TestLoginHandler_Handle(t *testing.T) {
	loginHandler, store := createTestLoginHandler(t)

	// 创建测试用户
	passwordSvc := crypto.NewPasswordService(4)
	hashedPassword, err := passwordSvc.HashPassword("Password123!")
	require.NoError(t, err)

	testUser := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(testUser)

	t.Run("成功登录", func(t *testing.T) {
		body := map[string]string{
			"email":    "test@example.com",
			"password": "Password123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/login", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp model.LoginResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.Equal(t, "Bearer", resp.TokenType)
	})

	t.Run("密码错误", func(t *testing.T) {
		body := map[string]string{
			"email":    "test@example.com",
			"password": "WrongPassword",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/login", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		// 统一错误处理返回的是 message 字段
		assert.Contains(t, resp["message"], "邮箱或密码错误")
	})

	t.Run("用户不存在", func(t *testing.T) {
		body := map[string]string{
			"email":    "nonexistent@example.com",
			"password": "Password123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/login", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("无效的JSON格式", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/login", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("账户被禁用", func(t *testing.T) {
		// 创建被禁用的用户
		disabledUser := &model.User{
			ID:            "disabled-user-id",
			Email:         "disabled@example.com",
			PasswordHash:  hashedPassword,
			EmailVerified: true,
			Status:        model.UserStatusDisabled,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(disabledUser)

		body := map[string]string{
			"email":    "disabled@example.com",
			"password": "Password123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/login", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

// ============================================================================
// RegisterHandler 测试
// ============================================================================

func TestRegisterHandler_Handle(t *testing.T) {
	registerHandler, store := createTestRegisterHandler(t)

	t.Run("成功注册", func(t *testing.T) {
		store.Reset()

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

		assert.NotEmpty(t, resp["message"])
	})

	t.Run("邮箱已存在", func(t *testing.T) {
		store.Reset()

		// 先注册一个用户
		body := map[string]string{
			"email":    "existing@example.com",
			"password": "Password123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()
		registerHandler.Handle(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		// 尝试用相同邮箱注册（应返回与成功相同的响应，防止用户枚举）
		req = httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()

		registerHandler.Handle(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("无效的JSON格式", func(t *testing.T) {
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

// ============================================================================
// UserInfoHandler 测试
// ============================================================================

func TestUserInfoHandler_Handle(t *testing.T) {
	store := mock.New()
	h := handler.NewUserInfoHandler(store)

	t.Run("未认证-返回401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/userinfo", nil)
		w := httptest.NewRecorder()

		h.Handle(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("已认证-返回用户信息", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/userinfo", nil)
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "user-123")
		ctx = context.WithValue(ctx, middleware.UserEmailKey, "user@example.com")
		ctx = context.WithValue(ctx, middleware.UserScopesKey, []string{"openid", "email"})
		w := httptest.NewRecorder()

		h.Handle(w, req.WithContext(ctx))

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "user-123", resp["sub"])
		assert.Equal(t, "user@example.com", resp["email"])
	})
}

// ============================================================================
// TokenHandler 测试
// ============================================================================

// createTestTokenHandler 创建测试用的Token处理器
func createTestTokenHandler(t *testing.T) (*handler.TokenHandler, *mock.Store) {
	store := mock.New()

	passwordSvc := crypto.NewPasswordService(4)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	tokenSvc := service.NewTokenService(jwtSvc, store)
	authSvc := service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute)
	oauthSvc := service.NewOAuthService(store, tokenSvc)

	tokenHandler := handler.NewTokenHandler(authSvc, oauthSvc)

	return tokenHandler, store
}

func TestTokenHandler_HandleToken_RefreshToken(t *testing.T) {
	tokenHandler, store := createTestTokenHandler(t)

	// 创建测试用户并登录获取Token
	passwordSvc := crypto.NewPasswordService(4)
	hashedPassword, _ := passwordSvc.HashPassword("Password123!")

	user := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(user)

	// 先获取一个有效的refresh token
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test-issuer", 15*time.Minute, 7*24*time.Hour)
	authSvc := service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute)

	loginResp, err := authSvc.Login(context.Background(), &model.LoginRequest{
		Email:    "test@example.com",
		Password: "Password123!",
	})
	require.NoError(t, err)

	t.Run("成功刷新Token", func(t *testing.T) {
		body := map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": loginResp.RefreshToken,
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("缺少refresh_token", func(t *testing.T) {
		body := map[string]string{
			"grant_type": "refresh_token",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("无效的refresh_token", func(t *testing.T) {
		body := map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": "invalid-token",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestTokenHandler_HandleToken_InvalidGrantType(t *testing.T) {
	tokenHandler, _ := createTestTokenHandler(t)

	body := map[string]string{
		"grant_type": "invalid_grant_type",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	tokenHandler.HandleToken(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "不支持的授权类型")
}

func TestTokenHandler_HandleToken_InvalidJSON(t *testing.T) {
	tokenHandler, _ := createTestTokenHandler(t)

	req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	tokenHandler.HandleToken(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTokenHandler_HandleToken_AuthorizationCode(t *testing.T) {
	tokenHandler, store := createTestTokenHandler(t)

	// 创建客户端
	client := &model.Client{
		ID:           "test-client",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Name:         "Test Client",
		RedirectURIs: []string{"http://localhost:3000/callback"},
		GrantTypes:   []string{model.GrantTypeAuthorizationCode},
		PublicClient: false,
	}
	store.AddClient(client)

	// 创建用户
	user := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  "hash",
		EmailVerified: true,
		Status:        model.UserStatusActive,
	}
	store.AddUser(user)

	t.Run("缺少code参数", func(t *testing.T) {
		body := map[string]string{
			"grant_type":   "authorization_code",
			"client_id":    "test-client-id",
			"redirect_uri": "http://localhost:3000/callback",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "缺少code参数")
	})

	t.Run("缺少client_id参数", func(t *testing.T) {
		body := map[string]string{
			"grant_type":   "authorization_code",
			"code":         "some-code",
			"redirect_uri": "http://localhost:3000/callback",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "缺少client_id参数")
	})

	t.Run("缺少redirect_uri参数", func(t *testing.T) {
		body := map[string]string{
			"grant_type": "authorization_code",
			"code":       "some-code",
			"client_id":  "test-client-id",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "缺少redirect_uri参数")
	})

	t.Run("无效的授权码", func(t *testing.T) {
		body := map[string]string{
			"grant_type":    "authorization_code",
			"code":          "invalid-code",
			"client_id":     "test-client-id",
			"redirect_uri":  "http://localhost:3000/callback",
			"client_secret": "test-client-secret",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "无效的授权码")
	})
}

func TestTokenHandler_HandleRevoke(t *testing.T) {
	tokenHandler, store := createTestTokenHandler(t)

	// 创建用户并登录
	passwordSvc := crypto.NewPasswordService(4)
	hashedPassword, _ := passwordSvc.HashPassword("Password123!")

	user := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
	}
	store.AddUser(user)

	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test-issuer", 15*time.Minute, 7*24*time.Hour)
	authSvc := service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute)

	loginResp, _ := authSvc.Login(context.Background(), &model.LoginRequest{
		Email:    "test@example.com",
		Password: "Password123!",
	})

	t.Run("成功撤销Token", func(t *testing.T) {
		body := map[string]string{
			"token": loginResp.AccessToken,
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token/revoke", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleRevoke(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("缺少token参数", func(t *testing.T) {
		body := map[string]string{}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token/revoke", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleRevoke(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "缺少token参数")
	})

	t.Run("无效的JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/token/revoke", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleRevoke(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ============================================================================
// AuthorizeHandler 测试 (已在 authorize_test.go 中覆盖，此处删除重复测试)
// ============================================================================

// ============================================================================
// LoginHandler 边界条件测试
// ============================================================================

func TestLoginHandler_BoundaryConditions(t *testing.T) {
	loginHandler, store := createTestLoginHandler(t)

	// 创建测试用户
	passwordSvc := crypto.NewPasswordService(4)
	hashedPassword, err := passwordSvc.HashPassword("Password123!")
	require.NoError(t, err)

	testUser := &model.User{
		ID:            "boundary-test-user",
		Email:         "boundary@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(testUser)

	t.Run("空密码输入", func(t *testing.T) {
		body := map[string]string{
			"email":    "boundary@example.com",
			"password": "",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/login", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空邮箱输入", func(t *testing.T) {
		body := map[string]string{
			"email":    "",
			"password": "Password123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/login", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少必填字段-email", func(t *testing.T) {
		body := map[string]string{
			"password": "Password123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/login", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少必填字段-password", func(t *testing.T) {
		body := map[string]string{
			"email": "boundary@example.com",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/login", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("超大请求体", func(t *testing.T) {
		// 创建一个超大的请求体（>1MB）
		largePassword := string(make([]byte, 2*1024*1024)) // 2MB
		body := map[string]string{
			"email":    "boundary@example.com",
			"password": largePassword,
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/login", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		loginHandler.Handle(w, req)

		// 应该返回400或413
		assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusRequestEntityTooLarge)
	})
}

// ============================================================================
// RegisterHandler 边界条件测试
// ============================================================================

func TestRegisterHandler_BoundaryConditions(t *testing.T) {
	registerHandler, store := createTestRegisterHandler(t)

	t.Run("重复邮箱注册", func(t *testing.T) {
		store.Reset()

		// 先创建一个用户
		existingUser := &model.User{
			ID:            "existing-user",
			Email:         "existing@example.com",
			PasswordHash:  "hash",
			EmailVerified: false,
			Status:        model.UserStatusActive,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		store.AddUser(existingUser)

		// 尝试用相同邮箱注册
		body := map[string]string{
			"email":    "existing@example.com",
			"password": "Password123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		registerHandler.Handle(w, req)

		// 应返回与成功相同的响应，防止用户枚举
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("无效邮箱格式", func(t *testing.T) {
		store.Reset()

		body := map[string]string{
			"email":    "invalid-email",
			"password": "Password123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		registerHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("弱密码", func(t *testing.T) {
		store.Reset()

		body := map[string]string{
			"email":    "weakpass@example.com",
			"password": "123",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		registerHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少必填字段-email", func(t *testing.T) {
		store.Reset()

		body := map[string]string{
			"password": "Password123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		registerHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少必填字段-password", func(t *testing.T) {
		store.Reset()

		body := map[string]string{
			"email": "nopass@example.com",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		registerHandler.Handle(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("超大请求体", func(t *testing.T) {
		store.Reset()

		// 创建超大请求体
		largeEmail := string(make([]byte, 2*1024*1024)) + "@example.com"
		body := map[string]string{
			"email":    largeEmail,
			"password": "Password123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		registerHandler.Handle(w, req)

		// 应该返回400或413
		assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusRequestEntityTooLarge)
	})
}
