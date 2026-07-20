// Package service 业务逻辑层接口定义
// 定义所有服务的接口，便于测试和依赖注入
package service

import (
	"context"
	"time"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/model"
)

// ============================================================================
// 认证服务接口
// ============================================================================

// AuthServiceInterface 认证服务接口
type AuthServiceInterface interface {
	// Register 用户注册
	Register(ctx context.Context, req *model.RegisterRequest) (*model.User, error)

	// Login 用户登录
	Login(ctx context.Context, req *model.LoginRequest) (*model.LoginResponse, error)

	// LoginWithAudit 带审计上下文的登录（支持IP限流）
	LoginWithAudit(ctx context.Context, req *model.LoginRequest, auditCtx *AuditContext) (*model.LoginResponse, error)

	// VerifyMFALogin 验证 MFA 登录（两阶段登录的第二阶段）
	// 客户端提交第一阶段返回的 mfa_challenge 令牌 + 验证方法 + 验证码
	// 验证成功后签发 Token；失败递增尝试次数，超限则使 Challenge 失效
	VerifyMFALogin(ctx context.Context, req *model.MFAVerifyRequest, ipAddress, userAgent string) (*model.LoginResponse, error)

	// RefreshToken 刷新Token
	RefreshToken(ctx context.Context, refreshToken string) (*model.LoginResponse, error)

	// RefreshTokenWithClientID 携带 clientID 刷新 Token（阶段 2.2）
	// 用于 OAuth 流程签发的 token 刷新，校验 clientID 与 token 归属一致
	RefreshTokenWithClientID(ctx context.Context, refreshToken, clientID string) (*model.LoginResponse, error)

	// Logout 用户登出
	Logout(ctx context.Context, accessToken string) error

	// LogoutAll 登出所有设备
	LogoutAll(ctx context.Context, userID string) error

	// ValidateToken 验证Token
	ValidateToken(ctx context.Context, accessToken string) (*crypto.AccessTokenClaims, error)

	// GetTokenOwnerID 查询 token 所属用户 ID
	// 阶段 B 审查修复（H3）：用于 /token/revoke 端点的 token 所有权校验
	// 按顺序尝试 access_token / refresh_token 查询
	// 返回 ("", nil) 表示 token 不存在，调用方应返回 204 不暴露存在性（RFC 7009 §2.2）
	GetTokenOwnerID(ctx context.Context, token string) (string, error)
}

// ============================================================================
// OAuth服务接口
// ============================================================================

// OAuthServiceInterface OAuth服务接口
type OAuthServiceInterface interface {
	// CreateAuthorizationCode 创建授权码
	CreateAuthorizationCode(
		ctx context.Context,
		clientID, userID, redirectURI string,
		scopes []string,
		codeChallenge, codeChallengeMethod string,
	) (string, error)

	// CreateAuthorizationCodeWithConsent 基于 consent_token 创建授权码（阶段 2.2）
	// 用户在 GET /authorize 拿到 consent_token 后，POST /authorize/approve 回传，
	// 由 service 校验签名/过期/用户归属/客户端/redirect_uri/scope/PKCE 后创建授权码
	//
	// 阶段 D 审查修复（H1）：增加 expectedState 参数用于校验 consent_token 内嵌 state
	// 防止 GET /authorize 与 POST /authorize/approve 之间 state 被替换。
	// expectedState 为空时不校验（向后兼容），生产环境调用方应始终传入。
	CreateAuthorizationCodeWithConsent(
		ctx context.Context,
		userID string,
		consentToken string,
		expectedState string,
	) (string, error)

	// IssueConsentToken 签发短期 consent_token（阶段 2.2）
	// 用于 GET /authorize 与 POST /authorize/approve 之间传递授权上下文，防 CSRF
	IssueConsentToken(
		ctx context.Context,
		userID, clientID, redirectURI string,
		scopes []string,
		state, codeChallenge, codeChallengeMethod string,
	) (string, error)

	// ExchangeAuthorizationCode 交换授权码
	ExchangeAuthorizationCode(
		ctx context.Context,
		code, clientID, clientSecret, redirectURI, codeVerifier string,
	) (*model.LoginResponse, error)

	// RevokeToken 撤销Token
	RevokeToken(ctx context.Context, token string) error

	// GetAccessTokenTTL 获取访问令牌的有效期（秒）
	GetAccessTokenTTL() time.Duration
}

// ============================================================================
// 审计服务接口
// ============================================================================

// AuditServiceInterface 审计服务接口
type AuditServiceInterface interface {
	// Log 记录审计日志（异步操作，失败不阻塞主流程）
	Log(ctx context.Context, entry *model.AuditLog)

	// ListLogs 列出审计日志
	ListLogs(ctx context.Context, userID string, eventType string, offset, limit int) ([]*model.AuditLog, int, error)
}

// ============================================================================
// 多因素认证服务接口
// ============================================================================

// MFAServiceInterface 多因素认证服务接口
type MFAServiceInterface interface {
	// SetupMFA 设置MFA
	SetupMFA(ctx context.Context, userID string) (*model.MFASetupResponse, error)

	// VerifyAndEnableMFA 验证并启用MFA
	VerifyAndEnableMFA(ctx context.Context, userID, code string) error

	// DisableMFA 禁用MFA
	DisableMFA(ctx context.Context, userID, code string) error

	// GetMFAStatus 获取MFA状态
	GetMFAStatus(ctx context.Context, userID string) (*model.MFAStatusResponse, error)

	// GenerateRecoveryCodes 生成MFA恢复码
	GenerateRecoveryCodes(ctx context.Context, userID string, count int) ([]string, error)

	// VerifyRecoveryCode 验证MFA恢复码
	VerifyRecoveryCode(ctx context.Context, userID, code, ipAddress string) (bool, error)

	// GetRecoveryCodeStatus 获取恢复码状态
	GetRecoveryCodeStatus(ctx context.Context, userID string) (int, error)

	// VerifyMFALoginCode 验证 MFA 登录验证码（两阶段登录的第二阶段）
	// method: "totp" 验证 TOTP 6 位数字；"recovery_code" 验证恢复码
	// 仅验证 code 正确性，不签发 Token（Token 由 AuthService 签发）
	// 失败返回 ErrInvalidMFACode；恢复码验证还可能返回 ErrTooManyRecoveryAttempts
	VerifyMFALoginCode(ctx context.Context, userID, method, code, ipAddress string) error
}

// ============================================================================
// 用户服务接口
// ============================================================================

// UserServiceInterface 用户服务接口
type UserServiceInterface interface {
	// SendVerificationEmail 发送验证邮件
	SendVerificationEmail(ctx context.Context, userID string) error

	// VerifyEmail 验证邮箱
	VerifyEmail(ctx context.Context, userID, token string) error

	// ForgotPassword 忘记密码
	ForgotPassword(ctx context.Context, email string) error

	// ResetPassword 重置密码
	ResetPassword(ctx context.Context, userID, token, newPassword string) error

	// ChangePassword 修改密码
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error
}

// ============================================================================
// 社交登录服务接口
// ============================================================================

// SocialLoginServiceInterface 社交登录服务接口
type SocialLoginServiceInterface interface {
	// GetProviders 获取支持的提供商列表
	GetProviders() []string

	// GetAuthorizationURL 获取授权URL
	GetAuthorizationURL(provider, state string) (string, error)

	// HandleCallback 处理回调（需验证state防止CSRF攻击）
	HandleCallback(ctx context.Context, provider, code, state string) (*model.LoginResponse, error)
}
