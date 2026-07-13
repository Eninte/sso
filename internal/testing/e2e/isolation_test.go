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

	// Drain-window tests use fakeClock for determinism.
	// Timeout tests use real time since context.WithTimeout is real-time.

	t.Run("exits successfully after drain window on zero deletes", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Short drain window (100ms) so the test runs fast with fakeClock.
		// Backoff 20→40→80→…ms, cumulative after 3 calls = 140ms.
		// Drain check after call 3: elapsed = 140ms ≥ 100ms → drain.
		fc := newFakeClock(time.Now())
		helper := NewIsolationHelper(db, nil).WithClock(fc)
		helper.DrainWindow = 100 * time.Millisecond

		setupPhase1to3(mock, pattern, extraUserID)

		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("context cancellation exits promptly", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Use real time for cancellation — fakeClock iterations are
		// instant and would outrun the cancel timer.
		helper := NewIsolationHelper(db, nil)
		helper.DrainWindow = 100 * time.Millisecond

		setupPhase1to3(mock, pattern, extraUserID)

		// The first iteration sleeps 20ms and completes its delete.
		// Cancel fires at 30ms, during the 2nd sleep.
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))

		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(30*time.Millisecond, cancel)

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("late-arriving audit write resets drain window", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		fc := newFakeClock(time.Now())
		helper := NewIsolationHelper(db, nil).WithClock(fc)
		helper.DrainWindow = 100 * time.Millisecond

		setupPhase1to3(mock, pattern, extraUserID)

		// Call 1: no writes → 0 rows.
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		// Call 2: late write → 1 row, resets lastDeleteTime.
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 1))
		// Calls 3-4: drain window from call 2 → return nil.
		// Backoff at call 3 = 80ms, cumulative from call 2 = 80ms.
		// Call 4: backoff = 100ms, cumulative = 180ms ≥ 100ms → drain.
		for i := 0; i < 2; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("timeout returns error when drain is not reached", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Use real time + long drain window so the 200ms context deadline
		// fires before the drain window completes.
		helper := NewIsolationHelper(db, nil)
		helper.DrainWindow = 2 * time.Second

		setupPhase1to3(mock, pattern, extraUserID)

		// With 200ms context and backoff 20→40→80→100ms, 3 calls fit
		// (cumulative 140ms) before the deadline fires during the 4th sleep.
		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 1))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no-deadline context uses settle timeout cap", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Use real time + long drain window so the 2s settle timeout fires
		// before the drain window completes.
		helper := NewIsolationHelper(db, nil)
		helper.DrainWindow = 5 * time.Second // longer than 2s settle cap

		setupPhase1to3(mock, pattern, extraUserID)

		// With 2s settle cap and backoff 20→40→80→100→100→…ms,
		// 21 calls fit (cumulative 1940ms) before the 22nd sleep
		// pushes past 2000ms → settle timeout.
		for i := 0; i < 21; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 1))
		}

		err = helper.CleanupTestDataByPattern(context.Background(), pattern, extraUserID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "settle timed out")
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
