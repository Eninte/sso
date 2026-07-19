// Package e2e provides test isolation helpers for E2E testing.
package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// clock abstracts time.Now for deterministic testing.
type clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// realClock delegates to the standard time package.
type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// fakeClock is a manually-advancing clock for tests.
// Call Advance to move time forward; Now returns the current fake time.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{now: start} }

func (fc *fakeClock) Now() time.Time {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.now
}

func (fc *fakeClock) Advance(d time.Duration) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.now = fc.now.Add(d)
}

// After returns a channel that fires after advancing by d.
// In tests this fires immediately (no real sleep), so the loop
// iterations are fully deterministic.
func (fc *fakeClock) After(d time.Duration) <-chan time.Time {
	fc.Advance(d)
	ch := make(chan time.Time, 1)
	ch <- fc.Now()
	return ch
}

// ============================================================================
// Test Data Isolation Helpers
// ============================================================================

// IsolationHelper provides utilities for test data isolation and cleanup.
type IsolationHelper struct {
	db    *sql.DB
	redis *redis.Client
	clock clock

	// DrainWindow is how long Phase 4 waits with zero deletes before
	// declaring the async audit queue drained.  Defaults to 1 s.
	DrainWindow time.Duration

	// SettleTimeout is the hard cap for Phase 4 polling.  The loop
	// terminates after this duration regardless of drain state.
	// Defaults to 2 s.  Must be longer than DrainWindow.
	SettleTimeout time.Duration

	// SweepTimeout is the hard cap for the post-poll sweep loop.
	// Defaults to 5 s.
	SweepTimeout time.Duration
}

// NewIsolationHelper creates a new isolation helper instance.
func NewIsolationHelper(db *sql.DB, redis *redis.Client) *IsolationHelper {
	return &IsolationHelper{
		db:    db,
		redis: redis,
		clock: realClock{},
	}
}

// WithClock replaces the time source (for testing).  Returns the same
// helper so calls can be chained: NewIsolationHelper(db, nil).WithClock(fc).
func (ih *IsolationHelper) WithClock(c clock) *IsolationHelper {
	ih.clock = c
	return ih
}

// drainWindow returns the configured drain window or the default (1 s).
func (ih *IsolationHelper) drainWindow() time.Duration {
	if ih.DrainWindow > 0 {
		return ih.DrainWindow
	}
	return 1 * time.Second
}

// settleTimeout returns the configured settle timeout or the default (2 s).
func (ih *IsolationHelper) settleTimeout() time.Duration {
	if ih.SettleTimeout > 0 {
		return ih.SettleTimeout
	}
	return 2 * time.Second
}

// sweepTimeout returns the configured sweep timeout or the default (5 s).
func (ih *IsolationHelper) sweepTimeout() time.Duration {
	if ih.SweepTimeout > 0 {
		return ih.SweepTimeout
	}
	return 5 * time.Second
}

// WithTransaction executes a function within a database transaction that is
// automatically rolled back after execution, ensuring test data isolation.
func (ih *IsolationHelper) WithTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
	if ih.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	tx, err := ih.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure rollback on panic or error
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // Re-throw panic after rollback
		}
	}()

	// Execute the function
	err = fn(tx)

	// Always rollback - this is for test isolation
	if rbErr := tx.Rollback(); rbErr != nil {
		return fmt.Errorf("transaction rollback failed: %w", rbErr)
	}

	return err
}

// WithRedisNamespace executes a function with Redis operations isolated to a
// specific namespace. All keys created within the namespace are automatically
// cleaned up after execution.
func (ih *IsolationHelper) WithRedisNamespace(ctx context.Context, namespace string, fn func(nsClient *NamespacedRedisClient) error) error {
	if ih.redis == nil {
		return fmt.Errorf("redis client is nil")
	}

	nsClient := &NamespacedRedisClient{
		client:    ih.redis,
		namespace: namespace,
	}

	// Execute the function
	err := fn(nsClient)

	// Clean up all keys in namespace
	cleanupErr := nsClient.CleanupNamespace(ctx)
	if cleanupErr != nil {
		if err != nil {
			return fmt.Errorf("function error: %w; cleanup error: %v", err, cleanupErr)
		}
		return fmt.Errorf("cleanup error: %w", cleanupErr)
	}

	return err
}

