//go:build !integration

package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
)

func setupMiniRedis(t *testing.T) (*miniredis.Miniredis, *cache.RedisCache) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)

	t.Cleanup(func() { mr.Close() })

	opts := &redis.Options{
		Addr: mr.Addr(),
	}

	rc, err := cache.NewRedisCacheWithOptions(opts)
	require.NoError(t, err)

	t.Cleanup(func() { rc.Close() })

	return mr, rc
}

func TestRedisCache_GetSet(t *testing.T) {
	mr, rc := setupMiniRedis(t)
	ctx := context.Background()

	t.Run("设置和获取", func(t *testing.T) {
		type TestData struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}

		err := rc.Set(ctx, "test-key", TestData{ID: "1", Name: "test"}, 5*time.Minute)
		require.NoError(t, err)

		var result TestData
		err = rc.Get(ctx, "test-key", &result)
		require.NoError(t, err)
		assert.Equal(t, "1", result.ID)
		assert.Equal(t, "test", result.Name)
	})

	t.Run("缓存未命中", func(t *testing.T) {
		var result string
		err := rc.Get(ctx, "nonexistent", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})

	t.Run("获取nil缓存值返回CacheMiss", func(t *testing.T) {
		mr.Set("nil-key", "NULL")

		var result string
		err := rc.Get(ctx, "nil-key", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})

	t.Run("获取损坏的JSON返回错误", func(t *testing.T) {
		mr.Set("bad-json", "not-json")

		var result struct {
			Name string `json:"name"`
		}
		err := rc.Get(ctx, "bad-json", &result)
		assert.Error(t, err)
	})
}

func TestRedisCache_Delete(t *testing.T) {
	_, rc := setupMiniRedis(t)
	ctx := context.Background()

	t.Run("删除存在的键", func(t *testing.T) {
		rc.Set(ctx, "del-key", "value", 5*time.Minute)
		err := rc.Delete(ctx, "del-key")
		assert.NoError(t, err)

		var result string
		err = rc.Get(ctx, "del-key", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})

	t.Run("删除不存在的键不报错", func(t *testing.T) {
		err := rc.Delete(ctx, "nonexistent")
		assert.NoError(t, err)
	})
}

func TestRedisCache_SetWithNilProtection(t *testing.T) {
	_, rc := setupMiniRedis(t)
	ctx := context.Background()

	t.Run("设置nil值", func(t *testing.T) {
		err := rc.SetWithNilProtection(ctx, "nil-key", nil, 5*time.Minute, 1*time.Second)
		require.NoError(t, err)

		var result string
		err = rc.Get(ctx, "nil-key", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})

	t.Run("设置非nil值", func(t *testing.T) {
		err := rc.SetWithNilProtection(ctx, "val-key", "hello", 5*time.Minute, 1*time.Second)
		require.NoError(t, err)

		var result string
		err = rc.Get(ctx, "val-key", &result)
		require.NoError(t, err)
		assert.Equal(t, "hello", result)
	})

	t.Run("序列化失败返回错误", func(t *testing.T) {
		ch := make(chan int)
		err := rc.SetWithNilProtection(ctx, "bad", ch, 5*time.Minute, 1*time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "序列化")
	})
}

func TestRedisCache_Ping(t *testing.T) {
	_, rc := setupMiniRedis(t)
	ctx := context.Background()

	t.Run("Ping成功", func(t *testing.T) {
		err := rc.Ping(ctx)
		assert.NoError(t, err)
	})
}

func TestRedisCache_DeletePattern(t *testing.T) {
	mr, rc := setupMiniRedis(t)
	ctx := context.Background()

	t.Run("按模式删除", func(t *testing.T) {
		mr.Set("user:1", `"v1"`)
		mr.Set("user:2", `"v2"`)
		mr.Set("token:abc", `"v3"`)

		err := rc.DeletePattern(ctx, "user:*")
		require.NoError(t, err)

		var result string
		err = rc.Get(ctx, "user:1", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		err = rc.Get(ctx, "user:2", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)

		err = rc.Get(ctx, "token:abc", &result)
		require.NoError(t, err)
		assert.Equal(t, "v3", result)
	})

	t.Run("空模式删除", func(t *testing.T) {
		err := rc.DeletePattern(ctx, "nonexistent:*")
		assert.NoError(t, err)
	})

	t.Run("上下文取消", func(t *testing.T) {
		mr.Set("key1", "v1")

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		err := rc.DeletePattern(cancelCtx, "*")
		assert.Error(t, err)
	})
}

func TestRedisCache_SetNonSerializable(t *testing.T) {
	_, rc := setupMiniRedis(t)
	ctx := context.Background()

	t.Run("Set不可序列化值", func(t *testing.T) {
		ch := make(chan int)
		err := rc.Set(ctx, "bad", ch, 5*time.Minute)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "序列化")
	})
}

func TestRedisCache_Close(t *testing.T) {
	t.Run("Close成功", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()

		rc, err := cache.NewRedisCache(mr.Host(), mr.Port(), "", 0)
		require.NoError(t, err)

		err = rc.Close()
		assert.NoError(t, err)
	})
}

func TestNewRedisCacheWithOptions(t *testing.T) {
	t.Run("连接成功", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()

		opts := &redis.Options{
			Addr: mr.Addr(),
		}

		rc, err := cache.NewRedisCacheWithOptions(opts)
		require.NoError(t, err)
		defer rc.Close()

		ctx := context.Background()
		err = rc.Set(ctx, "key", "value", 5*time.Minute)
		assert.NoError(t, err)
	})

	t.Run("连接失败", func(t *testing.T) {
		opts := &redis.Options{
			Addr:         "invalid-host:6379",
			DialTimeout:  100 * time.Millisecond,
			ReadTimeout:  100 * time.Millisecond,
			WriteTimeout: 100 * time.Millisecond,
		}

		_, err := cache.NewRedisCacheWithOptions(opts)
		assert.Error(t, err)
	})
}

func TestNewCache_RedisSuccess(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	t.Run("启用Redis且连接成功", func(t *testing.T) {
		opt := &cache.Option{
			RedisEnable: true,
			RedisHost:   mr.Host(),
			RedisPort:   mr.Port(),
			RedisDB:     0,
		}

		c, err := cache.NewCache(opt)
		require.NoError(t, err)
		defer c.Close()

		ctx := context.Background()
		err = c.Set(ctx, "key", "value", 5*time.Minute)
		assert.NoError(t, err)

		var result string
		err = c.Get(ctx, "key", &result)
		require.NoError(t, err)
		assert.Equal(t, "value", result)
	})
}

func TestNewCacheWithFallback_RedisSuccess(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	t.Run("启用Redis且连接成功", func(t *testing.T) {
		opt := &cache.Option{
			RedisEnable: true,
			RedisHost:   mr.Host(),
			RedisPort:   mr.Port(),
			RedisDB:     0,
		}

		c, err := cache.NewCacheWithFallback(opt)
		require.NoError(t, err)
		defer c.Close()

		ctx := context.Background()
		err = c.Set(ctx, "key", "value", 5*time.Minute)
		assert.NoError(t, err)
	})
}

func TestRedisCache_JSONRoundTrip(t *testing.T) {
	_, rc := setupMiniRedis(t)
	ctx := context.Background()

	t.Run("复杂结构序列化", func(t *testing.T) {
		type Complex struct {
			ID      int      `json:"id"`
			Tags    []string `json:"tags"`
			Active  bool     `json:"active"`
			Pointer *string  `json:"pointer,omitempty"`
		}

		original := Complex{
			ID:     42,
			Tags:   []string{"a", "b"},
			Active: true,
		}

		err := rc.Set(ctx, "complex", original, 5*time.Minute)
		require.NoError(t, err)

		var result Complex
		err = rc.Get(ctx, "complex", &result)
		require.NoError(t, err)
		assert.Equal(t, 42, result.ID)
		assert.Equal(t, []string{"a", "b"}, result.Tags)
		assert.True(t, result.Active)
	})
}

func TestRedisCache_GetOtherErrors(t *testing.T) {
	t.Run("Get连接错误", func(t *testing.T) {
		opts := &redis.Options{
			Addr:         "invalid-host:6379",
			DialTimeout:  100 * time.Millisecond,
			ReadTimeout:  100 * time.Millisecond,
			WriteTimeout: 100 * time.Millisecond,
		}

		rc, err := cache.NewRedisCacheWithOptions(opts)
		if err != nil {
			return
		}
		defer rc.Close()

		var result string
		err = rc.Get(context.Background(), "key", &result)
		assert.Error(t, err)
	})
}

func TestRedisCache_SetOtherErrors(t *testing.T) {
	t.Run("Set连接错误", func(t *testing.T) {
		opts := &redis.Options{
			Addr:         "invalid-host:6379",
			DialTimeout:  100 * time.Millisecond,
			ReadTimeout:  100 * time.Millisecond,
			WriteTimeout: 100 * time.Millisecond,
		}

		rc, err := cache.NewRedisCacheWithOptions(opts)
		if err != nil {
			return
		}
		defer rc.Close()

		err = rc.Set(context.Background(), "key", "value", 5*time.Minute)
		assert.Error(t, err)
	})
}

func TestRedisCache_DeleteOtherErrors(t *testing.T) {
	t.Run("Delete连接错误", func(t *testing.T) {
		opts := &redis.Options{
			Addr:         "invalid-host:6379",
			DialTimeout:  100 * time.Millisecond,
			ReadTimeout:  100 * time.Millisecond,
			WriteTimeout: 100 * time.Millisecond,
		}

		rc, err := cache.NewRedisCacheWithOptions(opts)
		// May fail to connect, which is expected
		if err != nil {
			return
		}
		defer rc.Close()

		err = rc.Delete(context.Background(), "key")
		assert.Error(t, err)
	})
}

func TestRedisCache_DeletePatternScanError(t *testing.T) {
	t.Run("DeletePattern连接错误", func(t *testing.T) {
		opts := &redis.Options{
			Addr:         "invalid-host:6379",
			DialTimeout:  100 * time.Millisecond,
			ReadTimeout:  100 * time.Millisecond,
			WriteTimeout: 100 * time.Millisecond,
		}

		rc, err := cache.NewRedisCacheWithOptions(opts)
		if err != nil {
			return
		}
		defer rc.Close()

		err = rc.DeletePattern(context.Background(), "*")
		assert.Error(t, err)
	})
}

func TestRedisCache_CloseError(t *testing.T) {
	t.Run("Close后再次操作", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)

		rc, err := cache.NewRedisCache(mr.Host(), mr.Port(), "", 0)
		require.NoError(t, err)

		err = rc.Close()
		assert.NoError(t, err)

		ctx := context.Background()
		err = rc.Set(ctx, "key", "value", 5*time.Minute)
		assert.Error(t, err)
	})
}

