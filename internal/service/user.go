// Package service 用户服务
// 处理用户资料管理、邮箱验证、密码重置等业务逻辑
package service

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/sso/internal/common"
	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/logging"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/serviceutil"
	"github.com/example/sso/internal/validator"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

var (
	ErrVerificationCodeInvalid = apperrors.ErrVerificationCodeInvalid
	ErrVerificationCodeExpired = apperrors.ErrVerificationCodeExpired
	ErrResetTokenInvalid       = apperrors.ErrResetTokenInvalid
	ErrResetTokenExpired       = apperrors.ErrResetTokenExpired
	ErrResetTokenUsed          = apperrors.ErrResetTokenUsed
	ErrEmailAlreadyVerified    = apperrors.ErrEmailAlreadyVerified
)

// ============================================================================
// 配置常量
// ============================================================================

const (
	VerificationTokenTTL = 15 * time.Minute // 验证令牌有效期（15分钟）
	ResetTokenTTL        = 1 * time.Hour    // 重置令牌有效期（1小时）
)

// ============================================================================
// UserService 用户服务
// ============================================================================

type UserService struct {
	store          store.Store
	passwordSvc    *crypto.PasswordService
	emailSvc       *EmailService
	baseURL        string
	auditSvc       *AuditService
	emailRateLimit *EmailRateLimiter
}

func NewUserService(
	store store.Store,
	passwordSvc *crypto.PasswordService,
	emailSvc *EmailService,
	baseURL string,
) *UserService {
	return &UserService{
		store:          store,
		passwordSvc:    passwordSvc,
		emailSvc:       emailSvc,
		baseURL:        baseURL,
		auditSvc:       NewAuditService(store),
		emailRateLimit: nil, // 默认不启用限流（向后兼容）
	}
}

// WithEmailRateLimit 设置邮件限流器
func (s *UserService) WithEmailRateLimit(rateLimiter *EmailRateLimiter) *UserService {
	s.emailRateLimit = rateLimiter
	return s
}

func NewUserServiceWithAudit(
	store store.Store,
	passwordSvc *crypto.PasswordService,
	emailSvc *EmailService,
	baseURL string,
	auditSvc *AuditService,
) *UserService {
	return &UserService{
		store:          store,
		passwordSvc:    passwordSvc,
		emailSvc:       emailSvc,
		baseURL:        baseURL,
		auditSvc:       auditSvc,
		emailRateLimit: nil, // 默认不启用限流（向后兼容）
	}
}

// ============================================================================
// 邮箱验证
// ============================================================================

func (s *UserService) SendVerificationEmail(ctx context.Context, userID string) error {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return serviceutil.WrapServiceError("查询用户", err)
	}

	if user.EmailVerified {
		return ErrEmailAlreadyVerified
	}

	// 检查邮件发送限流
	if s.emailRateLimit != nil {
		allowed, remaining, err := s.emailRateLimit.CheckLimit(ctx, user.Email)
		if err != nil {
			slog.Warn("检查邮件限流失败", "error", err, "email", user.Email)
		}
		if !allowed {
			ttl, _ := s.emailRateLimit.GetTTL(ctx, user.Email)
			return apperrors.Wrap(
				apperrors.ErrCodeEmailRateLimitExceeded,
				FormatRateLimitError(ttl),
				429,
				apperrors.ErrEmailRateLimitExceeded,
			)
		}
		slog.Debug("邮件限流检查通过", "email", user.Email, "remaining", remaining)
	}

	token, err := common.GenerateToken()
	if err != nil {
		return serviceutil.WrapServiceError("生成验证令牌", err)
	}

	expiresAt := time.Now().Add(VerificationTokenTTL)
	if err := s.store.StoreVerificationToken(ctx, userID, token, expiresAt); err != nil {
		return serviceutil.WrapServiceError("存储验证令牌", err)
	}

	verifyLink := fmt.Sprintf("%s/verify-email?token=%s&user_id=%s", s.baseURL, token, userID)

	if s.emailSvc != nil {
		if err := s.emailSvc.SendVerificationEmail(ctx, user.Email, user.Email, verifyLink); err != nil {
			return serviceutil.WrapServiceError("发送验证邮件", err)
		}
	}

	return nil
}

func (s *UserService) VerifyEmail(ctx context.Context, userID, token string) error {
	storedToken, err := s.store.GetVerificationToken(ctx, userID)
	if err != nil {
		return serviceutil.HandleStoreError(err, ErrVerificationCodeInvalid)
	}

	if subtle.ConstantTimeCompare([]byte(storedToken.Token), []byte(token)) != 1 {
		return ErrVerificationCodeInvalid
	}

	if storedToken.ExpiresAt.Before(time.Now()) {
		return ErrVerificationCodeExpired
	}

	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return serviceutil.WrapServiceError("查询用户", err)
	}

	user.EmailVerified = true
	user.Status = model.UserStatusActive
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return serviceutil.WrapServiceError("更新用户", err)
	}

	// 清理验证令牌（失败不影响主流程）
	if err := s.store.DeleteVerificationToken(ctx, userID); err != nil {
		slog.Warn("清理验证令牌失败", "error", err, "user_id", userID)
	}

	return nil
}

// ============================================================================
// 密码重置
// ============================================================================

