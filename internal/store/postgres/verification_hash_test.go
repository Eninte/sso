//go:build integration

// Package postgres_test 验证/重置令牌哈希存储集成测试
// T2 安全修复（H2）：verification_tokens / reset_tokens 的 token 列只存
// SHA-256 hex（64 位），明文不落库；读取返回哈希值
package postgres_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/common"
)

// hexHashPattern 匹配 64 位小写 hex（SHA-256）
var hexHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// TestStore_VerificationToken_HashStorage 验证邮箱验证令牌只存 hash
func TestStore_VerificationToken_HashStorage(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("verify-hash@example.com")
	require.NoError(t, store.Create(ctx, user))

	plainToken := "verify-plain-" + common.HashToken("seed-verify")[:24]
	expiresAt := time.Now().Add(15 * time.Minute)
	require.NoError(t, store.StoreVerificationToken(ctx, user.ID, plainToken, expiresAt))

	// 直连 DB 断言：token 列为 64 位 hex 且等于明文的 SHA-256，不等于明文
	var dbToken string
	err := db.QueryRowContext(ctx,
		"SELECT token FROM verification_tokens WHERE user_id = $1", user.ID,
	).Scan(&dbToken)
	require.NoError(t, err)
	assert.True(t, hexHashPattern.MatchString(dbToken), "token 列应为 64 位 hex，实际: %s", dbToken)
	assert.NotEqual(t, plainToken, dbToken, "token 列不得包含明文")
	assert.Equal(t, common.HashToken(plainToken), dbToken)

	// GetVerificationToken 返回哈希值
	got, err := store.GetVerificationToken(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, common.HashToken(plainToken), got.Token)
	assert.NotEqual(t, plainToken, got.Token)

	// 清理
	require.NoError(t, store.DeleteVerificationToken(ctx, user.ID))
}

// TestStore_ResetToken_HashStorage 验证密码重置令牌只存 hash
func TestStore_ResetToken_HashStorage(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("reset-hash@example.com")
	require.NoError(t, store.Create(ctx, user))

	plainToken := "reset-plain-" + common.HashToken("seed-reset")[:24]
	expiresAt := time.Now().Add(1 * time.Hour)
	require.NoError(t, store.StoreResetToken(ctx, user.ID, plainToken, expiresAt))

	// 直连 DB 断言：token 列为 64 位 hex 且等于明文的 SHA-256，不等于明文
	var dbToken string
	err := db.QueryRowContext(ctx,
		"SELECT token FROM reset_tokens WHERE user_id = $1", user.ID,
	).Scan(&dbToken)
	require.NoError(t, err)
	assert.True(t, hexHashPattern.MatchString(dbToken), "token 列应为 64 位 hex，实际: %s", dbToken)
	assert.NotEqual(t, plainToken, dbToken, "token 列不得包含明文")
	assert.Equal(t, common.HashToken(plainToken), dbToken)

	// GetResetToken 返回哈希值，MarkResetTokenUsed 流程不受影响
	got, err := store.GetResetToken(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, common.HashToken(plainToken), got.Token)
	assert.Nil(t, got.UsedAt)

	require.NoError(t, store.MarkResetTokenUsed(ctx, user.ID))
	got2, err := store.GetResetToken(ctx, user.ID)
	require.NoError(t, err)
	assert.NotNil(t, got2.UsedAt, "used_at 必须已设置")

	// 重复存储（先删后插路径）仍只存 hash
	newPlain := "reset-plain2-" + common.HashToken("seed-reset2")[:24]
	require.NoError(t, store.StoreResetToken(ctx, user.ID, newPlain, expiresAt))
	err = db.QueryRowContext(ctx,
		"SELECT token FROM reset_tokens WHERE user_id = $1", user.ID,
	).Scan(&dbToken)
	require.NoError(t, err)
	assert.Equal(t, common.HashToken(newPlain), dbToken)

	// 清理
	require.NoError(t, store.DeleteResetToken(ctx, user.ID))
}
