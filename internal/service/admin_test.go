// Package service_test 管理员服务单元测试
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
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
		err := adminSvc.DisableUser(ctx, "admin-operator", "disable-user-id")
		require.NoError(t, err)

		// 验证用户状态已更改
		result, err := store.GetByID(ctx, "disable-user-id")
		require.NoError(t, err)
		assert.Equal(t, "disabled", result.Status)
	})

	t.Run("禁用不存在的用户", func(t *testing.T) {
		err := adminSvc.DisableUser(ctx, "admin-operator", "nonexistent-id")
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

		// T14：Token 撤销前置，撤销失败时禁用中止并返回错误
		err := adminSvc.DisableUser(ctx, "admin-operator", "disable-revoke-fail-id")
		require.Error(t, err)

		// 验证用户状态未被更改（禁用未生效，不留"已禁用但 Token 存活"的中间状态）
		result, err := store.GetByID(ctx, "disable-revoke-fail-id")
		require.NoError(t, err)
		assert.Equal(t, model.UserStatusActive, result.Status)
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
		assert.Equal(t, "error", health.Status)
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
		err := adminSvc.DisableUser(ctx, "admin-operator", "nonexistent-id")
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

// ============================================================================
// T14：管理员操作防护测试（本人操作防护 + 末位活跃管理员保护）
// ============================================================================

// newTestAdmin 构造测试用管理员用户
func newTestAdmin(id string) *model.User {
	return &model.User{
		ID:           id,
		Email:        id + "@example.com",
		PasswordHash: "hash",
		Role:         model.UserRoleAdmin,
		Status:       model.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

func TestAdminService_DisableUser_Protection(t *testing.T) {
	ctx := context.Background()

	t.Run("禁止禁用本人账户", func(t *testing.T) {
		store := mock.New()
		_ = store.Create(ctx, newTestAdmin("admin-1"))
		_ = store.Create(ctx, newTestAdmin("admin-2"))
		adminSvc := service.NewAdminService(store)

		err := adminSvc.DisableUser(ctx, "admin-1", "admin-1")
		require.ErrorIs(t, err, apperrors.ErrSelfOperationForbidden)

		// 状态未被修改
		u, err := store.GetByID(ctx, "admin-1")
		require.NoError(t, err)
		assert.Equal(t, model.UserStatusActive, u.Status)
	})

	t.Run("禁止禁用最后一个活跃管理员", func(t *testing.T) {
		store := mock.New()
		_ = store.Create(ctx, newTestAdmin("admin-1"))
		adminSvc := service.NewAdminService(store)

		// 操作者是非管理员的审计账号等（ID 与目标不同即可）
		err := adminSvc.DisableUser(ctx, "operator", "admin-1")
		require.ErrorIs(t, err, apperrors.ErrLastActiveAdmin)

		u, err := store.GetByID(ctx, "admin-1")
		require.NoError(t, err)
		assert.Equal(t, model.UserStatusActive, u.Status)
	})

	t.Run("另有已禁用管理员时仍判定为末位活跃管理员", func(t *testing.T) {
		store := mock.New()
		_ = store.Create(ctx, newTestAdmin("admin-1"))
		disabledAdmin := newTestAdmin("admin-2")
		disabledAdmin.Status = model.UserStatusDisabled
		_ = store.Create(ctx, disabledAdmin)
		adminSvc := service.NewAdminService(store)

		// admin-2 已禁用不计入活跃数，admin-1 是唯一活跃管理员
		err := adminSvc.DisableUser(ctx, "operator", "admin-1")
		require.ErrorIs(t, err, apperrors.ErrLastActiveAdmin)
	})

	t.Run("存在其他活跃管理员时可正常禁用", func(t *testing.T) {
		store := mock.New()
		_ = store.Create(ctx, newTestAdmin("admin-1"))
		_ = store.Create(ctx, newTestAdmin("admin-2"))
		adminSvc := service.NewAdminService(store)

		err := adminSvc.DisableUser(ctx, "admin-2", "admin-1")
		require.NoError(t, err)

		u, err := store.GetByID(ctx, "admin-1")
		require.NoError(t, err)
		assert.Equal(t, model.UserStatusDisabled, u.Status)
	})

	t.Run("统计活跃管理员失败时中止禁用", func(t *testing.T) {
		store := mock.New()
		_ = store.Create(ctx, newTestAdmin("admin-1"))
		_ = store.Create(ctx, newTestAdmin("admin-2"))
		store.CountActiveAdminsErr = assert.AnError
		adminSvc := service.NewAdminService(store)

		err := adminSvc.DisableUser(ctx, "admin-2", "admin-1")
		require.Error(t, err)

		// 防护判定失败时不得继续执行禁用
		u, err := store.GetByID(ctx, "admin-1")
		require.NoError(t, err)
		assert.Equal(t, model.UserStatusActive, u.Status)
	})

	t.Run("禁用普通用户不触发管理员计数", func(t *testing.T) {
		store := mock.New()
		normalUser := &model.User{
			ID:           "normal-user",
			Email:        "normal@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleUser,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		_ = store.Create(ctx, normalUser)
		// 即使计数查询失败，普通用户禁用也不受影响（不调用 CountActiveAdmins）
		store.CountActiveAdminsErr = assert.AnError
		adminSvc := service.NewAdminService(store)

		err := adminSvc.DisableUser(ctx, "operator", "normal-user")
		require.NoError(t, err)

		u, err := store.GetByID(ctx, "normal-user")
		require.NoError(t, err)
		assert.Equal(t, model.UserStatusDisabled, u.Status)
	})
}

func TestAdminService_DeleteUser_Protection(t *testing.T) {
	ctx := context.Background()

	t.Run("禁止删除本人账户", func(t *testing.T) {
		store := mock.New()
		_ = store.Create(ctx, newTestAdmin("admin-1"))
		_ = store.Create(ctx, newTestAdmin("admin-2"))
		adminSvc := service.NewAdminService(store)

		err := adminSvc.DeleteUser(ctx, "admin-1", "admin-1")
		require.ErrorIs(t, err, apperrors.ErrSelfOperationForbidden)

		// 用户未被删除
		_, err = store.GetByID(ctx, "admin-1")
		require.NoError(t, err)
	})

	t.Run("禁止删除最后一个活跃管理员", func(t *testing.T) {
		store := mock.New()
		_ = store.Create(ctx, newTestAdmin("admin-1"))
		adminSvc := service.NewAdminService(store)

		err := adminSvc.DeleteUser(ctx, "operator", "admin-1")
		require.ErrorIs(t, err, apperrors.ErrLastActiveAdmin)

		_, err = store.GetByID(ctx, "admin-1")
		require.NoError(t, err)
	})

	t.Run("删除不存在的用户返回NotFound", func(t *testing.T) {
		store := mock.New()
		adminSvc := service.NewAdminService(store)

		err := adminSvc.DeleteUser(ctx, "operator", "nonexistent-id")
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})

	t.Run("删除用户时撤销Token失败则中止删除", func(t *testing.T) {
		store := mock.New()
		normalUser := &model.User{
			ID:           "del-revoke-fail-id",
			Email:        "del-revoke@example.com",
			PasswordHash: "hash",
			Role:         model.UserRoleUser,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		_ = store.Create(ctx, normalUser)
		store.RevokeAllUserTokensErr = assert.AnError
		adminSvc := service.NewAdminService(store)

		err := adminSvc.DeleteUser(ctx, "operator", "del-revoke-fail-id")
		require.Error(t, err)

		// 撤销失败时用户不得被删除
		_, err = store.GetByID(ctx, "del-revoke-fail-id")
		require.NoError(t, err)
	})

	t.Run("存在其他活跃管理员时可正常删除管理员", func(t *testing.T) {
		store := mock.New()
		_ = store.Create(ctx, newTestAdmin("admin-1"))
		_ = store.Create(ctx, newTestAdmin("admin-2"))
		adminSvc := service.NewAdminService(store)

		err := adminSvc.DeleteUser(ctx, "admin-2", "admin-1")
		require.NoError(t, err)

		_, err = store.GetByID(ctx, "admin-1")
		require.ErrorIs(t, err, apperrors.ErrNotFound)
	})
}