func (s *UserService) ForgotPassword(ctx context.Context, email string) error {
	logger := logging.WithContext(ctx)
	user, err := s.store.GetByEmail(ctx, email)
	if err != nil {
		// 安全设计：不泄露用户是否存在，但记录错误日志以便排查
		logger.Debug("ForgotPassword: 获取用户失败", "error", err, "email", email)
		return nil
	}

	// 检查邮件发送限流
	if s.emailRateLimit != nil {
		allowed, remaining, err := s.emailRateLimit.CheckLimit(ctx, email)
		if err != nil {
			logger.Warn("检查邮件限流失败", "error", err, "email", email)
		}
		if !allowed {
			ttl, _ := s.emailRateLimit.GetTTL(ctx, email)
			// 为了安全，不暴露限流错误，但记录日志
			logger.Warn("密码重置邮件发送受限", "email", email, "ttl_minutes", int(ttl.Minutes()))
			return apperrors.Wrap(
				apperrors.ErrCodeEmailRateLimitExceeded,
				FormatRateLimitError(ttl),
				429,
				apperrors.ErrEmailRateLimitExceeded,
			)
		}
		logger.Debug("邮件限流检查通过", "email", email, "remaining", remaining)
	}

	token, err := common.GenerateToken()
	if err != nil {
		logger.Error("ForgotPassword: 生成令牌失败", "error", err, "user_id", user.ID)
		return nil
	}

	expiresAt := time.Now().Add(ResetTokenTTL)
	if err := s.store.StoreResetToken(ctx, user.ID, token, expiresAt); err != nil {
		logger.Error("ForgotPassword: 存储重置令牌失败", "error", err, "user_id", user.ID)
		return nil
	}

	resetLink := fmt.Sprintf("%s/reset-password?token=%s&user_id=%s", s.baseURL, token, user.ID)

	if s.emailSvc != nil {
		if err := s.emailSvc.SendPasswordResetEmail(ctx, user.Email, user.Email, resetLink); err != nil {
			logger.Error("ForgotPassword: 发送重置邮件失败", "error", err, "user_id", user.ID)
			// 仍然返回 nil，不泄露内部错误
		}
	}

	return nil
}

func (s *UserService) ResetPasswordWithAudit(ctx context.Context, userID, token, newPassword string, ipAddress string) error {
	logger := logging.WithContext(ctx)
	// 验证密码强度
	if err := validator.ValidatePassword(newPassword); err != nil {
		return err
	}

	storedToken, err := s.store.GetResetToken(ctx, userID)
	if err != nil {
		return serviceutil.HandleStoreError(err, ErrResetTokenInvalid)
	}

	// 检查令牌是否已被使用
	if storedToken.UsedAt != nil {
		return ErrResetTokenUsed
	}

	if subtle.ConstantTimeCompare([]byte(storedToken.Token), []byte(token)) != 1 {
		return ErrResetTokenInvalid
	}

	if storedToken.ExpiresAt.Before(time.Now()) {
		return ErrResetTokenExpired
	}

	// 先标记令牌为已使用（防止重复使用）
	if err := s.store.MarkResetTokenUsed(ctx, userID); err != nil {
		logger.Error("标记重置令牌为已使用失败", "error", err, "user_id", userID)
		return serviceutil.WrapServiceError("标记令牌已使用", err)
	}

	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return serviceutil.WrapServiceError("查询用户", err)
	}

	hashedPassword, err := s.passwordSvc.HashPassword(newPassword)
	if err != nil {
		return serviceutil.WrapServiceError("哈希密码", err)
	}

	user.PasswordHash = hashedPassword
	user.UpdatedAt = time.Now()
	user.LoginAttempts = 0
	user.LockedUntil = nil

	if err := s.store.Update(ctx, user); err != nil {
		return serviceutil.WrapServiceError("更新用户", err)
	}

	// 清理重置令牌（失败不影响主流程）
	if err := s.store.DeleteResetToken(ctx, userID); err != nil {
		logger.Warn("清理重置令牌失败", "error", err, "user_id", userID)
	}
	// 撤销用户所有Token（失败不影响主流程）
	if err := s.store.RevokeAllUserTokens(ctx, userID); err != nil {
		logger.Warn("撤销用户Token失败", "error", err, "user_id", userID)
	}

	// 使用统一的审计日志工具记录密码重置事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventPasswordReset), userID, map[string]interface{}{
		"ip_address": ipAddress,
	})

	return nil
}

func (s *UserService) ResetPassword(ctx context.Context, userID, token, newPassword string) error {
	return s.ResetPasswordWithAudit(ctx, userID, token, newPassword, "")
}

// ============================================================================
// 密码修改
// ============================================================================

func (s *UserService) ChangePasswordWithAudit(ctx context.Context, userID, oldPassword, newPassword string, ipAddress string) error {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return serviceutil.WrapServiceError("查询用户", err)
	}

	if err := s.passwordSvc.VerifyPassword(user.PasswordHash, oldPassword); err != nil {
		// 使用统一的审计日志工具记录密码修改失败事件
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventPasswordChanged), userID, map[string]interface{}{
			"ip_address": ipAddress,
			"success":    false,
		})
		return apperrors.ErrInvalidCredentials
	}

	if err := validator.ValidatePassword(newPassword); err != nil {
		return err
	}

	hashedPassword, err := s.passwordSvc.HashPassword(newPassword)
	if err != nil {
		return serviceutil.WrapServiceError("哈希密码", err)
	}

	user.PasswordHash = hashedPassword
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return serviceutil.WrapServiceError("更新用户", err)
	}

	// 使用统一的审计日志工具记录密码修改成功事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventPasswordChanged), userID, map[string]interface{}{
		"ip_address": ipAddress,
		"success":    true,
	})

	return nil
}

func (s *UserService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	return s.ChangePasswordWithAudit(ctx, userID, oldPassword, newPassword, "")
}
