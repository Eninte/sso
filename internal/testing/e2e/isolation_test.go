package e2e

import (
	"context"
	"database/sql"
	"fmt"
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
		mock.ExpectExec(`DELETE FROM ` + table + ` WHERE`).
			WithArgs(pattern).WillReturnResult(sqlmock.NewResult(0, 0))
	}
}

func TestIsolationHelper_Phase4_AsyncPoll(t *testing.T) {
	const pattern = "%e2e_async_test%"
	const extraUserID = "uuid-async-user"

	sweepMock := func(mock sqlmock.Sqlmock) {
		// Two consecutive empty sweep passes
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		// Post-sweep COUNT check → 0 remaining
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		// Post-count drain re-sweep → no late writes
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
	}

	// All tests use fakeClock + short timeouts for determinism.
	// Phase 4 always returns nil — the final sweep guarantees cleanup.

	t.Run("exits successfully after drain window", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		fc := newFakeClock(time.Now())
		helper := NewIsolationHelper(db, nil).WithClock(fc)
		helper.DrainWindow = 100 * time.Millisecond
		helper.SettleTimeout = 1 * time.Second
		helper.SweepTimeout = 1 * time.Second

		setupPhase1to3(mock, pattern, extraUserID)

		// 3 poll calls (backoff 20+40+80=140ms ≥ 100ms drain) + 2 sweep calls
		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		sweepMock(mock)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("context cancellation still sweeps", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Use real time so the cancel fires mid-iteration.
		helper := NewIsolationHelper(db, nil)
		helper.DrainWindow = 1 * time.Second
		helper.SettleTimeout = 1 * time.Second
		helper.SweepTimeout = 1 * time.Second

		setupPhase1to3(mock, pattern, extraUserID)

		// Phase 1-3 finish in <1ms.  Phase 4 sleeps 20ms on first
		// iteration; cancel at 10ms fires settleCtx.Done() before the
		// timer → 0 poll calls, 2 sweep calls.
		sweepMock(mock)

		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(10*time.Millisecond, cancel)

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("late-arriving audit write is caught by sweep", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		fc := newFakeClock(time.Now())
		helper := NewIsolationHelper(db, nil).WithClock(fc)
		helper.DrainWindow = 100 * time.Millisecond
		helper.SettleTimeout = 1 * time.Second
		helper.SweepTimeout = 1 * time.Second

		setupPhase1to3(mock, pattern, extraUserID)

		// Poll: all return 0 — the async worker hasn't written yet.
		// Backoff 20+40+80=140ms ≥ 100ms drain → poll exits.
		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		// Sweep: first pass finds the late write (1 row), then 2 empty
		// passes → done.  This proves the sweep is what catches it.
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		// Post-sweep COUNT → 0 remaining
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		// Post-count drain re-sweep → no late writes
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("settle timeout still sweeps", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Use real time so the settle timeout fires during the loop.
		helper := NewIsolationHelper(db, nil)
		helper.DrainWindow = 2 * time.Second // longer than settle
		helper.SettleTimeout = 200 * time.Millisecond
		helper.SweepTimeout = 1 * time.Second

		setupPhase1to3(mock, pattern, extraUserID)

		// Backoff 20→40→80→…ms, settle at 200ms.
		// 3 poll calls (cumulative 140ms) + 2 sweep calls.
		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		sweepMock(mock)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no-deadline context uses settle timeout cap", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		helper := NewIsolationHelper(db, nil)
		helper.DrainWindow = 2 * time.Second
		helper.SettleTimeout = 200 * time.Millisecond
		helper.SweepTimeout = 1 * time.Second

		setupPhase1to3(mock, pattern, extraUserID)

		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		sweepMock(mock)

		err = helper.CleanupTestDataByPattern(context.Background(), pattern, extraUserID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("write after sweep is caught by retry", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		fc := newFakeClock(time.Now())
		helper := NewIsolationHelper(db, nil).WithClock(fc)
		helper.DrainWindow = 100 * time.Millisecond
		helper.SettleTimeout = 1 * time.Second
		helper.SweepTimeout = 1 * time.Second

		// All mocks in consumption order: attempt 1, then attempt 2.
		// sqlmock consumes expectations sequentially.

		// --- Attempt 1 ---
		setupPhase1to3(mock, pattern, extraUserID) // 8 mocks (SELECT + 7 DELETEs)
		// Phase 4 poll: 3 calls returning 0 → drain
		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		// Phase 4 sweep: 2 empty passes → done
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		// Post-sweep COUNT → 1 residual → ErrResidualAuditLogs → retry
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		// --- Retry delay: After(100ms) via fakeClock ---
		// (consumed by CleanupTestDataByPatternWithRetry between attempts)

		// --- Attempt 2 ---
		setupPhase1to3(mock, pattern, extraUserID) // 8 more mocks
		// Phase 4 poll: 3 calls returning 0
		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		// Phase 4 sweep: first pass finds late write (1 row), then 2 empty
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		// Post-sweep COUNT → 0 remaining → success
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		// Post-count drain re-sweep → no late writes
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = helper.CleanupTestDataByPatternWithRetry(ctx, pattern, 100*time.Millisecond, 2, extraUserID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("post-sweep count failure returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		fc := newFakeClock(time.Now())
		helper := NewIsolationHelper(db, nil).WithClock(fc)
		helper.DrainWindow = 100 * time.Millisecond
		helper.SettleTimeout = 1 * time.Second
		helper.SweepTimeout = 1 * time.Second

		setupPhase1to3(mock, pattern, extraUserID)
		// Phase 4 poll: 3 calls returning 0 → drain
		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		// Phase 4 sweep: 2 empty passes → done
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		// Post-sweep COUNT → query error → should propagate
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnError(fmt.Errorf("connection refused"))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "post-sweep count failed")
		assert.Contains(t, err.Error(), "connection refused")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("post-count drain catches late write after COUNT=0", func(t *testing.T) {
		// Race scenario: sweep finishes, COUNT returns 0, but an async
		// write commits during the post-count drain window.  The re-sweep
		// after the drain catches it.
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		fc := newFakeClock(time.Now())
		helper := NewIsolationHelper(db, nil).WithClock(fc)
		helper.DrainWindow = 100 * time.Millisecond
		helper.SettleTimeout = 1 * time.Second
		helper.SweepTimeout = 1 * time.Second

		setupPhase1to3(mock, pattern, extraUserID)
		// Phase 4 poll: 3 calls → drain
		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		// Sweep: 2 empty passes → done
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		// Post-sweep COUNT → 0 (the late write hasn't committed yet)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		// Post-count drain re-sweep → catches the late write (1 row)
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 1))
		// Re-count after catching late write → 0 remaining
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("post-count re-sweep DELETE failure returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		fc := newFakeClock(time.Now())
		helper := NewIsolationHelper(db, nil).WithClock(fc)
		helper.DrainWindow = 100 * time.Millisecond
		helper.SettleTimeout = 1 * time.Second
		helper.SweepTimeout = 1 * time.Second

		setupPhase1to3(mock, pattern, extraUserID)
		// Phase 4 poll → drain
		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		// Sweep: 2 empty passes
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		// Post-sweep COUNT → 0
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		// Post-count re-sweep DELETE → database error
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnError(fmt.Errorf("connection reset"))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "post-count re-sweep failed")
		assert.Contains(t, err.Error(), "connection reset")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("post-count re-check COUNT failure returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		fc := newFakeClock(time.Now())
		helper := NewIsolationHelper(db, nil).WithClock(fc)
		helper.DrainWindow = 100 * time.Millisecond
		helper.SettleTimeout = 1 * time.Second
		helper.SweepTimeout = 1 * time.Second

		setupPhase1to3(mock, pattern, extraUserID)
		// Phase 4 poll → drain
		for i := 0; i < 3; i++ {
			mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
				WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		// Sweep: 2 empty passes
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		// Post-sweep COUNT → 0
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		// Post-count re-sweep catches late write
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 1))
		// Re-count → database error
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnError(fmt.Errorf("connection refused"))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "post-count re-check failed")
		assert.Contains(t, err.Error(), "connection refused")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("post-count drain interrupted by sweep context expiry", func(t *testing.T) {
		// sweepCtx expires during the 200ms post-count drain window.
		// The function must return an error so WithRetry can retry.
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		helper := NewIsolationHelper(db, nil)
		helper.DrainWindow = 1 * time.Second
		helper.SettleTimeout = 1 * time.Millisecond // poll exits immediately, 0 DELETEs
		helper.SweepTimeout = 1 * time.Second

		setupPhase1to3(mock, pattern, extraUserID)
		// Sweep: 2 empty passes (poll did 0 iterations)
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnResult(sqlmock.NewResult(0, 0))
		// Post-sweep COUNT → 0
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs WHERE user_id IN`).
			WithArgs(extraUserID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		// No re-sweep mock — sweepCtx expires before drain completes.

		// sweepCtx = min(1s, parent_remaining).  Sweep loop does 1 drain
		// wait (200ms) then breaks on second empty pass, finishing at
		// ~200ms.  COUNT at ~200ms.  sweepCtx expires at 300ms, which
		// is before After(200ms) fires at ~400ms → Done wins.
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		err = helper.CleanupTestDataByPattern(ctx, pattern, extraUserID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "post-count drain interrupted")
		assert.NoError(t, mock.ExpectationsWereMet())
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
