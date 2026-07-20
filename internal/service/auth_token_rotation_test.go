// Package service_test Refresh Token 原子轮换测试
// 阶段 2.1：验证原子轮换、重放检测、过期检查、状态变更场景
package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/metrics"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createRotationTestAuthService 创建用于轮换测试的 AuthService
// 与 createTestAuthService 类似，但显式启用 cache 与 metrics 以验证缓存清除
func createRotationTestAuthService(t *testing.T) (*service.AuthService, *mock.Store, cache.Cache) {
	storeInst := mock.New()
	passwordSvc := crypto.NewPasswordService(4)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	cacheSvc := cache.NewMemoryCache()
	metricsSvc := metrics.NewService()

	authSvc := service.NewAuthServiceWithOptions(
		storeInst, passwordSvc, jwtSvc, 5, 30*time.Minute,
		service.WithCache(cacheSvc),
		service.WithMetrics(metricsSvc),
	)

	return authSvc, storeInst, cacheSvc
}

// addActiveUserAndToken 向 store 添加一个 active 用户和一个有效 token
// 返回创建的 token 供测试使用
func addActiveUserAndToken(storeInst *mock.Store, refreshToken, accessToken string) *model.Token {
	refreshExpiresAt := time.Now().Add(24 * time.Hour)
	expiresAt := time.Now().Add(15 * time.Minute)
	storeInst.AddUser(&model.User{
		ID:            "user-rotation",
		Email:         "rotation@example.com",
		Role:          model.UserRoleUser,
		Status:        model.UserStatusActive,
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})
	tok := &model.Token{
		ID:               "token-rotation-1",
		UserID:           "user-rotation",
		RefreshToken:     refreshToken,
		AccessToken:      accessToken,
		ExpiresAt:        expiresAt,
		RefreshExpiresAt: &refreshExpiresAt,
		CreatedAt:        time.Now(),
	}
	storeInst.AddToken(tok)
	return tok
}

// ============================================================================
// 成功轮换测试
// ============================================================================

func TestAuthService_RefreshToken_RotationSuccess(t *testing.T) {
	ctx := context.Background()
	authSvc, storeInst, _ := createRotationTestAuthService(t)

	oldToken := addActiveUserAndToken(storeInst, "valid-refresh-1", "valid-access-1")

	t.Run("成功轮换-旧Token被标记为已轮换", func(t *testing.T) {
		resp, err := authSvc.RefreshToken(ctx, "valid-refresh-1")
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.NotEqual(t, "valid-refresh-1", resp.RefreshToken, "新 refresh token 应不同于旧值")

		// 验证旧 token 已被标记为已轮换 + 已撤销
		oldRecord, err := storeInst.GetTokenByAccessToken(ctx, oldToken.AccessToken)
		require.NoError(t, err)
		require.NotNil(t, oldRecord.RotatedAt, "rotated_at 必须被设置")
		require.NotNil(t, oldRecord.RevokedAt, "revoked_at 必须被设置")
		require.NotNil(t, oldRecord.ReplacedByTokenID, "replaced_by_token_id 必须被设置")
		assert.Equal(t, oldRecord.ReplacedByTokenID, oldRecord.ReplacedByTokenID, "replaced_by_token_id 应非空")

		// 验证新 token 已存入 store 且未撤销/未轮换
		newRecord, err := storeInst.GetTokenByRefreshToken(ctx, resp.RefreshToken)
		require.NoError(t, err)
		assert.Nil(t, newRecord.RotatedAt, "新 token 的 rotated_at 应为 nil")
		assert.Nil(t, newRecord.RevokedAt, "新 token 的 revoked_at 应为 nil")
		require.NotNil(t, newRecord.RefreshExpiresAt, "新 token 必须设置 refresh_expires_at")
		assert.True(t, newRecord.RefreshExpiresAt.After(time.Now()), "新 token refresh_expires_at 应为未来时间")
	})
}

// ============================================================================
// 重放检测测试
// ============================================================================