// ============================================================================
// RedisCache.Increment / SetTTL / GetTTL / Client / WithMetrics 测试
// 覆盖原本 0% 的计数器与 TTL 管理方法
// ============================================================================

// TestRedisCache_Increment 测试原子递增计数器
func TestRedisCache_Increment(t *testing.T) {
	mr, rc := setupMiniRedis(t)
	ctx := context.Background()

	t.Run("首次递增_返回1", func(t *testing.T) {
		count, err := rc.Increment(ctx, "counter")
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("连续递增_值递增", func(t *testing.T) {
		key := "seq"
		var last int
		for i := 1; i <= 5; i++ {
			count, err := rc.Increment(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, i, count)
			last = count
		}
		assert.Equal(t, 5, last)
	})

	t.Run("不同key独立计数", func(t *testing.T) {
		c1, err := rc.Increment(ctx, "k1")
		require.NoError(t, err)
		c2, err := rc.Increment(ctx, "k2")
		require.NoError(t, err)
		assert.Equal(t, 1, c1)
		assert.Equal(t, 1, c2, "不同 key 应独立计数")
	})

	t.Run("miniredis状态一致", func(t *testing.T) {
		key := "verify"
		rc.Increment(ctx, key)
		rc.Increment(ctx, key)
		// miniredis 维护的真实状态应与返回值一致
		v, err := mr.Get(key)
		require.NoError(t, err)
		assert.Equal(t, "2", v)
	})
}

// TestRedisCache_SetTTL 测试设置键过期时间
func TestRedisCache_SetTTL(t *testing.T) {
	mr, rc := setupMiniRedis(t)
	ctx := context.Background()

	t.Run("为已存在key设置TTL", func(t *testing.T) {
		require.NoError(t, rc.Set(ctx, "k", "v", 5*time.Minute))
		require.NoError(t, rc.SetTTL(ctx, "k", 10*time.Second))
		assert.True(t, mr.TTL("k") <= 10*time.Second)
	})

	t.Run("为不存在的key设置TTL_key仍不存在", func(t *testing.T) {
		// go-redis 的 Expire 对不存在的 key 返回 nil error（结果为 false）
		// 此处验证语义：调用不报错，但 key 实际未创建
		err := rc.SetTTL(ctx, "nonexistent", 10*time.Second)
		assert.NoError(t, err, "SetTTL 对不存在的 key 不报错（Redis 语义）")
		assert.False(t, mr.Exists("nonexistent"), "key 不应被创建")
	})

	t.Run("设置0TTL_永不过期语义", func(t *testing.T) {
		require.NoError(t, rc.Set(ctx, "persist", "v", 5*time.Minute))
		// Expire 0 在 Redis 中表示移除 TTL（永久）
		// 注意：go-redis Expire(0) 实际行为依赖 Redis 版本，miniredis 会返回 false
		// 此处仅验证调用不 panic
		_ = rc.SetTTL(ctx, "persist", 0)
	})
}

// TestRedisCache_GetTTL 测试获取键剩余过期时间
func TestRedisCache_GetTTL(t *testing.T) {
	mr, rc := setupMiniRedis(t)
	ctx := context.Background()

	t.Run("已设置TTL的key_返回正数", func(t *testing.T) {
		require.NoError(t, rc.Set(ctx, "k", "v", 60*time.Second))
		ttl, err := rc.GetTTL(ctx, "k")
		require.NoError(t, err)
		assert.True(t, ttl > 0 && ttl <= 60*time.Second, "TTL 应在 (0, 60s] 范围内，实际 %v", ttl)
	})

	t.Run("不存在的key_返回0", func(t *testing.T) {
		ttl, err := rc.GetTTL(ctx, "nonexistent")
		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), ttl, "不存在的 key 应返回 0（语义：无 TTL 或已过期）")
	})

	t.Run("TTL随时间递减", func(t *testing.T) {
		require.NoError(t, rc.Set(ctx, "decay", "v", 10*time.Second))
		ttl1, _ := rc.GetTTL(ctx, "decay")
		mr.FastForward(5 * time.Second)
		ttl2, _ := rc.GetTTL(ctx, "decay")
		assert.Less(t, ttl2, ttl1, "FastForward 后 TTL 应递减")
	})
}

