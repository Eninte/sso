// Package service_test MFA服务单元测试
package service_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestMFAService 创建测试用的MFA服务
func createTestMFAService() (*service.MFAService, *mock.Store) {
	store := mock.New()
	mfaSvc := service.NewMFAService(store)
	// 设置测试用HMAC密钥
	mfaSvc.SetHMACKey([]byte("test-hmac-key-for-recovery-codes-32bytes"))
	return mfaSvc, store
}

// createTestUserWithMFA 创建带有MFA的测试用户
func createTestUserWithMFA(mfaEnabled bool, mfaSecret string) *model.User {
	return &model.User{
		ID:           "test-user-id",
		Email:        "test@example.com",
		PasswordHash: "hashed-password",
		Status:       model.UserStatusActive,
		MFAEnabled:   mfaEnabled,
		MFASecret:    mfaSecret,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

// ============================================================================
// NewMFAService 测试
// ============================================================================

func TestNewMFAService(t *testing.T) {
	store := mock.New()

	svc := service.NewMFAService(store)

	assert.NotNil(t, svc)
}

// ============================================================================
// SetupMFA 测试
// ============================================================================

func TestMFAService_SetupMFA(t *testing.T) {
	mfaSvc, store := createTestMFAService()
	ctx := context.Background()

	t.Run("成功设置MFA", func(t *testing.T) {
		store.Reset()
		user := createTestUserWithMFA(false, "")
		store.AddUser(user)

		resp, err := mfaSvc.SetupMFA(ctx, "test-user-id")

		require.NoError(t, err)
		assert.NotEmpty(t, resp.Secret)
		assert.NotEmpty(t, resp.QRCodeURL)
	})

	t.Run("MFA已启用", func(t *testing.T) {
		store.Reset()
		user := createTestUserWithMFA(true, "JBSWY3DPEHPK3PXP")
		store.AddUser(user)

		_, err := mfaSvc.SetupMFA(ctx, "test-user-id")

		assert.ErrorIs(t, err, service.ErrMFAAlreadyEnabled)
	})

	t.Run("用户不存在", func(t *testing.T) {
		store.Reset()

		_, err := mfaSvc.SetupMFA(ctx, "nonexistent-user")

		assert.Error(t, err)
	})
}

// ============================================================================
// VerifyAndEnableMFA 测试
// ============================================================================

func TestMFAService_VerifyAndEnableMFA(t *testing.T) {
	mfaSvc, store := createTestMFAService()
	ctx := context.Background()

	t.Run("MFA已启用", func(t *testing.T) {
		store.Reset()
		user := createTestUserWithMFA(true, "JBSWY3DPEHPK3PXP")
		store.AddUser(user)

		err := mfaSvc.VerifyAndEnableMFA(ctx, "test-user-id", "123456")

		assert.ErrorIs(t, err, service.ErrMFAAlreadyEnabled)
	})

	t.Run("MFA密钥为空", func(t *testing.T) {
		store.Reset()
		user := createTestUserWithMFA(false, "")
		store.AddUser(user)

		err := mfaSvc.VerifyAndEnableMFA(ctx, "test-user-id", "123456")

		assert.ErrorIs(t, err, service.ErrInvalidMFASecret)
	})

	t.Run("用户不存在", func(t *testing.T) {
		store.Reset()

		err := mfaSvc.VerifyAndEnableMFA(ctx, "nonexistent-user", "123456")

		assert.Error(t, err)
	})
}

// ============================================================================
// DisableMFA 测试
// ============================================================================

func TestMFAService_DisableMFA(t *testing.T) {
	mfaSvc, store := createTestMFAService()
	ctx := context.Background()

	t.Run("MFA未启用", func(t *testing.T) {
		store.Reset()
		user := createTestUserWithMFA(false, "")
		store.AddUser(user)

		err := mfaSvc.DisableMFA(ctx, "test-user-id", "123456")

		assert.ErrorIs(t, err, service.ErrMFANotEnabled)
	})

	t.Run("用户不存在", func(t *testing.T) {
		store.Reset()

		err := mfaSvc.DisableMFA(ctx, "nonexistent-user", "123456")

		assert.Error(t, err)
	})
}

// ============================================================================
// GetMFAStatus 测试
// ============================================================================

func TestMFAService_GetMFAStatus(t *testing.T) {
	mfaSvc, store := createTestMFAService()
	ctx := context.Background()

	t.Run("MFA已启用", func(t *testing.T) {
		store.Reset()
		user := createTestUserWithMFA(true, "JBSWY3DPEHPK3PXP")
		store.AddUser(user)

		status, err := mfaSvc.GetMFAStatus(ctx, "test-user-id")

		require.NoError(t, err)
		assert.True(t, status.Enabled)
	})

	t.Run("MFA未启用", func(t *testing.T) {
		store.Reset()
		user := createTestUserWithMFA(false, "")
		store.AddUser(user)

		status, err := mfaSvc.GetMFAStatus(ctx, "test-user-id")

		require.NoError(t, err)
		assert.False(t, status.Enabled)
	})

	t.Run("用户不存在", func(t *testing.T) {
		store.Reset()

		_, err := mfaSvc.GetMFAStatus(ctx, "nonexistent-user")

		assert.Error(t, err)
	})
}

// ============================================================================
// TOTP 辅助函数 (复制 mfa.go 中的逻辑用于测试)
// ============================================================================

func generateTestTOTP(secret string) string {
	secret = strings.ToUpper(strings.TrimSpace(secret))
	secretBytes, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)

	now := time.Now()
	timeStep := uint64(now.Unix() / 30)

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, timeStep)

	mac := hmac.New(sha1.New, secretBytes)
	mac.Write(buf)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	return fmt.Sprintf("%06d", code%1000000)
}