// tableCleanupDef defines the columns to match for cleaning a specific table.
type tableCleanupDef struct {
	table    string
	columns  []string
	uuidCols map[string]bool // columns that are UUID type need ::text cast for LIKE
}

// CleanupTestDataByPattern deletes database records matching a pattern.
// This is useful for cleaning up test data that escaped transaction boundaries.
// Each table uses only the columns it actually owns to avoid SQL errors.
// UUID columns are cast to text before applying LIKE, since PostgreSQL does not
// support the LIKE operator directly on UUID types.
//
// Audit logs are handled specially: since audit_logs.user_id stores server-generated
// UUIDs (not testIDs), we first collect user UUIDs that match the pattern from the
// users table, then delete audit logs referencing those UUIDs.
//
// extraUserIDs are additional user UUIDs to clean audit logs for. Use this when a
// test has already deleted the user (e.g. TestAdminDeleteUser) so the UUID can no
// longer be recovered from the users table. A brief poll handles the race where the
// audit service writes asynchronously after the cleanup query.
func (ih *IsolationHelper) CleanupTestDataByPattern(ctx context.Context, pattern string, extraUserIDs ...string) error {
	if ih.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Phase 1: Collect UUIDs of users matching the pattern (by email or id).
	userIDs, err := ih.collectUserIDsByPattern(ctx, pattern)
	if err != nil {
		return fmt.Errorf("collect user IDs failed: %w", err)
	}

	// Merge with extraUserIDs provided by the caller (e.g. for users already
	// deleted by the test itself). Deduplicate to avoid redundant deletes.
	if len(extraUserIDs) > 0 {
		seen := make(map[string]bool, len(userIDs))
		for _, id := range userIDs {
			seen[id] = true
		}
		for _, id := range extraUserIDs {
			if !seen[id] {
				userIDs = append(userIDs, id)
				seen[id] = true
			}
		}
	}

	// Phase 2: Delete audit logs for those users.
	// audit_logs.user_id stores UUIDs, not testIDs, so LIKE on the pattern
	// won't match. Instead, delete by the collected user UUIDs.
	if _, err := ih.deleteAuditLogsByUserIDs(ctx, userIDs); err != nil {
		return fmt.Errorf("cleanup audit_logs (by user_id) failed: %w", err)
	}

	// Phase 2b: Also delete audit logs whose details contain the testID pattern.
	// This handles the case where a test has already deleted the user (e.g.
	// TestAdminDeleteUser) — collectUserIDsByPattern returns nothing, but the
	// audit_logs (user.deleted, user.register, etc.) still reference that user
	// and their details text may contain the testID or email.
	if _, err := ih.db.ExecContext(ctx,
		`DELETE FROM audit_logs WHERE details::text LIKE $1`, pattern); err != nil {
		return fmt.Errorf("cleanup audit_logs (by details) failed: %w", err)
	}

	// Phase 3: Delete remaining tables in dependency order (foreign keys first).
	// audit_logs is excluded here — it was handled in Phase 2.
	if err := ih.deleteRemainingTables(ctx, pattern); err != nil {
		return err
	}

	// Phase 4: If extraUserIDs were provided, poll briefly for async audit
	// writes, then sweep in a loop.  The poll lets the async worker flush
	// naturally; the sweep loop catches writes that arrive during or after
	// the poll.
	//
	// KNOWN LIMITATION: This is best-effort.  Database polling cannot prove
	// the producer queue is empty — a write may commit after the sweep
	// loop exits.  For tests that exercise async audit paths (e.g.
	// TestAdminDeleteUser), prefer CleanupTestDataByPatternWithRetry which
	// runs the full cleanup multiple times.
	if len(extraUserIDs) > 0 {
		return ih.sweepAsyncAuditLogs(ctx, extraUserIDs)
	}

	return nil
}

