// Package service 业务逻辑层接口定义
// 定义所有服务的接口，便于测试和依赖注入
package service

import (
	"context"
	"time"

	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/model"
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

	// RefreshToken 刷新Token
	RefreshToken(ctx context.Context, refreshToken string) (*model.LoginResponse, error)

	// Logout 用户登出
	Logout(ctx context.Context, accessToken string) error

	// LogoutAll 登出所有设备
	LogoutAll(ctx context.Context, userID string) error

	// ValidateToken 验证Token
	ValidateToken(ctx context.Context, accessToken string) (*crypto.AccessTokenClaims, error)
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
// 邮件服务接口
// ============================================================================

// EmailServiceInterface 邮件服务接口
type EmailServiceInterface interface {
	// SendVerificationEmail 发送验证邮件
	SendVerificationEmail(to, token, baseURL string) error

	// SendPasswordResetEmail 发送密码重置邮件
	SendPasswordResetEmail(to, token, baseURL string) error
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
	GetAuthorizationURL(provider, redirectURI, state string) (string, error)

	// HandleCallback 处理回调（需验证state防止CSRF攻击）
	HandleCallback(ctx context.Context, provider, code, state, redirectURI string) (*model.LoginResponse, error)
}
