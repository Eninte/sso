//go:build integration

// Package postgres CountActiveAdmins 集成测试
package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store/postgres"
)

// TestStore_CountActiveAdmins 测试活跃管理员计数
// 覆盖 T14 新增的 COUNT 查询路径。
// 注意：测试库可能已存在其他管理员数据（如 E2E 准备脚本创建），
// 因此一律使用"增量"断言而非绝对值断言
func TestStore_CountActiveAdmins(t *testing.T) {
	s, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	// 记录基线（测试库中可能已有活跃管理员）
	baseline, err := s.CountActiveAdmins(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, baseline, 0)

	// 构造两个活跃管理员
	admin1 := newTestUser("countadmin1@example.com")
	admin1.Role = model.UserRoleAdmin
	require.NoError(t, s.Create(ctx, admin1))

	admin2 := newTestUser("countadmin2@example.com")
	admin2.Role = model.UserRoleAdmin
	require.NoError(t, s.Create(ctx, admin2))

	t.Run("新增活跃管理员后计数增加", func(t *testing.T) {
		count, err := s.CountActiveAdmins(ctx)
		require.NoError(t, err)
		assert.Equal(t, baseline+2, count, "新增 2 个活跃管理员后计数应增加 2")
	})

	t.Run("禁用的管理员不计入活跃数", func(t *testing.T) {
		// 禁用 admin2
		admin2.Status = model.UserStatusDisabled
		require.NoError(t, s.Update(ctx, admin2))

		count, err := s.CountActiveAdmins(ctx)
		require.NoError(t, err)
		assert.Equal(t, baseline+1, count, "禁用后活跃管理员计数应减少 1")
	})

	t.Run("普通用户不计入管理员数", func(t *testing.T) {
		normalUser := newTestUser("countnormal@example.com")
		normalUser.Role = model.UserRoleUser
		require.NoError(t, s.Create(ctx, normalUser))

		count, err := s.CountActiveAdmins(ctx)
		require.NoError(t, err)
		assert.Equal(t, baseline+1, count, "新增普通用户不应影响管理员计数")
	})
}

// 确保 postgres 包被引用（避免未使用导入）
var _ = postgres.New
