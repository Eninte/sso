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
	// 阶段 D 审查修复：原测试用 time.Now() 作为 ExpiresAt，
	// 但 CanVerify 用 ExpiresAt.Before(time.Now()) 判断过期（严格小于），
	// 当 ExpiresAt == time.Now() 时返回 true（未过期），
	// 而测试期望 false，导致偶发性失败。
	// 修复：使用已过去的时间（1ms前）确保确定性地过期
	pastTime := time.Now().Add(-1 * time.Millisecond)

	t.Run("恰好过期时间返回false", func(t *testing.T) {
		key := &model.KeyVersion{
			Status:    model.KeyStatusActive,
			ExpiresAt: &pastTime,
		}
		result := key.CanVerify()
		// ExpiresAt 已在 1ms 前过期，CanVerify 应返回 false
		assert.False(t, result)
	})

	t.Run("非常远的未来时间返回true", func(t *testing.T) {
		farFuture := time.Now().Add(100 * 365 * 24 * time.Hour) // 100年后
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
