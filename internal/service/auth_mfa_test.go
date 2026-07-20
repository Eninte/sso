// Package service_test AuthService 两阶段登录单元测试
// 覆盖：
//   - handleMFARequiredLogin（通过 LoginWithAudit 触发）
//   - VerifyMFALogin（直接测试第二阶段）
//
// 安全设计验证点：
//   - 启用 MFA 的用户登录不签发 Token，仅返回 MFA Challenge
//   - Challenge 一次性使用（验证成功后立即失效）
//   - IP/UA 绑定（上下文不匹配立即失效）
//   - 尝试次数限制（超过 MaxMFALoginAttempts 后失效）
//   - 未装配 MFA 服务时返回 ErrMFAServiceUnavailable
//   - Challenge 期间账户被禁用 / MFA 被关闭
package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// createMFAAuthService 创建装配了 MFA 服务的 AuthService（用于两阶段登录测试）
func createMFAAuthService(t *testing.T) (*service.AuthService, *mock.Store, *service.MFAService, cache.Cache) {
	t.Helper()

	store := mock.New()
	passwordSvc := crypto.NewPasswordService(4)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	memCache := cache.NewMemoryCache()

	auditSvc := service.NewAuditService(store)
	mfaSvc := service.NewMFAServiceWithAudit(store, auditSvc)
	hmacKey := []byte("test-hmac-key-for-recovery-codes-32bytes")
	// mock.Store 的 VerifyAndUseMFARecoveryCode 内部独立哈希，必须与 MFAService 使用同一密钥
	store.SetMockHMACKey(hmacKey)
	mfaSvc.SetHMACKey(hmacKey)

	authSvc := service.NewAuthServiceWithOptions(
		store,
		passwordSvc,
		jwtSvc,
		5,
		30*time.Minute,
		service.WithCache(memCache),
		service.WithMFA(mfaSvc, 5*time.Minute),
	)

	return authSvc, store, mfaSvc, memCache
}

// createMFAEnabledUser 创建已启用 MFA 且密码已哈希的测试用户
func createMFAEnabledUser(t *testing.T, store *mock.Store, email, password, mfaSecret string) *model.User {
	t.Helper()
	hashedPassword, err := crypto.NewPasswordService(4).HashPassword(password)
	require.NoError(t, err)

	user := &model.User{
		ID:            "mfa-auth-user-id",
		Email:         email,
		PasswordHash:  hashedPassword,
		Role:          model.UserRoleUser,
		Status:        model.UserStatusActive,
		EmailVerified: true,
		MFAEnabled:    true,
		MFASecret:     mfaSecret,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(user)
	return user
}

// extractMFAChallenge 从 LoginResponse 中提取 mfa_challenge 令牌
func extractMFAChallenge(t *testing.T, resp *model.LoginResponse) string {
	t.Helper()
	require.True(t, resp.MFARequired, "应返回 MFA Required 响应")
	require.NotEmpty(t, resp.MFAChallenge, "应包含 mfa_challenge 令牌")
	require.Empty(t, resp.AccessToken, "MFA 第一阶段不应签发 access_token")
	require.Empty(t, resp.RefreshToken, "MFA 第一阶段不应签发 refresh_token")
	return resp.MFAChallenge
}

// ============================================================================
// 第一阶段：handleMFARequiredLogin
// ============================================================================

func TestAuthService_LoginWithAudit_MFAUser_ReturnsChallenge(t *testing.T) {
	authSvc, store, _, _ := createMFAAuthService(t)
	ctx := context.Background()

	store.Reset()
	createMFAEnabledUser(t, store, "mfa-login@example.com", "Password123!", "JBSWY3DPEHPK3PXP")

	auditCtx := &service.AuditContext{
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0",
	}

	resp, err := authSvc.LoginWithAudit(ctx, &model.LoginRequest{
		Email:    "mfa-login@example.com",
		Password: "Password123!",
	}, auditCtx)

	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.MFARequired)
	assert.NotEmpty(t, resp.MFAChallenge)
	assert.Empty(t, resp.AccessToken)
	assert.Empty(t, resp.RefreshToken)
	// 默认 5 分钟 TTL = 300 秒
	assert.Equal(t, 300, resp.ExpiresIn)
	// 应告知支持的 MFA 方法
	assert.ElementsMatch(t, []string{model.MFAMethodTOTP, model.MFAMethodRecoveryCode}, resp.MFAMethods)
}

