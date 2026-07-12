// Package e2e provides test isolation helpers for E2E testing.
package e2e

import (
	"context"
	"database/sql"
	"fmt"

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

// CleanupTestDataByPattern deletes database records matching a pattern.
// This is useful for cleaning up test data that escaped transaction boundaries.
func (ih *IsolationHelper) CleanupTestDataByPattern(ctx context.Context, pattern string) error {
	if ih.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Clean up test data in dependency order (foreign keys first)
	tables := []string{
		"audit_logs",
		"verification_tokens",
		"reset_tokens",
		"authorization_codes",
		"tokens",
		"oauth_clients",
		"users",
	}

	for _, table := range tables {
		// #nosec G201 -- 表名来自内部常量列表，不是用户输入
		query := fmt.Sprintf("DELETE FROM %s WHERE email LIKE $1 OR user_id LIKE $1 OR token LIKE $1 OR code LIKE $1 OR client_id LIKE $1", table)
		_, err := ih.db.ExecContext(ctx, query, pattern)
		if err != nil {
			// Ignore errors for tables that don't have the column
			// In production, we'd want more specific error handling
			continue
		}
	}

	return nil
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
func (nrc *NamespacedRedisClient) Set(ctx context.Context, key string, value interface{}, expiration interface{}) error {
	namespacedKey := nrc.namespaceKey(key)
	return nrc.client.Set(ctx, namespacedKey, value, 0).Err()
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
