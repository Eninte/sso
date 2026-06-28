//go:build !integration

package cache_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
)

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

	t.Run("缓存过期", func(t *testing.T) {
		key := "expire-key"
		value := "test-value"

		err := c.Set(ctx, key, value, 1*time.Millisecond)
		require.NoError(t, err)

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

		err := c.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		err = c.Delete(ctx, key)
		require.NoError(t, err)

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

	t.Run("星号通配符匹配所有", func(t *testing.T) {
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
}

func TestMemoryCache_SetWithNilProtection(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()

	ctx := context.Background()

	t.Run("设置nil值使用nilTTL", func(t *testing.T) {
		key := "nil-key"
		err := c.SetWithNilProtection(ctx, key, nil, 5*time.Minute, 1*time.Millisecond)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		var result string
		err = c.Get(ctx, key, &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})

	t.Run("设置非nil值使用ttl", func(t *testing.T) {
		key := "non-nil-key"
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

func TestMemoryCache_Concurrent(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()

	ctx := context.Background()

	t.Run("并发读写安全", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 10
		opsPerGoroutine := 100

		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for i := 0; i < opsPerGoroutine; i++ {
					key := fmt.Sprintf("key-%d-%d", id, i)
					c.Set(ctx, key, i, 5*time.Minute)
				}
			}(g)
		}

		wg.Wait()

		for g := 0; g < numGoroutines; g++ {
			key := fmt.Sprintf("key-%d-0", g)
			var result int
			err := c.Get(ctx, key, &result)
			assert.NoError(t, err)
			assert.Equal(t, 0, result)
		}
	})
}

func TestMemoryCache_Close(t *testing.T) {
	ctx := context.Background()

	t.Run("多次调用Close安全", func(t *testing.T) {
		c := cache.NewMemoryCache()
		c.Set(ctx, "key", "value", 5*time.Minute)

		err := c.Close()
		assert.NoError(t, err)

		err = c.Close()
		assert.NoError(t, err)
	})
}

func TestMemoryCache_SerializationError(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()

	ctx := context.Background()

	t.Run("Set非序列化值", func(t *testing.T) {
		ch := make(chan int)
		err := c.Set(ctx, "bad-key", ch, 5*time.Minute)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "序列化")
	})
}

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
}

func TestNewCache(t *testing.T) {
	t.Run("禁用Redis返回MemoryCache", func(t *testing.T) {
		opt := &cache.Option{
			RedisEnable: false,
		}

		c, err := cache.NewCache(opt)
		require.NoError(t, err)
		defer c.Close()

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

		c, err := cache.NewCacheWithFallback(opt)
		require.NoError(t, err)
		defer c.Close()

		ctx := context.Background()
		err = c.Set(ctx, "key", "value", 5*time.Minute)
		assert.NoError(t, err)
	})
}

func TestMemoryCache_Expiration(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()

	ctx := context.Background()

	t.Run("过期条目被Get清理", func(t *testing.T) {
		c.Set(ctx, "short-ttl", "value", 1*time.Millisecond)
		c.Set(ctx, "long-ttl", "value", 5*time.Minute)

		time.Sleep(10 * time.Millisecond)

		var result string
		err := c.Get(ctx, "short-ttl", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		err = c.Get(ctx, "long-ttl", &result)
		assert.NoError(t, err)
	})
}

// ============================================================================
// MemoryCache.WithMetrics / Increment / SetTTL / GetTTL 测试
// 覆盖原本 0% 的指标回调与计数器/TTL 管理方法
// ============================================================================

// TestMemoryCache_WithMetrics 测试设置指标回调
func TestMemoryCache_WithMetrics(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	hitCount := 0
	missCount := 0

	returned := c.WithMetrics(
		func() { hitCount++ },
		func() { missCount++ },
	)
	assert.Same(t, c, returned, "WithMetrics 应返回缓存实例本身（链式调用）")

	// 触发一次 miss
	var v string
	err := c.Get(ctx, "missing", &v)
	assert.ErrorIs(t, err, cache.ErrCacheMiss)
	assert.Equal(t, 1, missCount, "未命中应触发 onMiss")
	assert.Equal(t, 0, hitCount)

	// 触发一次 hit
	require.NoError(t, c.Set(ctx, "exists", "value", 5*time.Minute))
	var got string
	require.NoError(t, c.Get(ctx, "exists", &got))
	assert.Equal(t, "value", got)
	assert.Equal(t, 1, hitCount, "命中应触发 onHit")
	assert.Equal(t, 1, missCount)
}

// TestMemoryCache_Increment 测试内存计数器递增
func TestMemoryCache_Increment(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	t.Run("首次递增_返回1", func(t *testing.T) {
		count, err := c.Increment(ctx, "counter")
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("连续递增_值递增", func(t *testing.T) {
		key := "seq"
		for i := 1; i <= 5; i++ {
			count, err := c.Increment(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, i, count)
		}
	})

	t.Run("不同key独立计数", func(t *testing.T) {
		c1, _ := c.Increment(ctx, "k1")
		c2, _ := c.Increment(ctx, "k2")
		assert.Equal(t, 1, c1)
		assert.Equal(t, 1, c2, "不同 key 应独立计数")
	})

	t.Run("递增后可通过Get读取", func(t *testing.T) {
		c.Increment(ctx, "readable")
		c.Increment(ctx, "readable")
		var val int
		require.NoError(t, c.Get(ctx, "readable", &val))
		assert.Equal(t, 2, val)
	})
}

// TestMemoryCache_SetTTL 测试内存缓存设置 TTL
func TestMemoryCache_SetTTL(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	t.Run("为已存在key设置TTL", func(t *testing.T) {
		require.NoError(t, c.Set(ctx, "k", "v", 5*time.Minute))
		require.NoError(t, c.SetTTL(ctx, "k", 50*time.Millisecond))

		var v string
		require.NoError(t, c.Get(ctx, "k", &v))
		time.Sleep(60 * time.Millisecond)
		err := c.Get(ctx, "k", &v)
		assert.ErrorIs(t, err, cache.ErrCacheMiss, "SetTTL 后过期应返回 ErrCacheMiss")
	})

	t.Run("为不存在的key设置TTL_返回错误", func(t *testing.T) {
		err := c.SetTTL(ctx, "nonexistent", 10*time.Second)
		assert.Error(t, err, "不存在的 key 设置 TTL 应返回错误")
	})
}

// TestMemoryCache_GetTTL 测试内存缓存获取剩余 TTL
func TestMemoryCache_GetTTL(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	t.Run("已设置TTL的key_返回正数", func(t *testing.T) {
		require.NoError(t, c.Set(ctx, "k", "v", 60*time.Second))
		ttl, err := c.GetTTL(ctx, "k")
		require.NoError(t, err)
		assert.True(t, ttl > 0 && ttl <= 60*time.Second, "TTL 应在 (0, 60s]，实际 %v", ttl)
	})

	t.Run("不存在的key_返回错误", func(t *testing.T) {
		ttl, err := c.GetTTL(ctx, "nonexistent")
		assert.Error(t, err, "不存在的 key 应返回错误")
		assert.Equal(t, time.Duration(0), ttl)
	})

	t.Run("SetTTL后GetTTL反映新值", func(t *testing.T) {
		require.NoError(t, c.Set(ctx, "override", "v", 5*time.Minute))
		require.NoError(t, c.SetTTL(ctx, "override", 10*time.Second))
		ttl, err := c.GetTTL(ctx, "override")
		require.NoError(t, err)
		assert.True(t, ttl > 0 && ttl <= 10*time.Second, "SetTTL(10s) 后 TTL 应 <= 10s，实际 %v", ttl)
	})
}
