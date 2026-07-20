//go:build integration

// Package postgres_test 社交账号存储集成测试
// 阶段 D 审查修复（覆盖率）：补充 social_account.go 的集成测试覆盖
//
// 测试覆盖：
//   - CreateSocialAccount：成功、唯一约束冲突
//   - GetSocialAccount：成功、不存在
//   - ListSocialAccountsByUserID：空、单条、多条
//   - DeleteSocialAccount：成功、不存在
//   - UpdateSocialAccount：成功、不存在
//   - CreateSocialAccountAtomic：成功、邮箱冲突、社交账号冲突
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/model"
	storepkg "github.com/example/sso/internal/store"
)

// newTestSocialAccount 构造测试用 SocialAccount
func newTestSocialAccount(userID, provider, providerUserID string) *model.SocialAccount {
	return &model.SocialAccount{
		ID:             uuid.New().String(),
		Provider:       provider,
		ProviderUserID: providerUserID,
		UserID:         userID,
		ProviderEmail:  "test-" + providerUserID + "@example.com",
		EmailVerified:  true,
		ProviderMetadata: map[string]string{
			"raw_id": providerUserID,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// ============================================================================
// CreateSocialAccount 测试
// ============================================================================

func TestStore_CreateSocialAccount_Success(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("social-create@example.com")
	require.NoError(t, store.Create(ctx, user))

	account := newTestSocialAccount(user.ID, model.ProviderGoogle, "test-google-"+uuid.New().String()[:8])
	err := store.CreateSocialAccount(ctx, account)
	assert.NoError(t, err)

	// 验证可读取
	retrieved, err := store.GetSocialAccount(ctx, account.Provider, account.ProviderUserID)
	require.NoError(t, err)
	assert.Equal(t, account.ID, retrieved.ID)
	assert.Equal(t, account.UserID, retrieved.UserID)
	assert.Equal(t, account.ProviderEmail, retrieved.ProviderEmail)
	assert.True(t, retrieved.EmailVerified)
	assert.Equal(t, account.ProviderUserID, retrieved.ProviderMetadata["raw_id"])
}

func TestStore_CreateSocialAccount_Conflict(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("social-conflict@example.com")
	require.NoError(t, store.Create(ctx, user))

	providerUserID := "test-google-" + uuid.New().String()[:8]
	account1 := newTestSocialAccount(user.ID, model.ProviderGoogle, providerUserID)
	require.NoError(t, store.CreateSocialAccount(ctx, account1))

	// 同一 (provider, provider_user_id) 应冲突
	account2 := newTestSocialAccount(user.ID, model.ProviderGoogle, providerUserID)
	account2.ID = uuid.New().String() // 不同的 ID
	err := store.CreateSocialAccount(ctx, account2)
	assert.ErrorIs(t, err, storepkg.ErrSocialAccountConflict)

	// 同一 (user_id, provider) 应冲突（不同 provider_user_id）
	account3 := newTestSocialAccount(user.ID, model.ProviderGoogle, "test-google-different-"+uuid.New().String()[:8])
	err = store.CreateSocialAccount(ctx, account3)
	assert.ErrorIs(t, err, storepkg.ErrSocialAccountConflict)
}

// ============================================================================
// GetSocialAccount 测试
// ============================================================================

func TestStore_GetSocialAccount_NotFound(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	_, err := store.GetSocialAccount(ctx, model.ProviderGoogle, "test-not-exist-"+uuid.New().String())
	assert.ErrorIs(t, err, storepkg.ErrNotFound)
}

// ============================================================================
// ListSocialAccountsByUserID 测试
// ============================================================================

func TestStore_ListSocialAccountsByUserID(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("social-list@example.com")
	require.NoError(t, store.Create(ctx, user))

	t.Run("空列表", func(t *testing.T) {
		accounts, err := store.ListSocialAccountsByUserID(ctx, user.ID)
		require.NoError(t, err)
		assert.Empty(t, accounts)
	})

	t.Run("单条记录", func(t *testing.T) {
		account := newTestSocialAccount(user.ID, model.ProviderGoogle, "test-list-google-"+uuid.New().String()[:8])
		require.NoError(t, store.CreateSocialAccount(ctx, account))

		accounts, err := store.ListSocialAccountsByUserID(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, accounts, 1)
		assert.Equal(t, account.ID, accounts[0].ID)
	})

	t.Run("多条记录（不同 provider）", func(t *testing.T) {
		// 上面已经绑定了 google，再绑定 github
		account := newTestSocialAccount(user.ID, model.ProviderGitHub, "test-list-github-"+uuid.New().String()[:8])
		require.NoError(t, store.CreateSocialAccount(ctx, account))

		accounts, err := store.ListSocialAccountsByUserID(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, accounts, 2)
	})
}

// ============================================================================
// DeleteSocialAccount 测试
// ============================================================================

func TestStore_DeleteSocialAccount(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("social-delete@example.com")
	require.NoError(t, store.Create(ctx, user))

	account := newTestSocialAccount(user.ID, model.ProviderGoogle, "test-delete-"+uuid.New().String()[:8])
	require.NoError(t, store.CreateSocialAccount(ctx, account))

	t.Run("成功删除", func(t *testing.T) {
		err := store.DeleteSocialAccount(ctx, account.Provider, account.ProviderUserID)
		assert.NoError(t, err)

		// 验证已删除
		_, err = store.GetSocialAccount(ctx, account.Provider, account.ProviderUserID)
		assert.ErrorIs(t, err, storepkg.ErrNotFound)
	})

	t.Run("删除不存在的记录", func(t *testing.T) {
		err := store.DeleteSocialAccount(ctx, account.Provider, "test-not-exist-"+uuid.New().String())
		assert.ErrorIs(t, err, storepkg.ErrNotFound)
	})
}

// ============================================================================
// UpdateSocialAccount 测试
// ============================================================================

func TestStore_UpdateSocialAccount(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("social-update@example.com")
	require.NoError(t, store.Create(ctx, user))

	account := newTestSocialAccount(user.ID, model.ProviderGoogle, "test-update-"+uuid.New().String()[:8])
	require.NoError(t, store.CreateSocialAccount(ctx, account))

	t.Run("成功更新", func(t *testing.T) {
		account.ProviderEmail = "updated-" + account.ProviderEmail
		account.EmailVerified = false
		account.ProviderMetadata = map[string]string{"updated": "true"}
		account.UpdatedAt = time.Now()

		err := store.UpdateSocialAccount(ctx, account)
		assert.NoError(t, err)

		retrieved, err := store.GetSocialAccount(ctx, account.Provider, account.ProviderUserID)
		require.NoError(t, err)
		assert.Equal(t, "updated-", retrieved.ProviderEmail[:8])
		assert.False(t, retrieved.EmailVerified)
		assert.Equal(t, "true", retrieved.ProviderMetadata["updated"])

		// user_id 不应被修改（防止账号接管）
		assert.Equal(t, user.ID, retrieved.UserID)
	})

	t.Run("更新不存在的记录", func(t *testing.T) {
		fakeAccount := newTestSocialAccount(user.ID, model.ProviderGitHub, "test-not-exist-"+uuid.New().String())
		err := store.UpdateSocialAccount(ctx, fakeAccount)
		assert.ErrorIs(t, err, storepkg.ErrNotFound)
	})
}

// ============================================================================
// CreateSocialAccountAtomic 测试
// ============================================================================

func TestStore_CreateSocialAccountAtomic_Success(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("social-atomic@example.com")
	account := newTestSocialAccount(user.ID, model.ProviderGoogle, "test-atomic-"+uuid.New().String()[:8])

	err := store.CreateSocialAccountAtomic(ctx, user, account)
	assert.NoError(t, err)

	// 验证用户已创建
	retrievedUser, err := store.GetByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.Email, retrievedUser.Email)

	// 验证社交账号已创建
	retrievedAccount, err := store.GetSocialAccount(ctx, account.Provider, account.ProviderUserID)
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrievedAccount.UserID)
}

func TestStore_CreateSocialAccountAtomic_DuplicateEmail(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	// 先创建一个用户
	existingUser := newTestUser("social-atomic-dup@example.com")
	require.NoError(t, store.Create(ctx, existingUser))

	// 再用同一 email 创建用户 + 社交账号 → 邮箱冲突
	newUser := newTestUser("social-atomic-dup@example.com")
	account := newTestSocialAccount(newUser.ID, model.ProviderGoogle, "test-atomic-dup-"+uuid.New().String()[:8])

	err := store.CreateSocialAccountAtomic(ctx, newUser, account)
	assert.ErrorIs(t, err, storepkg.ErrDuplicateEmail)

	// 事务应回滚，社交账号不应存在
	_, err = store.GetSocialAccount(ctx, account.Provider, account.ProviderUserID)
	assert.ErrorIs(t, err, storepkg.ErrNotFound)
}

func TestStore_CreateSocialAccountAtomic_SocialConflict(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	// 先创建一个用户 + 社交账号
	originalUser := newTestUser("social-atomic-conflict-1@example.com")
	require.NoError(t, store.Create(ctx, originalUser))

	providerUserID := "test-atomic-conflict-" + uuid.New().String()[:8]
	originalAccount := newTestSocialAccount(originalUser.ID, model.ProviderGoogle, providerUserID)
	require.NoError(t, store.CreateSocialAccount(ctx, originalAccount))

	// 再用不同 email 创建用户 + 同一 (provider, provider_user_id) → 冲突
	newUser := newTestUser("social-atomic-conflict-2@example.com")
	newAccount := newTestSocialAccount(newUser.ID, model.ProviderGoogle, providerUserID)

	err := store.CreateSocialAccountAtomic(ctx, newUser, newAccount)
	assert.ErrorIs(t, err, storepkg.ErrSocialAccountConflict)

	// 事务应回滚，新用户不应存在
	_, err = store.GetByEmail(ctx, newUser.Email)
	assert.ErrorIs(t, err, storepkg.ErrNotFound)
}
