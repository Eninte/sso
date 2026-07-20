// Package service_test 第三方登录服务单元测试
package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestJWTService 创建测试用的JWT服务
func createTestJWTService() *crypto.JWTService {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	return crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)
}

// ============================================================================
// NewSocialLoginService 测试
// ============================================================================

func TestNewSocialLoginService(t *testing.T) {
	storeInst := mock.New()
	jwtSvc := createTestJWTService()

	t.Run("无提供商配置", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "", "", "", "")
		assert.NotNil(t, svc)

		providers := svc.GetProviders()
		assert.Empty(t, providers)
	})

	t.Run("配置Google提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "google-id", "google-secret", "", "")
		assert.NotNil(t, svc)

		providers := svc.GetProviders()
		assert.Contains(t, providers, "google")
	})

	t.Run("配置GitHub提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "", "", "github-id", "github-secret")
		assert.NotNil(t, svc)

		providers := svc.GetProviders()
		assert.Contains(t, providers, "github")
	})

	t.Run("配置所有提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "google-id", "google-secret", "github-id", "github-secret")
		assert.NotNil(t, svc)

		providers := svc.GetProviders()
		assert.Contains(t, providers, "google")
		assert.Contains(t, providers, "github")
	})
}

// ============================================================================
// GetProviders 测试
// ============================================================================

func TestSocialLoginService_GetProviders(t *testing.T) {
	storeInst := mock.New()
	jwtSvc := createTestJWTService()

	t.Run("获取空提供商列表", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "", "", "", "")
		providers := svc.GetProviders()
		assert.Empty(t, providers)
	})

	t.Run("获取已配置的提供商列表", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "google-id", "google-secret", "github-id", "github-secret")
		providers := svc.GetProviders()
		assert.Len(t, providers, 2)
	})
}

// ============================================================================
// GetAuthorizationURL 测试
// ============================================================================

func TestSocialLoginService_GetAuthorizationURL(t *testing.T) {
	storeInst := mock.New()
	jwtSvc := createTestJWTService()

	t.Run("成功获取Google授权URL", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "google-id", "google-secret", "", "")

		url, err := svc.GetAuthorizationURL("google", "random-state")

		require.NoError(t, err)
		assert.Contains(t, url, "https://accounts.google.com/o/oauth2/v2/auth")
		assert.Contains(t, url, "client_id=google-id")
		assert.Contains(t, url, "state=random-state")
	})

	t.Run("成功获取GitHub授权URL", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "", "", "github-id", "github-secret")

		url, err := svc.GetAuthorizationURL("github", "random-state")

		require.NoError(t, err)
		assert.Contains(t, url, "https://github.com/login/oauth/authorize")
		assert.Contains(t, url, "client_id=github-id")
		assert.Contains(t, url, "state=random-state")
	})

	t.Run("不支持的提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "", "", "", "")

		_, err := svc.GetAuthorizationURL("unsupported", "state")

		assert.ErrorIs(t, err, service.ErrProviderNotSupported)
	})

	t.Run("空redirectURI使用默认值", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "google-id", "google-secret", "", "")

		url, err := svc.GetAuthorizationURL("google", "state")

		require.NoError(t, err)
		assert.Contains(t, url, "redirect_uri=http%3A%2F%2Flocalhost%3A9000%2Fauth%2Fgoogle%2Fcallback")
	})
}

// ============================================================================
// HandleCallback 测试
// ============================================================================

func TestSocialLoginService_HandleCallback(t *testing.T) {
	storeInst := mock.New()
	jwtSvc := createTestJWTService()

	t.Run("不支持的提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "", "", "", "")

		_, err := svc.HandleCallback(context.Background(), "unsupported", "code", "test-state")

		assert.ErrorIs(t, err, service.ErrProviderNotSupported)
	})

	t.Run("空redirectURI使用默认值-不支持的提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "", "", "", "")

		_, err := svc.HandleCallback(context.Background(), "github", "code", "test-state")

		assert.ErrorIs(t, err, service.ErrProviderNotSupported)
	})
}

// ============================================================================
// HandleCallback 完整流程测试 (使用NewSocialLoginServiceWithProviders)
// ============================================================================

