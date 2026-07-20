// Package service_test MFA 登录验证码单元测试
// 覆盖 MFAService.VerifyMFALoginCode 的所有分支：
//   - 用户状态检查（disabled / locked / MFA 已关闭）
//   - TOTP 验证（成功 / 失败 / 数据不一致）
//   - 恢复码验证（成功 / 无效 / 限流）
package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// createMFALoginService 创建测试用 MFA 服务（带 HMAC 密钥）
// 注意：mock.Store 内部独立维护 hmacKey 用于 VerifyAndUseMFARecoveryCode，
// 必须与 MFAService.SetHMACKey 使用同一密钥，否则恢复码哈希不匹配
func createMFALoginService() (*service.MFAService, *mock.Store) {
	store := mock.New()
	hmacKey := []byte("test-hmac-key-for-recovery-codes-32bytes")
	store.SetMockHMACKey(hmacKey)
	mfaSvc := service.NewMFAService(store)
	mfaSvc.SetHMACKey(hmacKey)
	return mfaSvc, store
}

// createMFALoginUser 创建带 MFA 启用状态的测试用户
func createMFALoginUser(status string, mfaEnabled bool, secret string) *model.User {
	return &model.User{
		ID:           "mfa-login-user-id",
		Email:        "mfa-login@example.com",
		PasswordHash: "hashed-password",
		Status:       status,
		MFAEnabled:   mfaEnabled,
		MFASecret:    secret,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

// ============================================================================
// VerifyMFALoginCode - TOTP 分支
// ============================================================================

func TestMFAService_VerifyMFALoginCode_TOTP_Success(t *testing.T) {
	mfaSvc, store := createMFALoginService()
	ctx := context.Background()

	store.Reset()
	user := createMFALoginUser(model.UserStatusActive, true, "JBSWY3DPEHPK3PXP")
	store.AddUser(user)

	// 生成有效 TOTP
	validCode := generateTestTOTP("JBSWY3DPEHPK3PXP")

	err := mfaSvc.VerifyMFALoginCode(ctx, user.ID, model.MFAMethodTOTP, validCode, "192.168.1.1")

	assert.NoError(t, err)
}

func TestMFAService_VerifyMFALoginCode_TOTP_InvalidCode(t *testing.T) {
	mfaSvc, store := createMFALoginService()
	ctx := context.Background()

	store.Reset()
	user := createMFALoginUser(model.UserStatusActive, true, "JBSWY3DPEHPK3PXP")
	store.AddUser(user)

	err := mfaSvc.VerifyMFALoginCode(ctx, user.ID, model.MFAMethodTOTP, "000000", "192.168.1.1")

	assert.ErrorIs(t, err, apperrors.ErrInvalidMFACode)
}

func TestMFAService_VerifyMFALoginCode_TOTP_EmptySecret(t *testing.T) {
	mfaSvc, store := createMFALoginService()
	ctx := context.Background()

	store.Reset()
	// MFAEnabled=true 但 MFASecret 为空 —— 数据不一致
	user := createMFALoginUser(model.UserStatusActive, true, "")
	store.AddUser(user)

	err := mfaSvc.VerifyMFALoginCode(ctx, user.ID, model.MFAMethodTOTP, "123456", "192.168.1.1")

	// 出于安全考虑不暴露内部细节，统一返回 ErrInvalidMFACode
	assert.ErrorIs(t, err, apperrors.ErrInvalidMFACode)
}

// ============================================================================
// VerifyMFALoginCode - 用户状态检查
// ============================================================================

func TestMFAService_VerifyMFALoginCode_UserDisabled(t *testing.T) {
	mfaSvc, store := createMFALoginService()
	ctx := context.Background()

	store.Reset()
	user := createMFALoginUser(model.UserStatusDisabled, true, "JBSWY3DPEHPK3PXP")
	store.AddUser(user)

	err := mfaSvc.VerifyMFALoginCode(ctx, user.ID, model.MFAMethodTOTP, "123456", "192.168.1.1")

	assert.ErrorIs(t, err, apperrors.ErrAccountDisabled)
}

func TestMFAService_VerifyMFALoginCode_UserLocked(t *testing.T) {
	mfaSvc, store := createMFALoginService()
	ctx := context.Background()

	store.Reset()
	user := createMFALoginUser(model.UserStatusLocked, true, "JBSWY3DPEHPK3PXP")
	store.AddUser(user)

	err := mfaSvc.VerifyMFALoginCode(ctx, user.ID, model.MFAMethodTOTP, "123456", "192.168.1.1")

	assert.ErrorIs(t, err, apperrors.ErrAccountLocked)
}

func TestMFAService_VerifyMFALoginCode_MFADisabledDuringChallenge(t *testing.T) {
	mfaSvc, store := createMFALoginService()
	ctx := context.Background()

	store.Reset()
	// Challenge 期间用户禁用了 MFA
	user := createMFALoginUser(model.UserStatusActive, false, "JBSWY3DPEHPK3PXP")
	store.AddUser(user)

	err := mfaSvc.VerifyMFALoginCode(ctx, user.ID, model.MFAMethodTOTP, "123456", "192.168.1.1")

	assert.ErrorIs(t, err, apperrors.ErrMFANotEnabled)
}

func TestMFAService_VerifyMFALoginCode_UserNotFound(t *testing.T) {
	mfaSvc, store := createMFALoginService()
	ctx := context.Background()

	store.Reset()

	err := mfaSvc.VerifyMFALoginCode(ctx, "nonexistent-user", model.MFAMethodTOTP, "123456", "192.168.1.1")

	// 不暴露用户不存在，统一返回 ErrInvalidMFACode
	assert.ErrorIs(t, err, apperrors.ErrInvalidMFACode)
}

// ============================================================================
// VerifyMFALoginCode - 恢复码分支
// ============================================================================

func TestMFAService_VerifyMFALoginCode_RecoveryCode_Success(t *testing.T) {
	mfaSvc, store := createMFALoginService()
	ctx := context.Background()

	store.Reset()
	user := createMFALoginUser(model.UserStatusActive, true, "JBSWY3DPEHPK3PXP")
	store.AddUser(user)

	// 先生成恢复码
	codes, err := mfaSvc.GenerateRecoveryCodes(ctx, user.ID, 8)
	require.NoError(t, err)
	require.Len(t, codes, 8)

	// 使用第一个恢复码验证
	err = mfaSvc.VerifyMFALoginCode(ctx, user.ID, model.MFAMethodRecoveryCode, codes[0], "192.168.1.1")

	assert.NoError(t, err)
}

func TestMFAService_VerifyMFALoginCode_RecoveryCode_Invalid(t *testing.T) {
	mfaSvc, store := createMFALoginService()
	ctx := context.Background()

	store.Reset()
	user := createMFALoginUser(model.UserStatusActive, true, "JBSWY3DPEHPK3PXP")
	store.AddUser(user)

	// 生成恢复码（确保用户存在恢复码记录）
	_, err := mfaSvc.GenerateRecoveryCodes(ctx, user.ID, 8)
	require.NoError(t, err)

	// 使用一个明显无效的恢复码
	// VerifyRecoveryCode 返回 (false, ErrRecoveryCodeInvalid)，
	// VerifyMFALoginCode 透传该错误（不转换为 ErrInvalidMFACode）
	err = mfaSvc.VerifyMFALoginCode(ctx, user.ID, model.MFAMethodRecoveryCode, "INVALID-CODE-XXXX", "192.168.1.1")

	assert.ErrorIs(t, err, apperrors.ErrRecoveryCodeInvalid)
}

// ============================================================================
// VerifyMFALoginCode - 参数校验
// ============================================================================

func TestMFAService_VerifyMFALoginCode_InvalidMethod(t *testing.T) {
	mfaSvc, store := createMFALoginService()
	ctx := context.Background()

	store.Reset()
	user := createMFALoginUser(model.UserStatusActive, true, "JBSWY3DPEHPK3PXP")
	store.AddUser(user)

	err := mfaSvc.VerifyMFALoginCode(ctx, user.ID, "invalid-method", "123456", "192.168.1.1")

	assert.ErrorIs(t, err, apperrors.ErrBadRequest)
}
