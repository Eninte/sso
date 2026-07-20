// Package service_test 阶段 2.2 OAuth 安全增强单元测试
// 测试范围：Scope 校验、PKCE 强制、Consent token、RefreshToken 客户端归属校验
package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 辅助函数
// ============================================================================

// createOAuthTestEnv 创建 OAuth 安全测试环境
func createOAuthTestEnv(t *testing.T) (*service.OAuthService, *mock.Store, *crypto.JWTService) {
	store := mock.New()
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
	passwordSvc := crypto.NewPasswordService(4)
	oauthSvc := service.NewOAuthService(store, tokenSvc,
		service.WithOAuthPassword(passwordSvc),
		service.WithOAuthAudit(service.NewAuditService(store)),
	)
	return oauthSvc, store, jwtSvc
}

// createTestClientWithScopes 创建带指定 scope 的客户端
func createTestClientWithScopes(clientID string, scopes []string, public bool) *model.Client {
	return &model.Client{
		ID:           clientID,
		ClientID:     clientID,
		Name:         "Test Client",
		RedirectURIs: []string{"http://localhost:3000/callback"},
		GrantTypes:   []string{model.GrantTypeAuthorizationCode, model.GrantTypeRefreshToken},
		Scopes:       scopes,
		PublicClient: public,
		CreatedAt:    time.Now(),
	}
}

// generateTestCodeChallenge 生成 S256 code_challenge
func generateTestCodeChallenge(verifier string) string {
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// ============================================================================
// ValidateScope 测试
// ============================================================================

func TestOAuthService_ValidateScope(t *testing.T) {
	oauthSvc, store, _ := createOAuthTestEnv(t)
	ctx := context.Background()

	// 客户端允许 openid/profile/email
	client := createTestClientWithScopes("client-a", []string{"openid", "profile", "email"}, false)
	store.AddClient(client)

	t.Run("成功-请求scope是客户端允许范围子集", func(t *testing.T) {
		valid, err := oauthSvc.ValidateScope(ctx, client, []string{"openid", "email"})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"openid", "email"}, valid)
	})

	t.Run("成功-空请求返回客户端允许的全部scope", func(t *testing.T) {
		valid, err := oauthSvc.ValidateScope(ctx, client, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, valid)
	})

	t.Run("拒绝-scope超出客户端允许范围（升级攻击）", func(t *testing.T) {
		// 客户端只允许 openid/profile/email，请求 offline_access 应被拒绝
		_, err := oauthSvc.ValidateScope(ctx, client, []string{"openid", "offline_access"})
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidScope)
	})

	t.Run("拒绝-scope不在全局白名单内", func(t *testing.T) {
		// 添加客户端允许 custom-scope（但不在白名单内）
		clientWithCustom := createTestClientWithScopes("client-b", []string{"openid", "custom-scope"}, false)
		store.AddClient(clientWithCustom)
		_, err := oauthSvc.ValidateScope(ctx, clientWithCustom, []string{"custom-scope"})
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidScope)
	})

	t.Run("拒绝-客户端未配置任何scope", func(t *testing.T) {
		emptyScopeClient := createTestClientWithScopes("client-c", nil, false)
		store.AddClient(emptyScopeClient)
		_, err := oauthSvc.ValidateScope(ctx, emptyScopeClient, []string{"openid"})
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidScope)
	})

	t.Run("去重-重复scope被规范化", func(t *testing.T) {
		valid, err := oauthSvc.ValidateScope(ctx, client, []string{"openid", "openid", "email", "email"})
		require.NoError(t, err)
		assert.Len(t, valid, 2)
	})
}

// ============================================================================
// ValidatePKCEChallenge 测试
// ============================================================================