func TestSocialLoginService_HandleCallback_FullFlow(t *testing.T) {
	t.Run("Google回调-创建新用户", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		tokenResp := map[string]interface{}{
			"access_token": "mock-google-token",
			"token_type":   "bearer",
		}
		// 阶段 2.3：必须返回 sub（ProviderUserID）和 email_verified=true
		userInfoResp := map[string]interface{}{
			"sub":            "google-user-123",
			"email":          "newuser@gmail.com",
			"email_verified": true,
			"name":           "New User",
		}

		// 创建mock服务器
		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenResp)
		})
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(userInfoResp)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		// 构造mock providers，URL指向mock服务器
		providers := map[string]*service.OAuthProvider{
			"google": {
				Name:         "google",
				ClientID:     "g-id",
				ClientSecret: "g-secret",
				AuthURL:      server.URL + "/auth",
				TokenURL:     server.URL + "/token",
				UserInfoURL:  server.URL + "/userinfo",
				Scopes:       []string{"email", "profile"},
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		// 先获取授权URL以生成state
		authURL, err := svc.GetAuthorizationURL("google", "")
		require.NoError(t, err)

		// 从URL中提取state
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")
		require.NotEmpty(t, state)

		resp, err := svc.HandleCallback(context.Background(), "google", "mock-code", state)

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.Equal(t, "Bearer", resp.TokenType)

		// 验证用户已创建
		user, err := storeInst.GetByEmail(context.Background(), "newuser@gmail.com")
		require.NoError(t, err)
		assert.Equal(t, "newuser@gmail.com", user.Email)
		assert.True(t, user.EmailVerified)

		// 阶段 2.3：验证 social_account 已创建
		account, err := storeInst.GetSocialAccount(context.Background(), "google", "google-user-123")
		require.NoError(t, err)
		assert.Equal(t, user.ID, account.UserID)
		assert.Equal(t, "google", account.Provider)
		assert.Equal(t, "google-user-123", account.ProviderUserID)
		assert.True(t, account.EmailVerified)
	})

	t.Run("Google回调-社交账号已存在-复用用户", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		// 阶段 2.3：预先创建用户 + social_account（模拟用户已绑定 Google 账号）
		hashedPw, _ := crypto.NewPasswordService(4).HashPassword("Pass123!")
		storeInst.AddUser(&model.User{
			ID:            "existing-user-id",
			Email:         "existing@gmail.com",
			PasswordHash:  hashedPw,
			EmailVerified: true,
			Status:        model.UserStatusActive,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})
		storeInst.AddSocialAccount(&model.SocialAccount{
			ID:             "sa-1",
			Provider:       "google",
			ProviderUserID: "google-user-123",
			UserID:         "existing-user-id",
			ProviderEmail:  "existing@gmail.com",
			EmailVerified:  true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		})

		tokenResp := map[string]interface{}{"access_token": "tok"}
		userInfoResp := map[string]interface{}{
			"sub":            "google-user-123",
			"email":          "existing@gmail.com",
			"email_verified": true,
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(tokenResp)
		})
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(userInfoResp)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		providers := map[string]*service.OAuthProvider{
			"google": {
				Name: "google", ClientID: "g-id", ClientSecret: "g-secret",
				TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/userinfo",
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		authURL, err := svc.GetAuthorizationURL("google", "")
		require.NoError(t, err)
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")
		require.NotEmpty(t, state)

		resp, err := svc.HandleCallback(context.Background(), "google", "code", state)

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)

		// 阶段 2.3：验证不会重复创建用户
		users, _, _ := storeInst.ListUsers(context.Background(), 0, 100)
		assert.Len(t, users, 1)
	})

	t.Run("阶段2.3-拒绝provider_email未验证", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		tokenResp := map[string]interface{}{"access_token": "tok"}
		// Google 返回 email_verified=false
		userInfoResp := map[string]interface{}{
			"sub":            "google-unverified-id",
			"email":          "unverified@gmail.com",
			"email_verified": false,
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(tokenResp)
		})
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(userInfoResp)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		providers := map[string]*service.OAuthProvider{
			"google": {
				Name: "google", ClientID: "g-id", ClientSecret: "g-secret",
				TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/userinfo",
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		authURL, _ := svc.GetAuthorizationURL("google", "")
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")

		_, err := svc.HandleCallback(context.Background(), "google", "code", state)

		// 阶段 2.3：未验证 email 应被拒绝
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrProviderEmailNotVerified)
	})

	t.Run("阶段2.3-拒绝email与本地账号冲突", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		// 预先创建本地账号（无 social_account 绑定）
		hashedPw, _ := crypto.NewPasswordService(4).HashPassword("Pass123!")
		storeInst.AddUser(&model.User{
			ID:            "local-user-id",
			Email:         "local@gmail.com",
			PasswordHash:  hashedPw,
			EmailVerified: true,
			Status:        model.UserStatusActive,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})

		tokenResp := map[string]interface{}{"access_token": "tok"}
		// Google 返回与本地账号相同的 email，但 ProviderUserID 不同（应拒绝合并）
		userInfoResp := map[string]interface{}{
			"sub":            "attacker-google-id",
			"email":          "local@gmail.com",
			"email_verified": true,
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(tokenResp)
		})
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(userInfoResp)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		providers := map[string]*service.OAuthProvider{
			"google": {
				Name: "google", ClientID: "g-id", ClientSecret: "g-secret",
				TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/userinfo",
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		authURL, _ := svc.GetAuthorizationURL("google", "")
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")

		_, err := svc.HandleCallback(context.Background(), "google", "code", state)

		// 阶段 2.3：email 冲突应拒绝自动合并，返回 ErrEmailConflictWithLocal
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrEmailConflictWithLocal)
	})

	t.Run("GitHub回调-无email-拒绝合成email", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		tokenResp := map[string]interface{}{"access_token": "gh-token"}
		// GitHub 风格：有 id 但无 email
		userInfoResp := map[string]interface{}{
			"id":    float64(12345),
			"login": "ghuser",
			"name":  "GitHub User",
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(tokenResp)
		})
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(userInfoResp)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		providers := map[string]*service.OAuthProvider{
			"github": {
				Name: "github", ClientID: "gh-id", ClientSecret: "gh-secret",
				TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/userinfo",
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		authURL, _ := svc.GetAuthorizationURL("github", "")
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")

		_, err := svc.HandleCallback(context.Background(), "github", "code", state)

		// 阶段 2.3：GitHub 无 email 时视为未验证，应拒绝
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrProviderEmailNotVerified)

		// 验证不会合成 login@github.com 创建用户
		users, _, _ := storeInst.ListUsers(context.Background(), 0, 100)
		assert.Len(t, users, 0)
	})

	t.Run("token交换失败", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		// 返回无效token响应（无access_token）
		errorResp := map[string]interface{}{"error": "invalid_grant"}

		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(errorResp)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		providers := map[string]*service.OAuthProvider{
			"google": {
				Name: "google", ClientID: "g-id", ClientSecret: "g-secret",
				TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/userinfo",
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		// 先获取授权URL以生成state
		authURL, err := svc.GetAuthorizationURL("google", "")
		require.NoError(t, err)
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")
		require.NotEmpty(t, state)

		_, err = svc.HandleCallback(context.Background(), "google", "bad-code", state)

		assert.ErrorIs(t, err, service.ErrOAuthCodeInvalid)
	})

	t.Run("阶段2.3-无provider_user_id-拒绝", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		tokenResp := map[string]interface{}{"access_token": "tok"}
		// 仅返回 name，没有 sub/id 字段
		userInfoResp := map[string]interface{}{
			"email":          "no-sub@gmail.com",
			"email_verified": true,
			"name":           "No Sub User",
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(tokenResp)
		})
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(userInfoResp)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		providers := map[string]*service.OAuthProvider{
			"google": {
				Name: "google", ClientID: "g-id", ClientSecret: "g-secret",
				TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/userinfo",
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		authURL, _ := svc.GetAuthorizationURL("google", "")
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")

		_, err := svc.HandleCallback(context.Background(), "google", "code", state)

		// 阶段 2.3：缺少 provider_user_id 应被拒绝
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrProviderUserIDMissing)
	})
}

