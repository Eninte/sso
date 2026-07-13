package e2e

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// IsolationHelper Tests
// ============================================================================

func TestIsolationHelper_WithTransaction(t *testing.T) {
	t.Run("successful transaction rollback", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectBegin()
		mock.ExpectRollback()

		helper := NewIsolationHelper(db, nil)

		err = helper.WithTransaction(context.Background(), func(tx *sql.Tx) error {
			// Test function executed successfully
			return nil
		})

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("transaction rollback after function error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectBegin()
		mock.ExpectRollback()

		helper := NewIsolationHelper(db, nil)

		expectedErr := assert.AnError
		err = helper.WithTransaction(context.Background(), func(tx *sql.Tx) error {
			return expectedErr
		})

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("nil database connection", func(t *testing.T) {
		helper := NewIsolationHelper(nil, nil)

		err := helper.WithTransaction(context.Background(), func(tx *sql.Tx) error {
			return nil
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database connection is nil")
	})
}

func TestIsolationHelper_WithRedisNamespace(t *testing.T) {
	t.Run("nil redis client", func(t *testing.T) {
		helper := NewIsolationHelper(nil, nil)

		err := helper.WithRedisNamespace(context.Background(), "test", func(nsClient *NamespacedRedisClient) error {
			return nil
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "redis client is nil")
	})
}

// ============================================================================
// Audit Log Cleanup Tests
// ============================================================================

func TestIsolationHelper_AuditLogCleanup(t *testing.T) {
	t.Run("collects user IDs then deletes audit logs", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		helper := NewIsolationHelper(db, nil)
		pattern := "%e2e_123_TestLogin%"

		// Phase 1: collectUserIDsByPattern queries users table
		userRows := sqlmock.NewRows([]string{"id"}).
			AddRow("uuid-user-1").
			AddRow("uuid-user-2")
		mock.ExpectQuery(`SELECT id::text FROM users WHERE email LIKE \$1 OR id::text LIKE \$1`).
			WithArgs(pattern).
			WillReturnRows(userRows)

		// Phase 2: deleteAuditLogsByUserIDs deletes audit logs for collected UUIDs
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN \(\$1, \$2\)`).
			WithArgs("uuid-user-1", "uuid-user-2").
			WillReturnResult(sqlmock.NewResult(0, 3))

		// Phase 2b: deleteAuditLogsByDetails deletes audit logs matching pattern in details
		mock.ExpectExec(`DELETE FROM audit_logs WHERE details::text LIKE \$1`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 1))

		// Phase 3: remaining tables (verification_tokens, reset_tokens, etc.)
		mock.ExpectExec(`DELETE FROM verification_tokens WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM reset_tokens WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM authorization_codes WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM tokens WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM oauth_clients WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM users WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))

		err = helper.CleanupTestDataByPattern(context.Background(), pattern)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("skips audit log deletion when no matching users", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		helper := NewIsolationHelper(db, nil)
		pattern := "%e2e_nonexistent%"

		// Phase 1: no matching users
		userRows := sqlmock.NewRows([]string{"id"})
		mock.ExpectQuery(`SELECT id::text FROM users WHERE email LIKE \$1 OR id::text LIKE \$1`).
			WithArgs(pattern).
			WillReturnRows(userRows)

		// Phase 2: skipped (no user IDs)

		// Phase 2b: deleteAuditLogsByDetails still runs (details may contain testID)
		mock.ExpectExec(`DELETE FROM audit_logs WHERE details::text LIKE \$1`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))

		// Phase 3: remaining tables still cleaned
		mock.ExpectExec(`DELETE FROM verification_tokens WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM reset_tokens WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM authorization_codes WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM tokens WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM oauth_clients WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM users WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))

		err = helper.CleanupTestDataByPattern(context.Background(), pattern)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("nil database connection", func(t *testing.T) {
		helper := NewIsolationHelper(nil, nil)
		err := helper.CleanupTestDataByPattern(context.Background(), "%test%")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database connection is nil")
	})
}

// ============================================================================
// Phase 4 Async Poll Tests
// ============================================================================

// setupPhase1to3 expects the standard Phase 1-3 queries for CleanupTestDataByPattern
// with the given pattern and one extraUserID. Callers add their own Phase 4 expectations.
func setupPhase1to3(mock sqlmock.Sqlmock, pattern, extraUserID string) {
	// Phase 1: collect user IDs
	userRows := sqlmock.NewRows([]string{"id"}).AddRow(extraUserID)
	mock.ExpectQuery(`SELECT id::text FROM users WHERE`).
		WithArgs(pattern).WillReturnRows(userRows)
	// Phase 2: delete audit logs by user ID
	mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
		WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 1))
	// Phase 2b: delete audit logs by details
	mock.ExpectExec(`DELETE FROM audit_logs WHERE details::text LIKE`).
		WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
	// Phase 3: remaining tables
	for _, table := range []string{
		"verification_tokens", "reset_tokens", "authorization_codes",
		"tokens", "oauth_clients", "users",
	} {
		mock.ExpectExec(`DELETE FROM `+table+` WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
	}
}

func TestIsolationHelper_Phase4_AsyncPoll(t *testing.T) {
	const pattern = "%e2e_async_test%"
	const extraUserID = "uuid-async-user"

	t.Run("polls until context deadline does not early-exit on zero deletes", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		helper := NewIsolationHelper(db, nil)

		setupPhase1to3(mock, pattern, extraUserID)

		// Phase 4: queue enough deletes for the deadline window.
		// With 200ms timeout and backoff 20→40→80→100ms, ~3 calls fit
		// before the context expires. All return 0 rows — the loop must
		// NOT stop on that; only the deadline ends it.
		const phase4Calls = 3
		for i := 0; i < phase4Calls; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		start := time.Now()
		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		elapsed := time.Since(start)

		// Phase 4 now returns an error when the context expires, since we
		// cannot confirm all async audit writes have been drained.
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		// Must have polled for roughly the full context budget, not returned early.
		assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(150),
			"should poll until context deadline, not exit early on zero deletes")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("context cancellation exits promptly", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		helper := NewIsolationHelper(db, nil)

		setupPhase1to3(mock, pattern, extraUserID)

		// Phase 4: one expectation consumed before cancel fires.
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))

		ctx, cancel := context.WithCancel(context.Background())
		// Cancel after 30ms — Phases 1-3 finish near-instantly with mocks,
		// so the cancel fires during Phase 4's first or second sleep.
		time.AfterFunc(30*time.Millisecond, cancel)

		start := time.Now()
		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		elapsed := time.Since(start)

		// Phase 4 now returns an error on cancellation.
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		// Must exit within ~one backoff window (≤100ms) of the cancel signal.
		assert.Less(t, elapsed.Milliseconds(), int64(200),
			"should exit promptly on context cancellation")
	})

	t.Run("late-arriving audit write is still cleaned", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		helper := NewIsolationHelper(db, nil)

		setupPhase1to3(mock, pattern, extraUserID)

		// Phase 4, call 1: arrives before the async worker writes → 0 rows.
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		// Phase 4, call 2: the worker has now written → 1 row deleted.
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 1))
		// Subsequent polls until deadline: all return 0.
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		// The late write was cleaned (call 2 deleted 1 row), but the context
		// expired before we could confirm no more writes are coming — so an
		// error is returned indicating incomplete settle.
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no-deadline context uses settle timeout cap instead of polling forever", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		helper := NewIsolationHelper(db, nil)

		setupPhase1to3(mock, pattern, extraUserID)

		// Phase 4: with context.Background() (no deadline), the 2-second
		// settle cap must terminate the loop. Queue enough expectations
		// to cover ~2s of polling at 20→40→80→100ms backoff.
		// Worst case: 20+40+80+100*16 ≈ 1740ms → ~18 calls.
		// We set an unlimited sequence to be safe.
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0)).
			WillDelayFor(0)
		// Use regexp match so sqlmock doesn't panic on unexpected calls —
		// it already returns an error for unmatched calls, which is fine.
		// However, to be robust, just set a high call count.
		for i := 0; i < 25; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}

		start := time.Now()
		err = helper.CleanupTestDataByPattern(context.Background(), pattern, extraUserID)
		elapsed := time.Since(start)

		// Must return an error (not nil) indicating settle timeout.
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "settle timed out")
		// Must terminate within roughly the 2s cap (plus some tolerance).
		assert.Less(t, elapsed.Milliseconds(), int64(3000),
			"should terminate at settle timeout cap, not poll forever")
		assert.Greater(t, elapsed.Milliseconds(), int64(1500),
			"should poll for roughly the full settle budget")
	})
}

