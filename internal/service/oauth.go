// Package service OAuth服务
// 处理OAuth2授权码流程和PKCE验证
package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/your-org/sso/internal/cache"
	"github.com/your-org/sso/internal/common"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

// 重新导出统一的错误，保持向后兼容
var (
	ErrInvalidClient        = apperrors.ErrInvalidClient
	ErrInvalidRedirectURI   = apperrors.ErrInvalidRedirectURI
	ErrInvalidGrantType     = apperrors.ErrInvalidGrantType
	ErrInvalidCode          = apperrors.ErrInvalidCode
	ErrCodeExpired          = apperrors.ErrCodeExpiredErr
	ErrCodeUsed             = apperrors.ErrCodeUsedErr
	ErrInvalidCodeVerifier  = apperrors.ErrInvalidCodeVerifier
	ErrInvalidCodeChallenge = apperrors.ErrInvalidPKCEChallenge
)

// ============================================================================
// OAuthService OAuth服务
// ============================================================================

// OAuthService OAuth服务
// 处理OAuth2授权码流程
type OAuthService struct {
	store    store.Store   // 数据存储
	auditSvc *AuditService // 审计服务
	cache    cache.Cache   // 缓存服务
}

// NewOAuthService 创建OAuth服务
func NewOAuthService(store store.Store) *OAuthService {
	return &OAuthService{
		store:    store,
		auditSvc: NewAuditService(store),
	}
}

// NewOAuthServiceWithAudit 创建OAuth服务（带审计服务注入）
func NewOAuthServiceWithAudit(store store.Store, auditSvc *AuditService) *OAuthService {
	return &OAuthService{
		store:    store,
		auditSvc: auditSvc,
	}
}

// NewOAuthServiceWithCache 创建带缓存的OAuth服务
func NewOAuthServiceWithCache(store store.Store, cacheSvc cache.Cache) *OAuthService {
	return &OAuthService{
		store:    store,
		auditSvc: NewAuditService(store),
		cache:    cacheSvc,
	}
}

// getClient 获取客户端（带缓存）
func (s *OAuthService) getClient(ctx context.Context, clientID string) (*model.Client, error) {
	// 检查缓存
	if s.cache != nil {
		var cachedClient model.Client
		cacheKey := cache.ClientKey(clientID)
		if err := s.cache.Get(ctx, cacheKey, &cachedClient); err == nil {
			return &cachedClient, nil
		}
	}

	// 缓存未命中，查询数据库
	client, err := s.store.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}

	// 缓存结果
	if s.cache != nil {
		cacheKey := cache.ClientKey(clientID)
		_ = s.cache.Set(ctx, cacheKey, client, cache.ClientTTL)
	}

	return client, nil
}

// ============================================================================
// 授权码流程
// ============================================================================

// CreateAuthorizationCode 创建授权码
func (s *OAuthService) CreateAuthorizationCode(
	ctx context.Context,
	clientID string,
	userID string,
	redirectURI string,
	scopes []string,
	codeChallenge string,
	codeChallengeMethod string,
) (string, error) {
	client, err := s.getClient(ctx, clientID)
	if err != nil {
		return "", ErrInvalidClient
	}

	if !s.store.ValidateRedirectURI(ctx, clientID, redirectURI) {
		return "", ErrInvalidRedirectURI
	}

	hasAuthCodeGrant := false
	for _, grant := range client.GrantTypes {
		if grant == model.GrantTypeAuthorizationCode {
			hasAuthCodeGrant = true
			break
		}
	}
	if !hasAuthCodeGrant {
		return "", ErrInvalidGrantType
	}

	if codeChallenge != "" {
		if codeChallengeMethod != "S256" && codeChallengeMethod != "plain" {
			return "", ErrInvalidCodeChallenge
		}
	}

	code, err := common.GenerateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("生成授权码失败: %w", err)
	}

	authCode := &model.AuthorizationCode{
		Code:                code,
		ClientID:            clientID,
		UserID:              userID,
		RedirectURI:         redirectURI,
		Scopes:              scopes,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
		CreatedAt:           time.Now(),
	}

	if err := s.store.StoreAuthorizationCode(ctx, authCode); err != nil {
		return "", fmt.Errorf("存储授权码失败: %w", err)
	}

	// 记录授权码创建审计日志
	if s.auditSvc != nil {
		s.auditSvc.LogAuthCodeCreated(ctx, userID, clientID, "")
	}

	return code, nil
}