// ============================================================================
// 阶段 D 审查修复（H2）：GitHub /user/emails API 补全 email_verified 测试
// ============================================================================

func TestSocialLoginService_HandleCallback_GitHub_EmailsAPI(t *testing.T) {
	// GitHub 标准 /user/emails 响应：两项均为 verified
	ghEmailsResp := []map[string]interface{}{
		{
			"email":      "ghuser@example.com",
			"primary":    true,
			"verified":   true,
			"visibility": "public",
		},
		{
			"email":      "ghuser-alt@example.com",
			"primary":    false,
			"verified":   true,
			"visibility": "private",
		},
	}

	t.Run("GitHub回调-通过/user/emails补全verified=true-创建新用户", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		tokenResp := map[string]interface{}{"access_token": "gh-token", "token_type": "bearer"}
		// GitHub /user 不返回 email_verified
		userInfoResp := map[string]interface{}{
			"id":    float64(12345),
			"login": "ghuser",
			"name":  "GitHub User",
			"email": "ghuser@example.com",
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenResp)
		})
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(userInfoResp)
		})
		mux.HandleFunc("/emails", func(w http.ResponseWriter, r *http.Request) {
			// 阶段 D 修复（H2）：校验 Authorization 头
			auth := r.Header.Get("Authorization")
			require.Equal(t, "Bearer gh-token", auth)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(ghEmailsResp)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		providers := map[string]*service.OAuthProvider{
			"github": {
				Name:         "github",
				ClientID:     "gh-id",
				ClientSecret: "gh-secret",
				TokenURL:     server.URL + "/token",
				UserInfoURL:  server.URL + "/userinfo",
				// 阶段 D 修复（H2）：指向 mock /emails 端点
				EmailsURL: server.URL + "/emails",
				Scopes:    []string{"user:email"},
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		authURL, err := svc.GetAuthorizationURL("github", "")
		require.NoError(t, err)
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")
		require.NotEmpty(t, state)

		resp, err := svc.HandleCallback(context.Background(), "github", "code", state)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)

		// 验证用户已创建且 email_verified=true
		user, err := storeInst.GetByEmail(context.Background(), "ghuser@example.com")
		require.NoError(t, err)
		assert.Equal(t, "ghuser@example.com", user.Email)
		assert.True(t, user.EmailVerified)

		// 验证 social_account 已创建且 EmailVerified=true
		account, err := storeInst.GetSocialAccount(context.Background(), "github", "12345")
		require.NoError(t, err)
		assert.True(t, account.EmailVerified)
	})

	t.Run("GitHub回调-/user/emails返回verified=false-拒绝登录", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		tokenResp := map[string]interface{}{"access_token": "gh-token"}
		userInfoResp := map[string]interface{}{
			"id":    float64(67890),
			"login": "unverified-user",
			"email": "unverified-gh@example.com",
		}
		// /user/emails 返回 verified=false
		emailsResp := []map[string]interface{}{
			{
				"email":    "unverified-gh@example.com",
				"primary":  true,
				"verified": false,
			},
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(tokenResp)
		})
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(userInfoResp)
		})
		mux.HandleFunc("/emails", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(emailsResp)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		providers := map[string]*service.OAuthProvider{
			"github": {
				Name:         "github",
				ClientID:     "gh-id",
				ClientSecret: "gh-secret",
				TokenURL:     server.URL + "/token",
				UserInfoURL:  server.URL + "/userinfo",
				EmailsURL:    server.URL + "/emails",
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		authURL, _ := svc.GetAuthorizationURL("github", "")
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")

		_, err := svc.HandleCallback(context.Background(), "github", "code", state)

		// 阶段 D 修复（H2）：未验证 email 应被拒绝
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrProviderEmailNotVerified)

		// 验证不会创建用户
		users, _, _ := storeInst.ListUsers(context.Background(), 0, 100)
		assert.Len(t, users, 0)
	})

	t.Run("GitHub回调-/user/emails API 5xx-保守保持false-拒绝登录", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		tokenResp := map[string]interface{}{"access_token": "gh-token"}
		userInfoResp := map[string]interface{}{
			"id":    float64(11111),
			"login": "api-fail-user",
			"email": "api-fail@example.com",
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(tokenResp)
		})
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(userInfoResp)
		})
		mux.HandleFunc("/emails", func(w http.ResponseWriter, r *http.Request) {
			// 模拟 GitHub API 5xx 错误
			w.WriteHeader(http.StatusInternalServerError)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		providers := map[string]*service.OAuthProvider{
			"github": {
				Name:         "github",
				ClientID:     "gh-id",
				ClientSecret: "gh-secret",
				TokenURL:     server.URL + "/token",
				UserInfoURL:  server.URL + "/userinfo",
				EmailsURL:    server.URL + "/emails",
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		authURL, _ := svc.GetAuthorizationURL("github", "")
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")

		_, err := svc.HandleCallback(context.Background(), "github", "code", state)

		// 阶段 D 修复（H2）：fail-secure，API 失败保守保持 EmailVerified=false
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrProviderEmailNotVerified)
	})

	t.Run("GitHub回调-identity.Email为空-取primary_verified_email填充", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		tokenResp := map[string]interface{}{"access_token": "gh-token"}
		// GitHub /user 未公开 email
		userInfoResp := map[string]interface{}{
			"id":    float64(22222),
			"login": "no-public-email",
			"name":  "No Public Email",
			// 无 email 字段
		}
		emailsResp := []map[string]interface{}{
			{
				"email":    "private@example.com",
				"primary":  true,
				"verified": true,
			},
			{
				"email":    "alt@example.com",
				"primary":  false,
				"verified": true,
			},
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(tokenResp)
		})
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(userInfoResp)
		})
		mux.HandleFunc("/emails", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(emailsResp)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		providers := map[string]*service.OAuthProvider{
			"github": {
				Name:         "github",
				ClientID:     "gh-id",
				ClientSecret: "gh-secret",
				TokenURL:     server.URL + "/token",
				UserInfoURL:  server.URL + "/userinfo",
				EmailsURL:    server.URL + "/emails",
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		authURL, _ := svc.GetAuthorizationURL("github", "")
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")

		resp, err := svc.HandleCallback(context.Background(), "github", "code", state)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)

		// 验证用户已使用 primary verified email 创建
		user, err := storeInst.GetByEmail(context.Background(), "private@example.com")
		require.NoError(t, err)
		assert.Equal(t, "private@example.com", user.Email)
		assert.True(t, user.EmailVerified)
	})

	t.Run("GitHub回调-无primary_verified_email-拒绝", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		tokenResp := map[string]interface{}{"access_token": "gh-token"}
		userInfoResp := map[string]interface{}{
			"id":    float64(33333),
			"login": "no-verified-email",
			"name":  "No Verified Email",
		}
		// identity.Email 为空，/user/emails 无 primary && verified 项
		emailsResp := []map[string]interface{}{
			{
				"email":    "primary-unverified@example.com",
				"primary":  true,
				"verified": false,
			},
			{
				"email":    "alt-verified@example.com",
				"primary":  false,
				"verified": true,
			},
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(tokenResp)
		})
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(userInfoResp)
		})
		mux.HandleFunc("/emails", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(emailsResp)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		providers := map[string]*service.OAuthProvider{
			"github": {
				Name:         "github",
				ClientID:     "gh-id",
				ClientSecret: "gh-secret",
				TokenURL:     server.URL + "/token",
				UserInfoURL:  server.URL + "/userinfo",
				EmailsURL:    server.URL + "/emails",
			},
		}

		svc := service.NewSocialLoginServiceWithProviders(storeInst, jwtSvc, providers, http.DefaultClient)

		authURL, _ := svc.GetAuthorizationURL("github", "")
		parsedURL, _ := url.Parse(authURL)
		state := parsedURL.Query().Get("state")

		_, err := svc.HandleCallback(context.Background(), "github", "code", state)

		// identity.Email 为空且无 primary && verified 项，保守保持 EmailVerified=false
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrProviderEmailNotVerified)
	})
}
