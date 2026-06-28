// MFA 设置/验证/禁用/状态逻辑（从 mfa.go 拆分）
package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- SHA1用于HOTP算法（RFC 4226）
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/serviceutil"
)

// MFA操作
// ============================================================================

func (s *MFAService) SetupMFAWithAudit(ctx context.Context, userID string, ipAddress string) (*model.MFASetupResponse, error) {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return nil, serviceutil.WrapServiceError("查询用户", err)
	}

	if user.MFAEnabled {
		return nil, ErrMFAAlreadyEnabled
	}

	secret, err := generateTOTPSecret()
	if err != nil {
		return nil, serviceutil.WrapServiceError("生成MFA密钥", err)
	}

	user.MFASecret = secret
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return nil, serviceutil.WrapServiceError("更新用户", err)
	}

	qrCodeURL := generateTOTPURL(secret, user.Email)

	// 使用统一的审计日志工具记录MFA设置事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventMFASetup), userID, map[string]interface{}{
		"ip_address": ipAddress,
	})

	return &model.MFASetupResponse{
		Secret:    secret,
		QRCodeURL: qrCodeURL,
	}, nil
}

func (s *MFAService) SetupMFA(ctx context.Context, userID string) (*model.MFASetupResponse, error) {
	return s.SetupMFAWithAudit(ctx, userID, "")
}

func (s *MFAService) VerifyAndEnableMFAWithAudit(ctx context.Context, userID, code string, ipAddress string) error {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return serviceutil.WrapServiceError("查询用户", err)
	}

	if user.MFAEnabled {
		return ErrMFAAlreadyEnabled
	}

	if user.MFASecret == "" {
		return ErrInvalidMFASecret
	}

	if !s.validateTOTPWithReplayProtection(userID, user.MFASecret, code) {
		return ErrInvalidTOTPCode
	}

	user.MFAEnabled = true
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return serviceutil.WrapServiceError("更新用户", err)
	}

	// 使用统一的审计日志工具记录MFA启用事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventMFAEnabled), userID, map[string]interface{}{
		"ip_address": ipAddress,
	})

	return nil
}

func (s *MFAService) VerifyAndEnableMFA(ctx context.Context, userID, code string) error {
	return s.VerifyAndEnableMFAWithAudit(ctx, userID, code, "")
}

func (s *MFAService) DisableMFAWithAudit(ctx context.Context, userID, code string, ipAddress string) error {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return serviceutil.WrapServiceError("查询用户", err)
	}

	if !user.MFAEnabled {
		return ErrMFANotEnabled
	}

	if !s.validateTOTPWithReplayProtection(userID, user.MFASecret, code) {
		return ErrInvalidTOTPCode
	}

	user.MFAEnabled = false
	user.MFASecret = ""
	user.UpdatedAt = time.Now()

	// 原子地禁用MFA并清除恢复码，避免出现"用户MFA已禁用但恢复码残留"的不一致状态
	if err := s.store.DisableMFAAndClearRecoveryCodes(ctx, user); err != nil {
		return serviceutil.WrapServiceError("禁用MFA并清除恢复码", err)
	}

	// 使用统一的审计日志工具记录MFA禁用事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventMFADisabled), userID, map[string]interface{}{
		"ip_address": ipAddress,
	})

	return nil
}

func (s *MFAService) DisableMFA(ctx context.Context, userID, code string) error {
	return s.DisableMFAWithAudit(ctx, userID, code, "")
}

func (s *MFAService) GetMFAStatus(ctx context.Context, userID string) (*model.MFAStatusResponse, error) {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &model.MFAStatusResponse{
		Enabled: user.MFAEnabled,
	}, nil
}

// ============================================================================
// TOTP实现
// ============================================================================

func generateTOTPSecret() (string, error) {
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

func generateTOTPURL(secret, email string) string {
	return fmt.Sprintf("otpauth://totp/SSO:%s?secret=%s&issuer=SSO", email, secret)
}

// validateTOTPWithReplayProtection 验证TOTP并防止重放攻击
// 允许±1时间步（90秒窗口）但记录使用防止重放
func (s *MFAService) validateTOTPWithReplayProtection(userID, secret, code string) bool {
	timeStep, ok := findMatchingTimeStep(secret, code)
	if !ok {
		return false
	}

	// 检查是否已使用（防止重放攻击）
	if s.isTOTPUsed(userID, code, timeStep) {
		return false
	}

	// 记录使用
	s.recordTOTPUsage(userID, code, timeStep)
	return true
}

// findMatchingTimeStep 在 ±1 时间窗口内查找匹配的 TOTP 时间步
// 返回匹配的 timeStep 和是否匹配成功
// 提取此辅助函数以消除 TOTP 验证逻辑的重复
func findMatchingTimeStep(secret, code string) (uint64, bool) {
	secret = strings.ToUpper(strings.TrimSpace(secret))
	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return 0, false
	}

	now := time.Now()
	baseTimeStep := now.Unix() / 30

	// 检查时间窗口: -1, 0, +1 (90秒窗口)
	for i := -1; i <= 1; i++ {
		var timeStep uint64

		if i < 0 {
			// 安全处理负偏移，防止整数下溢
			offset := uint64(-i)
			if baseTimeStep < int64(offset) { // #nosec G115 -- 安全的比较，baseTimeStep已验证为非负
				// 会发生下溢，跳过该时间窗口
				continue
			}
			timeStep = uint64(baseTimeStep) - offset // #nosec G115 -- 安全的减法，已验证baseTimeStep >= offset
		} else {
			// 正偏移总是安全的
			timeStep = uint64(baseTimeStep) + uint64(i) // #nosec G115 -- 安全的加法，i总是非负的
		}

		if generateHOTP(secretBytes, timeStep) == code {
			return timeStep, true
		}
	}

	return 0, false
}

func generateHOTP(secret []byte, counter uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	// RFC 6238 (TOTP) 和 RFC 4226 (HOTP) 标准规定使用SHA1哈希算法
	// 这是业界标准实现，被Google Authenticator、Authy等广泛应用
	// 参考: https://tools.ietf.org/html/rfc6238
	// 注意: 这里的sha1用于HMAC-SHA1，不是直接哈希，安全性有保障
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	return fmt.Sprintf("%06d", code%1000000)
}

// ============================================================================
// MFA恢复码功能
// ============================================================================

// GenerateRecoveryCodes 生成恢复码
// 返回明文恢复码（仅在生成时返回给用户）
// 使用HMAC-SHA256哈希存储，性能优于bcrypt且安全性足够（恢复码为高熵随机值）
