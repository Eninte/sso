//go:build integration

// Package postgres_test MFA恢复码存储单元测试
package postgres_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/store/postgres"
	"github.com/example/sso/internal/util/testutil"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// setupMFAStore 返回已 ping 通的真实 PG 连接（带重试与超时）
// 复用 testutil.ConnectTestDB，与全仓真实 DB 测试共享重试机制
func setupMFAStore(t *testing.T) (*postgres.Store, *sql.DB) {
	t.Helper()
	db := testutil.ConnectTestDB(t)
	return postgres.New(db), db
}

func cleanupMFAData(t *testing.T, db *sql.DB, userID string) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM mfa_recovery_codes WHERE user_id = $1", userID)
}

// ============================================================================
// SetMFARecoveryHMACKey 并发安全测试
// ============================================================================

func TestSetMFARecoveryHMACKey_Concurrent(t *testing.T) {
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			postgres.SetMFARecoveryHMACKey(string(rune('A' + idx%26)))
		}(i)
	}
	wg.Wait()
}

func TestSetMFARecoveryHMACKey_AfterEmpty(t *testing.T) {
	postgres.SetMFARecoveryHMACKey("")
	assert.NotPanics(t, func() {
		postgres.SetMFARecoveryHMACKey("valid-key-after-empty")
	})
}

// ============================================================================
// hashRecoveryCode 哈希逻辑测试（通过DB验证）
// ============================================================================

func TestHashRecoveryCode_EmptyKey_ReturnsError(t *testing.T) {
	st, db := setupMFAStore(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 设置空密钥
	postgres.SetMFARecoveryHMACKey("")
	defer postgres.SetMFARecoveryHMACKey("")

	// 验证应失败
	result, err := st.VerifyAndUseMFARecoveryCode(ctx, "00000000-0000-0000-0000-000000000001", "any-code")
	require.Error(t, err)
	assert.False(t, result)
	assert.Contains(t, err.Error(), "MFA recovery HMAC key not set")
}

func TestHashRecoveryCode_ValidKey_VerifyReturnsFalseForEmpty(t *testing.T) {
	st, db := setupMFAStore(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 设置密钥
	postgres.SetMFARecoveryHMACKey("test-verify-key")
	defer postgres.SetMFARecoveryHMACKey("")

	// 验证不存在的用户，应返回false
	result, err := st.VerifyAndUseMFARecoveryCode(ctx, "00000000-0000-0000-0000-000000000099", "non-existent-code")
	require.NoError(t, err)
	assert.False(t, result)
}
