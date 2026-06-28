// Package service OAuth服务
// 处理OAuth2授权码流程和PKCE验证
package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/common"
	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/serviceutil"
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
	store       store.Store             // 数据存储
	auditSvc    *AuditService           // 审计服务
	cache       cache.Cache             // 缓存服务
	tokenSvc    *TokenService           // Token生成服务
	passwordSvc *crypto.PasswordService // 密码服务（用于验证机密客户端密钥）
}

// OAuthServiceOption OAuthService 配置选项
type OAuthServiceOption func(*OAuthService)

// WithOAuthAudit 设置审计服务
func WithOAuthAudit(auditSvc *AuditService) OAuthServiceOption {
	return func(s *OAuthService) { s.auditSvc = auditSvc }
}

// WithOAuthCache 设置缓存
func WithOAuthCache(cacheSvc cache.Cache) OAuthServiceOption {
	return func(s *OAuthService) { s.cache = cacheSvc }
}

// WithOAuthPassword 设置密码服务（用于验证机密客户端密钥）
func WithOAuthPassword(passwordSvc *crypto.PasswordService) OAuthServiceOption {
	return func(s *OAuthService) { s.passwordSvc = passwordSvc }
}

// NewOAuthServiceWithOptions 创建带选项的OAuth服务
func NewOAuthServiceWithOptions(store store.Store, tokenSvc *TokenService, options ...OAuthServiceOption) *OAuthService {
	svc := &OAuthService{
		store:    store,
		auditSvc: NewAuditService(store),
		tokenSvc: tokenSvc,
	}
	for _, opt := range options {
		opt(svc)
	}
	return svc
}

// NewOAuthService 创建OAuth服务（兼容旧调用）
func NewOAuthService(store store.Store, tokenSvc *TokenService, options ...OAuthServiceOption) *OAuthService {
	return NewOAuthServiceWithOptions(store, tokenSvc, options...)
}

// NewOAuthServiceWithAudit 创建OAuth服务（带审计服务注入，兼容旧调用）
func NewOAuthServiceWithAudit(store store.Store, auditSvc *AuditService, tokenSvc *TokenService) *OAuthService {
	return NewOAuthServiceWithOptions(store, tokenSvc, WithOAuthAudit(auditSvc))
}

// NewOAuthServiceWithCache 创建带缓存的OAuth服务（兼容旧调用）
func NewOAuthServiceWithCache(store store.Store, cacheSvc cache.Cache, tokenSvc *TokenService) *OAuthService {
	return NewOAuthServiceWithOptions(store, tokenSvc, WithOAuthCache(cacheSvc))
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
		return nil, serviceutil.HandleStoreError(err, ErrInvalidClient)
	}

	// 缓存结果（失败不影响主流程）
	if s.cache != nil {
		cacheKey := cache.ClientKey(clientID)
		if err := s.cache.Set(ctx, cacheKey, client, cache.ClientTTL); err != nil {
			slog.Warn("缓存客户端信息失败", "error", err, "client_id", clientID)
		}
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
		return "", serviceutil.WrapServiceError("生成授权码", err)
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
		return "", serviceutil.WrapServiceError("存储授权码", err)
	}

	// 使用统一的审计日志工具记录授权码创建事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventAuthCodeCreated), userID, map[string]interface{}{
		"client_id": clientID,
	})

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
	// 验证授权码
	authCode, err := s.validateAuthorizationCode(ctx, code, clientID, redirectURI)
	if err != nil {
		return nil, err
	}

	// 验证客户端密钥
	if err := s.validateClientSecret(ctx, authCode, clientID, clientSecret); err != nil {
		return nil, err
	}

	// 验证PKCE
	if err := s.validatePKCE(ctx, authCode, clientID, codeVerifier); err != nil {
		return nil, err
	}

	// 标记授权码为已使用
	if err := s.markAuthorizationCodeUsed(ctx, authCode, clientID); err != nil {
		return nil, err
	}

	// 生成令牌响应
	return s.generateTokenResponse(ctx, authCode)
}

