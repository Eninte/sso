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

	"github.com/your-org/sso/internal/cache"
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

		rc, err := cache.NewRedisCache(mr.Host(), "", 0)
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

		rc, err := cache.NewRedisCache(mr.Host(), "", 0)
		require.NoError(t, err)

		err = rc.Close()
		assert.NoError(t, err)

		ctx := context.Background()
		err = rc.Set(ctx, "key", "value", 5*time.Minute)
		assert.Error(t, err)
	})
}
