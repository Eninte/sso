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
