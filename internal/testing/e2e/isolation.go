// Package e2e provides test isolation helpers for E2E testing.
package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ============================================================================
// Test Data Isolation Helpers
// ============================================================================

// IsolationHelper provides utilities for test data isolation and cleanup.
type IsolationHelper struct {
	db    *sql.DB
	redis *redis.Client
}

// NewIsolationHelper creates a new isolation helper instance.
func NewIsolationHelper(db *sql.DB, redis *redis.Client) *IsolationHelper {
	return &IsolationHelper{
		db:    db,
		redis: redis,
	}
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
	table      string
	columns    []string
	uuidCols   map[string]bool // columns that are UUID type need ::text cast for LIKE
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
func (ih *IsolationHelper) CleanupTestDataByPattern(ctx context.Context, pattern string) error {
	if ih.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Phase 1: Collect UUIDs of users matching the pattern (by email or id).
	userIDs, err := ih.collectUserIDsByPattern(ctx, pattern)
	if err != nil {
		return fmt.Errorf("collect user IDs failed: %w", err)
	}

	// Phase 2: Delete audit logs for those users.
	// audit_logs.user_id stores UUIDs, not testIDs, so LIKE on the pattern
	// won't match. Instead, delete by the collected user UUIDs.
	if err := ih.deleteAuditLogsByUserIDs(ctx, userIDs); err != nil {
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
// that don't contain the testID string.
func (ih *IsolationHelper) deleteAuditLogsByUserIDs(ctx context.Context, userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
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

	_, err := ih.db.ExecContext(ctx, query, args...)
	return err
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
