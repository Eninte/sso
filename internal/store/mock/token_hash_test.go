// Package mock Token 哈希查询测试
//
// 阶段 3.2：验证 mock store 的 token hash 查询行为，与 postgres 实现对齐：
//   - StoreToken 自动计算 hash
//   - GetTokenByAccessToken/GetTokenByRefreshToken 优先 hash 匹配，回退明文
//   - RevokeToken 优先 hash 匹配，回退明文
//   - RotateRefreshToken 优先 hash 匹配，回退明文
//   - 兼容旧数据：未设置 hash 的 token 仍可通过明文查询
package mock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/common"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
)

// TestStoreToken_HashAutoComputed 验证 StoreToken 自动计算 hash
func TestStoreToken_HashAutoComputed(t *testing.T) {
	m := New()
	ctx := context.Background()

	token := &model.Token{
		ID:           "token-1",
		AccessToken:  "access-xyz",
		RefreshToken: "refresh-xyz",
		UserID:       "user-1",
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	}
	// 故意不设置 hash
	require.NoError(t, m.StoreToken(ctx, token))

	// StoreToken 应自动计算 hash
	assert.Equal(t, common.HashToken("access-xyz"), token.AccessTokenHash)
	assert.Equal(t, common.HashToken("refresh-xyz"), token.RefreshTokenHash)
	assert.Equal(t, 64, len(token.AccessTokenHash))
	assert.Equal(t, 64, len(token.RefreshTokenHash))
}

// TestGetTokenByAccessToken_HashPriority 验证 hash 优先查询
func TestGetTokenByAccessToken_HashPriority(t *testing.T) {
	m := New()
	ctx := context.Background()

	token := &model.Token{
		ID:          "token-hash-1",
		AccessToken: "access-hash-test",
		UserID:      "user-1",
		ExpiresAt:   time.Now().Add(time.Hour),
		CreatedAt:   time.Now(),
	}
	require.NoError(t, m.StoreToken(ctx, token))

	// 通过 hash 应能查到
	got, err := m.GetTokenByAccessToken(ctx, "access-hash-test")
	require.NoError(t, err)
	assert.Equal(t, "token-hash-1", got.ID)
}

// TestGetTokenByAccessToken_LegacyFallback 验证旧数据回退到明文查询
func TestGetTokenByAccessToken_LegacyFallback(t *testing.T) {
	m := New()
	ctx := context.Background()

	// 使用 AddToken（测试辅助）直接插入，不计算 hash（模拟旧数据）
	legacyToken := &model.Token{
		ID:          "token-legacy-1",
		AccessToken: "legacy-access-no-hash",
		UserID:      "user-legacy",
		ExpiresAt:   time.Now().Add(time.Hour),
		CreatedAt:   time.Now(),
		// 不设置 AccessTokenHash，模拟旧数据
	}
	m.AddToken(legacyToken)

	// 通过明文应能查到（hash 为空，回退到明文）
	got, err := m.GetTokenByAccessToken(ctx, "legacy-access-no-hash")
	require.NoError(t, err)
	assert.Equal(t, "token-legacy-1", got.ID)
}

// TestGetTokenByRefreshToken_HashPriority 验证 refresh token hash 优先查询
func TestGetTokenByRefreshToken_HashPriority(t *testing.T) {
	m := New()
	ctx := context.Background()

	token := &model.Token{
		ID:           "token-refresh-1",
		AccessToken:  "access-1",
		RefreshToken: "refresh-hash-test",
		UserID:       "user-1",
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	}
	require.NoError(t, m.StoreToken(ctx, token))

	got, err := m.GetTokenByRefreshToken(ctx, "refresh-hash-test")
	require.NoError(t, err)
	assert.Equal(t, "token-refresh-1", got.ID)
}

// TestGetTokenByRefreshToken_LegacyFallback 验证 refresh token 旧数据回退到明文
func TestGetTokenByRefreshToken_LegacyFallback(t *testing.T) {
	m := New()
	ctx := context.Background()

	legacyToken := &model.Token{
		ID:           "token-legacy-refresh",
		AccessToken:  "legacy-access-2",
		RefreshToken: "legacy-refresh-no-hash",
		UserID:       "user-legacy",
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	}
	m.AddToken(legacyToken)

	got, err := m.GetTokenByRefreshToken(ctx, "legacy-refresh-no-hash")
	require.NoError(t, err)
	assert.Equal(t, "token-legacy-refresh", got.ID)
}

// TestRevokeToken_HashPriority 验证 RevokeToken 优先 hash 匹配
func TestRevokeToken_HashPriority(t *testing.T) {
	m := New()
	ctx := context.Background()

	token := &model.Token{
		ID:          "token-revoke-1",
		AccessToken: "access-revoke-test",
		UserID:      "user-1",
		ExpiresAt:   time.Now().Add(time.Hour),
		CreatedAt:   time.Now(),
	}
	require.NoError(t, m.StoreToken(ctx, token))

	// 通过 hash 撤销
	require.NoError(t, m.RevokeToken(ctx, "access-revoke-test"))

	// 再次查询应发现 token 已被撤销
	got, err := m.GetTokenByAccessToken(ctx, "access-revoke-test")
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)
}

// TestRevokeToken_LegacyFallback 验证 RevokeToken 旧数据回退到明文
func TestRevokeToken_LegacyFallback(t *testing.T) {
	m := New()
	ctx := context.Background()

	legacyToken := &model.Token{
		ID:          "token-revoke-legacy",
		AccessToken: "legacy-revoke-access",
		UserID:      "user-legacy",
		ExpiresAt:   time.Now().Add(time.Hour),
		CreatedAt:   time.Now(),
	}
	m.AddToken(legacyToken)

	// 通过明文撤销（hash 字段为空，回退到明文匹配）
	require.NoError(t, m.RevokeToken(ctx, "legacy-revoke-access"))

	got, err := m.GetTokenByAccessToken(ctx, "legacy-revoke-access")
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)
}

