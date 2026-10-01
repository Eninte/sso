// Package service fail-secure 分支错误注入测试
//
// 背景：以下生产路径在 store 错误时"宁可失败不可放行"（fail-secure）：
//   - 密码重置：MarkResetTokenUsed 失败必须中止重置，否则已使用的令牌可能被重放
//   - MFA 恢复码：VerifyAndUseMFARecoveryCode 失败不得视为验证通过
//   - OAuth 授权码：markAuthorizationCodeUsed 失败不得签发 token
//
// 这些分支此前因 mock 缺少错误注入点而无法单测（TestLoginFailureCounter_DatabaseError
// 曾因此名不副实）。mock 补齐注入字段后，在此固化其 fail-secure 行为。
package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store/mock"
)

// TestResetToken_MarkUsedDBError 测试标记令牌已使用失败时中止密码重置
func TestResetToken_MarkUsedDBError(t *testing.T) {
	mockStore := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	userSvc := NewUserService(mockStore, passwordSvc, nil, "http://localhost")

	ctx := context.Background()
	user := &model.User{
		ID:           "user-reset-dberr",
		Email:        "reset@example.com",
		PasswordHash: "$2a$04$oldpasswordhash",
		Status:       model.UserStatusActive,
	}
	mockStore.AddUser(user)
	mockStore.StoreResetToken(ctx, user.ID, "valid-token", time.Now().Add(1*time.Hour))

	// 注入 DB 错误
	mockStore.MarkResetTokenUsedErr = errors.New("db unavailable")

	err := userSvc.ResetPassword(ctx, user.ID, "valid-token", "NewPassword123!")
	require.Error(t, err, "标记令牌已使用失败时必须中止重置")

	// 密码不得被修改
	updated, getErr := mockStore.GetByID(ctx, user.ID)
	require.NoError(t, getErr)
	assert.Equal(t, "$2a$04$oldpasswordhash", updated.PasswordHash, "DB 错误时密码不得被修改")
}

// TestMFARecovery_VerifyDBError 测试恢复码数据库错误时不得放行
func TestMFARecovery_VerifyDBError(t *testing.T) {
	mockStore := mock.New()
	mfaSvc := NewMFAService(mockStore)
	mfaSvc.SetHMACKey([]byte("0123456789abcdef0123456789abcdef"))

	ctx := context.Background()

	// 注入 DB 错误（VerifyAndUseMFARecoveryCode 返回错误）
	mockStore.VerifyAndUseMFARecoveryCodeErr = errors.New("db unavailable")

	ok, err := mfaSvc.VerifyRecoveryCode(ctx, "user-mfa", "1234-5678", "203.0.113.5")
	assert.False(t, ok, "DB 错误时不得验证通过")
	assert.ErrorIs(t, err, ErrRecoveryCodeInvalid, "DB 错误应返回恢复码无效（fail-closed）")
}

// TestMFARecovery_GetUnusedCodesDBError 测试查询未使用恢复码失败时不得放行
func TestMFARecovery_GetUnusedCodesDBError(t *testing.T) {
	mockStore := mock.New()
	mfaSvc := NewMFAService(mockStore)
	mfaSvc.SetHMACKey([]byte("0123456789abcdef0123456789abcdef"))

	ctx := context.Background()

	mockStore.GetUnusedMFARecoveryCodesErr = errors.New("db unavailable")

	ok, err := mfaSvc.VerifyRecoveryCode(ctx, "user-mfa", "1234-5678", "203.0.113.5")
	assert.False(t, ok, "DB 错误时不得验证通过")
	assert.ErrorIs(t, err, ErrRecoveryCodeInvalid, "查询失败应返回恢复码无效（fail-closed）")
}

// TestMFARecovery_StoreCodesDBError 测试存储恢复码失败时返回错误
func TestMFARecovery_StoreCodesDBError(t *testing.T) {
	mockStore := mock.New()
	mfaSvc := NewMFAService(mockStore)
	mfaSvc.SetHMACKey([]byte("0123456789abcdef0123456789abcdef"))

	ctx := context.Background()
	mockStore.StoreMFARecoveryCodesErr = errors.New("db unavailable")

	codes, err := mfaSvc.GenerateRecoveryCodes(ctx, "user-mfa", 8)
	assert.Nil(t, codes)
	require.Error(t, err, "存储恢复码失败时必须返回错误，不得让用户以为已生成")
}
