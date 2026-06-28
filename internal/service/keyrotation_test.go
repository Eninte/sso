// Package service_test 密钥轮换服务测试
package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

func createTestKeyRotationService(t *testing.T) (*service.KeyRotationService, *mock.Store) {
	t.Helper()

	store := mock.New()

	// 创建JWT服务
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 创建审计服务
	auditSvc := service.NewAuditService(store)

	// 创建密钥轮换服务
	keyRotationSvc := service.NewKeyRotationService(store, jwtSvc, auditSvc, 24*time.Hour)

	return keyRotationSvc, store
}

// ============================================================================
// NewKeyRotationService 测试
// ============================================================================

func TestNewKeyRotationService(t *testing.T) {
	store := mock.New()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	auditSvc := service.NewAuditService(store)

	svc := service.NewKeyRotationService(store, jwtSvc, auditSvc, 24*time.Hour)

	assert.NotNil(t, svc)
}

// ============================================================================
// RotateKey 测试
// ============================================================================

func TestKeyRotationService_RotateKey(t *testing.T) {
	ctx := context.Background()

	t.Run("首次轮换（无活跃密钥）", func(t *testing.T) {
		svc, _ := createTestKeyRotationService(t)

		key, err := svc.RotateKey(ctx)

		require.NoError(t, err)
		assert.NotNil(t, key)
		assert.NotEmpty(t, key.ID)
		assert.Equal(t, model.KeyStatusActive, key.Status)
	})

	t.Run("轮换已有密钥", func(t *testing.T) {
		svc, store := createTestKeyRotationService(t)

		// 先创建一个活跃密钥
		firstKey, err := svc.RotateKey(ctx)
		require.NoError(t, err)

		// 再次轮换
		secondKey, err := svc.RotateKey(ctx)

		require.NoError(t, err)
		assert.NotEqual(t, firstKey.ID, secondKey.ID)

		// 验证第一个密钥已被弃用
		key, err := store.GetKeyByID(ctx, firstKey.ID)
		require.NoError(t, err)
		assert.Equal(t, model.KeyStatusDeprecated, key.Status)
	})
}

// ============================================================================
// CleanupExpiredKeys 测试
// ============================================================================

func TestKeyRotationService_CleanupExpiredKeys(t *testing.T) {
	ctx := context.Background()

	t.Run("清理过期密钥", func(t *testing.T) {
		svc, store := createTestKeyRotationService(t)

		// 创建一个已过期的弃用密钥
		pastTime := time.Now().Add(-1 * time.Hour)
		expiredKey := &model.KeyVersion{
			ID:        "expired-key-1",
			Status:    model.KeyStatusDeprecated,
			ExpiresAt: &pastTime,
		}
		err := store.StoreKey(ctx, expiredKey)
		require.NoError(t, err)

		// 清理过期密钥
		count, err := svc.CleanupExpiredKeys(ctx)

		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("无过期密钥", func(t *testing.T) {
		svc, _ := createTestKeyRotationService(t)

		count, err := svc.CleanupExpiredKeys(ctx)

		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

// ============================================================================
// RevokeKey 测试
// ============================================================================

func TestKeyRotationService_RevokeKey(t *testing.T) {
	ctx := context.Background()

	t.Run("撤销弃用密钥", func(t *testing.T) {
		svc, store := createTestKeyRotationService(t)

		// 先创建一个活跃密钥
		activeKey, err := svc.RotateKey(ctx)
		require.NoError(t, err)

		// 再轮换一次，使第一个密钥变为弃用
		_, err = svc.RotateKey(ctx)
		require.NoError(t, err)

		// 撤销第一个密钥
		err = svc.RevokeKey(ctx, activeKey.ID)
		require.NoError(t, err)

		// 验证密钥已被撤销
		key, err := store.GetKeyByID(ctx, activeKey.ID)
		require.NoError(t, err)
		assert.Equal(t, model.KeyStatusRevoked, key.Status)
	})

	t.Run("不能撤销活跃密钥", func(t *testing.T) {
		svc, _ := createTestKeyRotationService(t)

		// 创建活跃密钥
		activeKey, err := svc.RotateKey(ctx)
		require.NoError(t, err)

		// 尝试撤销活跃密钥应该失败
		err = svc.RevokeKey(ctx, activeKey.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot revoke active key")
	})

	t.Run("密钥不存在", func(t *testing.T) {
		svc, _ := createTestKeyRotationService(t)

		err := svc.RevokeKey(ctx, "nonexistent-key")
		assert.Error(t, err)
	})
}

// ============================================================================
// GetKeyStatus 测试
// ============================================================================

func TestKeyRotationService_GetKeyStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("获取密钥状态", func(t *testing.T) {
		svc, _ := createTestKeyRotationService(t)

		// 创建两个密钥
		_, err := svc.RotateKey(ctx)
		require.NoError(t, err)

		_, err = svc.RotateKey(ctx)
		require.NoError(t, err)

		// 获取状态
		keys, err := svc.GetKeyStatus(ctx)

		require.NoError(t, err)
		assert.Len(t, keys, 2)
	})
}

// ============================================================================
// InitializeFirstKey 测试
// ============================================================================

func TestKeyRotationService_InitializeFirstKey(t *testing.T) {
	ctx := context.Background()

	t.Run("首次初始化", func(t *testing.T) {
		svc, _ := createTestKeyRotationService(t)

		key, err := svc.InitializeFirstKey(ctx)

		require.NoError(t, err)
		assert.NotNil(t, key)
		assert.Equal(t, model.KeyStatusActive, key.Status)
	})

	t.Run("已有活跃密钥时返回现有密钥", func(t *testing.T) {
		svc, _ := createTestKeyRotationService(t)

		// 先初始化一次
		firstKey, err := svc.InitializeFirstKey(ctx)
		require.NoError(t, err)

		// 再次初始化应返回相同密钥
		sameKey, err := svc.InitializeFirstKey(ctx)

		require.NoError(t, err)
		assert.Equal(t, firstKey.ID, sameKey.ID)
	})
}