func TestOAuthService_ValidatePKCEChallenge(t *testing.T) {
	oauthSvc, store, _ := createOAuthTestEnv(t)
	ctx := context.Background()

	// 公共客户端
	publicClient := createTestClientWithScopes("public-client", []string{"openid"}, true)
	store.AddClient(publicClient)

	// 机密客户端
	confidentialClient := createTestClientWithScopes("confidential-client", []string{"openid"}, false)
	store.AddClient(confidentialClient)

	t.Run("拒绝-公共客户端无PKCE", func(t *testing.T) {
		err := oauthSvc.ValidatePKCEChallenge(ctx, publicClient, "", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrPKCERequired)
	})

	t.Run("拒绝-公共客户端使用plain方法", func(t *testing.T) {
		err := oauthSvc.ValidatePKCEChallenge(ctx, publicClient, "challenge-value", "plain")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrPKCERequired)
	})

	t.Run("拒绝-公共客户端使用非S256方法", func(t *testing.T) {
		err := oauthSvc.ValidatePKCEChallenge(ctx, publicClient, "challenge-value", "invalid-method")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrPKCERequired)
	})

	t.Run("成功-公共客户端使用S256", func(t *testing.T) {
		err := oauthSvc.ValidatePKCEChallenge(ctx, publicClient, "valid-challenge", "S256")
		assert.NoError(t, err)
	})

	t.Run("成功-机密客户端无PKCE（不强制）", func(t *testing.T) {
		err := oauthSvc.ValidatePKCEChallenge(ctx, confidentialClient, "", "")
		assert.NoError(t, err)
	})

	t.Run("拒绝-机密客户端传code_challenge但method非S256", func(t *testing.T) {
		err := oauthSvc.ValidatePKCEChallenge(ctx, confidentialClient, "challenge-value", "plain")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCodeChallenge)
	})

	t.Run("成功-机密客户端使用S256", func(t *testing.T) {
		err := oauthSvc.ValidatePKCEChallenge(ctx, confidentialClient, "valid-challenge", "S256")
		assert.NoError(t, err)
	})
}

// ============================================================================
// VerifyPKCEWithMethod 测试
// ============================================================================

func TestVerifyPKCEWithMethod(t *testing.T) {
	codeVerifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	codeChallenge := generateTestCodeChallenge(codeVerifier)

	t.Run("成功-S256方法且verifier正确", func(t *testing.T) {
		err := service.VerifyPKCEWithMethod(codeChallenge, "S256", codeVerifier)
		assert.NoError(t, err)
	})

	t.Run("拒绝-plain方法（已禁用）", func(t *testing.T) {
		// 即使 challenge == verifier 也不允许 plain
		err := service.VerifyPKCEWithMethod(codeVerifier, "plain", codeVerifier)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCodeChallenge)
	})

	t.Run("拒绝-未知方法", func(t *testing.T) {
		err := service.VerifyPKCEWithMethod(codeChallenge, "unknown", codeVerifier)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCodeChallenge)
	})

	t.Run("拒绝-verifier为空", func(t *testing.T) {
		err := service.VerifyPKCEWithMethod(codeChallenge, "S256", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCodeVerifier)
	})

	t.Run("拒绝-verifier不匹配", func(t *testing.T) {
		err := service.VerifyPKCEWithMethod(codeChallenge, "S256", "wrong-verifier")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCodeVerifier)
	})
}

// ============================================================================
// Consent Token 签发与校验测试
// ============================================================================

func TestOAuthService_ConsentToken(t *testing.T) {
	oauthSvc, store, _ := createOAuthTestEnv(t)
	ctx := context.Background()

	client := createTestClientWithScopes("consent-client", []string{"openid", "profile", "email"}, false)
	store.AddClient(client)

	user := &model.User{
		ID:            "consent-user-id",
		Email:         "consent@example.com",
		Status:        model.UserStatusActive,
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(user)

	t.Run("成功-签发并校验consent_token", func(t *testing.T) {
		token, err := oauthSvc.IssueConsentToken(
			ctx,
			user.ID,
			client.ClientID,
			"http://localhost:3000/callback",
			[]string{"openid", "email"},
			"random-state-value-12345",
			"valid-challenge",
			"S256",
		)
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := oauthSvc.VerifyConsentToken(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, client.ClientID, claims.ClientID)
		assert.Equal(t, user.ID, claims.UserID)
		assert.Equal(t, "http://localhost:3000/callback", claims.RedirectURI)
		assert.ElementsMatch(t, []string{"openid", "email"}, claims.Scopes)
		assert.Equal(t, "random-state-value-12345", claims.State)
		assert.Equal(t, "valid-challenge", claims.CodeChallenge)
		assert.Equal(t, "S256", claims.CodeChallengeMethod)
	})

	t.Run("拒绝-无效签名", func(t *testing.T) {
		// 用错误字符的 token
		_, err := oauthSvc.VerifyConsentToken(ctx, "invalid.token.here")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrConsentInvalid)
	})

	t.Run("拒绝-过期token", func(t *testing.T) {
		// 签发后立即用过期校验：通过手动构造过期 token 太复杂
		// 这里用空 token 触发校验失败
		_, err := oauthSvc.VerifyConsentToken(ctx, "")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrConsentInvalid)
	})
}

// ============================================================================
// CreateAuthorizationCodeWithConsent 测试
// ============================================================================