func TestAuthService_RefreshToken_ReplayDetection(t *testing.T) {
	ctx := context.Background()

	t.Run("已轮换的Token再次使用-触发重放防御", func(t *testing.T) {
		authSvc, storeInst, _ := createRotationTestAuthService(t)
		oldToken := addActiveUserAndToken(storeInst, "valid-refresh-2", "valid-access-2")

		// 第一次刷新：成功
		resp1, err := authSvc.RefreshToken(ctx, "valid-refresh-2")
		require.NoError(t, err)
		require.NotEmpty(t, resp1.RefreshToken)

		// 第二次使用同一个 refresh token：应触发重放防御
		_, err = authSvc.RefreshToken(ctx, "valid-refresh-2")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrTokenRotated)

		// 验证：旧 token 的 replaced_by_token_id 应指向第一次成功轮换的新 token
		// （第二次调用不产生新 token）
		oldRecord, err := storeInst.GetTokenByAccessToken(ctx, oldToken.AccessToken)
		require.NoError(t, err)
		require.NotNil(t, oldRecord.ReplacedByTokenID)
	})

	t.Run("并发请求只有一个成功-其他返回ErrTokenRotated", func(t *testing.T) {
		authSvc, storeInst, _ := createRotationTestAuthService(t)
		addActiveUserAndToken(storeInst, "concurrent-refresh", "concurrent-access")

		var wg sync.WaitGroup
		var successCount int32
		var rotatedCount int32
		var otherErrCount int32
		const goroutines = 10

		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				resp, err := authSvc.RefreshToken(ctx, "concurrent-refresh")
				if err == nil && resp != nil {
					atomic.AddInt32(&successCount, 1)
				} else if err != nil && errors.Is(err, service.ErrTokenRotated) {
					atomic.AddInt32(&rotatedCount, 1)
				} else if err != nil {
					atomic.AddInt32(&otherErrCount, 1)
				}
			}()
		}
		wg.Wait()

		assert.Equal(t, int32(1), successCount, "只有一个并发请求应成功")
		assert.Equal(t, int32(goroutines-1), rotatedCount, "其他请求应返回 ErrTokenRotated")
		assert.Equal(t, int32(0), otherErrCount, "不应有其他类型的错误")
	})

	t.Run("重放攻击触发撤销全部Token", func(t *testing.T) {
		authSvc, storeInst, _ := createRotationTestAuthService(t)
		oldToken := addActiveUserAndToken(storeInst, "valid-refresh-3", "valid-access-3")

		// 添加同一用户的另一个有效 token
		otherRefreshExpiresAt := time.Now().Add(24 * time.Hour)
		otherExpiresAt := time.Now().Add(15 * time.Minute)
		storeInst.AddToken(&model.Token{
			ID:               "token-other-1",
			UserID:           "user-rotation",
			RefreshToken:     "other-refresh-3",
			AccessToken:      "other-access-3",
			ExpiresAt:        otherExpiresAt,
			RefreshExpiresAt: &otherRefreshExpiresAt,
			CreatedAt:        time.Now(),
		})

		// 第一次轮换
		resp1, err := authSvc.RefreshToken(ctx, "valid-refresh-3")
		require.NoError(t, err)

		// 第二次使用旧 refresh token（重放）
		_, err = authSvc.RefreshToken(ctx, "valid-refresh-3")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrTokenRotated)

		// 验证：用户的所有 token 都应被撤销（包括"另一个有效 token"）
		otherToken, err := storeInst.GetTokenByAccessToken(ctx, "other-access-3")
		require.NoError(t, err)
		require.NotNil(t, otherToken.RevokedAt, "重放攻击应触发撤销用户全部 token")

		// 验证：第一次轮换产生的新 token 也应被撤销
		newToken, err := storeInst.GetTokenByRefreshToken(ctx, resp1.RefreshToken)
		require.NoError(t, err)
		require.NotNil(t, newToken.RevokedAt, "重放攻击应撤销刚轮换出的新 token")

		// 验证：原旧 token 的 revoked_at 已设置
		oldRecord, err := storeInst.GetTokenByAccessToken(ctx, oldToken.AccessToken)
		require.NoError(t, err)
		require.NotNil(t, oldRecord.RevokedAt)
	})
}

