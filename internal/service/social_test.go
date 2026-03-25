// Package service_test 第三方登录服务单元测试
package service_test

import (
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
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store"
	"github.com/your-org/sso/internal/store/mock"
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

// redirectHTTPClient 将所有请求重定向到mock服务器的HTTP客户端
type redirectHTTPClient struct {
	server *httptest.Server
}

func (r *redirectHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// 重写URL到mock服务器
	newURL := r.server.URL + req.URL.Path
	if req.URL.RawQuery != "" {
		newURL += "?" + req.URL.RawQuery
	}
	newReq, _ := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	newReq.Header = req.Header.Clone()
	return http.DefaultClient.Do(newReq)
}

// newMockSocialService 创建带有mock HTTP服务器的社交登录服务
func newMockSocialService(t *testing.T, tokenResp, userInfoResp interface{}) (*service.SocialLoginService, *mock.Store) {
	t.Helper()

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
	t.Cleanup(server.Close)

	storeInst := mock.New()
	jwtSvc := createTestJWTService()

	// 使用NewSocialLoginService创建service
	// 然后通过设置providers的URL来使用mock服务器
	// 由于providers未导出，我们使用redirectHTTPClient
	// 注意：providers的TokenURL是固定的，但HTTPClient会被调用
	// 所以我们用redirectHTTPClient重定向所有请求到mock server
	svc := service.NewSocialLoginService(storeInst, jwtSvc, "g-id", "g-secret", "gh-id", "gh-secret")

	// 通过替换HTTPClient来mock请求
	// 但providers里的URL还是真实的，所以redirectHTTPClient会把
	// 真实URL的请求重定向到mock server
	// 这需要访问未导出的httpClient字段...
	// 由于无法直接设置，我们只能测试公开方法

	return svc, storeInst
}

// ============================================================================
// NewSocialLoginService 测试
// ============================================================================

func TestNewSocialLoginService(t *testing.T) {
	storeInst := mock.New()
	jwtSvc := createTestJWTService()

	t.Run("无提供商配置", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "", "", "", "")
		assert.NotNil(t, svc)

		providers := svc.GetProviders()
		assert.Empty(t, providers)
	})

	t.Run("配置Google提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "google-id", "google-secret", "", "")
		assert.NotNil(t, svc)

		providers := svc.GetProviders()
		assert.Contains(t, providers, "google")
	})

	t.Run("配置GitHub提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "", "", "github-id", "github-secret")
		assert.NotNil(t, svc)

		providers := svc.GetProviders()
		assert.Contains(t, providers, "github")
	})

	t.Run("配置所有提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "google-id", "google-secret", "github-id", "github-secret")
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
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "", "", "", "")
		providers := svc.GetProviders()
		assert.Empty(t, providers)
	})

	t.Run("获取已配置的提供商列表", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "google-id", "google-secret", "github-id", "github-secret")
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
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "google-id", "google-secret", "", "")

		url, err := svc.GetAuthorizationURL("google", "http://localhost/callback", "random-state")

		require.NoError(t, err)
		assert.Contains(t, url, "https://accounts.google.com/o/oauth2/v2/auth")
		assert.Contains(t, url, "client_id=google-id")
		assert.Contains(t, url, "state=random-state")
	})

	t.Run("成功获取GitHub授权URL", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "", "", "github-id", "github-secret")

		url, err := svc.GetAuthorizationURL("github", "http://localhost/callback", "random-state")

		require.NoError(t, err)
		assert.Contains(t, url, "https://github.com/login/oauth/authorize")
		assert.Contains(t, url, "client_id=github-id")
		assert.Contains(t, url, "state=random-state")
	})

	t.Run("不支持的提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "", "", "", "")

		_, err := svc.GetAuthorizationURL("unsupported", "http://localhost/callback", "state")

		assert.ErrorIs(t, err, service.ErrProviderNotSupported)
	})

	t.Run("空redirectURI使用默认值", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "google-id", "google-secret", "", "")

		url, err := svc.GetAuthorizationURL("google", "", "state")

		require.NoError(t, err)
		assert.Contains(t, url, "redirect_uri=http%3A%2F%2Flocalhost%3A9090%2Fauth%2Fgoogle%2Fcallback")
	})
}

// ============================================================================
// HandleCallback 测试
// ============================================================================

func TestSocialLoginService_HandleCallback(t *testing.T) {
	storeInst := mock.New()
	jwtSvc := createTestJWTService()

	t.Run("不支持的提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "", "", "", "")

		_, err := svc.HandleCallback(context.Background(), "unsupported", "code", "http://localhost/callback")

		assert.ErrorIs(t, err, service.ErrProviderNotSupported)
	})

	t.Run("空redirectURI使用默认值-不支持的提供商", func(t *testing.T) {
		svc := service.NewSocialLoginService(storeInst, jwtSvc, "", "", "", "")

		_, err := svc.HandleCallback(context.Background(), "github", "code", "")

		assert.ErrorIs(t, err, service.ErrProviderNotSupported)
	})
}

