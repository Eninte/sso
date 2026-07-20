// Package service_test OAuth服务单元测试
package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
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
func createTestJWTServiceForOAuth() *crypto.JWTService {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	return crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)
}

// createTestTokenService 创建测试用的Token服务
func createTestTokenService(store *mock.Store) *service.TokenService {
	jwtSvc := createTestJWTServiceForOAuth()
	return service.NewTokenService(jwtSvc, store)
}

// createTestOAuthService 创建测试用的OAuth服务
func createTestOAuthService(t *testing.T) (*service.OAuthService, *mock.Store) {
	store := mock.New()
	tokenSvc := createTestTokenService(store)
	passwordSvc := crypto.NewPasswordService(10)
	oauthSvc := service.NewOAuthService(store, tokenSvc, service.WithOAuthPassword(passwordSvc))
	return oauthSvc, store
}

// createTestClient 创建测试用的OAuth客户端（ClientSecret 为 bcrypt 哈希）
func createTestClient() *model.Client {
	passwordSvc := crypto.NewPasswordService(10)
	secretHash, _ := passwordSvc.HashPassword("test-client-secret")
	return &model.Client{
		ID:           "test-client-id",
		ClientID:     "test-client-id",
		ClientSecret: secretHash,
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
	oauthSvc, storeInst := createTestOAuthService(t)
	ctx := context.Background()

	// 阶段 2.4：RevokeToken 行为对齐 Postgres
	//   - 不存在的 token：不报错（UPDATE 0 行也返回 nil）
	//   - 已撤销的 token：不报错（不覆盖原撤销时间，幂等）
	//   - 成功撤销：清缓存 + 记审计
	t.Run("撤销不存在的Token不报错（幂等）", func(t *testing.T) {
		err := oauthSvc.RevokeToken(ctx, "nonexistent-token")
		assert.NoError(t, err)
	})

	t.Run("撤销空Token不报错（幂等）", func(t *testing.T) {
		err := oauthSvc.RevokeToken(ctx, "")
		assert.NoError(t, err)
	})

	t.Run("成功撤销已存在的Token", func(t *testing.T) {
		// 准备一个有效 token
		token := &model.Token{
			ID:          "token-test-id",
			AccessToken: "valid-access-token",
			RefreshToken: "valid-refresh-token",
			UserID:      "user-test",
			ExpiresAt:   time.Now().Add(time.Hour),
			CreatedAt:   time.Now(),
		}
		require.NoError(t, storeInst.StoreToken(ctx, token))

		err := oauthSvc.RevokeToken(ctx, "valid-access-token")
		assert.NoError(t, err)

		// 验证 token 已被撤销
		revoked, err := storeInst.GetTokenByAccessToken(ctx, "valid-access-token")
		require.NoError(t, err)
		assert.NotNil(t, revoked.RevokedAt)
	})

	t.Run("重复撤销不覆盖原撤销时间", func(t *testing.T) {
		token := &model.Token{
			ID:          "token-duplicate-id",
			AccessToken: "dup-access-token",
			RefreshToken: "dup-refresh-token",
			UserID:      "user-dup",
			ExpiresAt:   time.Now().Add(time.Hour),
			CreatedAt:   time.Now(),
		}
		require.NoError(t, storeInst.StoreToken(ctx, token))

		// 第一次撤销
		require.NoError(t, oauthSvc.RevokeToken(ctx, "dup-access-token"))
		first, _ := storeInst.GetTokenByAccessToken(ctx, "dup-access-token")
		require.NotNil(t, first.RevokedAt)
		firstRevokedAt := *first.RevokedAt

		// 等待一毫秒确保时间戳不同
		time.Sleep(time.Millisecond)

		// 第二次撤销
		require.NoError(t, oauthSvc.RevokeToken(ctx, "dup-access-token"))
		second, _ := storeInst.GetTokenByAccessToken(ctx, "dup-access-token")
		require.NotNil(t, second.RevokedAt)

		// 阶段 2.4：验证撤销时间未被覆盖
		assert.Equal(t, firstRevokedAt, *second.RevokedAt,
			"重复撤销不应覆盖首次撤销时间戳")
	})

	t.Run("撤销失败返回错误", func(t *testing.T) {
		storeInst.RevokeTokenErr = assert.AnError
		defer func() { storeInst.RevokeTokenErr = nil }()

		err := oauthSvc.RevokeToken(ctx, "any-token")
		assert.Error(t, err)
	})
}

// ============================================================================
// NewOAuthServiceWithAudit 测试
// ============================================================================

func TestOAuthService_NewOAuthServiceWithAudit(t *testing.T) {
	store := mock.New()
	tokenSvc := createTestTokenService(store)

	t.Run("创建带审计的OAuth服务", func(t *testing.T) {
		auditSvc := service.NewAuditService(store)
		defer auditSvc.Close()

		oauthSvc := service.NewOAuthServiceWithAudit(store, auditSvc, tokenSvc)
		assert.NotNil(t, oauthSvc)
	})
}

// ============================================================================
// NewOAuthServiceWithCache 测试
// ============================================================================

func TestOAuthService_NewOAuthServiceWithCache(t *testing.T) {
	store := mock.New()
	tokenSvc := createTestTokenService(store)

	t.Run("创建带缓存的OAuth服务", func(t *testing.T) {
		// 使用mock cache
		oauthSvc := service.NewOAuthServiceWithCache(store, nil, tokenSvc)
		assert.NotNil(t, oauthSvc)
	})
}
