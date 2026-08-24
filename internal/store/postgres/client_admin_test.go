//go:build integration

// Package postgres_test 客户端管理（管理员 API）PostgreSQL 集成测试
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

// newTestClient 构造测试客户端（client_id 以 test- 前缀便于 cleanup）
func newTestClient(name string) *model.Client {
	return &model.Client{
		ID:           uuid.New().String(),
		ClientID:     "test-client-" + uuid.New().String(),
		ClientSecret: "$2a$10$testhashvalue0123456789abc",
		Name:         name,
		RedirectURIs: []string{"https://example.com/callback"},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		Scopes:       []string{"openid", "profile", "email"},
		PublicClient: false,
		IsActive:     true,
		CreatedAt:    time.Now(),
	}
}

// newTestTokenWithClient 构造关联到指定客户端的 token
// 注意：tokens 表对 refresh_token_hash 有唯一索引，每行必须填充唯一值
func newTestTokenWithClient(userID, clientID, accessToken string) *model.Token {
	return &model.Token{
		ID:           uuid.New().String(),
		AccessToken:  "test-" + accessToken,
		RefreshToken: "test-refresh-" + accessToken,
		UserID:       userID,
		ClientID:     &clientID,
		Scopes:       []string{"openid"},
		ExpiresAt:    time.Now().Add(15 * time.Minute),
		CreatedAt:    time.Now(),
	}
}

func TestClientStore_CreateGetRoundtrip(t *testing.T) {
	store, db := setupTestStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupTestData(t, db) })

	client := newTestClient("roundtrip")
	require.NoError(t, store.CreateClient(ctx, client))

	got, err := store.GetByClientID(ctx, client.ClientID)
	require.NoError(t, err)
	assert.Equal(t, client.ClientID, got.ClientID)
	assert.Equal(t, "roundtrip", got.Name)
	assert.True(t, got.IsActive, "新创建的客户端默认启用")
	assert.False(t, got.UpdatedAt.IsZero(), "updated_at 由数据库 DEFAULT 填充")
}

func TestClientStore_ListClientsPagination(t *testing.T) {
	store, db := setupTestStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupTestData(t, db) })

	// 依次创建 3 个客户端（CreatedAt 递增，便于验证排序）
	var createdIDs []string
	for i := 0; i < 3; i++ {
		client := newTestClient("list")
		client.CreatedAt = time.Now().Add(time.Duration(i) * time.Second)
		require.NoError(t, store.CreateClient(ctx, client))
		createdIDs = append(createdIDs, client.ClientID)
	}

	// 全量拉取（limit 足够大），验证本测试创建的客户端按 created_at DESC 出现
	clients, total, err := store.ListClients(ctx, 0, 1000)
	require.NoError(t, err)
	require.Len(t, clients, total)
	// 记录每个本测试客户端在结果中的位置（结果本身按 created_at DESC）
	position := map[string]int{}
	for i, c := range clients {
		position[c.ClientID] = i
	}
	for _, id := range createdIDs {
		_, ok := position[id]
		require.True(t, ok, "本测试创建的客户端 %s 应出现在列表中", id)
	}
	// createdIDs 按下标 0→2 创建时间递增，DESC 排序下最新的（index 2）应排在最前
	assert.Less(t, position[createdIDs[2]], position[createdIDs[1]])
	assert.Less(t, position[createdIDs[1]], position[createdIDs[0]])

	// 分页语义：offset 推进不重叠、每页不超过 limit
	page1, total, err := store.ListClients(ctx, 0, 2)
	require.NoError(t, err)
	require.Len(t, page1, 2)

	page2, total2, err := store.ListClients(ctx, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, total, total2, "total 不随分页变化")
	require.Len(t, page2, 2)
	// 两页结果互不重叠
	seen := map[string]bool{}
	for _, c := range page1 {
		seen[c.ClientID] = true
	}
	for _, c := range page2 {
		assert.False(t, seen[c.ClientID], "分页不应出现重复")
	}

	// offset 越界返回空切片
	overflow, _, err := store.ListClients(ctx, total+10, 10)
	require.NoError(t, err)
	assert.Empty(t, overflow)
}

