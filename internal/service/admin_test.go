// Package service_test 管理员服务单元测试
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/cache"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// AdminService 测试
// ============================================================================

func TestNewAdminService(t *testing.T) {
	store := mock.New()
	adminSvc := service.NewAdminService(store)
	assert.NotNil(t, adminSvc)
}

func TestAdminService_ListUsers(t *testing.T) {
	store := mock.New()
	adminSvc := service.NewAdminService(store)
	ctx := context.Background()

	// 添加测试用户
	user1 := &model.User{
		ID:           "user-1",
		Email:        "user1@example.com",
		PasswordHash: "hash",
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	user2 := &model.User{
		ID:           "user-2",
		Email:        "user2@example.com",
		PasswordHash: "hash",
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_ = store.Create(ctx, user1)
	_ = store.Create(ctx, user2)

	t.Run("成功列出用户", func(t *testing.T) {
		users, total, err := adminSvc.ListUsers(ctx, 0, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 2)
		assert.GreaterOrEqual(t, len(users), 2)
	})

	t.Run("分页查询", func(t *testing.T) {
		users, _, err := adminSvc.ListUsers(ctx, 0, 1)
		require.NoError(t, err)
		assert.Equal(t, 1, len(users))
	})
}

func TestAdminService_GetUser(t *testing.T) {
	store := mock.New()
	adminSvc := service.NewAdminService(store)
	ctx := context.Background()

	// 添加测试用户
	user := &model.User{
		ID:           "test-user-id",
		Email:        "test@example.com",
		PasswordHash: "hash",
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_ = store.Create(ctx, user)

	t.Run("成功获取用户", func(t *testing.T) {
		result, err := adminSvc.GetUser(ctx, "test-user-id")
		require.NoError(t, err)
		assert.Equal(t, "test-user-id", result.ID)
		assert.Equal(t, "test@example.com", result.Email)
	})

	t.Run("用户不存在", func(t *testing.T) {
		_, err := adminSvc.GetUser(ctx, "nonexistent-id")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, apperrors.ErrNotFound))
	})
}

func TestAdminService_DisableUser(t *testing.T) {
	store := mock.New()
	adminSvc := service.NewAdminService(store)
	ctx := context.Background()

	// 添加测试用户
	user := &model.User{
		ID:           "disable-user-id",
		Email:        "disable@example.com",
		PasswordHash: "hash",
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_ = store.Create(ctx, user)

	t.Run("成功禁用用户", func(t *testing.T) {
		err := adminSvc.DisableUser(ctx, "disable-user-id")
		require.NoError(t, err)

		// 验证用户状态已更改
		result, err := store.GetByID(ctx, "disable-user-id")
		require.NoError(t, err)
		assert.Equal(t, "disabled", result.Status)
	})

	t.Run("禁用不存在的用户", func(t *testing.T) {
		err := adminSvc.DisableUser(ctx, "nonexistent-id")
		assert.Error(t, err)
	})

	t.Run("禁用用户时撤销Token失败", func(t *testing.T) {
		store := mock.New()
		store.RevokeAllUserTokensErr = assert.AnError
		adminSvc := service.NewAdminService(store)
		ctx := context.Background()

		// 添加测试用户
		user := &model.User{
			ID:           "disable-revoke-fail-id",
			Email:        "disable-revoke@example.com",
			PasswordHash: "hash",
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		_ = store.Create(ctx, user)

		// 撤销失败不应影响主流程
		err := adminSvc.DisableUser(ctx, "disable-revoke-fail-id")
		require.NoError(t, err)

		// 验证用户状态已更改（主流程成功）
		result, err := store.GetByID(ctx, "disable-revoke-fail-id")
		require.NoError(t, err)
		assert.Equal(t, "disabled", result.Status)
	})
}

func TestAdminService_EnableUser(t *testing.T) {
	store := mock.New()
	adminSvc := service.NewAdminService(store)
	ctx := context.Background()

	// 添加禁用的用户
	user := &model.User{
		ID:            "enable-user-id",
		Email:         "enable@example.com",
		PasswordHash:  "hash",
		Status:        "disabled",
		LoginAttempts: 5,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	_ = store.Create(ctx, user)

	t.Run("成功启用用户", func(t *testing.T) {
		err := adminSvc.EnableUser(ctx, "enable-user-id")
		require.NoError(t, err)

		// 验证用户状态和登录尝试次数已重置
		result, err := store.GetByID(ctx, "enable-user-id")
		require.NoError(t, err)
		assert.Equal(t, model.UserStatusActive, result.Status)
		assert.Equal(t, 0, result.LoginAttempts)
		assert.Nil(t, result.LockedUntil)
	})

	t.Run("启用不存在的用户", func(t *testing.T) {
		err := adminSvc.EnableUser(ctx, "nonexistent-id")
		assert.Error(t, err)
	})
}

func TestAdminService_SystemHealth(t *testing.T) {
	store := mock.New()
	adminSvc := service.NewAdminService(store)
	ctx := context.Background()

	t.Run("获取系统健康状态", func(t *testing.T) {
		health, err := adminSvc.SystemHealth(ctx)
		require.NoError(t, err)
		assert.Equal(t, "ok", health.Status)
		assert.Equal(t, "ok", health.Database)
		assert.Equal(t, "dev", health.Version)
		assert.Equal(t, "unknown", health.BuildTime)
		assert.NotZero(t, health.Timestamp)
	})

	t.Run("数据库连接失败", func(t *testing.T) {
		store := mock.New()
		store.PingErr = assert.AnError
		adminSvc := service.NewAdminService(store)

		health, err := adminSvc.SystemHealth(ctx)
		require.NoError(t, err)
		assert.Equal(t, "ok", health.Status)
		assert.Equal(t, "error", health.Database)
	})
}

func TestAdminService_CleanupExpired(t *testing.T) {
	store := mock.New()
	adminSvc := service.NewAdminService(store)
	ctx := context.Background()

	t.Run("清理过期数据", func(t *testing.T) {
		err := adminSvc.CleanupExpired(ctx)
		assert.NoError(t, err)
	})
}

// ============================================================================
// DisableUser 不存在测试
// ============================================================================

func TestAdminService_DisableUser_NotFound(t *testing.T) {
	store := mock.New()
	adminSvc := service.NewAdminService(store)
	ctx := context.Background()

	t.Run("禁用不存在的用户", func(t *testing.T) {
		err := adminSvc.DisableUser(ctx, "nonexistent-id")
		assert.Error(t, err)
	})
}

// ============================================================================
// AdminService with cache 测试
// ============================================================================

func TestAdminService_GetUser_WithCache(t *testing.T) {
	store := mock.New()
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	adminSvc := service.NewAdminServiceWithCache(store, memCache)
	ctx := context.Background()

	// 添加测试用户
	user := &model.User{
		ID:           "cache-user-id",
		Email:        "cacheuser@example.com",
		PasswordHash: "hash",
		Role:         model.UserRoleUser,
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_ = store.Create(ctx, user)

	t.Run("首次获取用户（缓存未命中）", func(t *testing.T) {
		result, err := adminSvc.GetUser(ctx, "cache-user-id")
		require.NoError(t, err)
		assert.Equal(t, "cache-user-id", result.ID)
	})

	t.Run("再次获取用户（缓存命中）", func(t *testing.T) {
		result, err := adminSvc.GetUser(ctx, "cache-user-id")
		require.NoError(t, err)
		assert.Equal(t, "cache-user-id", result.ID)
	})
}