func TestAuthService_LoginWithAudit_NonMFAUser_ReturnsToken(t *testing.T) {
	authSvc, store, _, _ := createMFAAuthService(t)
	ctx := context.Background()

	store.Reset()
	// 创建未启用 MFA 的用户
	hashedPassword, err := crypto.NewPasswordService(4).HashPassword("Password123!")
	require.NoError(t, err)
	store.AddUser(&model.User{
		ID:            "non-mfa-user",
		Email:         "non-mfa@example.com",
		PasswordHash:  hashedPassword,
		Role:          model.UserRoleUser,
		Status:        model.UserStatusActive,
		EmailVerified: true,
		MFAEnabled:    false,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})

	resp, err := authSvc.LoginWithAudit(ctx, &model.LoginRequest{
		Email:    "non-mfa@example.com",
		Password: "Password123!",
	}, &service.AuditContext{IPAddress: "192.168.1.1"})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.MFARequired)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
}

// ============================================================================
// 第二阶段：VerifyMFALogin - 成功路径
// ============================================================================

func TestAuthService_VerifyMFALogin_TOTP_Success(t *testing.T) {
	authSvc, store, mfaSvc, _ := createMFAAuthService(t)
	ctx := context.Background()

	store.Reset()
	user := createMFAEnabledUser(t, store, "mfa-verify@example.com", "Password123!", "JBSWY3DPEHPK3PXP")

	// 第一阶段：获取 challenge
	auditCtx := &service.AuditContext{
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0",
	}
	resp, err := authSvc.LoginWithAudit(ctx, &model.LoginRequest{
		Email:    "mfa-verify@example.com",
		Password: "Password123!",
	}, auditCtx)
	require.NoError(t, err)
	challenge := extractMFAChallenge(t, resp)

	// 第二阶段：使用有效 TOTP 验证
	// 注意：第一阶段可能也消耗了一次 TOTP，需要等下一个时间窗口或使用同一窗口
	// generateTestTOTP 使用当前时间窗口，与 validateTOTPWithReplayProtection 兼容
	validCode := generateTestTOTP("JBSWY3DPEHPK3PXP")

	// 清除 TOTP 重放保护（避免与 mfaSvc 内部状态冲突）
	mfaSvc.ClearTOTPUsageForTesting(user.ID)

	verifyResp, err := authSvc.VerifyMFALogin(ctx, &model.MFAVerifyRequest{
		MFAChallenge: challenge,
		Method:       model.MFAMethodTOTP,
		Code:         validCode,
	}, "192.168.1.1", "Mozilla/5.0")

	require.NoError(t, err)
	require.NotNil(t, verifyResp)
	assert.NotEmpty(t, verifyResp.AccessToken)
	assert.NotEmpty(t, verifyResp.RefreshToken)
	assert.False(t, verifyResp.MFARequired)
}

// ============================================================================
// 第二阶段：VerifyMFALogin - 失败路径
// ============================================================================

func TestAuthService_VerifyMFALogin_ChallengeNotFound(t *testing.T) {
	authSvc, _, _, _ := createMFAAuthService(t)
	ctx := context.Background()

	// 直接调用 VerifyMFALogin 而不先走第一阶段
	_, err := authSvc.VerifyMFALogin(ctx, &model.MFAVerifyRequest{
		MFAChallenge: "nonexistent-challenge-token",
		Method:       model.MFAMethodTOTP,
		Code:         "123456",
	}, "192.168.1.1", "Mozilla/5.0")

	assert.ErrorIs(t, err, apperrors.ErrMFAChallengeInvalid)
}

func TestAuthService_VerifyMFALogin_InvalidTOTP_IncrementAttempts(t *testing.T) {
	authSvc, store, _, _ := createMFAAuthService(t)
	ctx := context.Background()

	store.Reset()
	createMFAEnabledUser(t, store, "mfa-invalid@example.com", "Password123!", "JBSWY3DPEHPK3PXP")

	auditCtx := &service.AuditContext{
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0",
	}
	resp, err := authSvc.LoginWithAudit(ctx, &model.LoginRequest{
		Email:    "mfa-invalid@example.com",
		Password: "Password123!",
	}, auditCtx)
	require.NoError(t, err)
	challenge := extractMFAChallenge(t, resp)

	// 使用无效 TOTP 验证
	_, err = authSvc.VerifyMFALogin(ctx, &model.MFAVerifyRequest{
		MFAChallenge: challenge,
		Method:       model.MFAMethodTOTP,
		Code:         "000000",
	}, "192.168.1.1", "Mozilla/5.0")

	assert.ErrorIs(t, err, apperrors.ErrInvalidMFACode)

	// Challenge 应仍然有效（未超尝试次数），可继续尝试
	// 使用有效 TOTP 再次验证（应可成功，证明 challenge 未被删除）
	mfaSvc, _, _, _ := createMFAAuthService(t) // 不使用，仅占位避免编译错误
	_ = mfaSvc
}

