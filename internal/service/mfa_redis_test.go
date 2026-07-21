// Package service MFA Redis 限流与重放记录测试（T9：M3+L1）
// 使用 miniredis 避免依赖真实 Redis（无 build tag，CI 中也运行）
package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/store/mock"
)

// setupMFARedis 创建 miniredis 实例与注入缓存的 MFAService
func setupMFARedis(t *testing.T) (*miniredis.Miniredis, *MFAService) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	rc, err := cache.NewRedisCacheWithOptions(&redis.Options{Addr: mr.Addr()})
	require.NoError(t, err)
	t.Cleanup(func() { rc.Close() })

	svc := NewMFAService(mock.New())
	svc.SetCache(rc)
	t.Cleanup(func() { svc.Close() })
	return mr, svc
}

// setupMFABrokenRedis 创建注入"已宕机 Redis"缓存的 MFAService（验证内存降级）
// 构造缓存需要可用连接，因此先启动 miniredis 完成构造，再关闭模拟故障
func setupMFABrokenRedis(t *testing.T) *MFAService {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)

	rc, err := cache.NewRedisCacheWithOptions(&redis.Options{Addr: mr.Addr()})
	require.NoError(t, err)
	t.Cleanup(func() { rc.Close() })

	mr.Close() // 构造完成后关闭服务端，后续操作必然失败

	svc := NewMFAService(mock.New())
	svc.SetCache(rc)
	t.Cleanup(func() { svc.Close() })
	return svc
}

// ============================================================================
// 恢复码限流（Redis 路径）
// ============================================================================

func TestMFAService_RecoveryRateLimit_Redis(t *testing.T) {
	ctx := context.Background()

	t.Run("计数累积_未达上限不限流", func(t *testing.T) {
		mr, svc := setupMFARedis(t)

		for i := 0; i < 3; i++ {
			svc.recordRecoveryFailure(ctx, "user-1")
		}
		assert.False(t, svc.checkRecoveryRateLimit(ctx, "user-1"))

		// Redis 键应为 INCR 计数
		val, err := mr.Get(mfaRecoveryAttemptsKeyPrefix + "user-1")
		require.NoError(t, err)
		assert.Equal(t, "3", val)
	})

	t.Run("达上限拒绝", func(t *testing.T) {
		_, svc := setupMFARedis(t)

		for i := 0; i < maxRecoveryAttempts; i++ {
			svc.recordRecoveryFailure(ctx, "user-1")
		}
		assert.True(t, svc.checkRecoveryRateLimit(ctx, "user-1"))
	})

	t.Run("TTL过期恢复", func(t *testing.T) {
		mr, svc := setupMFARedis(t)

		for i := 0; i < maxRecoveryAttempts; i++ {
			svc.recordRecoveryFailure(ctx, "user-1")
		}
		assert.True(t, svc.checkRecoveryRateLimit(ctx, "user-1"))

		// 锁定期 15 分钟过后键过期，限流解除
		mr.FastForward(recoveryLockoutDuration + time.Minute)
		assert.False(t, svc.checkRecoveryRateLimit(ctx, "user-1"))
	})

	t.Run("尝试窗口过期_未达上限计数清零", func(t *testing.T) {
		mr, svc := setupMFARedis(t)

		for i := 0; i < 3; i++ {
			svc.recordRecoveryFailure(ctx, "user-1")
		}
		// 尝试窗口 30 分钟过后计数键过期
		mr.FastForward(recoveryAttemptWindow + time.Minute)
		assert.False(t, svc.checkRecoveryRateLimit(ctx, "user-1"))
		assert.False(t, mr.Exists(mfaRecoveryAttemptsKeyPrefix+"user-1"))
	})

	t.Run("clearRecoveryAttempts清除Redis键", func(t *testing.T) {
		mr, svc := setupMFARedis(t)

		for i := 0; i < maxRecoveryAttempts; i++ {
			svc.recordRecoveryFailure(ctx, "user-1")
		}
		assert.True(t, svc.checkRecoveryRateLimit(ctx, "user-1"))

		svc.clearRecoveryAttempts(ctx, "user-1")
		assert.False(t, svc.checkRecoveryRateLimit(ctx, "user-1"))
		assert.False(t, mr.Exists(mfaRecoveryAttemptsKeyPrefix+"user-1"))
	})
}