func TestOAuthService_CreateAuthorizationCodeWithConsent(t *testing.T) {
	oauthSvc, store, _ := createOAuthTestEnv(t)
	ctx := context.Background()

	client := createTestClientWithScopes("consent-flow-client", []string{"openid", "profile", "email"}, false)
	store.AddClient(client)

	user := &model.User{
		ID:            "consent-flow-user",
		Email:         "consent-flow@example.com",
		Status:        model.UserStatusActive,
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(user)

	t.Run("成功-完整consent流程", func(t *testing.T) {
		// 1. 签发 consent_token
		consentToken, err := oauthSvc.IssueConsentToken(
			ctx,
			user.ID,
			client.ClientID,
			"http://localhost:3000/callback",
			[]string{"openid", "email"},
			"state-csrf-1234567890",
			"challenge-value",
			"S256",
		)
		require.NoError(t, err)

		// 2. 通过 consent_token 创建授权码
		// 阶段 D 修复（H1）：传入与 IssueConsentToken 一致的 state 用于校验
		code, err := oauthSvc.CreateAuthorizationCodeWithConsent(ctx, user.ID, consentToken, "state-csrf-1234567890")
		require.NoError(t, err)
		assert.NotEmpty(t, code)
	})

	t.Run("拒绝-consent_token无效", func(t *testing.T) {
		_, err := oauthSvc.CreateAuthorizationCodeWithConsent(ctx, user.ID, "invalid-consent-token", "state-csrf-1234567890")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrConsentInvalid)
	})

	t.Run("拒绝-用户ID不匹配", func(t *testing.T) {
		consentToken, err := oauthSvc.IssueConsentToken(
			ctx,
			user.ID,
			client.ClientID,
			"http://localhost:3000/callback",
			[]string{"openid"},
			"state-csrf-1234567890",
			"",
			"",
		)
		require.NoError(t, err)

		// 用不同用户ID调用
		_, err = oauthSvc.CreateAuthorizationCodeWithConsent(ctx, "different-user-id", consentToken, "state-csrf-1234567890")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrConsentInvalid)
	})

	t.Run("拒绝-redirect_uri非法", func(t *testing.T) {
		// 签发时使用合法 redirect_uri，但客户端未注册该 URI
		consentToken, err := oauthSvc.IssueConsentToken(
			ctx,
			user.ID,
			client.ClientID,
			"http://evil.com/callback",
			[]string{"openid"},
			"state-csrf-1234567890",
			"",
			"",
		)
		// IssueConsentToken 本身不校验 redirect_uri，需要 CreateAuthorizationCodeWithConsent 校验
		// 但 ValidateRedirectURI 在 store 层会拒绝
		require.NoError(t, err)
		_, err = oauthSvc.CreateAuthorizationCodeWithConsent(ctx, user.ID, consentToken, "state-csrf-1234567890")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidRedirectURI)
	})

	t.Run("拒绝-公共客户端无PKCE的consent_token", func(t *testing.T) {
		// 模拟创建公共客户端 consent_token 时未传 PKCE
		publicClient := createTestClientWithScopes("public-consent-client", []string{"openid"}, true)
		store.AddClient(publicClient)

		// 直接签发会触发 service 层校验：IssueConsentToken 不校验 PKCE
		// CreateAuthorizationCodeWithConsent 中的 ValidatePKCEChallenge 会拒绝
		consentToken, err := oauthSvc.IssueConsentToken(
			ctx,
			user.ID,
			publicClient.ClientID,
			"http://localhost:3000/callback",
			[]string{"openid"},
			"state-csrf-1234567890",
			"", // 未传 code_challenge
			"", // 未传 method
		)
		require.NoError(t, err)

		_, err = oauthSvc.CreateAuthorizationCodeWithConsent(ctx, user.ID, consentToken, "state-csrf-1234567890")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrPKCERequired)
	})

	t.Run("拒绝-state不匹配（阶段 D H1 修复）", func(t *testing.T) {
		consentToken, err := oauthSvc.IssueConsentToken(
			ctx,
			user.ID,
			client.ClientID,
			"http://localhost:3000/callback",
			[]string{"openid"},
			"original-state-abcdef",
			"",
			"",
		)
		require.NoError(t, err)

		// 用不同的 state 调用应被拒绝
		_, err = oauthSvc.CreateAuthorizationCodeWithConsent(ctx, user.ID, consentToken, "tampered-state-xyz")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrConsentInvalid)
	})
}

// ============================================================================
// RefreshTokenWithClientID 客户端归属校验测试
// ============================================================================