// ============================================================================
// findOrCreateUser 逻辑测试 (通过mock store)
// ============================================================================

func TestSocialLoginService_FindOrCreateUser(t *testing.T) {
	t.Run("用户已存在-直接返回", func(t *testing.T) {
		storeInst := mock.New()

		hashedPw, _ := crypto.NewPasswordService(10).HashPassword("Test1234!")
		storeInst.AddUser(&model.User{
			ID:            "existing-user-id",
			Email:         "existing@gmail.com",
			PasswordHash:  hashedPw,
			EmailVerified: true,
			Status:        model.UserStatusActive,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})

		user, err := storeInst.GetByEmail(context.Background(), "existing@gmail.com")
		require.NoError(t, err)
		assert.Equal(t, "existing@gmail.com", user.Email)
		assert.True(t, user.EmailVerified)
	})

	t.Run("用户不存在-创建新用户", func(t *testing.T) {
		storeInst := mock.New()

		_, err := storeInst.GetByEmail(context.Background(), "newuser@gmail.com")
		assert.True(t, apperrors.Is(err, store.ErrNotFound))

		now := time.Now()
		newUser := &model.User{
			ID:            "new-social-user-id",
			Email:         "newuser@gmail.com",
			EmailVerified: true,
			Status:        model.UserStatusActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		err = storeInst.Create(context.Background(), newUser)
		require.NoError(t, err)

		user, err := storeInst.GetByEmail(context.Background(), "newuser@gmail.com")
		require.NoError(t, err)
		assert.Equal(t, "newuser@gmail.com", user.Email)
		assert.True(t, user.EmailVerified)
	})

	t.Run("GitHub用户无email-使用login生成", func(t *testing.T) {
		storeInst := mock.New()

		login := "githubuser"
		email := login + "@github.com"

		now := time.Now()
		newUser := &model.User{
			ID:            "github-user-id",
			Email:         email,
			EmailVerified: true,
			Status:        model.UserStatusActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		err := storeInst.Create(context.Background(), newUser)
		require.NoError(t, err)

		user, err := storeInst.GetByEmail(context.Background(), email)
		require.NoError(t, err)
		assert.Equal(t, "githubuser@github.com", user.Email)
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
		userInfoResp := map[string]interface{}{
			"email": "newuser@gmail.com",
			"name":  "New User",
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

		resp, err := svc.HandleCallback(context.Background(), "google", "mock-code", "http://localhost/callback")

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.Equal(t, "Bearer", resp.TokenType)

		// 验证用户已创建
		user, err := storeInst.GetByEmail(context.Background(), "newuser@gmail.com")
		require.NoError(t, err)
		assert.Equal(t, "newuser@gmail.com", user.Email)
		assert.True(t, user.EmailVerified)
	})

	t.Run("Google回调-用户已存在", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		// 预先创建用户
		hashedPw, _ := crypto.NewPasswordService(10).HashPassword("Pass123!")
		storeInst.AddUser(&model.User{
			ID:            "existing-id",
			Email:         "existing@gmail.com",
			PasswordHash:  hashedPw,
			EmailVerified: true,
			Status:        model.UserStatusActive,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})

		tokenResp := map[string]interface{}{"access_token": "tok"}
		userInfoResp := map[string]interface{}{"email": "existing@gmail.com"}

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

		resp, err := svc.HandleCallback(context.Background(), "google", "code", "http://localhost")

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
	})

	t.Run("GitHub回调-无email使用login", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		tokenResp := map[string]interface{}{"access_token": "gh-token"}
		userInfoResp := map[string]interface{}{
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

		resp, err := svc.HandleCallback(context.Background(), "github", "code", "http://localhost")

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)

		// 验证GitHub用户使用login@github.com作为email
		user, err := storeInst.GetByEmail(context.Background(), "ghuser@github.com")
		require.NoError(t, err)
		assert.Equal(t, "ghuser@github.com", user.Email)
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

		_, err := svc.HandleCallback(context.Background(), "google", "bad-code", "http://localhost")

		assert.ErrorIs(t, err, service.ErrOAuthCodeInvalid)
	})

	t.Run("用户信息获取失败-无email", func(t *testing.T) {
		storeInst := mock.New()
		jwtSvc := createTestJWTService()

		tokenResp := map[string]interface{}{"access_token": "tok"}
		// GitHub风格：无email且无login
		userInfoResp := map[string]interface{}{"name": "No Email User"}

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

		_, err := svc.HandleCallback(context.Background(), "github", "code", "http://localhost")

		assert.ErrorIs(t, err, service.ErrSocialLoginFailed)
	})
}
