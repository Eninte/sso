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

func TestHashRecoveryCode_Deterministic(t *testing.T) {
	st, db := setupMFAStore(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 设置固定密钥
	postgres.SetMFARecoveryHMACKey("deterministic-test-key-abc")
	defer postgres.SetMFARecoveryHMACKey("")

	// 清理
	cleanupMFAData(t, db, "det-user")
	defer cleanupMFAData(t, db, "det-user")

	// 通过StoreMFARecoveryCodes测试哈希逻辑
	// 先存储恢复码
	err := st.StoreMFARecoveryCodes(ctx, "det-user", []string{"code1", "code2"})
	require.NoError(t, err)

	// 获取未使用的恢复码
	codes, err := st.GetUnusedMFARecoveryCodes(ctx, "det-user")
	require.NoError(t, err)
	assert.Len(t, codes, 2)

	// 再次调用同一哈希密钥，应产生相同结果
	err = st.StoreMFARecoveryCodes(ctx, "det-user", []string{"code1", "code2"})
	require.NoError(t, err)

	codes2, err := st.GetUnusedMFARecoveryCodes(ctx, "det-user")
	require.NoError(t, err)
	assert.Len(t, codes2, 2)
	assert.Equal(t, codes, codes2)
}

func TestHashRecoveryCode_EmptyKey_ReturnsError(t *testing.T) {
	st, db := setupMFAStore(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 设置空密钥
	postgres.SetMFARecoveryHMACKey("")
	defer postgres.SetMFARecoveryHMACKey("")

	// 验证应失败
	result, err := st.VerifyAndUseMFARecoveryCode(ctx, "empty-key-user", "any-code")
	require.Error(t, err)
	assert.False(t, result)
	assert.Contains(t, err.Error(), "MFA recovery HMAC key not set")
}

func TestHashRecoveryCode_DifferentKeysDifferentHashes(t *testing.T) {
	st, db := setupMFAStore(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cleanupMFAData(t, db, "diff-key-user")
	defer cleanupMFAData(t, db, "diff-key-user")

	// 用key-A存储恢复码
	postgres.SetMFARecoveryHMACKey("key-A-for-storage")
	require.NoError(t, st.StoreMFARecoveryCodes(ctx, "diff-key-user", []string{"code-A"}))

	// 用key-B验证，应找不到匹配
	postgres.SetMFARecoveryHMACKey("key-B-for-verification")
	result, err := st.VerifyAndUseMFARecoveryCode(ctx, "diff-key-user", "code-A")
	require.NoError(t, err)
	assert.False(t, result)

	// 用key-A验证，应找到匹配
	postgres.SetMFARecoveryHMACKey("key-A-for-storage")
	result2, err := st.VerifyAndUseMFARecoveryCode(ctx, "diff-key-user", "code-A")
	require.NoError(t, err)
	assert.True(t, result2)

	postgres.SetMFARecoveryHMACKey("")
}