// ExchangeAuthorizationCode 交换授权码
func (s *OAuthService) ExchangeAuthorizationCode(
	ctx context.Context,
	code string,
	clientID string,
	clientSecret string,
	redirectURI string,
	codeVerifier string,
) (*model.LoginResponse, error) {
	authCode, err := s.store.GetAuthorizationCode(ctx, code)
	if err != nil {
		// 记录无效授权码审计日志
		if s.auditSvc != nil {
			s.auditSvc.LogAuthCodeInvalid(ctx, "", clientID, "", "invalid_code")
		}
		return nil, ErrInvalidCode
	}

	if authCode.ClientID != clientID {
		// 记录无效客户端审计日志
		if s.auditSvc != nil {
			s.auditSvc.LogAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "invalid_client")
		}
		return nil, ErrInvalidClient
	}

	if authCode.RedirectURI != redirectURI {
		// 记录无效重定向URI审计日志
		if s.auditSvc != nil {
			s.auditSvc.LogAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "invalid_redirect_uri")
		}
		return nil, ErrInvalidRedirectURI
	}

	if authCode.ExpiresAt.Before(time.Now()) {
		// 记录授权码过期审计日志
		if s.auditSvc != nil {
			s.auditSvc.LogAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "code_expired")
		}
		return nil, ErrCodeExpired
	}

	if authCode.UsedAt != nil {
		// 记录授权码已使用审计日志
		if s.auditSvc != nil {
			s.auditSvc.LogAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "code_used")
		}
		return nil, ErrCodeUsed
	}

	client, err := s.getClient(ctx, clientID)
	if err != nil {
		return nil, ErrInvalidClient
	}

	if !client.PublicClient {
		if !compareClientSecret(client.ClientSecret, clientSecret) {
			// 记录无效客户端密钥审计日志
			if s.auditSvc != nil {
				s.auditSvc.LogAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "invalid_client_secret")
			}
			return nil, ErrInvalidClient
		}
	}

	if authCode.CodeChallenge != "" {
		if err := verifyPKCE(authCode.CodeChallenge, authCode.CodeChallengeMethod, codeVerifier); err != nil {
			// 记录PKCE验证失败审计日志
			if s.auditSvc != nil {
				s.auditSvc.LogAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "pkce_verification_failed")
			}
			return nil, ErrInvalidCodeVerifier
		}
	}

	now := time.Now()
	authCode.UsedAt = &now
	if err := s.store.UpdateAuthorizationCode(ctx, authCode); err != nil {
		return nil, fmt.Errorf("更新授权码状态失败: %w", err)
	}

	// 记录授权码使用审计日志
	if s.auditSvc != nil {
		s.auditSvc.LogAuthCodeUsed(ctx, authCode.UserID, clientID, "")
	}

	// TODO: 实现完整的令牌生成逻辑
	// 当前返回占位符，需要实现:
	// 1. 生成 access_token 和 refresh_token
	// 2. 存储 token 记录到数据库
	// 3. 返回真实的令牌响应
	user, err := s.store.GetByID(ctx, authCode.UserID)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	_ = user // TODO: 使用用户信息生成令牌

	return &model.LoginResponse{
		AccessToken:  "access_token_placeholder",
		RefreshToken: "refresh_token_placeholder",
		TokenType:    "Bearer",
		ExpiresIn:    900,
	}, nil
}

// RevokeToken 撤销Token
func (s *OAuthService) RevokeToken(ctx context.Context, token string) error {
	err := s.store.RevokeToken(ctx, token)
	if err == nil && s.auditSvc != nil {
		// 记录Token撤销审计日志
		s.auditSvc.LogTokenRevoke(ctx, "", "", "")
	}
	return err
}

// ============================================================================
// 辅助函数
// ============================================================================

func verifyPKCE(challenge, method, verifier string) error {
	if method == "plain" {
		if challenge != verifier {
			return ErrInvalidCodeVerifier
		}
		return nil
	}

	if method == "S256" {
		h := sha256.New()
		h.Write([]byte(verifier))
		hash := h.Sum(nil)
		expected := base64.RawURLEncoding.EncodeToString(hash)

		if subtle.ConstantTimeCompare([]byte(challenge), []byte(expected)) != 1 {
			return ErrInvalidCodeVerifier
		}
		return nil
	}

	return ErrInvalidCodeChallenge
}

func compareClientSecret(stored, provided string) bool {
	return subtle.ConstantTimeCompare([]byte(stored), []byte(provided)) == 1
}