// ============================================================================
// VerifyAndEnableMFA 完整流程测试
// ============================================================================

func TestMFAService_VerifyAndEnableMFA_FullFlow(t *testing.T) {
	mfaSvc, store := createTestMFAService()
	ctx := context.Background()

	t.Run("完整MFA启用流程", func(t *testing.T) {
		store.Reset()
		user := createTestUserWithMFA(false, "")
		store.AddUser(user)

		// 1. 设置MFA获取secret
		setupResp, err := mfaSvc.SetupMFA(ctx, "test-user-id")
		require.NoError(t, err)
		assert.NotEmpty(t, setupResp.Secret)

		// 2. 用secret生成有效TOTP
		validCode := generateTestTOTP(setupResp.Secret)

		// 3. 验证并启用MFA
		err = mfaSvc.VerifyAndEnableMFA(ctx, "test-user-id", validCode)
		assert.NoError(t, err)

		// 4. 验证MFA已启用
		status, err := mfaSvc.GetMFAStatus(ctx, "test-user-id")
		require.NoError(t, err)
		assert.True(t, status.Enabled)
	})

	t.Run("无效TOTP验证码", func(t *testing.T) {
		store.Reset()
		user := createTestUserWithMFA(false, "JBSWY3DPEHPK3PXP")
		store.AddUser(user)

		err := mfaSvc.VerifyAndEnableMFA(ctx, "test-user-id", "000000")

		assert.ErrorIs(t, err, service.ErrInvalidTOTPCode)
	})

	t.Run("启用后禁用MFA流程", func(t *testing.T) {
		store.Reset()

		// 先设置并启用MFA
		user := createTestUserWithMFA(false, "")
		store.AddUser(user)

		setupResp, err := mfaSvc.SetupMFA(ctx, "test-user-id")
		require.NoError(t, err)

		validCode := generateTestTOTP(setupResp.Secret)
		err = mfaSvc.VerifyAndEnableMFA(ctx, "test-user-id", validCode)
		require.NoError(t, err)

		// 禁用MFA
		disableCode := generateTestTOTP(setupResp.Secret)
		err = mfaSvc.DisableMFA(ctx, "test-user-id", disableCode)
		assert.NoError(t, err)

		// 验证MFA已禁用
		status, err := mfaSvc.GetMFAStatus(ctx, "test-user-id")
		require.NoError(t, err)
		assert.False(t, status.Enabled)
	})

	t.Run("禁用MFA-无效验证码", func(t *testing.T) {
		store.Reset()
		user := createTestUserWithMFA(true, "JBSWY3DPEHPK3PXP")
		store.AddUser(user)

		err := mfaSvc.DisableMFA(ctx, "test-user-id", "999999")

		assert.ErrorIs(t, err, service.ErrInvalidTOTPCode)
	})
}