func TestClientStore_UpdateClient(t *testing.T) {
	store, db := setupTestStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupTestData(t, db) })

	client := newTestClient("update")
	require.NoError(t, store.CreateClient(ctx, client))

	client.Name = "update-v2"
	client.RedirectURIs = []string{"https://new.example.com/cb"}
	client.GrantTypes = []string{"authorization_code"}
	client.Scopes = []string{"openid", "email"}
	require.NoError(t, store.UpdateClient(ctx, client))

	got, err := store.GetByClientID(ctx, client.ClientID)
	require.NoError(t, err)
	assert.Equal(t, "update-v2", got.Name)
	assert.Equal(t, []string{"https://new.example.com/cb"}, got.RedirectURIs)
	assert.Equal(t, []string{"authorization_code"}, got.GrantTypes)
	assert.Equal(t, []string{"openid", "email"}, got.Scopes)

	// 不存在 → ErrNotFound
	missing := newTestClient("missing")
	err = store.UpdateClient(ctx, missing)
	assert.ErrorIs(t, err, storepkg.ErrNotFound)
}

func TestClientStore_RotateClientSecret(t *testing.T) {
	store, db := setupTestStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupTestData(t, db) })

	client := newTestClient("rotate")
	require.NoError(t, store.CreateClient(ctx, client))
	originalHash := client.ClientSecret

	require.NoError(t, store.RotateClientSecret(ctx, client.ClientID, "$2a$10$newhashvalue0123456789abc"))

	got, err := store.GetByClientID(ctx, client.ClientID)
	require.NoError(t, err)
	assert.NotEqual(t, originalHash, got.ClientSecret)
	assert.Equal(t, "$2a$10$newhashvalue0123456789abc", got.ClientSecret)

	// 不存在 → ErrNotFound
	err = store.RotateClientSecret(ctx, "test-client-missing", "hash")
	assert.ErrorIs(t, err, storepkg.ErrNotFound)
}

func TestClientStore_SetClientActive(t *testing.T) {
	store, db := setupTestStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupTestData(t, db) })

	client := newTestClient("active")
	require.NoError(t, store.CreateClient(ctx, client))

	require.NoError(t, store.SetClientActive(ctx, client.ClientID, false))
	got, err := store.GetByClientID(ctx, client.ClientID)
	require.NoError(t, err)
	assert.False(t, got.IsActive)

	require.NoError(t, store.SetClientActive(ctx, client.ClientID, true))
	got, err = store.GetByClientID(ctx, client.ClientID)
	require.NoError(t, err)
	assert.True(t, got.IsActive)

	// 不存在 → ErrNotFound
	err = store.SetClientActive(ctx, "test-client-missing", false)
	assert.ErrorIs(t, err, storepkg.ErrNotFound)
}

func TestClientStore_RevokeAllClientTokens(t *testing.T) {
	store, db := setupTestStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupTestData(t, db) })

	user := newTestUser("revoke-clients@example.com")
	require.NoError(t, store.Create(ctx, user))

	clientA := newTestClient("revokeA")
	clientB := newTestClient("revokeB")
	require.NoError(t, store.CreateClient(ctx, clientA))
	require.NoError(t, store.CreateClient(ctx, clientB))

	tokA1 := newTestTokenWithClient(user.ID, clientA.ClientID, "a1")
	tokA2 := newTestTokenWithClient(user.ID, clientA.ClientID, "a2")
	tokB := newTestTokenWithClient(user.ID, clientB.ClientID, "b1")
	require.NoError(t, store.StoreToken(ctx, tokA1))
	require.NoError(t, store.StoreToken(ctx, tokA2))
	require.NoError(t, store.StoreToken(ctx, tokB))

	require.NoError(t, store.RevokeAllClientTokens(ctx, clientA.ClientID))

	gotA1, err := store.GetTokenByAccessToken(ctx, tokA1.AccessToken)
	require.NoError(t, err)
	assert.NotNil(t, gotA1.RevokedAt, "clientA 的未撤销 token 应被撤销")

	gotA2, err := store.GetTokenByAccessToken(ctx, tokA2.AccessToken)
	require.NoError(t, err)
	assert.NotNil(t, gotA2.RevokedAt)

	gotB, err := store.GetTokenByAccessToken(ctx, tokB.AccessToken)
	require.NoError(t, err)
	assert.Nil(t, gotB.RevokedAt, "clientB 的 token 不受影响")
	assert.Equal(t, clientB.ClientID, *gotB.ClientID)
}

