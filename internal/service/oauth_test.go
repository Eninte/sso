// Package service_test OAuth服务单元测试
package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestOAuthService 创建测试用的OAuth服务
func createTestOAuthService(t *testing.T) (*service.OAuthService, *mock.Store) {
	store := mock.New()
	oauthSvc := service.NewOAuthService(store)
	return oauthSvc, store
}

// createTestClient 创建测试用的OAuth客户端
func createTestClient() *model.Client {
	return &model.Client{
		ID:           "test-client-id",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Name:         "Test Client",
		RedirectURIs: []string{"http://localhost:3000/callback"},
		GrantTypes:   []string{model.GrantTypeAuthorizationCode, model.GrantTypeRefreshToken},
		Scopes:       []string{"openid", "profile", "email"},
		PublicClient: false,
		CreatedAt:    time.Now(),
	}
}

// createTestUser 创建测试用的用户
func createTestUser() *model.User {
	return &model.User{
		ID:           "test-user-id",
		Email:        "test@example.com",
		PasswordHash: "hashed-password",
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

// generateCodeChallenge 生成PKCE挑战码
func generateCodeChallenge(codeVerifier string) string {
	h := sha256.New()
	h.Write([]byte(codeVerifier))
	hash := h.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(hash)
}

// ============================================================================
// CreateAuthorizationCode 测试
// ============================================================================

func TestOAuthService_CreateAuthorizationCode(t *testing.T) {
	oauthSvc, store := createTestOAuthService(t)
	ctx := context.Background()

	// 准备测试数据
	client := createTestClient()
	store.AddClient(client)

	t.Run("成功创建授权码", func(t *testing.T) {
		code, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			"test-client-id",
			"test-user-id",
			"http://localhost:3000/callback",
			[]string{"openid", "profile"},
			"",
			"",
		)

		require.NoError(t, err)
		assert.NotEmpty(t, code)
		// 授权码长度取决于base64编码，不固定为32
		assert.Greater(t, len(code), 20)
	})

	t.Run("带PKCE的授权码", func(t *testing.T) {
		codeVerifier := "test-code-verifier-1234567890"
		codeChallenge := generateCodeChallenge(codeVerifier)

		code, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			"test-client-id",
			"test-user-id",
			"http://localhost:3000/callback",
			[]string{"openid"},
			codeChallenge,
			"S256",
		)

		require.NoError(t, err)
		assert.NotEmpty(t, code)
	})

	t.Run("无效的客户端", func(t *testing.T) {
		_, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			"invalid-client-id",
			"test-user-id",
			"http://localhost:3000/callback",
			[]string{"openid"},
			"",
			"",
		)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidClient)
	})

	t.Run("无效的重定向URI", func(t *testing.T) {
		_, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			"test-client-id",
			"test-user-id",
			"http://evil.com/callback",
			[]string{"openid"},
			"",
			"",
		)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidRedirectURI)
	})

	t.Run("无效的PKCE方法", func(t *testing.T) {
		_, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			"test-client-id",
			"test-user-id",
			"http://localhost:3000/callback",
			[]string{"openid"},
			"test-challenge",
			"invalid-method",
		)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCodeChallenge)
	})
}

// ============================================================================
// ExchangeAuthorizationCode 测试
// ============================================================================