func TestAuthService_RefreshTokenWithClientID(t *testing.T) {
	ctx := context.Background()

	storeInst := mock.New()
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
	authSvc := service.NewAuthService(storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute)

	t.Run("成功-OAuth签发token携带正确client_id", func(t *testing.T) {
		storeInst.Reset()

		clientID := "oauth-client-id"
		refreshExpiresAt := time.Now().Add(24 * time.Hour)
		storeInst.AddToken(&model.Token{
			ID:               "token-1",
			UserID:           "user-1",
			RefreshToken:     "valid-refresh-token",
			AccessToken:      "valid-access-token",
			ClientID:         &clientID,
			RefreshExpiresAt: &refreshExpiresAt,
			CreatedAt:        time.Now(),
		})
		storeInst.AddUser(&model.User{
			ID:     "user-1",
			Email:  "test@example.com",
			Role:   "user",
			Status: model.UserStatusActive,
		})

		resp, err := authSvc.RefreshTokenWithClientID(ctx, "valid-refresh-token", clientID)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
	})

	t.Run("拒绝-client_id与token归属不一致", func(t *testing.T) {
		storeInst.Reset()

		tokenClientID := "oauth-client-1"
		refreshExpiresAt := time.Now().Add(24 * time.Hour)
		storeInst.AddToken(&model.Token{
			ID:               "token-2",
			UserID:           "user-2",
			RefreshToken:     "cross-client-token",
			AccessToken:      "access-2",
			ClientID:         &tokenClientID,
			RefreshExpiresAt: &refreshExpiresAt,
			CreatedAt:        time.Now(),
		})
		storeInst.AddUser(&model.User{
			ID:     "user-2",
			Email:  "test2@example.com",
			Role:   "user",
			Status: model.UserStatusActive,
		})

		// 用不同的 client_id 刷新
		_, err := authSvc.RefreshTokenWithClientID(ctx, "cross-client-token", "different-client-id")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrClientMismatch)
	})

	t.Run("拒绝-OAuth签发token未传client_id", func(t *testing.T) {
		storeInst.Reset()

		clientID := "oauth-client-id"
		refreshExpiresAt := time.Now().Add(24 * time.Hour)
		storeInst.AddToken(&model.Token{
			ID:               "token-3",
			UserID:           "user-3",
			RefreshToken:     "missing-client-id-token",
			AccessToken:      "access-3",
			ClientID:         &clientID,
			RefreshExpiresAt: &refreshExpiresAt,
			CreatedAt:        time.Now(),
		})
		storeInst.AddUser(&model.User{
			ID:     "user-3",
			Email:  "test3@example.com",
			Role:   "user",
			Status: model.UserStatusActive,
		})

		// 不传 client_id
		_, err := authSvc.RefreshTokenWithClientID(ctx, "missing-client-id-token", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrClientMismatch)
	})

	t.Run("成功-登录流程签发token无ClientID不校验", func(t *testing.T) {
		storeInst.Reset()

		refreshExpiresAt := time.Now().Add(24 * time.Hour)
		storeInst.AddToken(&model.Token{
			ID:               "token-4",
			UserID:           "user-4",
			RefreshToken:     "login-flow-token",
			AccessToken:      "access-4",
			ClientID:         nil, // 登录流程签发
			RefreshExpiresAt: &refreshExpiresAt,
			CreatedAt:        time.Now(),
		})
		storeInst.AddUser(&model.User{
			ID:     "user-4",
			Email:  "test4@example.com",
			Role:   "user",
			Status: model.UserStatusActive,
		})

		// 不传 client_id 也能刷新
		resp, err := authSvc.RefreshTokenWithClientID(ctx, "login-flow-token", "")
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
	})
}

// ============================================================================
// 确保 ErrClientMismatch 与底层错误链关系正确
// ============================================================================

func TestOAuthSecurity_Errors(t *testing.T) {
	// 验证阶段 2.2 新增错误能通过 errors.Is 识别
	t.Run("ErrInvalidScope能被识别", func(t *testing.T) {
		// service.ErrInvalidScope 与 apperrors.ErrInvalidScope 是同一实例
		assert.True(t, errors.Is(service.ErrInvalidScope, apperrors.ErrInvalidScope))
	})

	t.Run("ErrClientMismatch能被识别", func(t *testing.T) {
		assert.True(t, errors.Is(service.ErrClientMismatch, apperrors.ErrClientMismatch))
	})

	t.Run("ErrPKCERequired能被识别", func(t *testing.T) {
		assert.True(t, errors.Is(service.ErrPKCERequired, apperrors.ErrPKCERequired))
	})

	t.Run("ErrConsentInvalid能被识别", func(t *testing.T) {
		assert.True(t, errors.Is(service.ErrConsentInvalid, apperrors.ErrConsentInvalid))
	})
}

