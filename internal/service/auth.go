// Package service 业务逻辑层
// 处理用户认证相关的业务逻辑
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

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

// AuthService 认证服务
// 处理用户认证相关的业务逻辑
type AuthService struct {
	store           store.Store             // 数据存储
	passwordSvc     *crypto.PasswordService // 密码服务
	jwtSvc          *crypto.JWTService      // JWT服务
	maxAttempts     int                     // 最大登录尝试次数
	lockoutDuration time.Duration           // 锁定时长
	metricsSvc      *metrics.MetricsService // 指标服务（可选）
}

// NewAuthService 创建认证服务
func NewAuthService(
	store store.Store,
	passwordSvc *crypto.PasswordService,
	jwtSvc *crypto.JWTService,
	maxAttempts int,
	lockoutDuration time.Duration,
	metricsSvc ...*metrics.MetricsService,
) *AuthService {
	var m *metrics.MetricsService
	if len(metricsSvc) > 0 {
		m = metricsSvc[0]
	}
	return &AuthService{
		store:           store,
		passwordSvc:     passwordSvc,
		jwtSvc:          jwtSvc,
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
		metricsSvc:      m,
	}
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
func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest) (*model.LoginResponse, error) {
	// 1. 验证输入参数
	if err := validator.ValidateLoginRequest(req.Email, req.Password); err != nil {
		return nil, err
	}

	// 2. 获取用户
	user, err := s.store.GetByEmail(ctx, req.Email)
	if err != nil {
		if apperrors.Is(err, store.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// 3. 检查账户状态
	if user.Status == model.UserStatusDisabled {
		return nil, ErrAccountDisabled
	}
	if user.Status == model.UserStatusLocked {
		// 检查锁定是否已过期
		if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
			return nil, ErrAccountLocked
		}
		// 锁定已过期，解锁账户
		user.Status = model.UserStatusActive
		user.LoginAttempts = 0
	}

	// 4. 验证密码
	if err := s.passwordSvc.VerifyPassword(user.PasswordHash, req.Password); err != nil {
		// 增加登录失败次数
		user.LoginAttempts++
		var lockedUntil *time.Time
		if user.LoginAttempts >= s.maxAttempts {
			t := time.Now().Add(s.lockoutDuration)
			lockedUntil = &t
			user.Status = model.UserStatusLocked
			// 记录账户锁定指标
			s.incrementMetric("auth_account_locked_total")
		}
		if updateErr := s.store.UpdateLoginAttempts(ctx, user.ID, user.LoginAttempts, lockedUntil); updateErr != nil {
			slog.Warn("更新登录尝试次数失败", "error", updateErr, "user_id", user.ID)
		}
		// 记录登录失败指标
		s.incrementMetric("auth_login_failed_total")
		return nil, ErrInvalidCredentials
	}

	// 5. 登录成功，重置失败次数
	user.LoginAttempts = 0
	user.UpdatedAt = time.Now()
	if err := s.store.Update(ctx, user); err != nil {
		slog.Warn("更新用户登录状态失败", "error", err, "user_id", user.ID)
	}

	// 记录登录成功指标
	s.incrementMetric("auth_login_total")

	// 6. 生成Token
	return s.generateTokenPair(ctx, user.ID, user.Email, []string{"openid", "profile", "email"}, "")
}

// ============================================================================
// Token刷新功能
// ============================================================================

// maxRevokeRetries Token撤销最大重试次数
const maxRevokeRetries = 3

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
			// 等待一段时间后重试
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
			continue
		}
		return nil
	}
	return fmt.Errorf("Token撤销失败，已重试%d次: %w", maxRevokeRetries, lastErr)
}

// RefreshToken 刷新Token
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*model.LoginResponse, error) {
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

	// 使用带重试的Token撤销
	if revokeErr := s.revokeTokenWithRetry(ctx, tokenRecord.AccessToken); revokeErr != nil {
		slog.Error("撤销旧Token失败，已达到最大重试次数",
			"error", revokeErr,
			"user_id", tokenRecord.UserID,
			"token_id", tokenRecord.ID,
		)
		// 记录错误但不阻止新Token生成，因为旧Token可能已经被撤销
		// 在生产环境中，这里应该触发告警
	}

	s.incrementMetric("auth_token_refresh_total")
	return s.generateTokenPair(ctx, user.ID, user.Email, tokenRecord.Scopes, tokenRecord.ClientID)
}

// ============================================================================
// 登出功能
// ============================================================================

// Logout 用户登出
func (s *AuthService) Logout(ctx context.Context, accessToken string) error {
	s.incrementMetric("auth_token_revoke_total")
	return s.store.RevokeToken(ctx, accessToken)
}

// LogoutAll 登出所有设备
func (s *AuthService) LogoutAll(ctx context.Context, userID string) error {
	return s.store.RevokeAllUserTokens(ctx, userID)
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

	tokenRecord, err := s.store.GetTokenByAccessToken(ctx, accessToken)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if tokenRecord.RevokedAt != nil {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ============================================================================
// 内部辅助方法
// ============================================================================

// generateTokenPair 生成Token对
func (s *AuthService) generateTokenPair(
	ctx context.Context,
	userID, email string,
	scopes []string,
	clientID string,
) (*model.LoginResponse, error) {
	accessToken, err := s.jwtSvc.GenerateAccessToken(userID, email, scopes)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.jwtSvc.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	tokenRecord := &model.Token{
		ID:           uuid.New().String(),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		UserID:       userID,
		ClientID:     clientID,
		Scopes:       scopes,
		ExpiresAt:    time.Now().Add(s.jwtSvc.GetAccessTokenTTL()),
		CreatedAt:    time.Now(),
	}
	if err := s.store.StoreToken(ctx, tokenRecord); err != nil {
		return nil, err
	}

	return &model.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.jwtSvc.GetAccessTokenTTL().Seconds()),
	}, nil
}