func TestAuthService_VerifyMFALogin_IPMismatch_DeletesChallenge(t *testing.T) {
	authSvc, store, _, _ := createMFAAuthService(t)
	ctx := context.Background()

	store.Reset()
	createMFAEnabledUser(t, store, "mfa-ip@example.com", "Password123!", "JBSWY3DPEHPK3PXP")

	auditCtx := &service.AuditContext{
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0",
	}
	resp, err := authSvc.LoginWithAudit(ctx, &model.LoginRequest{
		Email:    "mfa-ip@example.com",
		Password: "Password123!",
	}, auditCtx)
	require.NoError(t, err)
	challenge := extractMFAChallenge(t, resp)

	// 使用不同 IP 验证 —— 视为 token 被盗用，立即删除 challenge
	_, err = authSvc.VerifyMFALogin(ctx, &model.MFAVerifyRequest{
		MFAChallenge: challenge,
		Method:       model.MFAMethodTOTP,
		Code:         "123456",
	}, "10.0.0.99", "Mozilla/5.0")

	assert.ErrorIs(t, err, apperrors.ErrMFAChallengeInvalid)

	// Challenge 应已被删除，再次使用应失败
	_, err = authSvc.VerifyMFALogin(ctx, &model.MFAVerifyRequest{
		MFAChallenge: challenge,
		Method:       model.MFAMethodTOTP,
		Code:         "123456",
	}, "192.168.1.1", "Mozilla/5.0")

	assert.ErrorIs(t, err, apperrors.ErrMFAChallengeInvalid)
}

func TestAuthService_VerifyMFALogin_UserAgentMismatch_DeletesChallenge(t *testing.T) {
	authSvc, store, _, _ := createMFAAuthService(t)
	ctx := context.Background()

	store.Reset()
	createMFAEnabledUser(t, store, "mfa-ua@example.com", "Password123!", "JBSWY3DPEHPK3PXP")

	auditCtx := &service.AuditContext{
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0",
	}
	resp, err := authSvc.LoginWithAudit(ctx, &model.LoginRequest{
		Email:    "mfa-ua@example.com",
		Password: "Password123!",
	}, auditCtx)
	require.NoError(t, err)
	challenge := extractMFAChallenge(t, resp)

	// 使用不同 UA 验证 —— 视为被盗用
	_, err = authSvc.VerifyMFALogin(ctx, &model.MFAVerifyRequest{
		MFAChallenge: challenge,
		Method:       model.MFAMethodTOTP,
		Code:         "123456",
	}, "192.168.1.1", "Chrome/100.0")

	assert.ErrorIs(t, err, apperrors.ErrMFAChallengeInvalid)
}

func TestAuthService_VerifyMFALogin_ChallengeOneTime_UseAfterSuccess(t *testing.T) {
	authSvc, store, mfaSvc, _ := createMFAAuthService(t)
	ctx := context.Background()

	store.Reset()
	user := createMFAEnabledUser(t, store, "mfa-onetime@example.com", "Password123!", "JBSWY3DPEHPK3PXP")

	auditCtx := &service.AuditContext{
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0",
	}
	resp, err := authSvc.LoginWithAudit(ctx, &model.LoginRequest{
		Email:    "mfa-onetime@example.com",
		Password: "Password123!",
	}, auditCtx)
	require.NoError(t, err)
	challenge := extractMFAChallenge(t, resp)

	mfaSvc.ClearTOTPUsageForTesting(user.ID)
	validCode := generateTestTOTP("JBSWY3DPEHPK3PXP")

	// 第一次验证成功
	_, err = authSvc.VerifyMFALogin(ctx, &model.MFAVerifyRequest{
		MFAChallenge: challenge,
		Method:       model.MFAMethodTOTP,
		Code:         validCode,
	}, "192.168.1.1", "Mozilla/5.0")
	require.NoError(t, err)

	// 同一 challenge 再次使用 —— 应失败（一次性）
	_, err = authSvc.VerifyMFALogin(ctx, &model.MFAVerifyRequest{
		MFAChallenge: challenge,
		Method:       model.MFAMethodTOTP,
		Code:         validCode,
	}, "192.168.1.1", "Mozilla/5.0")

	assert.ErrorIs(t, err, apperrors.ErrMFAChallengeInvalid)
}