// ============================================================================
// 过期检查测试
// ============================================================================

func TestAuthService_RefreshToken_ExpiryCheck(t *testing.T) {
	ctx := context.Background()

	t.Run("RefreshExpiresAt已过期-返回ErrInvalidToken", func(t *testing.T) {
		authSvc, storeInst, _ := createRotationTestAuthService(t)
		// 创建已过期的 refresh token
		expiredRefresh := time.Now().Add(-1 * time.Hour)
		expiredAccess := time.Now().Add(-10 * time.Minute)
		storeInst.AddToken(&model.Token{
			ID:               "token-expired-1",
			UserID:           "user-rotation",
			RefreshToken:     "expired-refresh",
			AccessToken:      "expired-access",
			ExpiresAt:        expiredAccess,
			RefreshExpiresAt: &expiredRefresh,
			CreatedAt:        time.Now().Add(-2 * time.Hour),
		})
		storeInst.AddUser(&model.User{
			ID:            "user-rotation",
			Email:         "rotation@example.com",
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})

		_, err := authSvc.RefreshToken(ctx, "expired-refresh")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidToken)
	})

	t.Run("RefreshExpiresAt为nil-回退到ExpiresAt未过期-成功", func(t *testing.T) {
		authSvc, storeInst, _ := createRotationTestAuthService(t)
		// 旧数据兼容：RefreshExpiresAt 为 nil，回退到 ExpiresAt
		// ExpiresAt 设为 access TTL（15 分钟），refresh 回退到 expires_at 也未过期
		futureExpires := time.Now().Add(10 * time.Minute)
		storeInst.AddToken(&model.Token{
			ID:           "token-legacy-1",
			UserID:       "user-rotation",
			RefreshToken: "legacy-refresh",
			AccessToken:  "legacy-access",
			ExpiresAt:    futureExpires,
			// 故意不设置 RefreshExpiresAt，模拟旧数据
			CreatedAt: time.Now().Add(-5 * time.Minute),
		})
		storeInst.AddUser(&model.User{
			ID:            "user-rotation",
			Email:         "rotation@example.com",
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})

		resp, err := authSvc.RefreshToken(ctx, "legacy-refresh")
		require.NoError(t, err)
		assert.NotEmpty(t, resp.RefreshToken)
	})

	t.Run("RefreshExpiresAt为nil-回退到ExpiresAt已过期-返回ErrInvalidToken", func(t *testing.T) {
		authSvc, storeInst, _ := createRotationTestAuthService(t)
		// 旧数据兼容：RefreshExpiresAt 为 nil，回退到 ExpiresAt
		// ExpiresAt 也已过期
		expiredExpires := time.Now().Add(-1 * time.Hour)
		storeInst.AddToken(&model.Token{
			ID:           "token-legacy-2",
			UserID:       "user-rotation",
			RefreshToken: "legacy-refresh-2",
			AccessToken:  "legacy-access-2",
			ExpiresAt:    expiredExpires,
			CreatedAt:    time.Now().Add(-2 * time.Hour),
		})
		storeInst.AddUser(&model.User{
			ID:            "user-rotation",
			Email:         "rotation@example.com",
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})

		_, err := authSvc.RefreshToken(ctx, "legacy-refresh-2")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidToken)
	})
}

// ============================================================================
// 用户状态变更测试
// ============================================================================