// TestMFAService_RecoveryRateLimit_RedisDownFallback 验证 Redis 不可用时降级内存限流
func TestMFAService_RecoveryRateLimit_RedisDownFallback(t *testing.T) {
	ctx := context.Background()
	svc := setupMFABrokenRedis(t)

	for i := 0; i < maxRecoveryAttempts; i++ {
		svc.recordRecoveryFailure(ctx, "user-1")
	}
	assert.True(t, svc.checkRecoveryRateLimit(ctx, "user-1"), "Redis 故障时应降级为内存限流并锁定")

	svc.clearRecoveryAttempts(ctx, "user-1")
	assert.False(t, svc.checkRecoveryRateLimit(ctx, "user-1"))
}

// ============================================================================
// TOTP 重放保护（Redis 路径）
// ============================================================================

func TestMFAService_TOTPReplay_Redis(t *testing.T) {
	ctx := context.Background()

	t.Run("同timeStep重放拒绝", func(t *testing.T) {
		_, svc := setupMFARedis(t)

		assert.True(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100), "首次使用应放行")
		assert.False(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100), "同 timeStep 重放应拒绝")
	})

	t.Run("不同timeStep放行", func(t *testing.T) {
		_, svc := setupMFARedis(t)

		assert.True(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100))
		assert.True(t, svc.markTOTPUsed(ctx, "user-1", "222222", 101), "不同 timeStep 应放行")
	})

	t.Run("不同用户互不影响", func(t *testing.T) {
		_, svc := setupMFARedis(t)

		assert.True(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100))
		assert.True(t, svc.markTOTPUsed(ctx, "user-2", "111111", 100))
	})

	t.Run("TTL过期后同码可再用", func(t *testing.T) {
		mr, svc := setupMFARedis(t)

		assert.True(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100))
		mr.FastForward(totpReplayWindow + time.Second)
		assert.True(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100), "90 秒窗口过后键过期应放行")
	})

	// L1 回归：先用 T 码再用 T+1 码后，T 码仍被拒绝
	// 旧实现每用户仅一条记录，T+1 覆盖 T 后 T 码可二次使用
	t.Run("L1回归_旧timeStep码不可二次使用", func(t *testing.T) {
		_, svc := setupMFARedis(t)

		assert.True(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100), "T 码首次使用")
		assert.True(t, svc.markTOTPUsed(ctx, "user-1", "222222", 101), "T+1 码首次使用")
		assert.False(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100), "T 码不得因 T+1 记录覆盖而二次使用")
	})
}

// TestMFAService_TOTPReplay_RedisDownFallback 验证 Redis 不可用时降级内存记录
func TestMFAService_TOTPReplay_RedisDownFallback(t *testing.T) {
	ctx := context.Background()
	svc := setupMFABrokenRedis(t)

	assert.True(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100), "首次使用应放行（内存降级）")
	assert.False(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100), "重放应拒绝（内存降级）")
}

// TestMFAService_TOTPReplay_MemoryL1 验证内存路径同样修复 L1（多 timeStep 记录）
func TestMFAService_TOTPReplay_MemoryL1(t *testing.T) {
	svc := NewMFAService(mock.New()) // 无缓存，纯内存路径
	defer svc.Close()
	ctx := context.Background()

	assert.True(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100), "T 码首次使用")
	assert.True(t, svc.markTOTPUsed(ctx, "user-1", "222222", 101), "T+1 码首次使用")
	assert.False(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100), "内存路径下 T 码仍不得二次使用")
}

// TestMFAService_ClearTOTPUsageForTesting_Redis 验证测试清理同时覆盖 Redis 键
func TestMFAService_ClearTOTPUsageForTesting_Redis(t *testing.T) {
	ctx := context.Background()
	mr, svc := setupMFARedis(t)

	assert.True(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100))
	assert.True(t, mr.Exists(mfaTOTPUsedKeyPrefix+"user-1:100"))

	svc.ClearTOTPUsageForTesting("user-1")
	assert.False(t, mr.Exists(mfaTOTPUsedKeyPrefix+"user-1:100"))
	// 清理后同码可再次使用
	assert.True(t, svc.markTOTPUsed(ctx, "user-1", "111111", 100))
}