// deleteRemainingTables deletes rows matching pattern from all non-audit tables
// in dependency order (foreign keys first). UUID columns are cast to text so
// LIKE works on PostgreSQL.
func (ih *IsolationHelper) deleteRemainingTables(ctx context.Context, pattern string) error {
	tables := []tableCleanupDef{
		{
			table:    "verification_tokens",
			columns:  []string{"user_id", "token"},
			uuidCols: map[string]bool{"user_id": true},
		},
		{
			table:    "reset_tokens",
			columns:  []string{"user_id", "token"},
			uuidCols: map[string]bool{"user_id": true},
		},
		{
			table:    "authorization_codes",
			columns:  []string{"code", "client_id", "user_id"},
			uuidCols: map[string]bool{"user_id": true},
		},
		{
			table:    "tokens",
			columns:  []string{"access_token", "refresh_token", "user_id", "client_id"},
			uuidCols: map[string]bool{"user_id": true},
		},
		{
			table:    "oauth_clients",
			columns:  []string{"client_id", "name"},
			uuidCols: map[string]bool{},
		},
		{
			table:    "users",
			columns:  []string{"email", "id"},
			uuidCols: map[string]bool{"id": true},
		},
	}

	for _, t := range tables {
		// Build WHERE clause from only the columns this table actually has.
		// UUID columns are cast to text so LIKE works on PostgreSQL.
		where := ""
		args := []interface{}{pattern}
		for i, col := range t.columns {
			if i > 0 {
				where += " OR "
			}
			if t.uuidCols[col] {
				where += fmt.Sprintf("%s::text LIKE $1", col)
			} else {
				where += fmt.Sprintf("%s LIKE $1", col)
			}
		}
		// #nosec G201 -- table name and columns come from internal constant list, not user input
		query := fmt.Sprintf("DELETE FROM %s WHERE %s", t.table, where)
		if _, err := ih.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("cleanup %s failed: %w", t.table, err)
		}
	}
	return nil
}

// sweepAsyncAuditLogs polls for async audit writes and sweeps them in a loop.
//
// The poll lets the async worker flush naturally; the sweep loop catches
// writes that arrive during or after the poll.  This is best-effort —
// database polling cannot prove the producer queue is empty.
func (ih *IsolationHelper) sweepAsyncAuditLogs(ctx context.Context, extraUserIDs []string) error {
	ih.pollAsyncAuditWrites(ctx, extraUserIDs)

	sweepTimeout := ih.sweepTimeout()
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < sweepTimeout {
			sweepTimeout = remaining
		}
	}
	// Use context.Background() so the sweep is not canceled when the
	// parent context is canceled — cleanup should always run to
	// completion.  We still respect the parent's deadline (above) to
	// avoid exceeding the declared total budget.
	sweepCtx, sweepCancel := context.WithTimeout(context.Background(), sweepTimeout)
	defer sweepCancel()

	const sweepDrainWindow = 200 * time.Millisecond
	consecutiveEmpty := 0
	for {
		deleted, err := ih.deleteAuditLogsByUserIDs(sweepCtx, extraUserIDs)
		if err != nil {
			return fmt.Errorf("audit-log sweep failed: %w", err)
		}
		if deleted > 0 {
			consecutiveEmpty = 0
		} else {
			consecutiveEmpty++
			if consecutiveEmpty >= 2 {
				break
			}
		}
		select {
		case <-sweepCtx.Done():
			return fmt.Errorf("audit-log sweep timed out: some async audit logs may not have been cleaned up")
		case <-ih.clock.After(sweepDrainWindow):
		}
	}
	// Post-sweep check: if records still exist, signal the caller to retry.
	remaining, err := ih.countAuditLogsByUserIDs(sweepCtx, extraUserIDs)
	if err != nil {
		return fmt.Errorf("audit-log post-sweep count failed: %w", err)
	}
	if remaining > 0 {
		return ErrResidualAuditLogs
	}

	// Drain window: wait briefly for writes that committed between the
	// last sweep DELETE and the COUNT query, then sweep once more.
	// This narrows the race window but cannot eliminate it entirely —
	// the producer may still write after this re-check.
	const postCountDrain = 200 * time.Millisecond
	select {
	case <-sweepCtx.Done():
		// Cannot confirm late writes are clean — signal retry.
		return fmt.Errorf("audit-log post-count drain interrupted: %w", sweepCtx.Err())
	case <-ih.clock.After(postCountDrain):
		deleted, err := ih.deleteAuditLogsByUserIDs(sweepCtx, extraUserIDs)
		if err != nil {
			return fmt.Errorf("audit-log post-count re-sweep failed: %w", err)
		}
		if deleted > 0 {
			// Caught a late write. Re-count to give WithRetry a
			// chance to do another full attempt.
			remaining, err := ih.countAuditLogsByUserIDs(sweepCtx, extraUserIDs)
			if err != nil {
				return fmt.Errorf("audit-log post-count re-check failed: %w", err)
			}
			if remaining > 0 {
				return ErrResidualAuditLogs
			}
		}
	}
	return nil
}

