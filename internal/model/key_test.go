package model_test

import (
	"testing"
	"time"

	"github.com/example/sso/internal/model"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
// KeyVersion 状态方法测试
// ============================================================================

func TestKeyVersion_IsActive(t *testing.T) {
	tests := []struct {
		name     string
		status   model.KeyStatus
		expected bool
	}{
		{"active状态返回true", model.KeyStatusActive, true},
		{"deprecated状态返回false", model.KeyStatusDeprecated, false},
		{"revoked状态返回false", model.KeyStatusRevoked, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &model.KeyVersion{Status: tt.status}
			assert.Equal(t, tt.expected, key.IsActive())
		})
	}
}

func TestKeyVersion_IsDeprecated(t *testing.T) {
	tests := []struct {
		name     string
		status   model.KeyStatus
		expected bool
	}{
		{"deprecated状态返回true", model.KeyStatusDeprecated, true},
		{"active状态返回false", model.KeyStatusActive, false},
		{"revoked状态返回false", model.KeyStatusRevoked, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &model.KeyVersion{Status: tt.status}
			assert.Equal(t, tt.expected, key.IsDeprecated())
		})
	}
}

func TestKeyVersion_IsRevoked(t *testing.T) {
	tests := []struct {
		name     string
		status   model.KeyStatus
		expected bool
	}{
		{"revoked状态返回true", model.KeyStatusRevoked, true},
		{"active状态返回false", model.KeyStatusActive, false},
		{"deprecated状态返回false", model.KeyStatusDeprecated, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &model.KeyVersion{Status: tt.status}
			assert.Equal(t, tt.expected, key.IsRevoked())
		})
	}
}

func TestKeyVersion_CanVerify(t *testing.T) {
	now := time.Now()
	pastTime := now.Add(-1 * time.Hour)
	futureTime := now.Add(1 * time.Hour)

	tests := []struct {
		name      string
		status    model.KeyStatus
		expiresAt *time.Time
		expected  bool
	}{
		{"active状态无过期时间返回true", model.KeyStatusActive, nil, true},
		{"active状态未过期返回true", model.KeyStatusActive, &futureTime, true},
		{"active状态已过期返回false", model.KeyStatusActive, &pastTime, false},
		{"deprecated状态无过期时间返回true", model.KeyStatusDeprecated, nil, true},
		{"deprecated状态未过期返回true", model.KeyStatusDeprecated, &futureTime, true},
		{"deprecated状态已过期返回false", model.KeyStatusDeprecated, &pastTime, false},
		{"revoked状态无过期时间返回false", model.KeyStatusRevoked, nil, false},
		{"revoked状态未过期返回false", model.KeyStatusRevoked, &futureTime, false},
		{"revoked状态已过期返回false", model.KeyStatusRevoked, &pastTime, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &model.KeyVersion{
				Status:    tt.status,
				ExpiresAt: tt.expiresAt,
			}
			assert.Equal(t, tt.expected, key.CanVerify())
		})
	}
}

// ============================================================================
// KeyVersion 边界条件测试
// ============================================================================

func TestKeyVersion_CanVerify_BoundaryConditions(t *testing.T) {
	// 测试恰好过期的情况
	now := time.Now()

	t.Run("恰好过期时间返回false", func(t *testing.T) {
		key := &model.KeyVersion{
			Status:    model.KeyStatusActive,
			ExpiresAt: &now,
		}
		// 由于time.Now()在执行时可能已经稍微过去，这个测试可能有细微差异
		// 但逻辑上应该是false
		result := key.CanVerify()
		// 如果ExpiresAt恰好等于或稍早于当前时间，应该返回false
		if key.ExpiresAt.Before(time.Now()) || key.ExpiresAt.Equal(time.Now()) {
			assert.False(t, result)
		}
	})

	t.Run("非常远的未来时间返回true", func(t *testing.T) {
		farFuture := now.Add(100 * 365 * 24 * time.Hour) // 100年后
		key := &model.KeyVersion{
			Status:    model.KeyStatusActive,
			ExpiresAt: &farFuture,
		}
		assert.True(t, key.CanVerify())
	})
}

// ============================================================================
// KeyStatus 常量测试
// ============================================================================

func TestKeyStatus_Constants(t *testing.T) {
	assert.Equal(t, model.KeyStatus("active"), model.KeyStatusActive)
	assert.Equal(t, model.KeyStatus("deprecated"), model.KeyStatusDeprecated)
	assert.Equal(t, model.KeyStatus("revoked"), model.KeyStatusRevoked)
}