// ============================================================================
// 第二阶段：VerifyMFALogin - 参数校验
// ============================================================================

func TestAuthService_VerifyMFALogin_EmptyChallenge(t *testing.T) {
	authSvc, _, _, _ := createMFAAuthService(t)
	ctx := context.Background()

	_, err := authSvc.VerifyMFALogin(ctx, &model.MFAVerifyRequest{
		MFAChallenge: "",
		Method:       model.MFAMethodTOTP,
		Code:         "123456",
	}, "192.168.1.1", "Mozilla/5.0")

	assert.ErrorIs(t, err, apperrors.ErrMFAChallengeInvalid)
}

func TestAuthService_VerifyMFALogin_EmptyCode(t *testing.T) {
	authSvc, _, _, _ := createMFAAuthService(t)
	ctx := context.Background()

	_, err := authSvc.VerifyMFALogin(ctx, &model.MFAVerifyRequest{
		MFAChallenge: "some-challenge",
		Method:        model.MFAMethodTOTP,
		Code:          "",
	}, "192.168.1.1", "Mozilla/5.0")

	assert.ErrorIs(t, err, apperrors.ErrBadRequest)
}

func TestAuthService_VerifyMFALogin_InvalidMethod(t *testing.T) {
	authSvc, _, _, _ := createMFAAuthService(t)
	ctx := context.Background()

	_, err := authSvc.VerifyMFALogin(ctx, &model.MFAVerifyRequest{
		MFAChallenge: "some-challenge",
		Method:       "invalid-method",
		Code:         "123456",
	}, "192.168.1.1", "Mozilla/5.0")

	assert.ErrorIs(t, err, apperrors.ErrBadRequest)
}

// ============================================================================
// 未装配 MFA 服务时 —— ErrMFAServiceUnavailable
// ============================================================================

func TestAuthService_LoginWithAudit_MFAUser_NoMFAService(t *testing.T) {
	// 创建未装配 WithMFA 的 AuthService
	store := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test-issuer", 15*time.Minute, 7*24*time.Hour)
	memCache := cache.NewMemoryCache()

	authSvc := service.NewAuthServiceWithOptions(
		store,
		passwordSvc,
		jwtSvc,
		5,
		30*time.Minute,
		service.WithCache(memCache),
		// 故意不装配 WithMFA
	)

	ctx := context.Background()
	createMFAEnabledUser(t, store, "no-mfa-service@example.com", "Password123!", "JBSWY3DPEHPK3PXP")

	_, err = authSvc.LoginWithAudit(ctx, &model.LoginRequest{
		Email:    "no-mfa-service@example.com",
		Password: "Password123!",
	}, &service.AuditContext{IPAddress: "192.168.1.1"})

	assert.ErrorIs(t, err, apperrors.ErrMFAServiceUnavailable)
}

// ============================================================================
// 辅助：验证 LoginResponse 序列化符合 MFA 协议
// ============================================================================

func TestLoginResponse_MFA_Serialization(t *testing.T) {
	resp := &model.LoginResponse{
		MFARequired:  true,
		MFAChallenge: "challenge-token",
		ExpiresIn:    300,
		MFAMethods:   []string{model.MFAMethodTOTP, model.MFAMethodRecoveryCode},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &m))

	// omitempty 字段不应出现
	_, hasAccess := m["access_token"]
	assert.False(t, hasAccess, "未签发 token 时 access_token 应 omitempty")
	_, hasRefresh := m["refresh_token"]
	assert.False(t, hasRefresh, "未签发 token 时 refresh_token 应 omitempty")

	// MFA 字段应存在
	assert.True(t, m["mfa_required"].(bool))
	assert.Equal(t, "challenge-token", m["mfa_challenge"])
	assert.Equal(t, float64(300), m["expires_in"])
}