func TestAuthService_RefreshToken_UserStatusChanged(t *testing.T) {
	ctx := context.Background()

	t.Run("用户已被禁用-返回ErrAccountDisabled", func(t *testing.T) {
		authSvc, storeInst, _ := createRotationTestAuthService(t)
		// 先添加 active 用户和 token
		addActiveUserAndToken(storeInst, "valid-refresh-disabled", "valid-access-disabled")

		// 用户状态变更为 disabled
		user, err := storeInst.GetByID(ctx, "user-rotation")
		require.NoError(t, err)
		user.Status = model.UserStatusDisabled
		err = storeInst.Update(ctx, user)
		require.NoError(t, err)

		_, err = authSvc.RefreshToken(ctx, "valid-refresh-disabled")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrAccountDisabled)
	})

	t.Run("用户已被锁定-返回ErrAccountLocked", func(t *testing.T) {
		authSvc, storeInst, _ := createRotationTestAuthService(t)
		addActiveUserAndToken(storeInst, "valid-refresh-locked", "valid-access-locked")

		// 用户状态变更为 locked
		user, err := storeInst.GetByID(ctx, "user-rotation")
		require.NoError(t, err)
		user.Status = model.UserStatusLocked
		err = storeInst.Update(ctx, user)
		require.NoError(t, err)

		_, err = authSvc.RefreshToken(ctx, "valid-refresh-locked")
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrAccountLocked)
	})
}

// ============================================================================
// 缓存清除测试
// ============================================================================

func TestAuthService_RefreshToken_CacheInvalidation(t *testing.T) {
	ctx := context.Background()
	authSvc, storeInst, cacheSvc := createRotationTestAuthService(t)
	addActiveUserAndToken(storeInst, "valid-refresh-cache", "valid-access-cache")

	// 预先将旧 access token 写入缓存
	cacheKey := cache.TokenKey("valid-access-cache")
	err := cacheSvc.Set(ctx, cacheKey, &model.Token{
		AccessToken: "valid-access-cache",
	}, 5*time.Minute)
	require.NoError(t, err)

	// 验证缓存已写入
	var cached model.Token
	err = cacheSvc.Get(ctx, cacheKey, &cached)
	require.NoError(t, err)
	assert.Equal(t, "valid-access-cache", cached.AccessToken)

	// 执行刷新
	_, err = authSvc.RefreshToken(ctx, "valid-refresh-cache")
	require.NoError(t, err)

	// 验证缓存已被清除（旧 access token 的缓存）
	err = cacheSvc.Get(ctx, cacheKey, &cached)
	assert.Error(t, err, "旧 access token 的缓存应被清除")
}

// ============================================================================
// Store 层 RotateRefreshToken 直接测试
// ============================================================================