// CleanupTestDataByPatternWithRetry runs CleanupTestDataByPattern up to
// maxAttempts times, with a delay between attempts.  Each attempt re-runs
// the full cleanup (Phases 1–4), catching audit logs that were written by
// the async worker after the previous attempt's sweep completed.
//
// Use this for tests that exercise async audit paths (e.g. user deletion
// that triggers user.deleted audit writes via a worker pool).
func (ih *IsolationHelper) CleanupTestDataByPatternWithRetry(ctx context.Context, pattern string, delayBetweenAttempts time.Duration, maxAttempts int, extraUserIDs ...string) error {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("cleanup retry interrupted after %d attempts: %w", attempt, lastErr)
			case <-ih.clock.After(delayBetweenAttempts):
			}
		}
		lastErr = ih.CleanupTestDataByPattern(ctx, pattern, extraUserIDs...)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("cleanup failed after %d attempts: %w", maxAttempts, lastErr)
}

// ErrResidualAuditLogs is returned when audit logs remain after the sweep.
// Callers (e.g. CleanupTestDataByPatternWithRetry) can use this to decide
// whether to retry the cleanup.
var ErrResidualAuditLogs = fmt.Errorf("residual audit logs detected after sweep")

// pollAsyncAuditWrites polls the database with exponential backoff until
// the drain window elapses with no new writes, or the settle timeout fires.
// This is a best-effort wait — the caller should run a final cleanup sweep
// afterwards to catch any writes that arrived during or after the poll.
func (ih *IsolationHelper) pollAsyncAuditWrites(ctx context.Context, extraUserIDs []string) {
	settleTimeout := ih.settleTimeout()

	settleCtx, settleCancel := context.WithTimeout(ctx, settleTimeout)
	defer settleCancel()

	backoff := 20 * time.Millisecond
	const maxBackoff = 100 * time.Millisecond

	drainWindow := ih.drainWindow()
	lastDeleteTime := ih.clock.Now()

	for {
		select {
		case <-settleCtx.Done():
			return
		case <-ih.clock.After(backoff):
		}

		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		deleted, err := ih.deleteAuditLogsByUserIDs(settleCtx, extraUserIDs)
		if err != nil {
			return // context expired or DB error — caller will sweep
		}

		if deleted > 0 {
			lastDeleteTime = ih.clock.Now()
		} else if ih.clock.Now().Sub(lastDeleteTime) >= drainWindow {
			return
		}
	}
}

