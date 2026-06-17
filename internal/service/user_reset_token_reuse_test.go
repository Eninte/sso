// Package service 密码重置令牌重用防护测试
package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store/mock"
)

// TestResetToken_PreventReuse 测试密码重置令牌不能被重复使用
func TestResetToken_PreventReuse(t *testing.T) {
	mockStore := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	userSvc := NewUserService(mockStore, passwordSvc, nil, "http://localhost")

	ctx := context.Background()

	// 创建测试用户
	user := &model.User{
		ID:           "user-123",
		Email:        "test@example.com",
		PasswordHash: "$2a$04$oldpasswordhash",
		Status:       model.UserStatusActive,
	}
	mockStore.AddUser(user)

	// 存储重置令牌
	token := "valid-reset-token"
	mockStore.StoreResetToken(ctx, user.ID, token, time.Now().Add(1*time.Hour))

	t.Run("第一次使用令牌_成功", func(t *testing.T) {
		err := userSvc.ResetPassword(ctx, user.ID, token, "NewPassword123!")
		assert.NoError(t, err)

		// 验证密码已更新
		updatedUser, err := mockStore.GetByID(ctx, user.ID)
		require.NoError(t, err)
		assert.NotEqual(t, "$2a$04$oldpasswordhash", updatedUser.PasswordHash)
	})

	t.Run("第二次使用相同令牌_失败", func(t *testing.T) {
		// 重新存储令牌并标记为已使用
		now := time.Now()
		mockStore.StoreResetToken(ctx, user.ID, token, time.Now().Add(1*time.Hour))
		storedToken, _ := mockStore.GetResetToken(ctx, user.ID)
		storedToken.UsedAt = &now

		err := userSvc.ResetPassword(ctx, user.ID, token, "AnotherPassword456!")
		assert.ErrorIs(t, err, ErrResetTokenUsed)
	})
}

// TestResetToken_ConcurrentUse 测试并发使用令牌的情况
func TestResetToken_ConcurrentUse(t *testing.T) {
	mockStore := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	userSvc := NewUserService(mockStore, passwordSvc, nil, "http://localhost")

	ctx := context.Background()

	// 创建测试用户
	user := &model.User{
		ID:           "user-concurrent",
		Email:        "concurrent@example.com",
		PasswordHash: "$2a$04$oldpasswordhash",
		Status:       model.UserStatusActive,
	}
	mockStore.AddUser(user)

	t.Run("模拟并发请求", func(t *testing.T) {
		// 存储重置令牌
		token := "concurrent-token"
		mockStore.StoreResetToken(ctx, user.ID, token, time.Now().Add(1*time.Hour))

		// 第一个请求成功
		err1 := userSvc.ResetPassword(ctx, user.ID, token, "Xk9#mP2$vL7!")
		assert.NoError(t, err1)

		// 重新存储令牌并模拟第二个并发请求（令牌已被标记为使用）
		mockStore.StoreResetToken(ctx, user.ID, token, time.Now().Add(1*time.Hour))
		storedToken, _ := mockStore.GetResetToken(ctx, user.ID)
		now := time.Now()
		storedToken.UsedAt = &now

		err2 := userSvc.ResetPassword(ctx, user.ID, token, "Password2!")
		assert.ErrorIs(t, err2, ErrResetTokenUsed)
	})
}

// TestResetToken_UsedAtField 测试UsedAt字段的正确性
func TestResetToken_UsedAtField(t *testing.T) {
	mockStore := mock.New()
	ctx := context.Background()

	userID := "user-usedat"
	token := "test-token"

	t.Run("新令牌UsedAt为NULL", func(t *testing.T) {
		err := mockStore.StoreResetToken(ctx, userID, token, time.Now().Add(1*time.Hour))
		require.NoError(t, err)

		storedToken, err := mockStore.GetResetToken(ctx, userID)
		require.NoError(t, err)
		assert.Nil(t, storedToken.UsedAt, "新令牌的UsedAt应该为NULL")
	})

	t.Run("标记令牌为已使用", func(t *testing.T) {
		err := mockStore.MarkResetTokenUsed(ctx, userID)
		require.NoError(t, err)

		storedToken, err := mockStore.GetResetToken(ctx, userID)
		require.NoError(t, err)
		assert.NotNil(t, storedToken.UsedAt, "已使用令牌的UsedAt应该不为NULL")
	})

	t.Run("重复标记已使用的令牌_失败", func(t *testing.T) {
		err := mockStore.MarkResetTokenUsed(ctx, userID)
		assert.Error(t, err, "重复标记应该返回错误")
	})
}

// TestResetToken_ExpiredAndUsed 测试过期且已使用的令牌
func TestResetToken_ExpiredAndUsed(t *testing.T) {
	mockStore := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	userSvc := NewUserService(mockStore, passwordSvc, nil, "http://localhost")

	ctx := context.Background()

	user := &model.User{
		ID:           "user-expired-used",
		Email:        "expired@example.com",
		PasswordHash: "$2a$04$oldpasswordhash",
		Status:       model.UserStatusActive,
	}
	mockStore.AddUser(user)

	t.Run("已使用的令牌_优先检查", func(t *testing.T) {
		// 存储一个已使用的令牌（即使未过期）
		token := "used-token"
		mockStore.StoreResetToken(ctx, user.ID, token, time.Now().Add(1*time.Hour))
		now := time.Now()
		storedToken, _ := mockStore.GetResetToken(ctx, user.ID)
		storedToken.UsedAt = &now

		err := userSvc.ResetPassword(ctx, user.ID, token, "NewPassword123!")
		assert.ErrorIs(t, err, ErrResetTokenUsed, "应该优先检查令牌是否已使用")
	})

	t.Run("过期的令牌_未使用", func(t *testing.T) {
		// 存储一个过期但未使用的令牌
		token := "expired-token"
		mockStore.StoreResetToken(ctx, user.ID, token, time.Now().Add(-1*time.Hour))

		err := userSvc.ResetPassword(ctx, user.ID, token, "NewPassword123!")
		assert.ErrorIs(t, err, ErrResetTokenExpired, "过期令牌应该返回过期错误")
	})
}