// TestRedisCache_Client 测试获取底层 Redis 客户端
func TestRedisCache_Client(t *testing.T) {
	_, rc := setupMiniRedis(t)

	t.Run("返回非nil客户端", func(t *testing.T) {
		c := rc.Client()
		assert.NotNil(t, c, "Client() 应返回底层 *redis.Client")
	})

	t.Run("客户端可直接执行命令", func(t *testing.T) {
		ctx := context.Background()
		c := rc.Client()
		require.NotNil(t, c)
		require.NoError(t, c.Set(ctx, "direct", "yes", 0).Err())
		var val string
		require.NoError(t, c.Get(ctx, "direct").Scan(&val))
		assert.Equal(t, "yes", val)
	})
}

// TestRedisCache_WithMetrics 测试设置指标回调
func TestRedisCache_WithMetrics(t *testing.T) {
	_, rc := setupMiniRedis(t)
	ctx := context.Background()

	hitCount := 0
	missCount := 0

	// 设置指标回调
	returned := rc.WithMetrics(
		func() { hitCount++ },
		func() { missCount++ },
	)
	assert.Same(t, rc, returned, "WithMetrics 应返回缓存实例本身（链式调用）")

	// 触发一次 miss（key 不存在）
	var v string
	err := rc.Get(ctx, "missing", &v)
	assert.Error(t, err)
	assert.Equal(t, 1, missCount, "未命中应触发 onMiss 回调")
	assert.Equal(t, 0, hitCount)

	// 设置后触发一次 hit
	require.NoError(t, rc.Set(ctx, "exists", "value", 5*time.Minute))
	var got string
	require.NoError(t, rc.Get(ctx, "exists", &got))
	assert.Equal(t, "value", got)
	assert.Equal(t, 1, hitCount, "命中应触发 onHit 回调")
}
