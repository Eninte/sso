// Package postgres ExistsUserByRole 集成测试
package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store/postgres"
)

// TestStore_ExistsUserByRole 测试按角色查询用户是否存在
// 覆盖 #3 新增的 EXISTS 查询路径，确保不返回全表数据
func TestStore_ExistsUserByRole(t *testing.T) {
	s, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	// 构造一个 admin 角色用户
	adminUser := newTestUser("existsadmin@example.com")
	adminUser.Role = model.UserRoleAdmin
	require.NoError(t, s.Create(ctx, adminUser))

	// 构造一个普通用户
	normalUser := newTestUser("existsnormal@example.com")
	normalUser.Role = model.UserRoleUser
	require.NoError(t, s.Create(ctx, normalUser))

	t.Run("存在admin用户_返回true", func(t *testing.T) {
		exists, err := s.ExistsUserByRole(ctx, model.UserRoleAdmin)
		require.NoError(t, err)
		assert.True(t, exists, "已存在 admin 角色用户应返回 true")
	})

	t.Run("存在普通用户_返回true", func(t *testing.T) {
		exists, err := s.ExistsUserByRole(ctx, model.UserRoleUser)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("不存在的角色_返回false", func(t *testing.T) {
		// 使用一个不会出现的角色值
		exists, err := s.ExistsUserByRole(ctx, "nonexistent-role-xyz")
		require.NoError(t, err)
		assert.False(t, exists, "不存在的角色应返回 false 而非错误")
	})
}

// TestStore_ExistsUserByRole_NoMatchingRole 无匹配角色的场景
// 验证"不存在"语义返回 (false, nil) 而非错误
func TestStore_ExistsUserByRole_NoMatchingRole(t *testing.T) {
	s, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	// 使用一个不会出现的角色值
	exists, err := s.ExistsUserByRole(ctx, "unique-no-such-role-"+t.Name())
	require.NoError(t, err)
	assert.False(t, exists, "无匹配角色的查询应返回 false 而非错误")
}

// 确保 postgres 包被引用（避免未使用导入）
var _ = postgres.New
