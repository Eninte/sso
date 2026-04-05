// Package postgres_test MFA恢复码存储单元测试
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"
	"github.com/your-org/sso/internal/store/postgres"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

func setupMFAStore(t *testing.T) (*postgres.Store, *sql.DB) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("跳过集成测试：未设置DATABASE_URL环境变量")
	}
	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))
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