// ============================================================================
// NamespacedRedisClient Tests
// ============================================================================

func TestNamespacedRedisClient_NamespaceKey(t *testing.T) {
	client := &NamespacedRedisClient{
		namespace: "test:123",
	}

	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "simple key",
			key:      "user:123",
			expected: "test:123:user:123",
		},
		{
			name:     "key with colon",
			key:      "session:abc:data",
			expected: "test:123:session:abc:data",
		},
		{
			name:     "empty key",
			key:      "",
			expected: "test:123:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.namespaceKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNamespacedRedisClient_GetNamespace(t *testing.T) {
	namespace := "test:namespace:123"
	client := &NamespacedRedisClient{
		namespace: namespace,
	}

	assert.Equal(t, namespace, client.GetNamespace())
}

// ============================================================================
// Integration Tests (require real Redis)
// ============================================================================

func TestNamespacedRedisClient_Integration(t *testing.T) {
	// Skip if Redis is not available
	redisAddr := "localhost:6379"
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping integration test")
	}
	defer client.Close()

	t.Run("set and get with namespace", func(t *testing.T) {
		ctx := context.Background()
		namespace := "test:integration:" + time.Now().Format("20060102150405")

		nsClient := &NamespacedRedisClient{
			client:    client,
			namespace: namespace,
		}

		// Set a value
		err := nsClient.Set(ctx, "key1", "value1", 0)
		require.NoError(t, err)

		// Get the value
		val, err := nsClient.Get(ctx, "key1")
		require.NoError(t, err)
		assert.Equal(t, "value1", val)

		// Verify the key has namespace prefix in Redis
		exists, err := client.Exists(ctx, namespace+":key1").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), exists)

		// Cleanup
		err = nsClient.CleanupNamespace(ctx)
		require.NoError(t, err)

		// Verify cleanup
		exists, err = client.Exists(ctx, namespace+":key1").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})

	t.Run("cleanup namespace with multiple keys", func(t *testing.T) {
		ctx := context.Background()
		namespace := "test:cleanup:" + time.Now().Format("20060102150405")

		nsClient := &NamespacedRedisClient{
			client:    client,
			namespace: namespace,
		}

		// Set multiple values
		err := nsClient.Set(ctx, "key1", "value1", 0)
		require.NoError(t, err)
		err = nsClient.Set(ctx, "key2", "value2", 0)
		require.NoError(t, err)
		err = nsClient.Set(ctx, "key3", "value3", 0)
		require.NoError(t, err)

		// Verify all keys exist
		exists, err := client.Exists(ctx, namespace+":key1", namespace+":key2", namespace+":key3").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(3), exists)

		// Cleanup namespace
		err = nsClient.CleanupNamespace(ctx)
		require.NoError(t, err)

		// Verify all keys are deleted
		exists, err = client.Exists(ctx, namespace+":key1", namespace+":key2", namespace+":key3").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})
}

func TestIsolationHelper_Integration(t *testing.T) {
	// Skip if Redis is not available
	redisAddr := "localhost:6379"
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping integration test")
	}
	defer redisClient.Close()

	t.Run("with redis namespace integration", func(t *testing.T) {
		ctx := context.Background()
		helper := NewIsolationHelper(nil, redisClient)
		namespace := "test:helper:" + time.Now().Format("20060102150405")

		// Execute function with namespace
		err := helper.WithRedisNamespace(ctx, namespace, func(nsClient *NamespacedRedisClient) error {
			// Set some test data
			err := nsClient.Set(ctx, "testkey", "testvalue", 0)
			require.NoError(t, err)

			// Verify we can read it
			val, err := nsClient.Get(ctx, "testkey")
			require.NoError(t, err)
			assert.Equal(t, "testvalue", val)

			return nil
		})

		require.NoError(t, err)

		// Verify cleanup - key should not exist
		exists, err := redisClient.Exists(ctx, namespace+":testkey").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})
}
