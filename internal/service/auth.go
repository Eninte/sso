// Package service 业务逻辑层
// 处理用户认证相关的业务逻辑
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/your-org/sso/internal/cache"
	"github.com/your-org/sso/internal/crypto"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/metrics"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
	"github.com/your-org/sso/internal/validator"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

// 重新导出统一的错误，保持向后兼容
var (
	ErrInvalidCredentials = apperrors.ErrInvalidCredentials
	ErrAccountLocked      = apperrors.ErrAccountLocked
	ErrAccountDisabled    = apperrors.ErrAccountDisabled
	ErrInvalidToken       = apperrors.ErrInvalidToken
)

// ============================================================================
// AuthService 认证服务
// ============================================================================

// AuthServiceOption AuthService配置选项
type AuthServiceOption func(*AuthService)

// WithCache 设置缓存服务
func WithCache(cacheSvc cache.Cache) AuthServiceOption {
	return func(s *AuthService) {
		s.cache = cacheSvc
	}
}

// WithAudit 设置审计服务
func WithAudit(auditSvc *AuditService) AuthServiceOption {
	return func(s *AuthService) {
		s.auditSvc = auditSvc
	}
}

// WithMetrics 设置指标服务
func WithMetrics(metricsSvc *metrics.Service) AuthServiceOption {
	return func(s *AuthService) {
		s.metricsSvc = metricsSvc
	}
}

// AuthService 认证服务
// 处理用户认证相关的业务逻辑
type AuthService struct {
	store           store.Store             // 数据存储
	passwordSvc     *crypto.PasswordService // 密码服务
	jwtSvc          *crypto.JWTService      // JWT服务
	tokenSvc        *TokenService           // Token生成服务
	maxAttempts     int                     // 最大登录尝试次数
	lockoutDuration time.Duration           // 锁定时长
	metricsSvc      *metrics.Service        // 指标服务（可选）
	auditSvc        *AuditService           // 审计服务
	cache           cache.Cache             // 缓存服务（可选）
}

// NewAuthService 创建AuthService实例
// 支持通过选项函数配置可选依赖（缓存、审计服务等）
func NewAuthService(
	store store.Store,
	passwordSvc *crypto.PasswordService,
	jwtSvc *crypto.JWTService,
	maxAttempts int,
	lockoutDuration time.Duration,
	metricsSvc ...*metrics.Service,
) *AuthService {
	var m *metrics.Service
	if len(metricsSvc) > 0 {
		m = metricsSvc[0]
	}
	return &AuthService{
		store:           store,
		passwordSvc:     passwordSvc,
		jwtSvc:          jwtSvc,
		tokenSvc:        NewTokenService(jwtSvc, store),
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
		metricsSvc:      m,
		auditSvc:        NewAuditService(store),
	}
}

// NewAuthServiceWithOptions 创建带选项的AuthService实例
func NewAuthServiceWithOptions(
	store store.Store,
	passwordSvc *crypto.PasswordService,
	jwtSvc *crypto.JWTService,
	maxAttempts int,
	lockoutDuration time.Duration,
	options ...AuthServiceOption,
) *AuthService {
	svc := &AuthService{
		store:           store,
		passwordSvc:     passwordSvc,
		jwtSvc:          jwtSvc,
		tokenSvc:        NewTokenService(jwtSvc, store),
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
		auditSvc:        NewAuditService(store),
	}

	for _, opt := range options {
		opt(svc)
	}

	return svc
}

// incrementMetric 增加指标计数（安全调用）
func (s *AuthService) incrementMetric(name string) {
	if s.metricsSvc != nil {
		s.metricsSvc.Increment(name)
	}
}

// ============================================================================
// 注册功能
// ============================================================================