// validateAuthorizationCode 验证授权码有效性
func (s *OAuthService) validateAuthorizationCode(
	ctx context.Context,
	code string,
	clientID string,
	redirectURI string,
) (*model.AuthorizationCode, error) {
	authCode, err := s.store.GetAuthorizationCode(ctx, code)
	if err != nil {
		s.logAuthCodeInvalid(ctx, "", clientID, "", "invalid_code")
		return nil, serviceutil.HandleStoreError(err, ErrInvalidCode)
	}

	if authCode.ClientID != clientID {
		s.logAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "invalid_client")
		return nil, ErrInvalidClient
	}

	if authCode.RedirectURI != redirectURI {
		s.logAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "invalid_redirect_uri")
		return nil, ErrInvalidRedirectURI
	}

	if authCode.ExpiresAt.Before(time.Now()) {
		s.logAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "code_expired")
		return nil, ErrCodeExpired
	}

	if authCode.UsedAt != nil {
		s.logAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "code_used")
		return nil, ErrCodeUsed
	}

	return authCode, nil
}

// validateClientSecret 验证客户端密钥
func (s *OAuthService) validateClientSecret(
	ctx context.Context,
	authCode *model.AuthorizationCode,
	clientID string,
	clientSecret string,
) error {
	client, err := s.getClient(ctx, clientID)
	if err != nil {
		return ErrInvalidClient
	}

	if !client.PublicClient {
		if s.passwordSvc == nil {
			s.logAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "password_service_not_initialized")
			return ErrInvalidClient
		}
		if err := s.passwordSvc.VerifyPassword(client.ClientSecret, clientSecret); err != nil {
			s.logAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "invalid_client_secret")
			return ErrInvalidClient
		}
	}

	return nil
}

// validatePKCE 验证PKCE码验证器
func (s *OAuthService) validatePKCE(
	ctx context.Context,
	authCode *model.AuthorizationCode,
	clientID string,
	codeVerifier string,
) error {
	if authCode.CodeChallenge != "" {
		if err := verifyPKCE(authCode.CodeChallenge, authCode.CodeChallengeMethod, codeVerifier); err != nil {
			s.logAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "pkce_verification_failed")
			return ErrInvalidCodeVerifier
		}
	}
	return nil
}

// markAuthorizationCodeUsed 标记授权码为已使用
func (s *OAuthService) markAuthorizationCodeUsed(
	ctx context.Context,
	authCode *model.AuthorizationCode,
	clientID string,
) error {
	now := time.Now()
	authCode.UsedAt = &now
	if err := s.store.UpdateAuthorizationCode(ctx, authCode); err != nil {
		return serviceutil.WrapServiceError("更新授权码状态", err)
	}

	// 使用统一的审计日志工具记录授权码使用事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventAuthCodeUsed), authCode.UserID, map[string]interface{}{
		"client_id": clientID,
	})

	return nil
}

// generateTokenResponse 生成令牌响应
func (s *OAuthService) generateTokenResponse(
	ctx context.Context,
	authCode *model.AuthorizationCode,
) (*model.LoginResponse, error) {
	user, err := s.store.GetByID(ctx, authCode.UserID)
	if err != nil {
		return nil, serviceutil.WrapServiceError("获取用户信息", err)
	}

	if s.tokenSvc == nil {
		return nil, fmt.Errorf("token服务未初始化")
	}

	clientID := &authCode.ClientID
	return s.tokenSvc.GenerateTokenPair(
		ctx,
		user.ID,
		user.Email,
		user.Role,
		authCode.Scopes,
		clientID,
	)
}

// logAuthCodeInvalid 记录无效授权码审计日志
func (s *OAuthService) logAuthCodeInvalid(ctx context.Context, userID, clientID, ipAddress, reason string) {
	// 使用统一的审计日志工具记录无效授权码事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventAuthCodeInvalid), userID, map[string]interface{}{
		"client_id":  clientID,
		"ip_address": ipAddress,
		"reason":     reason,
	})
}

// RevokeToken 撤销Token
func (s *OAuthService) RevokeToken(ctx context.Context, token string) error {
	err := s.store.RevokeToken(ctx, token)
	if err != nil {
		return serviceutil.WrapServiceError("撤销Token", err)
	}

	// 使用统一的审计日志工具记录Token撤销事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventTokenRevoke), "", map[string]interface{}{})
	return nil
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

// GetAccessTokenTTL 获取访问令牌的有效期
func (s *OAuthService) GetAccessTokenTTL() time.Duration {
	if s.tokenSvc != nil && s.tokenSvc.jwtSvc != nil {
		return s.tokenSvc.jwtSvc.GetAccessTokenTTL()
	}
	// 默认值：15分钟
	return 15 * time.Minute
}
