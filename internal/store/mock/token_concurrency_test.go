// Package mock Token 写方法并发安全测试
//
// 阶段 D 审查修复（竞态）：验证 GetTokenByAccessToken/GetTokenByRefreshToken 与
// RevokeToken / RevokeAllUserTokens / RotateRefreshToken 并发时不发生数据竞争。
//
// 回归保护：此前 RevokeToken 与 RevokeAllUserTokens 原地修改 map 中的 *model.Token 的
// RevokedAt 字段，与 Getter 锁外读取同一指针产生 race（CI -race 检出）。
// 改为拷贝-替换后，Getter 返回的指针指向不可变快照，写者替换 map 中的指针不影响已返回快照。
package mock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
)

// TestRevokeToken_ConcurrentWithGet 验证 GetTokenByAccessToken 与 RevokeToken 并发无 race。
//
// 拷贝-替换语义下：Getter 拿到的指针指向某次写之前/之后的不可变快照，
// 写者后续拷贝替换不影响该快照，因此锁外读取字段安全。
//
// 本测试在 -race 下运行：若 RevokeToken 仍原地修改 map 中的 *model.Token，
// race 检测器会在此报错。
func TestRevokeToken_ConcurrentWithGet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := New()
	require.NoError(t, m.StoreToken(ctx, &model.Token{
		ID:          "race-token",
		AccessToken: "race-access",
		UserID:      "race-user",
		ExpiresAt:   time.Now().Add(time.Hour),
		CreatedAt:   time.Now(),
	}))

	const goroutines = 30
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// 读者：并发 Get + 锁外读字段
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				got, err := m.GetTokenByAccessToken(ctx, "race-access")
				if err == nil && got != nil {
					// 模拟调用方锁外读取：拷贝替换下应安全
					_ = got.RevokedAt
					_ = got.UserID
					_ = got.AccessTokenHash
				}
			}
		}()
	}

	// 写者：反复 RevokeToken（幂等）
	var revokeErrs int32
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if err := m.RevokeToken(ctx, "race-access"); err != nil {
					atomic.AddInt32(&revokeErrs, 1)
				}
			}
		}()
	}

	wg.Wait()

	// 最终状态：应已撤销，且无错误返回
	got, err := m.GetTokenByAccessToken(ctx, "race-access")
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt, "token 应被撤销")
	assert.Equal(t, int32(0), revokeErrs, "RevokeToken 不应返回错误")
}

// TestRevokeAllUserTokens_ConcurrentWithGet 验证 RevokeAllUserTokens 与 Get 并发无 race。
//
// 覆盖多 token 场景：一个用户的多个 token 同时被并发 Get 与 RevokeAllUserTokens。
func TestRevokeAllUserTokens_ConcurrentWithGet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := New()
	const tokenCount = 5
	for i := 0; i < tokenCount; i++ {
		require.NoError(t, m.StoreToken(ctx, &model.Token{
			ID:          fmt.Sprintf("race-token-%d", i),
			AccessToken: fmt.Sprintf("race-access-%d", i),
			UserID:      "race-user-all",
			ExpiresAt:   time.Now().Add(time.Hour),
			CreatedAt:   time.Now(),
		}))
	}

	const goroutines = 30
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// 读者：并发 Get 所有 token
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				for k := 0; k < tokenCount; k++ {
					got, err := m.GetTokenByAccessToken(ctx, fmt.Sprintf("race-access-%d", k))
					if err == nil && got != nil {
						_ = got.RevokedAt
						_ = got.UserID
					}
				}
			}
		}()
	}

	// 写者：反复 RevokeAllUserTokens
	var revokeErrs int32
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if err := m.RevokeAllUserTokens(ctx, "race-user-all"); err != nil {
					atomic.AddInt32(&revokeErrs, 1)
				}
			}
		}()
	}

	wg.Wait()

	// 最终状态：所有 token 都应被撤销
	for k := 0; k < tokenCount; k++ {
		got, err := m.GetTokenByAccessToken(ctx, fmt.Sprintf("race-access-%d", k))
		require.NoError(t, err)
		require.NotNilf(t, got.RevokedAt, "token %d 应被撤销", k)
	}
	assert.Equal(t, int32(0), revokeErrs, "RevokeAllUserTokens 不应返回错误")
}

// TestRotateRefreshToken_ConcurrentWithGet 验证 RotateRefreshToken 与 Get 并发无 race。
//
// RotateRefreshToken 此前已修复为拷贝替换；本测试作为回归保护，
// 防止后续重构退回原地修改。
func TestRotateRefreshToken_ConcurrentWithGet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := New()
	refreshExpiresAt := time.Now().Add(24 * time.Hour)
	require.NoError(t, m.StoreToken(ctx, &model.Token{
		ID:               "rotate-race-old",
		AccessToken:      "rotate-race-access",
		RefreshToken:     "rotate-race-refresh",
		UserID:           "rotate-race-user",
		RefreshExpiresAt: &refreshExpiresAt,
		ExpiresAt:        time.Now().Add(time.Hour),
		CreatedAt:        time.Now(),
	}))

	const goroutines = 30
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// 读者：持续 Get 旧 access token，锁外读多字段
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				got, err := m.GetTokenByAccessToken(ctx, "rotate-race-access")
				if err == nil && got != nil {
					_ = got.RevokedAt
					_ = got.RotatedAt
					_ = got.ReplacedByTokenID
				}
			}
		}()
	}

	// 写者：并发尝试轮换；只有 1 个成功，其余返回 ErrTokenRotated
	var success int32
	var rotated int32
	var otherErrs int32
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			newToken := &model.Token{
				ID:               fmt.Sprintf("rotate-race-new-%d", idx),
				AccessToken:      fmt.Sprintf("rotate-race-new-access-%d", idx),
				RefreshToken:     fmt.Sprintf("rotate-race-new-refresh-%d", idx),
				UserID:           "rotate-race-user",
				RefreshExpiresAt: &refreshExpiresAt,
				ExpiresAt:        time.Now().Add(time.Hour),
				CreatedAt:        time.Now(),
			}
			err := m.RotateRefreshToken(ctx, "rotate-race-refresh", newToken)
			switch {
			case err == nil:
				atomic.AddInt32(&success, 1)
			case errors.Is(err, store.ErrTokenRotated):
				atomic.AddInt32(&rotated, 1)
			default:
				atomic.AddInt32(&otherErrs, 1)
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int32(1), success, "仅一次 RotateRefreshToken 应成功")
	assert.Equal(t, int32(goroutines-1), rotated, "其余应返回 ErrTokenRotated")
	assert.Equal(t, int32(0), otherErrs, "不应有其他错误")
}