func TestOAuthService_ExchangeAuthorizationCode(t *testing.T) {
	oauthSvc, store := createTestOAuthService(t)
	ctx := context.Background()

	// 准备测试数据
	client := createTestClient()
	store.AddClient(client)

	user := createTestUser()
	store.AddUser(user)

	t.Run("成功交换授权码", func(t *testing.T) {
		// 先创建授权码
		code, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			"test-client-id",
			"test-user-id",
			"http://localhost:3000/callback",
			[]string{"openid", "profile"},
			"",
			"",
		)
		require.NoError(t, err)

		// 交换授权码
		token, err := oauthSvc.ExchangeAuthorizationCode(
			ctx,
			code,
			"test-client-id",
			"test-client-secret",
			"http://localhost:3000/callback",
			"",
		)

		require.NoError(t, err)
		assert.NotEmpty(t, token.AccessToken)
		assert.NotEmpty(t, token.RefreshToken)
		assert.Equal(t, "Bearer", token.TokenType)
	})

	t.Run("带PKCE的授权码交换", func(t *testing.T) {
		codeVerifier := "test-code-verifier-1234567890"
		codeChallenge := generateCodeChallenge(codeVerifier)

		// 创建带PKCE的授权码
		code, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			"test-client-id",
			"test-user-id",
			"http://localhost:3000/callback",
			[]string{"openid"},
			codeChallenge,
			"S256",
		)
		require.NoError(t, err)

		// 使用正确的code_verifier交换
		token, err := oauthSvc.ExchangeAuthorizationCode(
			ctx,
			code,
			"test-client-id",
			"test-client-secret",
			"http://localhost:3000/callback",
			codeVerifier,
		)

		require.NoError(t, err)
		assert.NotEmpty(t, token.AccessToken)
	})

	t.Run("无效的授权码", func(t *testing.T) {
		_, err := oauthSvc.ExchangeAuthorizationCode(
			ctx,
			"invalid-code",
			"test-client-id",
			"test-client-secret",
			"http://localhost:3000/callback",
			"",
		)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCode)
	})

	t.Run("无效的客户端密钥", func(t *testing.T) {
		// 创建授权码
		code, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			"test-client-id",
			"test-user-id",
			"http://localhost:3000/callback",
			[]string{"openid"},
			"",
			"",
		)
		require.NoError(t, err)

		// 使用错误的密钥
		_, err = oauthSvc.ExchangeAuthorizationCode(
			ctx,
			code,
			"test-client-id",
			"wrong-secret",
			"http://localhost:3000/callback",
			"",
		)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidClient)
	})

	t.Run("授权码已使用", func(t *testing.T) {
		// 创建授权码
		code, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			"test-client-id",
			"test-user-id",
			"http://localhost:3000/callback",
			[]string{"openid"},
			"",
			"",
		)
		require.NoError(t, err)

		// 第一次交换
		_, err = oauthSvc.ExchangeAuthorizationCode(
			ctx,
			code,
			"test-client-id",
			"test-client-secret",
			"http://localhost:3000/callback",
			"",
		)
		require.NoError(t, err)

		// 第二次交换应该失败
		_, err = oauthSvc.ExchangeAuthorizationCode(
			ctx,
			code,
			"test-client-id",
			"test-client-secret",
			"http://localhost:3000/callback",
			"",
		)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrCodeUsed)
	})

	t.Run("无效的PKCE验证器", func(t *testing.T) {
		codeVerifier := "test-code-verifier-1234567890"
		codeChallenge := generateCodeChallenge(codeVerifier)

		// 创建带PKCE的授权码
		code, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			"test-client-id",
			"test-user-id",
			"http://localhost:3000/callback",
			[]string{"openid"},
			codeChallenge,
			"S256",
		)
		require.NoError(t, err)

		// 使用错误的code_verifier
		_, err = oauthSvc.ExchangeAuthorizationCode(
			ctx,
			code,
			"test-client-id",
			"test-client-secret",
			"http://localhost:3000/callback",
			"wrong-code-verifier",
		)

		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCodeVerifier)
	})
}

// ============================================================================
// RevokeToken 测试
// ============================================================================

func TestOAuthService_RevokeToken(t *testing.T) {
	oauthSvc, _ := createTestOAuthService(t)
	ctx := context.Background()

	t.Run("撤销不存在的Token返回错误", func(t *testing.T) {
		err := oauthSvc.RevokeToken(ctx, "nonexistent-token")
		assert.Error(t, err)
	})

	t.Run("撤销空Token返回错误", func(t *testing.T) {
		err := oauthSvc.RevokeToken(ctx, "")
		assert.Error(t, err)
	})
}
