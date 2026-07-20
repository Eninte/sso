// MFA 登录验证逻辑（两阶段登录第二阶段）
// 仅验证 code 正确性，不签发 Token —— Token 由 AuthService.VerifyMFALogin 签发
package service

import (
	"context"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/serviceutil"
)

// VerifyMFALoginCode 验证 MFA 登录验证码
//
// method 取值：
//   - model.MFAMethodTOTP          : 验证 6 位 TOTP 数字（30 秒时间窗口，±1 容差）
//   - model.MFAMethodRecoveryCode   : 验证恢复码（16 字符 XXXX-XXXX-XXXX-XXXX）
//
// 安全设计：
//   - TOTP 复用既有 validateTOTPWithReplayProtection（90 秒窗口防重放）
//   - 恢复码复用既有 VerifyRecoveryCode（HMAC-SHA256 + 恒定时间比较 + 限流）
//   - 验证前再次查询用户，防止 challenge 期间用户被禁用/MFA 被关闭
//   - 验证失败统一返回 ErrInvalidMFACode，不暴露具体失败原因（防信息枚举）
//   - 验证成功记录审计日志
func (s *MFAService) VerifyMFALoginCode(ctx context.Context, userID, method, code, ipAddress string) error {
	// 重新查询用户，防止 challenge 期间状态变更（账户被禁用、MFA 被关闭）
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return serviceutil.HandleStoreError(err, apperrors.ErrInvalidMFACode)
	}

	// 检查账户状态
	if user.Status == model.UserStatusDisabled {
		return apperrors.ErrAccountDisabled
	}
	if user.Status == model.UserStatusLocked {
		return apperrors.ErrAccountLocked
	}

	// 检查 MFA 仍处于启用状态
	if !user.MFAEnabled {
		// 用户在 challenge 期间禁用了 MFA，拒绝登录
		return apperrors.ErrMFANotEnabled
	}

	switch method {
	case model.MFAMethodTOTP:
		if user.MFASecret == "" {
			// 数据不一致：MFAEnabled=true 但 MFASecret 为空
			// 安全起见拒绝登录，不暴露内部错误细节
			auditutil.SafeAuditLog(ctx, s.auditSvc, "mfa_login_inconsistent_state", userID, map[string]interface{}{
				"reason":     "mfa_enabled_but_secret_empty",
				"ip_address": ipAddress,
			})
			return apperrors.ErrInvalidMFACode
		}

		if !s.validateTOTPWithReplayProtection(userID, user.MFASecret, code) {
			return apperrors.ErrInvalidMFACode
		}

		auditutil.SafeAuditLog(ctx, s.auditSvc, "mfa_login_totp_success", userID, map[string]interface{}{
			"ip_address": ipAddress,
		})
		return nil

	case model.MFAMethodRecoveryCode:
		valid, err := s.VerifyRecoveryCode(ctx, userID, code, ipAddress)
		if err != nil {
			// 恢复码限流（ErrTooManyRecoveryAttempts）等错误透传
			return err
		}
		if !valid {
			return apperrors.ErrInvalidMFACode
		}
		// VerifyRecoveryCode 内部已记录审计日志
		return nil

	default:
		return apperrors.ErrBadRequest.WithDetails("invalid MFA method: " + method)
	}
}
