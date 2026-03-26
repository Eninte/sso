// Package service 用户服务
// 处理用户资料管理、邮箱验证、密码重置等业务逻辑
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/your-org/sso/internal/common"
	"github.com/your-org/sso/internal/crypto"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/store"
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
	VerificationTokenTTL = 24 * time.Hour
	ResetTokenTTL        = 1 * time.Hour
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
		return err
	}

	if user.EmailVerified {
		return ErrEmailAlreadyVerified
	}

	token, err := common.GenerateToken()
	if err != nil {
		return fmt.Errorf("生成验证令牌失败: %w", err)
	}

	expiresAt := time.Now().Add(VerificationTokenTTL)
	if err := s.store.StoreVerificationToken(ctx, userID, token, expiresAt); err != nil {
		return err
	}

	verifyLink := fmt.Sprintf("%s/verify-email?token=%s&user_id=%s", s.baseURL, token, userID)

	if s.emailSvc != nil {
		if err := s.emailSvc.SendVerificationEmail(ctx, user.Email, user.Email, verifyLink); err != nil {
			return fmt.Errorf("发送验证邮件失败: %w", err)
		}
	}

	return nil
}

func (s *UserService) VerifyEmail(ctx context.Context, userID, token string) error {
	storedToken, err := s.store.GetVerificationToken(ctx, userID)
	if err != nil {
		if apperrors.Is(err, store.ErrNotFound) {
			return ErrVerificationCodeInvalid
		}
		return err
	}

	if storedToken.Token != token {
		return ErrVerificationCodeInvalid
	}

	if storedToken.ExpiresAt.Before(time.Now()) {
		return ErrVerificationCodeExpired
	}

	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	user.EmailVerified = true
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return err
	}

	_ = s.store.DeleteVerificationToken(ctx, userID)

	return nil
}

// ============================================================================
// 密码重置
// ============================================================================

func (s *UserService) ForgotPassword(ctx context.Context, email string) error {
	user, err := s.store.GetByEmail(ctx, email)
	if err != nil {
		return nil
	}

	token, err := common.GenerateToken()
	if err != nil {
		return nil
	}

	expiresAt := time.Now().Add(ResetTokenTTL)
	if err := s.store.StoreResetToken(ctx, user.ID, token, expiresAt); err != nil {
		return nil
	}

	resetLink := fmt.Sprintf("%s/reset-password?token=%s&user_id=%s", s.baseURL, token, user.ID)

	if s.emailSvc != nil {
		return s.emailSvc.SendPasswordResetEmail(ctx, user.Email, user.Email, resetLink)
	}

	return nil
}

func (s *UserService) ResetPasswordWithAudit(ctx context.Context, userID, token, newPassword string, ipAddress string) error {
	storedToken, err := s.store.GetResetToken(ctx, userID)
	if err != nil {
		if apperrors.Is(err, store.ErrNotFound) {
			return ErrResetTokenInvalid
		}
		return err
	}

	if storedToken.Token != token {
		return ErrResetTokenInvalid
	}

	if storedToken.ExpiresAt.Before(time.Now()) {
		return ErrResetTokenExpired
	}

	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	hashedPassword, err := s.passwordSvc.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("哈希密码失败: %w", err)
	}

	user.PasswordHash = hashedPassword
	user.UpdatedAt = time.Now()
	user.LoginAttempts = 0
	user.LockedUntil = nil

	if err := s.store.Update(ctx, user); err != nil {
		return err
	}

	_ = s.store.DeleteResetToken(ctx, userID)
	_ = s.store.RevokeAllUserTokens(ctx, userID)

	if s.auditSvc != nil {
		s.auditSvc.LogPasswordReset(ctx, userID, ipAddress)
	}

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
		return err
	}

	if err := s.passwordSvc.VerifyPassword(user.PasswordHash, oldPassword); err != nil {
		if s.auditSvc != nil {
			s.auditSvc.LogPasswordChanged(ctx, userID, ipAddress, false)
		}
		return apperrors.ErrInvalidCredentials
	}

	if err := validator.ValidatePassword(newPassword); err != nil {
		return err
	}

	hashedPassword, err := s.passwordSvc.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("哈希密码失败: %w", err)
	}

	user.PasswordHash = hashedPassword
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return err
	}

	if s.auditSvc != nil {
		s.auditSvc.LogPasswordChanged(ctx, userID, ipAddress, true)
	}

	return nil
}

func (s *UserService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	return s.ChangePasswordWithAudit(ctx, userID, oldPassword, newPassword, "")
}
