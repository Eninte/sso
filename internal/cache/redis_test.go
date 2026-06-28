//go:build integration

// Package cache 缓存单元测试
package cache_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/util/testutil"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// testRedisAddr 返回测试用 Redis 地址（来自环境变量）
//
// 与 testutil.RedisAddr 共享同一套环境变量（REDIS_TEST_ADDR）。
// 未配置时 t.Skip，避免无 Redis 环境下尝试连接硬编码地址。
func testRedisAddr(t *testing.T) string {
	t.Helper()
	addr := testutil.RedisAddr()
	if addr == "" {
		t.Skip("跳过真实 Redis 测试：未设置 REDIS_TEST_ADDR 环境变量")
	}
	return addr
}

// ============================================================================
// MemoryCache 测试
// ============================================================================

func TestMemoryCache_GetSet(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()

	ctx := context.Background()

	t.Run("设置和获取缓存", func(t *testing.T) {
		type TestData struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}

		key := "test-key"
		value := TestData{ID: "123", Name: "test"}

		// 设置缓存
		err := c.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		// 获取缓存
		var result TestData
		err = c.Get(ctx, key, &result)
		require.NoError(t, err)
		assert.Equal(t, value.ID, result.ID)
		assert.Equal(t, value.Name, result.Name)
	})

	t.Run("缓存未命中", func(t *testing.T) {
		var result string
		err := c.Get(ctx, "nonexistent-key", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})

	t.Run("缓存过期", func(t *testing.T) {
		key := "expire-key"
		value := "test-value"

		// 设置很短的TTL
		err := c.Set(ctx, key, value, 1*time.Millisecond)
		require.NoError(t, err)

		// 等待过期
		time.Sleep(10 * time.Millisecond)

		var result string
		err = c.Get(ctx, key, &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})
}

func TestMemoryCache_Delete(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()

	ctx := context.Background()

	t.Run("删除缓存", func(t *testing.T) {
		key := "delete-key"
		value := "test-value"

		// 设置缓存
		err := c.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		// 删除缓存
		err = c.Delete(ctx, key)
		require.NoError(t, err)

		// 验证已删除
		var result string
		err = c.Get(ctx, key, &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})
}

func TestMemoryCache_DeletePattern(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()

	ctx := context.Background()

	t.Run("按模式删除缓存", func(t *testing.T) {
		// 设置多个缓存
		c.Set(ctx, "user:123", "user1", 5*time.Minute)
		c.Set(ctx, "user:456", "user2", 5*time.Minute)
		c.Set(ctx, "token:abc", "token1", 5*time.Minute)

		// 删除所有user前缀的缓存
		err := c.DeletePattern(ctx, "user:*")
		require.NoError(t, err)

		// 验证user缓存已删除
		var result string
		err = c.Get(ctx, "user:123", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		err = c.Get(ctx, "user:456", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		// 验证token缓存仍然存在
		err = c.Get(ctx, "token:abc", &result)
		assert.NoError(t, err)
	})
}

// ============================================================================
// 缓存键函数测试
// ============================================================================

func TestCacheKeys(t *testing.T) {
	t.Run("TokenKey", func(t *testing.T) {
		key := cache.TokenKey("access-token-123")
		assert.Equal(t, "token:access-token-123", key)
	})

	t.Run("UserIDKey", func(t *testing.T) {
		key := cache.UserIDKey("user-123")
		assert.Equal(t, "user:user-123", key)
	})

	t.Run("UserEmailKey", func(t *testing.T) {
		key := cache.UserEmailKey("test@example.com")
		assert.Equal(t, "user:email:test@example.com", key)
	})

	t.Run("ClientKey", func(t *testing.T) {
		key := cache.ClientKey("client-123")
		assert.Equal(t, "client:client-123", key)
	})

	t.Run("Set非序列化值", func(t *testing.T) {
		c := cache.NewMemoryCache()
		defer c.Close()

		// channel类型无法JSON序列化
		ch := make(chan int)
		err := c.Set(context.Background(), "bad-key", ch, 5*time.Minute)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "序列化")
	})
}

// ============================================================================
// MemoryCache cleanup 测试
// ============================================================================

func TestMemoryCache_Cleanup(t *testing.T) {
	ctx := context.Background()

	t.Run("过期条目被Get清理", func(t *testing.T) {
		c := cache.NewMemoryCache()
		defer c.Close()

		// 设置一个很短的TTL
		c.Set(ctx, "short-ttl", "value", 1*time.Millisecond)
		c.Set(ctx, "long-ttl", "value", 5*time.Minute)

		// 等待过期
		time.Sleep(10 * time.Millisecond)

		// Get会触发过期删除
		var result string
		err := c.Get(ctx, "short-ttl", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		// 长TTL应该仍然存在
		err = c.Get(ctx, "long-ttl", &result)
		assert.NoError(t, err)
	})

	t.Run("Close后cleanup退出", func(t *testing.T) {
		c := cache.NewMemoryCache()
		c.Set(ctx, "key", "value", 5*time.Minute)

		// Close应停止cleanup goroutine
		err := c.Close()
		assert.NoError(t, err)

		// 稍等以确保goroutine退出
		time.Sleep(50 * time.Millisecond)
	})
}

// ============================================================================
// DeletePattern 通配符匹配测试
// ============================================================================

func TestMemoryCache_DeletePattern_Wildcard(t *testing.T) {
	ctx := context.Background()

	t.Run("星号通配符匹配所有", func(t *testing.T) {
		c := cache.NewMemoryCache()
		defer c.Close()

		c.Set(ctx, "key1", "v1", 5*time.Minute)
		c.Set(ctx, "key2", "v2", 5*time.Minute)
		c.Set(ctx, "other", "v3", 5*time.Minute)

		err := c.DeletePattern(ctx, "*")
		require.NoError(t, err)

		var result string
		err = c.Get(ctx, "key1", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		err = c.Get(ctx, "other", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})

	t.Run("精确匹配", func(t *testing.T) {
		c := cache.NewMemoryCache()
		defer c.Close()

		c.Set(ctx, "exact-key", "v1", 5*time.Minute)
		c.Set(ctx, "other-key", "v2", 5*time.Minute)

		err := c.DeletePattern(ctx, "exact-key")
		require.NoError(t, err)

		var result string
		err = c.Get(ctx, "exact-key", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		err = c.Get(ctx, "other-key", &result)
		assert.NoError(t, err)
	})

	t.Run("前缀匹配", func(t *testing.T) {
		c := cache.NewMemoryCache()
		defer c.Close()

		c.Set(ctx, "user:123", "v1", 5*time.Minute)
		c.Set(ctx, "user:456", "v2", 5*time.Minute)
		c.Set(ctx, "token:abc", "v3", 5*time.Minute)

		err := c.DeletePattern(ctx, "user:*")
		require.NoError(t, err)

		var result string
		err = c.Get(ctx, "user:123", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		err = c.Get(ctx, "user:456", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		err = c.Get(ctx, "token:abc", &result)
		assert.NoError(t, err)
	})
}

// ============================================================================
// SetWithNilProtection 测试
// ============================================================================

func TestMemoryCache_SetWithNilProtection(t *testing.T) {
	ctx := context.Background()

	t.Run("设置nil值使用nilTTL", func(t *testing.T) {
		c := cache.NewMemoryCache()
		defer c.Close()

		key := "nil-key"
		// 设置nil值，使用很短的nilTTL
		err := c.SetWithNilProtection(ctx, key, nil, 5*time.Minute, 1*time.Millisecond)
		require.NoError(t, err)

		// 等待过期
		time.Sleep(10 * time.Millisecond)

		// 应该返回cache miss
		var result string
		err = c.Get(ctx, key, &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})

	t.Run("设置非nil值使用ttl", func(t *testing.T) {
		c := cache.NewMemoryCache()
		defer c.Close()

		key := "non-nil-key"
		value := "test-value"
		// 设置非nil值
		err := c.SetWithNilProtection(ctx, key, value, 5*time.Minute, 1*time.Minute)
		require.NoError(t, err)

		// 应该能获取到值
		var result string
		err = c.Get(ctx, key, &result)
		require.NoError(t, err)
		assert.Equal(t, value, result)
	})

	t.Run("序列化失败返回错误", func(t *testing.T) {
		c := cache.NewMemoryCache()
		defer c.Close()

		// channel类型无法JSON序列化
		ch := make(chan int)
		err := c.SetWithNilProtection(ctx, "bad-key", ch, 5*time.Minute, 1*time.Minute)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "序列化")
	})
}

// ============================================================================
// MemoryCache 并发测试
// ============================================================================

func TestMemoryCache_Concurrent(t *testing.T) {
	ctx := context.Background()

	t.Run("并发读写安全", func(t *testing.T) {
		c := cache.NewMemoryCache()
		defer c.Close()

		// 并发写入
		done := make(chan struct{})
		for i := 0; i < 10; i++ {
			go func(i int) {
				key := fmt.Sprintf("key-%d", i)
				c.Set(ctx, key, i, 5*time.Minute)
				done <- struct{}{}
			}(i)
		}

		// 等待所有写入完成
		for i := 0; i < 10; i++ {
			<-done
		}

		// 验证所有值都存在
		for i := 0; i < 10; i++ {
			var result int
			key := fmt.Sprintf("key-%d", i)
			err := c.Get(ctx, key, &result)
			assert.NoError(t, err)
			assert.Equal(t, i, result)
		}
	})
}

// ============================================================================
// MemoryCache Close 测试
// ============================================================================

func TestMemoryCache_Close(t *testing.T) {
	ctx := context.Background()

	t.Run("多次调用Close安全", func(t *testing.T) {
		c := cache.NewMemoryCache()
		c.Set(ctx, "key", "value", 5*time.Minute)

		// 第一次Close
		err := c.Close()
		assert.NoError(t, err)

		// 第二次Close应该也是安全的
		err = c.Close()
		assert.NoError(t, err)
	})
}

// ============================================================================
// NewCache 工厂函数测试
// ============================================================================

func TestNewCache(t *testing.T) {
	t.Run("禁用Redis返回MemoryCache", func(t *testing.T) {
		opt := &cache.Option{
			RedisEnable: false,
		}

		c, err := cache.NewCache(opt)
		require.NoError(t, err)
		defer c.Close()

		// 验证是MemoryCache
		ctx := context.Background()
		err = c.Set(ctx, "key", "value", 5*time.Minute)
		assert.NoError(t, err)

		var result string
		err = c.Get(ctx, "key", &result)
		assert.NoError(t, err)
		assert.Equal(t, "value", result)
	})

	t.Run("启用Redis但连接失败返回错误", func(t *testing.T) {
		opt := &cache.Option{
			RedisEnable:   true,
			RedisHost:     "invalid-host",
			RedisPassword: "",
			RedisDB:       0,
		}

		_, err := cache.NewCache(opt)
		assert.Error(t, err)
	})
}

func TestNewCacheWithFallback(t *testing.T) {
	t.Run("禁用Redis返回MemoryCache", func(t *testing.T) {
		opt := &cache.Option{
			RedisEnable: false,
		}

		c, err := cache.NewCacheWithFallback(opt)
		require.NoError(t, err)
		defer c.Close()

		// 验证是MemoryCache
		ctx := context.Background()
		err = c.Set(ctx, "key", "value", 5*time.Minute)
		assert.NoError(t, err)
	})

	t.Run("启用Redis但连接失败降级到MemoryCache", func(t *testing.T) {
		opt := &cache.Option{
			RedisEnable:   true,
			RedisHost:     "invalid-host",
			RedisPassword: "",
			RedisDB:       0,
		}

		// 应该不返回错误，而是降级到MemoryCache
		c, err := cache.NewCacheWithFallback(opt)
		require.NoError(t, err)
		defer c.Close()

		// 验证可以使用（说明是MemoryCache）
		ctx := context.Background()
		err = c.Set(ctx, "key", "value", 5*time.Minute)
		assert.NoError(t, err)
	})
}

// ============================================================================
// RedisCache 测试（使用真实 Redis）
// ============================================================================

func TestRedisCache_GetSet(t *testing.T) {
	c, err := cache.NewRedisCacheWithOptions(&redis.Options{
		Addr: testRedisAddr(t),
	})
	require.NoError(t, err)
	defer c.Close()

	ctx := context.Background()

	t.Run("设置和获取缓存", func(t *testing.T) {
		type TestData struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}

		key := "redis-test-key"
		value := TestData{ID: "123", Name: "test"}

		err := c.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		var result TestData
		err = c.Get(ctx, key, &result)
		require.NoError(t, err)
		assert.Equal(t, value.ID, result.ID)
		assert.Equal(t, value.Name, result.Name)
	})

	t.Run("缓存未命中", func(t *testing.T) {
		var result string
		err := c.Get(ctx, "nonexistent-key", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})
}

func TestRedisCache_Delete(t *testing.T) {
	//
	//

	c, err := cache.NewRedisCacheWithOptions(&redis.Options{
		Addr: testRedisAddr(t),
	})
	require.NoError(t, err)
	defer c.Close()

	ctx := context.Background()

	t.Run("删除缓存", func(t *testing.T) {
		key := "redis-delete-key"
		value := "test-value"

		err := c.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		err = c.Delete(ctx, key)
		require.NoError(t, err)

		var result string
		err = c.Get(ctx, key, &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})
}

func TestRedisCache_DeletePattern(t *testing.T) {
	//
	//

	c, err := cache.NewRedisCacheWithOptions(&redis.Options{
		Addr: testRedisAddr(t),
	})
	require.NoError(t, err)
	defer c.Close()

	ctx := context.Background()

	t.Run("按模式删除缓存", func(t *testing.T) {
		c.Set(ctx, "user:123", "user1", 5*time.Minute)
		c.Set(ctx, "user:456", "user2", 5*time.Minute)
		c.Set(ctx, "token:abc", "token1", 5*time.Minute)

		err := c.DeletePattern(ctx, "user:*")
		require.NoError(t, err)

		var result string
		err = c.Get(ctx, "user:123", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		err = c.Get(ctx, "user:456", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		err = c.Get(ctx, "token:abc", &result)
		assert.NoError(t, err)
	})
}

func TestRedisCache_SetWithNilProtection(t *testing.T) {
	//
	//

	c, err := cache.NewRedisCacheWithOptions(&redis.Options{
		Addr: testRedisAddr(t),
	})
	require.NoError(t, err)
	defer c.Close()

	ctx := context.Background()

	t.Run("设置nil值使用nilTTL", func(t *testing.T) {
		key := "redis-nil-key"
		err := c.SetWithNilProtection(ctx, key, nil, 5*time.Minute, 1*time.Minute)
		require.NoError(t, err)

		var result string
		err = c.Get(ctx, key, &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})

	t.Run("设置非nil值使用ttl", func(t *testing.T) {
		key := "redis-non-nil-key"
		value := "test-value"
		err := c.SetWithNilProtection(ctx, key, value, 5*time.Minute, 1*time.Minute)
		require.NoError(t, err)

		var result string
		err = c.Get(ctx, key, &result)
		require.NoError(t, err)
		assert.Equal(t, value, result)
	})

	t.Run("序列化失败返回错误", func(t *testing.T) {
		ch := make(chan int)
		err := c.SetWithNilProtection(ctx, "bad-key", ch, 5*time.Minute, 1*time.Minute)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "序列化")
	})
}

func TestRedisCache_Ping(t *testing.T) {
	//
	//

	c, err := cache.NewRedisCacheWithOptions(&redis.Options{
		Addr: testRedisAddr(t),
	})
	require.NoError(t, err)
	defer c.Close()

	ctx := context.Background()
	err = c.Ping(ctx)
	assert.NoError(t, err)
}

func TestRedisCache_Close(t *testing.T) {
	//
	//

	c, err := cache.NewRedisCacheWithOptions(&redis.Options{
		Addr: testRedisAddr(t),
	})
	require.NoError(t, err)

	err = c.Close()
	assert.NoError(t, err)
}

func TestNewRedisCacheWithOptions(t *testing.T) {
	t.Run("连接成功", func(t *testing.T) {
		//
		//

		c, err := cache.NewRedisCacheWithOptions(&redis.Options{
			Addr: testRedisAddr(t),
		})
		require.NoError(t, err)
		assert.NotNil(t, c)
		defer c.Close()
	})

	t.Run("连接失败", func(t *testing.T) {
		_, err := cache.NewRedisCacheWithOptions(&redis.Options{
			Addr: "invalid:99999",
		})
		assert.Error(t, err)
	})
}

func TestRedisCache_SetNonSerializable(t *testing.T) {
	//
	//

	c, err := cache.NewRedisCacheWithOptions(&redis.Options{
		Addr: testRedisAddr(t),
	})
	require.NoError(t, err)
	defer c.Close()

	ctx := context.Background()

	t.Run("Set非序列化值", func(t *testing.T) {
		ch := make(chan int)
		err := c.Set(ctx, "bad-key", ch, 5*time.Minute)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "序列化")
	})
}