// Register 用户注册
// 1. 验证输入
// 2. 检查邮箱是否已注册
// 3. 哈希密码
// 4. 创建用户记录
func (s *AuthService) Register(ctx context.Context, req *model.RegisterRequest) (*model.User, error) {
	// 1. 验证输入参数
	if err := validator.ValidateRegisterRequest(req.Email, req.Password); err != nil {
		return nil, err
	}

	// 2. 检查邮箱是否已注册
	existingUser, err := s.store.GetByEmail(ctx, req.Email)
	if err != nil && !apperrors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if existingUser != nil {
		return nil, apperrors.ErrEmailExists
	}

	// 3. 哈希密码
	hashedPassword, err := s.passwordSvc.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	// 4. 创建用户
	now := time.Now()
	user := &model.User{
		ID:           uuid.New().String(),
		Email:        req.Email,
		PasswordHash: hashedPassword,
		Status:       model.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.store.Create(ctx, user); err != nil {
		return nil, err
	}

	// 记录注册成功指标
	s.incrementMetric("auth_register_total")

	return user, nil
}

// ============================================================================
// 登录功能
// ============================================================================

// Login 用户登录
// 1. 验证输入
// 2. 获取用户
// 3. 检查账户状态
// 4. 验证密码
// 5. 生成Token
type AuditContext struct {
	IPAddress string
	UserAgent string
}

func (s *AuthService) LoginWithAudit(ctx context.Context, req *model.LoginRequest, auditCtx *AuditContext) (*model.LoginResponse, error) {
	if err := validator.ValidateLoginRequest(req.Email, req.Password); err != nil {
		return nil, err
	}

	user, err := s.store.GetByEmail(ctx, req.Email)
	if err != nil {
		if apperrors.Is(err, store.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if user.Status == model.UserStatusDisabled {
		return nil, ErrAccountDisabled
	}
	if user.Status == model.UserStatusLocked {
		if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
			return nil, ErrAccountLocked
		}
		// 使用原子操作解锁过期账户，避免竞态条件
		if unlockErr := s.store.UnlockExpiredAccount(ctx, user.ID); unlockErr != nil {
			if !apperrors.Is(unlockErr, store.ErrNotFound) {
				slog.Warn("解锁过期账户失败", "error", unlockErr, "user_id", user.ID)
			}
			// 即使解锁失败也继续尝试登录（可能是并发解锁）
		}
	}

	if err := s.passwordSvc.VerifyPassword(user.PasswordHash, req.Password); err != nil {
		// 使用原子操作递增登录尝试次数，避免竞态条件
		attempts, locked, _, incErr := s.store.IncrementLoginAttempts(ctx, user.ID, s.maxAttempts, s.lockoutDuration)
		if incErr != nil {
			slog.Warn("更新登录尝试次数失败", "error", incErr, "user_id", user.ID)
		} else if locked {
			s.incrementMetric("auth_account_locked_total")
			if s.auditSvc != nil && auditCtx != nil {
				s.auditSvc.LogAccountLocked(ctx, user.ID, auditCtx.IPAddress)
			}
			slog.Warn("账户因多次登录失败被锁定", "user_id", user.ID, "attempts", attempts)
		}
		s.incrementMetric("auth_login_failed_total")
		if s.auditSvc != nil && auditCtx != nil {
			s.auditSvc.LogUserLogin(ctx, user.ID, user.Email, auditCtx.IPAddress, auditCtx.UserAgent, false)
		}
		return nil, ErrInvalidCredentials
	}

	// 登录成功，重置登录尝试次数
	if err := s.store.ResetLoginAttempts(ctx, user.ID); err != nil {
		slog.Warn("重置登录尝试次数失败", "error", err, "user_id", user.ID)
	}

	s.incrementMetric("auth_login_total")

	if s.auditSvc != nil && auditCtx != nil {
		s.auditSvc.LogUserLogin(ctx, user.ID, user.Email, auditCtx.IPAddress, auditCtx.UserAgent, true)
	}

	return s.generateTokenPair(ctx, user.ID, user.Email, user.Role, []string{"openid", "profile", "email"}, "")
}

func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest) (*model.LoginResponse, error) {
	return s.LoginWithAudit(ctx, req, nil)
}

// ============================================================================
// Token刷新功能
// ============================================================================

// maxRevokeRetries Token撤销最大重试次数
const (
	maxRevokeRetries     = 3
	revokeRetryBaseDelay = 100 * time.Millisecond
)

// revokeTokenWithRetry 带重试的Token撤销
func (s *AuthService) revokeTokenWithRetry(ctx context.Context, accessToken string) error {
	var lastErr error
	for i := 0; i < maxRevokeRetries; i++ {
		if err := s.store.RevokeToken(ctx, accessToken); err != nil {
			lastErr = err
			slog.Warn("Token撤销失败，准备重试",
				"error", err,
				"attempt", i+1,
				"max_retries", maxRevokeRetries,
			)
			time.Sleep(time.Duration(i+1) * revokeRetryBaseDelay)
			continue
		}

		// 清除缓存（失败不影响主流程）
		if s.cache != nil {
			cacheKey := cache.TokenKey(accessToken)
			if err := s.cache.Delete(ctx, cacheKey); err != nil {
				slog.Warn("清除Token缓存失败", "error", err, "token", accessToken)
			}
		}

		return nil
	}
	return fmt.Errorf("token撤销失败，已重试%d次: %w", maxRevokeRetries, lastErr)
}

// RefreshToken 刷新Token
func (s *AuthService) RefreshTokenWithAudit(ctx context.Context, refreshToken string, auditCtx *AuditContext) (*model.LoginResponse, error) {
	tokenRecord, err := s.store.GetTokenByRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	if tokenRecord.RevokedAt != nil {
		return nil, ErrInvalidToken
	}

	user, err := s.store.GetByID(ctx, tokenRecord.UserID)
	if err != nil {
		return nil, err
	}

	if revokeErr := s.revokeTokenWithRetry(ctx, tokenRecord.AccessToken); revokeErr != nil {
		slog.Error("撤销旧Token失败，已达到最大重试次数",
			"error", revokeErr,
			"user_id", tokenRecord.UserID,
			"token_id", tokenRecord.ID,
		)
	}

	s.incrementMetric("auth_token_refresh_total")

	if s.auditSvc != nil && auditCtx != nil {
		s.auditSvc.LogTokenRefresh(ctx, user.ID, tokenRecord.ClientID, auditCtx.IPAddress)
	}

	return s.generateTokenPair(ctx, user.ID, user.Email, user.Role, tokenRecord.Scopes, tokenRecord.ClientID)
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*model.LoginResponse, error) {
	return s.RefreshTokenWithAudit(ctx, refreshToken, nil)
}

// ============================================================================
// 登出功能
// ============================================================================

// LogoutWithAudit 用户登出（带审计日志）
func (s *AuthService) LogoutWithAudit(ctx context.Context, accessToken string, auditCtx *AuditContext) error {
	claims, err := s.jwtSvc.ValidateAccessToken(accessToken)
	if err == nil && s.auditSvc != nil && auditCtx != nil {
		s.auditSvc.LogUserLogout(ctx, claims.Subject, auditCtx.IPAddress)
	}
	if err := s.revokeTokenWithRetry(ctx, accessToken); err != nil {
		slog.Error("登出时撤销Token失败",
			"error", err,
			"token_prefix", maskToken(accessToken),
		)
		return fmt.Errorf("登出失败: %w", err)
	}
	s.incrementMetric("auth_token_revoke_total")
	return nil
}

// Logout 用户登出
func (s *AuthService) Logout(ctx context.Context, accessToken string) error {
	return s.LogoutWithAudit(ctx, accessToken, nil)
}

// LogoutAllWithAudit 登出所有设备（带审计日志）
func (s *AuthService) LogoutAllWithAudit(ctx context.Context, userID string, auditCtx *AuditContext) error {
	if err := s.store.RevokeAllUserTokens(ctx, userID); err != nil {
		slog.Error("撤销所有Token失败",
			"error", err,
			"user_id", userID,
		)
		return fmt.Errorf("登出所有设备失败: %w", err)
	}

	// 清除该用户相关的缓存（失败不影响主流程）
	if s.cache != nil {
		if err := s.cache.DeletePattern(ctx, cache.TokenCachePrefix+"*"); err != nil {
			slog.Warn("清除用户Token缓存失败", "error", err, "user_id", userID)
		}
	}

	s.incrementMetric("auth_logout_all_total")
	if s.auditSvc != nil && auditCtx != nil {
		s.auditSvc.LogLogoutAll(ctx, userID, auditCtx.IPAddress)
	}
	return nil
}

// LogoutAll 登出所有设备
func (s *AuthService) LogoutAll(ctx context.Context, userID string) error {
	return s.LogoutAllWithAudit(ctx, userID, nil)
}

// maskToken 掩盖Token用于日志记录（只显示前8位）
func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:8] + "..."
}

