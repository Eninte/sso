// Package service OAuth服务
// 处理OAuth2授权码流程和PKCE验证
package service

import (
	"context"
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
//
// 阶段 2.2 安全增强：
//   - 强制 scope 校验（防升级）：请求 scope 必须是 client.Scopes 子集且在白名单内
//   - 强制 PKCE 校验：公共客户端必须使用 PKCE 且 method=S256，禁用 plain
//   - 校验 grant_type：客户端必须注册了 authorization_code
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

	// 阶段 2.2: 强制 scope 校验（防升级）
	validScopes, err := s.ValidateScope(ctx, client, scopes)
	if err != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventAuthCodeInvalid), userID, map[string]interface{}{
			"client_id": clientID,
			"reason":     "invalid_scope",
		})
		return "", err
	}

	// 阶段 2.2: 强制 PKCE 校验
	if err := s.ValidatePKCEChallenge(ctx, client, codeChallenge, codeChallengeMethod); err != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventAuthCodeInvalid), userID, map[string]interface{}{
			"client_id": clientID,
			"reason":     "pkce_required",
		})
		return "", err
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
		Scopes:              validScopes,
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

// CreateAuthorizationCodeWithConsent 基于 consent_token 创建授权码（阶段 2.2）
//
// 流程：
//  1. 校验 consent_token 的签名、过期、issuer
//  2. 校验 consent_token 中的 user_id 必须等于当前登录用户
//  3. 获取客户端，校验 redirect_uri 与 grant_type
//  4. 深度防御：再次校验 scope（防篡改）与 PKCE（防绕过）
//  5. 生成授权码并存储
//
// 安全设计：
//   - consent_token 由 GET /authorize 签发，POST /authorize/approve 回传
//   - 中间用户/客户端无法篡改 token 内容（RS256 签名）
//   - state 在 consent_token 中传递，防止 GET 与 POST 之间被替换
func (s *OAuthService) CreateAuthorizationCodeWithConsent(
	ctx context.Context,
	userID string,
	consentToken string,
) (string, error) {
	// 1. 校验 consent_token
	claims, err := s.VerifyConsentToken(ctx, consentToken)
	if err != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventAuthCodeInvalid), userID, map[string]interface{}{
			"reason": "consent_token_invalid",
		})
		return "", err
	}

	// 2. 校验 consent_token 中的 user_id 必须等于当前登录用户
	if claims.UserID != userID {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventAuthCodeInvalid), userID, map[string]interface{}{
			"client_id": claims.ClientID,
			"reason":    "consent_user_mismatch",
		})
		return "", ErrConsentInvalid
	}

	// 3. 获取客户端
	client, err := s.getClient(ctx, claims.ClientID)
	if err != nil {
		return "", ErrInvalidClient
	}

	// 4. 校验 redirect_uri
	if !s.store.ValidateRedirectURI(ctx, claims.ClientID, claims.RedirectURI) {
		return "", ErrInvalidRedirectURI
	}

	// 5. 校验 grant_type
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

	// 6. 深度防御：再次校验 scope（防篡改）
	validScopes, err := s.ValidateScope(ctx, client, claims.Scopes)
	if err != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventAuthCodeInvalid), userID, map[string]interface{}{
			"client_id": claims.ClientID,
			"reason":    "invalid_scope_in_consent",
		})
		return "", err
	}

	// 7. 深度防御：再次校验 PKCE
	if err := s.ValidatePKCEChallenge(ctx, client, claims.CodeChallenge, claims.CodeChallengeMethod); err != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventAuthCodeInvalid), userID, map[string]interface{}{
			"client_id": claims.ClientID,
			"reason":    "pkce_invalid_in_consent",
		})
		return "", err
	}

	// 8. 生成授权码
	code, err := common.GenerateRandomString(32)
	if err != nil {
		return "", serviceutil.WrapServiceError("生成授权码", err)
	}

	authCode := &model.AuthorizationCode{
		Code:                code,
		ClientID:            claims.ClientID,
		UserID:              userID,
		RedirectURI:         claims.RedirectURI,
		Scopes:              validScopes,
		CodeChallenge:       claims.CodeChallenge,
		CodeChallengeMethod: claims.CodeChallengeMethod,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
		CreatedAt:           time.Now(),
	}

	if err := s.store.StoreAuthorizationCode(ctx, authCode); err != nil {
		return "", serviceutil.WrapServiceError("存储授权码", err)
	}

	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventAuthCodeCreated), userID, map[string]interface{}{
		"client_id": claims.ClientID,
		"consent":   true,
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

// validatePKCE 验证PKCE码验证器（阶段 2.2 安全增强：强制 S256，禁用 plain）
func (s *OAuthService) validatePKCE(
	ctx context.Context,
	authCode *model.AuthorizationCode,
	clientID string,
	codeVerifier string,
) error {
	if authCode.CodeChallenge != "" {
		// 使用 VerifyPKCEWithMethod 强制 S256，禁用 plain 方法
		if err := VerifyPKCEWithMethod(authCode.CodeChallenge, authCode.CodeChallengeMethod, codeVerifier); err != nil {
			s.logAuthCodeInvalid(ctx, authCode.UserID, clientID, "", "pkce_verification_failed")
			// 直接返回错误以区分 ErrInvalidCodeChallenge（method 非法）与 ErrInvalidCodeVerifier（verifier 不匹配）
			return err
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
		return nil, fmt.Errorf("token service is not initialized")
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
//
// 阶段 2.4：统一撤销路径
//   - 调用 store.RevokeToken 撤销
//   - 同步清 token 缓存，确保撤销立即生效（与 AuthService.LogoutWithAudit 行为一致）
//   - 记录审计日志
//   - 不带重试（OAuth 撤销失败由调用方决定是否重试，避免掩盖 DB 异常）
func (s *OAuthService) RevokeToken(ctx context.Context, token string) error {
	if err := s.store.RevokeToken(ctx, token); err != nil {
		return serviceutil.WrapServiceError("撤销Token", err)
	}

	// 阶段 2.4：同步清 token 缓存
	if s.cache != nil {
		cacheKey := cache.TokenKey(token)
		if err := s.cache.Delete(ctx, cacheKey); err != nil {
			slog.Warn("撤销Token时清除缓存失败", "error", err)
		}
	}

	// 使用统一的审计日志工具记录Token撤销事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventTokenRevoke), "", map[string]interface{}{})
	return nil
}

// ============================================================================
// 辅助函数
// ============================================================================

// 注：旧的 verifyPKCE 函数已被 VerifyPKCEWithMethod 替代
// VerifyPKCEWithMethod 在 oauth_security.go 中实现，强制 S256，禁用 plain 方法

// GetAccessTokenTTL 获取访问令牌的有效期
func (s *OAuthService) GetAccessTokenTTL() time.Duration {
	if s.tokenSvc != nil && s.tokenSvc.jwtSvc != nil {
		return s.tokenSvc.jwtSvc.GetAccessTokenTTL()
	}
	// 默认值：15分钟
	return 15 * time.Minute
}
