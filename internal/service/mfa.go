// Package service MFA服务
// 处理多因素认证相关的业务逻辑
package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
	"github.com/your-org/sso/internal/util/auditutil"
	"github.com/your-org/sso/internal/util/serviceutil"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

var (
	ErrMFAAlreadyEnabled = apperrors.ErrMFAAlreadyEnabled
	ErrMFANotEnabled     = apperrors.ErrMFANotEnabled
	ErrInvalidTOTPCode   = apperrors.ErrInvalidTOTPCode
	ErrTOTPCodeExpired   = apperrors.ErrTOTPCodeExpired
	ErrInvalidMFASecret  = apperrors.ErrInvalidMFASecret
)

// ============================================================================
// MFAService MFA服务
// ============================================================================

type MFAService struct {
	store    store.Store
	auditSvc *AuditService
}

func NewMFAService(store store.Store) *MFAService {
	return &MFAService{
		store:    store,
		auditSvc: NewAuditService(store),
	}
}

func NewMFAServiceWithAudit(store store.Store, auditSvc *AuditService) *MFAService {
	return &MFAService{
		store:    store,
		auditSvc: auditSvc,
	}
}

// ============================================================================
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

	if !validateTOTP(user.MFASecret, code) {
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

	if !validateTOTP(user.MFASecret, code) {
		return ErrInvalidTOTPCode
	}

	user.MFAEnabled = false
	user.MFASecret = ""
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return serviceutil.WrapServiceError("更新用户", err)
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

func validateTOTP(secret, code string) bool {
	secret = strings.ToUpper(strings.TrimSpace(secret))
	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return false
	}

	now := time.Now()
	baseTimeStep := now.Unix() / 30
	
	// 检查时间窗口: -1, 0, +1 (90秒窗口)
	for i := -1; i <= 1; i++ {
		var timeStep uint64
		
		if i < 0 {
			// 安全处理负偏移，防止整数下溢
			offset := uint64(-i)
			if baseTimeStep < int64(offset) {
				// 会发生下溢，跳过该时间窗口
				continue
			}
			timeStep = uint64(baseTimeStep) - offset
		} else {
			// 正偏移总是安全的
			timeStep = uint64(baseTimeStep) + uint64(i)
		}
		
		expectedCode := generateHOTP(secretBytes, timeStep)
		if expectedCode == code {
			return true
		}
	}

	return false
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