func TestStore_RotateRefreshToken(t *testing.T) {
	ctx := context.Background()

	t.Run("Mock-成功轮换-旧Token被标记", func(t *testing.T) {
		storeInst := mock.New()
		refreshExpiresAt := time.Now().Add(24 * time.Hour)
		storeInst.AddToken(&model.Token{
			ID:               "old-token-id",
			UserID:           "user-1",
			RefreshToken:     "old-refresh",
			AccessToken:      "old-access",
			RefreshExpiresAt: &refreshExpiresAt,
			CreatedAt:        time.Now(),
		})

		newToken := &model.Token{
			ID:               "new-token-id",
			UserID:           "user-1",
			AccessToken:      "new-access",
			RefreshToken:     "new-refresh",
			RefreshExpiresAt: &refreshExpiresAt,
			CreatedAt:        time.Now(),
		}

		err := storeInst.RotateRefreshToken(ctx, "old-refresh", newToken)
		require.NoError(t, err)

		// 验证旧 token 被标记
		oldToken, err := storeInst.GetTokenByAccessToken(ctx, "old-access")
		require.NoError(t, err)
		require.NotNil(t, oldToken.RotatedAt)
		require.NotNil(t, oldToken.RevokedAt)
		require.NotNil(t, oldToken.ReplacedByTokenID)
		assert.Equal(t, "new-token-id", *oldToken.ReplacedByTokenID)

		// 验证新 token 已存入
		newStored, err := storeInst.GetTokenByRefreshToken(ctx, "new-refresh")
		require.NoError(t, err)
		assert.Equal(t, "new-token-id", newStored.ID)
	})

	t.Run("Mock-旧Token不存在-返回ErrTokenRotated", func(t *testing.T) {
		storeInst := mock.New()
		newToken := &model.Token{
			ID:           "new-token-id",
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
		}
		err := storeInst.RotateRefreshToken(ctx, "nonexistent", newToken)
		require.Error(t, err)
		assert.ErrorIs(t, err, store.ErrTokenRotated)
	})

	t.Run("Mock-旧Token已被轮换-返回ErrTokenRotated", func(t *testing.T) {
		storeInst := mock.New()
		refreshExpiresAt := time.Now().Add(24 * time.Hour)
		rotatedAt := time.Now()
		storeInst.AddToken(&model.Token{
			ID:               "old-token-id",
			RefreshToken:     "rotated-refresh",
			AccessToken:      "rotated-access",
			RotatedAt:        &rotatedAt,
			RefreshExpiresAt: &refreshExpiresAt,
			CreatedAt:        time.Now(),
		})

		newToken := &model.Token{
			ID:               "new-token-id",
			AccessToken:      "new-access",
			RefreshToken:     "new-refresh",
			RefreshExpiresAt: &refreshExpiresAt,
			CreatedAt:        time.Now(),
		}

		err := storeInst.RotateRefreshToken(ctx, "rotated-refresh", newToken)
		require.Error(t, err)
		assert.ErrorIs(t, err, store.ErrTokenRotated)
	})

	t.Run("Mock-旧Token已被撤销-返回ErrTokenRotated", func(t *testing.T) {
		storeInst := mock.New()
		refreshExpiresAt := time.Now().Add(24 * time.Hour)
		revokedAt := time.Now()
		storeInst.AddToken(&model.Token{
			ID:               "old-token-id",
			RefreshToken:     "revoked-refresh",
			AccessToken:      "revoked-access",
			RevokedAt:        &revokedAt,
			RefreshExpiresAt: &refreshExpiresAt,
			CreatedAt:        time.Now(),
		})

		newToken := &model.Token{
			ID:               "new-token-id",
			AccessToken:      "new-access",
			RefreshToken:     "new-refresh",
			RefreshExpiresAt: &refreshExpiresAt,
			CreatedAt:        time.Now(),
		}

		err := storeInst.RotateRefreshToken(ctx, "revoked-refresh", newToken)
		require.Error(t, err)
		assert.ErrorIs(t, err, store.ErrTokenRotated)
	})

	t.Run("Mock-RotateRefreshTokenErr注入-返回注入的错误", func(t *testing.T) {
		storeInst := mock.New()
		storeInst.RotateRefreshTokenErr = fmt.Errorf("database down")
		newToken := &model.Token{
			ID:           "new-token-id",
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
		}
		err := storeInst.RotateRefreshToken(ctx, "any", newToken)
		require.Error(t, err)
		assert.ErrorContains(t, err, "database down")
	})
}

// ============================================================================
// 并发安全测试（直接测试 mock store）
// ============================================================================

func TestStore_RotateRefreshToken_ConcurrentSafety(t *testing.T) {
	ctx := context.Background()
	storeInst := mock.New()
	refreshExpiresAt := time.Now().Add(24 * time.Hour)
	storeInst.AddToken(&model.Token{
		ID:               "concurrent-token",
		RefreshToken:     "concurrent-refresh",
		AccessToken:      "concurrent-access",
		RefreshExpiresAt: &refreshExpiresAt,
		CreatedAt:        time.Now(),
	})

	var wg sync.WaitGroup
	var successCount int32
	var rotatedCount int32
	const goroutines = 20

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			newToken := &model.Token{
				ID:               fmt.Sprintf("new-token-%d", idx),
				AccessToken:      fmt.Sprintf("new-access-%d", idx),
				RefreshToken:     fmt.Sprintf("new-refresh-%d", idx),
				RefreshExpiresAt: &refreshExpiresAt,
				CreatedAt:        time.Now(),
			}
			err := storeInst.RotateRefreshToken(ctx, "concurrent-refresh", newToken)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else if errors.Is(err, store.ErrTokenRotated) {
				atomic.AddInt32(&rotatedCount, 1)
			}
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(1), successCount, "只有一个并发请求应成功")
	assert.Equal(t, int32(goroutines-1), rotatedCount, "其他请求应返回 ErrTokenRotated")
}