func TestClientStore_DeleteClient(t *testing.T) {
	store, db := setupTestStore(t)
	ctx := context.Background()
	t.Cleanup(func() { cleanupTestData(t, db) })

	user := newTestUser("delete-client@example.com")
	require.NoError(t, store.Create(ctx, user))

	clientA := newTestClient("deleteA")
	clientB := newTestClient("deleteB")
	require.NoError(t, store.CreateClient(ctx, clientA))
	require.NoError(t, store.CreateClient(ctx, clientB))

	// clientA：一个未撤销 token + 一个已撤销 token + 一个授权码；clientB 各一份作对照
	tokAUnrevoked := newTestTokenWithClient(user.ID, clientA.ClientID, "unrevoked")
	tokARevoked := newTestTokenWithClient(user.ID, clientA.ClientID, "revoked")
	tokB := newTestTokenWithClient(user.ID, clientB.ClientID, "b1")
	require.NoError(t, store.StoreToken(ctx, tokAUnrevoked))
	require.NoError(t, store.StoreToken(ctx, tokARevoked))
	require.NoError(t, store.StoreToken(ctx, tokB))
	require.NoError(t, store.RevokeToken(ctx, tokARevoked.AccessToken)) // 标记已撤销

	codeA := &model.AuthorizationCode{
		Code:      "test-code-" + uuid.New().String(),
		ClientID:  clientA.ClientID,
		UserID:    user.ID,
		Scopes:    []string{"openid"},
		ExpiresAt: time.Now().Add(10 * time.Minute),
		CreatedAt: time.Now(),
	}
	codeB := &model.AuthorizationCode{
		Code:      "test-code-" + uuid.New().String(),
		ClientID:  clientB.ClientID,
		UserID:    user.ID,
		Scopes:    []string{"openid"},
		ExpiresAt: time.Now().Add(10 * time.Minute),
		CreatedAt: time.Now(),
	}
	require.NoError(t, store.StoreAuthorizationCode(ctx, codeA))
	require.NoError(t, store.StoreAuthorizationCode(ctx, codeB))

	// 执行删除
	require.NoError(t, store.DeleteClient(ctx, clientA.ClientID))

	// 1. 客户端行删除
	_, err := store.GetByClientID(ctx, clientA.ClientID)
	assert.ErrorIs(t, err, storepkg.ErrNotFound)

	// 2. clientA 授权码被删，clientB 授权码保留
	_, err = store.GetAuthorizationCode(ctx, codeA.Code)
	assert.ErrorIs(t, err, storepkg.ErrNotFound)
	gotCodeB, err := store.GetAuthorizationCode(ctx, codeB.Code)
	require.NoError(t, err)
	assert.Equal(t, clientB.ClientID, gotCodeB.ClientID)

	// 3. 历史 token 行保留：client_id 置空、revoked_at 非空
	gotTokA, err := store.GetTokenByAccessToken(ctx, tokAUnrevoked.AccessToken)
	require.NoError(t, err)
	assert.Nil(t, gotTokA.ClientID, "token.client_id 应被置空（FK SET NULL）")
	assert.NotNil(t, gotTokA.RevokedAt, "未撤销 token 应先撤销再置空")

	gotTokRevoked, err := store.GetTokenByAccessToken(ctx, tokARevoked.AccessToken)
	require.NoError(t, err)
	assert.Nil(t, gotTokRevoked.ClientID)
	assert.NotNil(t, gotTokRevoked.RevokedAt, "原已撤销 token 保持撤销状态")

	// 4. clientB 的 token 不受影响
	gotTokB, err := store.GetTokenByAccessToken(ctx, tokB.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, clientB.ClientID, *gotTokB.ClientID)
	assert.Nil(t, gotTokB.RevokedAt)

	// 5. 删除不存在的客户端 → ErrNotFound
	err = store.DeleteClient(ctx, "test-client-missing")
	assert.ErrorIs(t, err, storepkg.ErrNotFound)
}
