// Package service 用户服务
// 处理用户资料管理、邮箱验证、密码重置等业务逻辑
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/your-org/sso/internal/common"
	"github.com/your-org/sso/internal/crypto"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
	"github.com/your-org/sso/internal/util/auditutil"
	"github.com/your-org/sso/internal/util/serviceutil"
	"github.com/your-org/sso/internal/validator"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

var (
	ErrVerificationCodeInvalid = apperrors.ErrVerificationCodeInvalid
	ErrVerificationCodeExpired = apperrors.ErrVerificationCodeExpired
	ErrResetTokenInvalid       = apperrors.ErrResetTokenInvalid
	ErrResetTokenExpired       = apperrors.ErrResetTokenExpired
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
	store       store.Store
	passwordSvc *crypto.PasswordService
	emailSvc    *EmailService
	baseURL     string
	auditSvc    *AuditService
}

func NewUserService(
	store store.Store,
	passwordSvc *crypto.PasswordService,
	emailSvc *EmailService,
	baseURL string,
) *UserService {
	return &UserService{
		store:       store,
		passwordSvc: passwordSvc,
		emailSvc:    emailSvc,
		baseURL:     baseURL,
		auditSvc:    NewAuditService(store),
	}
}

func NewUserServiceWithAudit(
	store store.Store,
	passwordSvc *crypto.PasswordService,
	emailSvc *EmailService,
	baseURL string,
	auditSvc *AuditService,
) *UserService {
	return &UserService{
		store:       store,
		passwordSvc: passwordSvc,
		emailSvc:    emailSvc,
		baseURL:     baseURL,
		auditSvc:    auditSvc,
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

	if storedToken.Token != token {
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
	user, err := s.store.GetByEmail(ctx, email)
	if err != nil {
		// 安全设计：不泄露用户是否存在，但记录错误日志以便排查
		slog.Debug("ForgotPassword: 获取用户失败", "error", err, "email", email)
		return nil
	}

	token, err := common.GenerateToken()
	if err != nil {
		slog.Error("ForgotPassword: 生成令牌失败", "error", err, "user_id", user.ID)
		return nil
	}

	expiresAt := time.Now().Add(ResetTokenTTL)
	if err := s.store.StoreResetToken(ctx, user.ID, token, expiresAt); err != nil {
		slog.Error("ForgotPassword: 存储重置令牌失败", "error", err, "user_id", user.ID)
		return nil
	}

	resetLink := fmt.Sprintf("%s/reset-password?token=%s&user_id=%s", s.baseURL, token, user.ID)

	if s.emailSvc != nil {
		if err := s.emailSvc.SendPasswordResetEmail(ctx, user.Email, user.Email, resetLink); err != nil {
			slog.Error("ForgotPassword: 发送重置邮件失败", "error", err, "user_id", user.ID)
			// 仍然返回 nil，不泄露内部错误
		}
	}

	return nil
}

func (s *UserService) ResetPasswordWithAudit(ctx context.Context, userID, token, newPassword string, ipAddress string) error {
	// 验证密码强度
	if err := validator.ValidatePassword(newPassword); err != nil {
		return err
	}

	storedToken, err := s.store.GetResetToken(ctx, userID)
	if err != nil {
		return serviceutil.HandleStoreError(err, ErrResetTokenInvalid)
	}

	if storedToken.Token != token {
		return ErrResetTokenInvalid
	}

	if storedToken.ExpiresAt.Before(time.Now()) {
		return ErrResetTokenExpired
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
		slog.Warn("清理重置令牌失败", "error", err, "user_id", userID)
	}
	// 撤销用户所有Token（失败不影响主流程）
	if err := s.store.RevokeAllUserTokens(ctx, userID); err != nil {
		slog.Warn("撤销用户Token失败", "error", err, "user_id", userID)
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