// collectUserIDsByPattern queries the users table for UUIDs matching the pattern.
// Used to bridge the gap between testID-based patterns and UUID-based foreign keys
// in audit_logs.
func (ih *IsolationHelper) collectUserIDsByPattern(ctx context.Context, pattern string) ([]string, error) {
	rows, err := ih.db.QueryContext(ctx,
		`SELECT id::text FROM users WHERE email LIKE $1 OR id::text LIKE $1`, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// deleteAuditLogsByUserIDs deletes audit log entries for the given user UUIDs.
// This handles the case where audit_logs.user_id stores server-generated UUIDs
// that don't contain the testID string. Returns the number of rows deleted.
func (ih *IsolationHelper) deleteAuditLogsByUserIDs(ctx context.Context, userIDs []string) (int64, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}

	// Build IN clause: DELETE FROM audit_logs WHERE user_id IN ($1, $2, ...)
	query := "DELETE FROM audit_logs WHERE user_id IN ("
	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query += ")"

	res, err := ih.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// countAuditLogsByUserIDs returns the number of audit log entries for the
// given user UUIDs.  Used after a sweep to detect records that arrived after
// the sweep loop exited.
func (ih *IsolationHelper) countAuditLogsByUserIDs(ctx context.Context, userIDs []string) (int64, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}

	query := "SELECT COUNT(*) FROM audit_logs WHERE user_id IN ("
	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query += ")"

	var count int64
	if err := ih.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ============================================================================
// Namespaced Redis Client
// ============================================================================

// NamespacedRedisClient wraps a Redis client to automatically prefix all keys
// with a namespace, providing key isolation for parallel tests.
type NamespacedRedisClient struct {
	client    *redis.Client
	namespace string
}

// Set stores a value with the namespaced key.
func (nrc *NamespacedRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	namespacedKey := nrc.namespaceKey(key)
	return nrc.client.Set(ctx, namespacedKey, value, expiration).Err()
}

// Get retrieves a value using the namespaced key.
func (nrc *NamespacedRedisClient) Get(ctx context.Context, key string) (string, error) {
	namespacedKey := nrc.namespaceKey(key)
	return nrc.client.Get(ctx, namespacedKey).Result()
}

// Del deletes keys using namespaced keys.
func (nrc *NamespacedRedisClient) Del(ctx context.Context, keys ...string) error {
	namespacedKeys := make([]string, len(keys))
	for i, key := range keys {
		namespacedKeys[i] = nrc.namespaceKey(key)
	}
	return nrc.client.Del(ctx, namespacedKeys...).Err()
}

// Exists checks if keys exist using namespaced keys.
func (nrc *NamespacedRedisClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	namespacedKeys := make([]string, len(keys))
	for i, key := range keys {
		namespacedKeys[i] = nrc.namespaceKey(key)
	}
	return nrc.client.Exists(ctx, namespacedKeys...).Result()
}

// CleanupNamespace deletes all keys with this namespace prefix.
func (nrc *NamespacedRedisClient) CleanupNamespace(ctx context.Context) error {
	pattern := nrc.namespace + ":*"

	iter := nrc.client.Scan(ctx, 0, pattern, 0).Iterator()
	keysToDelete := []string{}

	for iter.Next(ctx) {
		keysToDelete = append(keysToDelete, iter.Val())
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to scan keys: %w", err)
	}

	if len(keysToDelete) > 0 {
		if err := nrc.client.Del(ctx, keysToDelete...).Err(); err != nil {
			return fmt.Errorf("failed to delete keys: %w", err)
		}
	}

	return nil
}

// namespaceKey prefixes a key with the namespace.
func (nrc *NamespacedRedisClient) namespaceKey(key string) string {
	return fmt.Sprintf("%s:%s", nrc.namespace, key)
}

// GetNamespace returns the current namespace.
func (nrc *NamespacedRedisClient) GetNamespace() string {
	return nrc.namespace
}

// ============================================================================
// Test Identifier Helpers
// ============================================================================

// SanitizeTestName converts a test name to a safe identifier component,
// replacing non-alphanumeric characters with hyphens.
// Exported so external test suites (e.g., test/e2e) can generate
// testIDs compatible with the cleanup framework.
func SanitizeTestName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// GenerateTestID produces a unique test identifier in the same format used
// by the cleanup framework. External test suites can call this to generate
// a testID that is compatible with CleanupTestDataByPattern.
//
// Format: e2e_<unix_nano>_<sanitized_test_name>
func GenerateTestID(testName string) string {
	return fmt.Sprintf("e2e_%d_%s", time.Now().UnixNano(), SanitizeTestName(testName))
}