// ============================================================================
// Token验证功能
// ============================================================================

// ValidateToken 验证Token
func (s *AuthService) ValidateToken(ctx context.Context, accessToken string) (*crypto.AccessTokenClaims, error) {
	claims, err := s.jwtSvc.ValidateAccessToken(accessToken)
	if err != nil {
		return nil, err
	}

	// 检查缓存
	if s.cache != nil {
		var cachedToken model.Token
		cacheKey := cache.TokenKey(accessToken)
		if err := s.cache.Get(ctx, cacheKey, &cachedToken); err == nil {
			if cachedToken.RevokedAt != nil {
				return nil, ErrInvalidToken
			}
			return claims, nil
		}
	}

	// 缓存未命中，查询数据库
	tokenRecord, err := s.store.GetTokenByAccessToken(ctx, accessToken)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if tokenRecord.RevokedAt != nil {
		return nil, ErrInvalidToken
	}

	// 缓存结果（失败不影响主流程）
	if s.cache != nil {
		cacheKey := cache.TokenKey(accessToken)
		if err := s.cache.Set(ctx, cacheKey, tokenRecord, cache.TokenTTL); err != nil {
			slog.Warn("缓存Token记录失败", "error", err)
		}
	}

	return claims, nil
}

// ============================================================================
// 内部辅助方法
// ============================================================================

// generateTokenPair 生成Token对
// 使用TokenService统一处理Token生成逻辑
func (s *AuthService) generateTokenPair(
	ctx context.Context,
	userID, email, role string,
	scopes []string,
	clientID string,
) (*model.LoginResponse, error) {
	return s.tokenSvc.GenerateTokenPair(ctx, userID, email, role, scopes, clientID)
}