// TestRevokeToken_Idempotent 验证 RevokeToken 幂等性（不覆盖原撤销时间）
func TestRevokeToken_Idempotent(t *testing.T) {
	m := New()
	ctx := context.Background()

	token := &model.Token{
		ID:          "token-idempotent",
		AccessToken: "access-idempotent",
		UserID:      "user-1",
		ExpiresAt:   time.Now().Add(time.Hour),
		CreatedAt:   time.Now(),
	}
	require.NoError(t, m.StoreToken(ctx, token))

	// 第一次撤销
	require.NoError(t, m.RevokeToken(ctx, "access-idempotent"))
	firstRevoke := token.RevokedAt
	require.NotNil(t, firstRevoke)

	// 第二次撤销不应报错，且不应覆盖原撤销时间
	require.NoError(t, m.RevokeToken(ctx, "access-idempotent"))
	assert.Equal(t, firstRevoke, token.RevokedAt, "二次撤销不应覆盖原撤销时间")
}

// TestRotateRefreshToken_HashPriority 验证 RotateRefreshToken 优先 hash 匹配
func TestRotateRefreshToken_HashPriority(t *testing.T) {
	m := New()
	ctx := context.Background()

	oldToken := &model.Token{
		ID:           "token-rotate-old",
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		UserID:       "user-rotate",
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	}
	require.NoError(t, m.StoreToken(ctx, oldToken))

	newToken := &model.Token{
		ID:           "token-rotate-new",
		AccessToken:  "access-new",
		RefreshToken: "refresh-new",
		UserID:       "user-rotate",
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	}

	// 通过 hash 轮换
	require.NoError(t, m.RotateRefreshToken(ctx, "refresh-old", newToken))

	// 旧 token 应被标记为已轮换
	gotOld, err := m.GetTokenByAccessToken(ctx, "access-old")
	require.NoError(t, err)
	require.NotNil(t, gotOld.RotatedAt, "旧 token 应被标记为已轮换")
	require.NotNil(t, gotOld.RevokedAt, "旧 token 应被标记为已撤销")

	// 新 token 应能通过 hash 查询
	gotNew, err := m.GetTokenByAccessToken(ctx, "access-new")
	require.NoError(t, err)
	assert.Equal(t, "token-rotate-new", gotNew.ID)
	assert.Equal(t, common.HashToken("access-new"), gotNew.AccessTokenHash)
	assert.Equal(t, common.HashToken("refresh-new"), gotNew.RefreshTokenHash)
}

// TestRotateRefreshToken_LegacyFallback 验证 RotateRefreshToken 旧数据回退到明文
func TestRotateRefreshToken_LegacyFallback(t *testing.T) {
	m := New()
	ctx := context.Background()

	// 模拟旧数据：使用 AddToken 直接插入，不设置 hash
	legacyOld := &model.Token{
		ID:           "token-legacy-rotate-old",
		AccessToken:  "legacy-access-rotate",
		RefreshToken: "legacy-refresh-rotate",
		UserID:       "user-legacy-rotate",
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	}
	m.AddToken(legacyOld)

	newToken := &model.Token{
		ID:           "token-legacy-rotate-new",
		AccessToken:  "legacy-access-new",
		RefreshToken: "legacy-refresh-new",
		UserID:       "user-legacy-rotate",
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	}

	// 通过明文回退轮换（hash 字段为空）
	require.NoError(t, m.RotateRefreshToken(ctx, "legacy-refresh-rotate", newToken))

	// 旧 token 应被标记为已轮换
	gotOld, err := m.GetTokenByAccessToken(ctx, "legacy-access-rotate")
	require.NoError(t, err)
	require.NotNil(t, gotOld.RotatedAt, "旧 token 应被标记为已轮换")
}

// TestRotateRefreshToken_ReplayDetection 验证重放检测
func TestRotateRefreshToken_ReplayDetection(t *testing.T) {
	m := New()
	ctx := context.Background()

	oldToken := &model.Token{
		ID:           "token-replay-old",
		AccessToken:  "access-replay",
		RefreshToken: "refresh-replay",
		UserID:       "user-replay",
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	}
	require.NoError(t, m.StoreToken(ctx, oldToken))

	newToken1 := &model.Token{
		ID:           "token-replay-new-1",
		AccessToken:  "access-new-1",
		RefreshToken: "refresh-new-1",
		UserID:       "user-replay",
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	}

	// 第一次轮换成功
	require.NoError(t, m.RotateRefreshToken(ctx, "refresh-replay", newToken1))

	// 第二次使用同一 refresh token 应失败（重放攻击）
	newToken2 := &model.Token{
		ID:           "token-replay-new-2",
		AccessToken:  "access-new-2",
		RefreshToken: "refresh-new-2",
		UserID:       "user-replay",
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	}
	err := m.RotateRefreshToken(ctx, "refresh-replay", newToken2)
	assert.ErrorIs(t, err, store.ErrTokenRotated, "重放应返回 ErrTokenRotated")
}

// TestRevokeToken_NotFoundNoError 验证不存在的 token 不报错（RFC 7009 幂等）
func TestRevokeToken_NotFoundNoError(t *testing.T) {
	m := New()
	ctx := context.Background()

	// 不存在的 token 不报错
	err := m.RevokeToken(ctx, "non-existent-token")
	assert.NoError(t, err, "不存在的 token 不应报错（RFC 7009 幂等）")
}