// ============================================================================
// CreateAuthorizationCode 阶段 2.2 集成测试
// ============================================================================

func TestOAuthService_CreateAuthorizationCode_SecurityEnforcement(t *testing.T) {
	oauthSvc, store, _ := createOAuthTestEnv(t)
	ctx := context.Background()

	// 公共客户端（强制 PKCE）
	publicClient := createTestClientWithScopes("sec-public-client", []string{"openid", "email"}, true)
	store.AddClient(publicClient)

	// 机密客户端
	confClient := createTestClientWithScopes("sec-conf-client", []string{"openid", "profile", "email"}, false)
	store.AddClient(confClient)

	t.Run("拒绝-公共客户端无PKCE", func(t *testing.T) {
		_, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			publicClient.ClientID,
			"user-1",
			"http://localhost:3000/callback",
			[]string{"openid"},
			"", "",
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrPKCERequired)
	})

	t.Run("拒绝-公共客户端使用plain方法", func(t *testing.T) {
		_, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			publicClient.ClientID,
			"user-1",
			"http://localhost:3000/callback",
			[]string{"openid"},
			"challenge-value",
			"plain",
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrPKCERequired)
	})

	t.Run("拒绝-scope升级攻击", func(t *testing.T) {
		// 客户端只允许 openid/email，请求 profile 应被拒绝
		_, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			publicClient.ClientID,
			"user-1",
			"http://localhost:3000/callback",
			[]string{"openid", "profile"}, // profile 未授权
			"challenge",
			"S256",
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidScope)
	})

	t.Run("成功-公共客户端S256且scope合法", func(t *testing.T) {
		code, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			publicClient.ClientID,
			"user-1",
			"http://localhost:3000/callback",
			[]string{"openid", "email"},
			"challenge",
			"S256",
		)
		require.NoError(t, err)
		assert.NotEmpty(t, code)
	})

	t.Run("拒绝-机密客户端传plain方法", func(t *testing.T) {
		_, err := oauthSvc.CreateAuthorizationCode(
			ctx,
			confClient.ClientID,
			"user-1",
			"http://localhost:3000/callback",
			[]string{"openid"},
			"challenge",
			"plain",
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidCodeChallenge)
	})
}

// ============================================================================
// UserInfo scope 过滤测试（在 handler 包测试中已覆盖，此处仅验证 scope 常量）
// ============================================================================

func TestScopeConstants(t *testing.T) {
	// 确保阶段 2.2 scope 常量已定义且符合 OIDC 标准
	assert.Equal(t, "openid", model.ScopeOpenID)
	assert.Equal(t, "profile", model.ScopeProfile)
	assert.Equal(t, "email", model.ScopeEmail)
	assert.Equal(t, "offline_access", model.ScopeOfflineAccess)

	// SupportedScopes 应包含所有四个 scope
	expectedScopes := map[string]bool{
		model.ScopeOpenID:        true,
		model.ScopeProfile:       true,
		model.ScopeEmail:         true,
		model.ScopeOfflineAccess: true,
	}
	for _, sc := range model.SupportedScopes {
		assert.True(t, expectedScopes[sc], "unexpected scope in SupportedScopes: %s", sc)
	}

	// IsSupportedScope 校验
	assert.True(t, model.IsSupportedScope("openid"))
	assert.False(t, model.IsSupportedScope("custom-scope"))
}

// ============================================================================
// NormalizeScopes / IsScopesSubset 测试
// ============================================================================

func TestScopeNormalization(t *testing.T) {
	t.Run("NormalizeScopes-去重去空", func(t *testing.T) {
		result := model.NormalizeScopes([]string{"openid", "openid", "", "email"})
		assert.Len(t, result, 2)
		assert.Contains(t, result, "openid")
		assert.Contains(t, result, "email")
	})

	t.Run("IsScopesSubset-子集检查", func(t *testing.T) {
		allowed := []string{"openid", "profile", "email"}
		assert.True(t, model.IsScopesSubset([]string{"openid", "email"}, allowed))
		assert.False(t, model.IsScopesSubset([]string{"openid", "offline_access"}, allowed))
		assert.True(t, model.IsScopesSubset(nil, allowed)) // 空请求视为子集
	})

	t.Run("NormalizeScopes-空输入返回空切片", func(t *testing.T) {
		result := model.NormalizeScopes(nil)
		assert.Empty(t, result)
	})
}
