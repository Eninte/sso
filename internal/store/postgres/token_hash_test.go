//go:build integration

// Package postgres_test Token hash 存储集成测试
// 阶段 D 审查修复（H6）：验证 Token hash 字段在 Postgres 中的正确存储与查询
//
// 测试覆盖：
//   - StoreToken 时 access_token_hash / refresh_token_hash 自动计算并存储
//   - GetTokenByAccessToken / GetTokenByRefreshToken 通过 hash 查询命中
//   - RevokeToken 通过 hash 查询定位并撤销
//   - RotateRefreshToken 原子轮换使用 hash 查询定位旧 token
//   - 兼容旧数据：hash 字段为 NULL 时回退到明文查询
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/common"
	"github.com/example/sso/internal/model"
	storepkg "github.com/example/sso/internal/store"
)

// TestStore_TokenHash_StoreAndQuery 验证 StoreToken 自动计算 hash 并通过 hash 查询
// 阶段 D 修复（H6）：覆盖 hash 自动计算 + 通过 hash 查询的核心路径
func TestStore_TokenHash_StoreAndQuery(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("hash-token@example.com")
	require.NoError(t, store.Create(ctx, user))

	uniqueClientID := "test-hash-" + uuid.New().String()[:8]
	testClient := &model.Client{
		ID:           uuid.New().String(),
		ClientID:     uniqueClientID,
		ClientSecret: "secret",
		Name:         "Hash Test Client",
		RedirectURIs: []string{"http://localhost"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.CreateClient(ctx, testClient))

	accessToken := "test-hash-access-" + uuid.New().String()
	refreshToken := "test-hash-refresh-" + uuid.New().String()

	token := &model.Token{
		ID:           uuid.New().String(),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		UserID:       user.ID,
		ClientID:     ptrTo(uniqueClientID),
		Scopes:       []string{"openid"},
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.StoreToken(ctx, token))

	// 验证 hash 字段已存储
	var accessHash, refreshHash *string
	err := db.QueryRowContext(ctx,
		"SELECT access_token_hash, refresh_token_hash FROM tokens WHERE id = $1",
		token.ID,
	).Scan(&accessHash, &refreshHash)
	require.NoError(t, err)
	require.NotNil(t, accessHash, "access_token_hash 必须已存储")
	require.NotNil(t, refreshHash, "refresh_token_hash 必须已存储")
	assert.Equal(t, common.HashToken(accessToken), *accessHash)
	assert.Equal(t, common.HashToken(refreshToken), *refreshHash)

	// 通过 access_token 查询（应命中 hash 索引）
	retrieved, err := store.GetTokenByAccessToken(ctx, accessToken)
	require.NoError(t, err)
	assert.Equal(t, token.ID, retrieved.ID)
	assert.Equal(t, accessToken, retrieved.AccessToken)

	// 通过 refresh_token 查询（应命中 hash 索引）
	retrieved2, err := store.GetTokenByRefreshToken(ctx, refreshToken)
	require.NoError(t, err)
	assert.Equal(t, token.ID, retrieved2.ID)
	assert.Equal(t, refreshToken, retrieved2.RefreshToken)
}

// TestStore_TokenHash_RevokeByHash 验证 RevokeToken 通过 hash 定位记录
// 阶段 D 修复（H6）：RevokeToken 应通过 hash 索引执行 UPDATE，避免明文出现在 WHERE
func TestStore_TokenHash_RevokeByHash(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("hash-revoke@example.com")
	require.NoError(t, store.Create(ctx, user))

	uniqueClientID := "test-hash-revoke-" + uuid.New().String()[:8]
	testClient := &model.Client{
		ID:           uuid.New().String(),
		ClientID:     uniqueClientID,
		ClientSecret: "secret",
		Name:         "Hash Revoke Client",
		RedirectURIs: []string{"http://localhost"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.CreateClient(ctx, testClient))

	accessToken := "test-revoke-hash-access-" + uuid.New().String()
	token := &model.Token{
		ID:           uuid.New().String(),
		AccessToken:  accessToken,
		RefreshToken: "test-revoke-hash-refresh-" + uuid.New().String(),
		UserID:       user.ID,
		ClientID:     ptrTo(uniqueClientID),
		Scopes:       []string{"openid"},
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.StoreToken(ctx, token))

	// 通过 access_token 撤销（应通过 hash 定位）
	require.NoError(t, store.RevokeToken(ctx, accessToken))

	// 验证 revoked_at 已设置
	retrieved, err := store.GetTokenByAccessToken(ctx, accessToken)
	require.NoError(t, err)
	require.NotNil(t, retrieved.RevokedAt, "revoked_at 必须已设置")
}

// TestStore_TokenHash_RotateByHash 验证 RotateRefreshToken 通过 hash 定位旧 token
// 阶段 D 修复（H6）：原子轮换的 UPDATE...WHERE rotated_at IS NULL 应通过 hash 索引执行
func TestStore_TokenHash_RotateByHash(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("hash-rotate@example.com")
	require.NoError(t, store.Create(ctx, user))

	uniqueClientID := "test-hash-rotate-" + uuid.New().String()[:8]
	testClient := &model.Client{
		ID:           uuid.New().String(),
		ClientID:     uniqueClientID,
		ClientSecret: "secret",
		Name:         "Hash Rotate Client",
		RedirectURIs: []string{"http://localhost"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.CreateClient(ctx, testClient))

	oldRefresh := "test-rotate-old-refresh-" + uuid.New().String()
	refreshExpiresAt := time.Now().Add(24 * time.Hour)

	oldToken := &model.Token{
		ID:               uuid.New().String(),
		AccessToken:      "test-rotate-old-access-" + uuid.New().String(),
		RefreshToken:     oldRefresh,
		UserID:           user.ID,
		ClientID:         ptrTo(uniqueClientID),
		Scopes:           []string{"openid"},
		ExpiresAt:        time.Now().Add(15 * time.Minute),
		RefreshExpiresAt: &refreshExpiresAt,
		CreatedAt:        time.Now(),
	}
	require.NoError(t, store.StoreToken(ctx, oldToken))

	newToken := &model.Token{
		ID:               uuid.New().String(),
		AccessToken:      "test-rotate-new-access-" + uuid.New().String(),
		RefreshToken:     "test-rotate-new-refresh-" + uuid.New().String(),
		UserID:           user.ID,
		ClientID:         ptrTo(uniqueClientID),
		Scopes:           []string{"openid"},
		ExpiresAt:        time.Now().Add(15 * time.Minute),
		RefreshExpiresAt: &refreshExpiresAt,
		CreatedAt:        time.Now(),
	}

	// 通过 refresh_token 轮换（应通过 hash 定位旧 token）
	err := store.RotateRefreshToken(ctx, oldRefresh, newToken)
	require.NoError(t, err)

	// 验证旧 token 已标记轮换
	oldRetrieved, err := store.GetTokenByAccessToken(ctx, oldToken.AccessToken)
	require.NoError(t, err)
	require.NotNil(t, oldRetrieved.RotatedAt, "rotated_at 必须已设置")
	require.NotNil(t, oldRetrieved.RevokedAt, "revoked_at 必须已设置")
	require.NotNil(t, oldRetrieved.ReplacedByTokenID, "replaced_by_token_id 必须已设置")
	assert.Equal(t, newToken.ID, *oldRetrieved.ReplacedByTokenID)

	// 验证第二次轮换返回 ErrTokenRotated（已轮换的 token 不能再次轮换）
	err = store.RotateRefreshToken(ctx, oldRefresh, newToken)
	assert.ErrorIs(t, err, storepkg.ErrTokenRotated)
}

// TestStore_TokenHash_LegacyDataFallback 验证兼容旧数据：hash 字段为 NULL 时回退到明文查询
// 阶段 D 修复（H6）：保证迁移期间旧数据仍可查询
func TestStore_TokenHash_LegacyDataFallback(t *testing.T) {
	store, db := setupTestStore(t)
	t.Cleanup(func() {
		cleanupTestData(t, db)
		db.Close()
	})
	ctx := context.Background()

	user := newTestUser("hash-legacy@example.com")
	require.NoError(t, store.Create(ctx, user))

	uniqueClientID := "test-hash-legacy-" + uuid.New().String()[:8]
	testClient := &model.Client{
		ID:           uuid.New().String(),
		ClientID:     uniqueClientID,
		ClientSecret: "secret",
		Name:         "Hash Legacy Client",
		RedirectURIs: []string{"http://localhost"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.CreateClient(ctx, testClient))

	// 直接插入一行 hash 字段为 NULL 的记录，模拟旧数据
	accessToken := "test-legacy-access-" + uuid.New().String()
	refreshToken := "test-legacy-refresh-" + uuid.New().String()
	tokenID := uuid.New().String()
	_, err := db.ExecContext(ctx, `
		INSERT INTO tokens (id, access_token, refresh_token, user_id, client_id, scopes, expires_at, created_at, access_token_hash, refresh_token_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, NULL)
	`,
		tokenID, accessToken, refreshToken, user.ID, uniqueClientID,
		[]string{"openid"},
		time.Now().Add(1*time.Hour), time.Now(),
	)
	require.NoError(t, err)

	// 通过 access_token 查询（hash 为 NULL 应回退到明文查询）
	retrieved, err := store.GetTokenByAccessToken(ctx, accessToken)
	require.NoError(t, err)
	assert.Equal(t, tokenID, retrieved.ID)
	assert.Equal(t, accessToken, retrieved.AccessToken)

	// 通过 refresh_token 查询（hash 为 NULL 应回退到明文查询）
	retrieved2, err := store.GetTokenByRefreshToken(ctx, refreshToken)
	require.NoError(t, err)
	assert.Equal(t, tokenID, retrieved2.ID)
	assert.Equal(t, refreshToken, retrieved2.RefreshToken)
}
