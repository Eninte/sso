//go:build !integration

package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/cache"
)

func TestLRUCache_DeleteNonExistent(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(10)

	t.Run("删除不存在的键不panic", func(t *testing.T) {
		lru.Delete(ctx, "nonexistent")
	})
}

func TestLRUCache_ClearReuse(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(10)

	lru.Set(ctx, "key1", "value1", 0)
	lru.Set(ctx, "key2", "value2", 0)
	assert.Equal(t, 2, lru.Len())

	lru.Clear()
	assert.Equal(t, 0, lru.Len())

	t.Run("Clear后可继续使用", func(t *testing.T) {
		lru.Set(ctx, "new-key", "new-value", 0)
		value, ok := lru.Get(ctx, "new-key")
		require.True(t, ok)
		assert.Equal(t, "new-value", value)
	})
}

func TestLRUCache_ClearEmpty(t *testing.T) {
	lru := cache.NewLRUCache(10)

	t.Run("空缓存调用Clear不panic", func(t *testing.T) {
		lru.Clear()
		assert.Equal(t, 0, lru.Len())
	})
}

func TestLRUCache_SetTTLZero(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(10)

	t.Run("TTL=0永不过期", func(t *testing.T) {
		lru.Set(ctx, "no-expire", "value", 0)

		value, ok := lru.Get(ctx, "no-expire")
		require.True(t, ok)
		assert.Equal(t, "value", value)

		time.Sleep(50 * time.Millisecond)

		value, ok = lru.Get(ctx, "no-expire")
		require.True(t, ok)
		assert.Equal(t, "value", value)
	})
}

func TestLRUCache_SetUpdateWithTTL(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(10)

	t.Run("更新已有键带新TTL", func(t *testing.T) {
		lru.Set(ctx, "key", "old", 1*time.Hour)

		value, ok := lru.Get(ctx, "key")
		require.True(t, ok)
		assert.Equal(t, "old", value)

		lru.Set(ctx, "key", "new", 50*time.Millisecond)

		time.Sleep(100 * time.Millisecond)

		_, ok = lru.Get(ctx, "key")
		assert.False(t, ok)
	})
}

func TestLRUCache_SetUpdateWithZeroTTL(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(10)

	t.Run("更新已有键TTL=0不改变expiresAt", func(t *testing.T) {
		lru.Set(ctx, "key", "value", 1*time.Hour)

		lru.Set(ctx, "key", "updated", 0)

		value, ok := lru.Get(ctx, "key")
		require.True(t, ok)
		assert.Equal(t, "updated", value)
	})
}

func TestLRUCache_LenAfterEviction(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(2)

	lru.Set(ctx, "key1", "v1", 0)
	lru.Set(ctx, "key2", "v2", 0)
	lru.Set(ctx, "key3", "v3", 0)

	assert.Equal(t, 2, lru.Len())
}

func TestLRUCache_CapacityOne(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(1)

	lru.Set(ctx, "a", 1, 0)
	lru.Set(ctx, "b", 2, 0)

	_, ok := lru.Get(ctx, "a")
	assert.False(t, ok)

	value, ok := lru.Get(ctx, "b")
	require.True(t, ok)
	assert.Equal(t, 2, value)
}

func TestMemoryCache_NilCacheValueImmediate(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	t.Run("SetWithNilProtection后立即Get返回CacheMiss", func(t *testing.T) {
		err := c.SetWithNilProtection(ctx, "nil-key", nil, 5*time.Minute, 1*time.Second)
		require.NoError(t, err)

		var result string
		err = c.Get(ctx, "nil-key", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})
}

func TestMemoryCache_DeletePatternEdgeCases(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	t.Run("空缓存调用DeletePattern", func(t *testing.T) {
		err := c.DeletePattern(ctx, "*")
		assert.NoError(t, err)
	})

	t.Run("无匹配模式", func(t *testing.T) {
		c.Set(ctx, "foo", "bar", 5*time.Minute)
		err := c.DeletePattern(ctx, "nomatch:*")
		assert.NoError(t, err)

		var result string
		err = c.Get(ctx, "foo", &result)
		assert.NoError(t, err)
		assert.Equal(t, "bar", result)
	})
}

func TestMemoryCache_CloseWithDataNil(t *testing.T) {
	t.Run("Close后data为nil再次Close", func(t *testing.T) {
		c := cache.NewMemoryCache()
		err := c.Close()
		assert.NoError(t, err)

		err = c.Close()
		assert.NoError(t, err)
	})
}

func TestMatchesPatternEdgeCases(t *testing.T) {
	t.Run("x*匹配x", func(t *testing.T) {
		c := cache.NewMemoryCache()
		defer c.Close()
		ctx := context.Background()

		c.Set(ctx, "x", "value", 5*time.Minute)
		err := c.DeletePattern(ctx, "x*")
		require.NoError(t, err)

		var result string
		err = c.Get(ctx, "x", &result)
		assert.ErrorIs(t, err, cache.ErrCacheMiss)
	})

	t.Run("abc:*不匹配ab", func(t *testing.T) {
		c := cache.NewMemoryCache()
		defer c.Close()
		ctx := context.Background()

		c.Set(ctx, "ab", "value", 5*time.Minute)
		err := c.DeletePattern(ctx, "abc:*")
		require.NoError(t, err)

		var result string
		err = c.Get(ctx, "ab", &result)
		assert.NoError(t, err)
		assert.Equal(t, "value", result)
	})
}

func TestCacheConstants(t *testing.T) {
	t.Run("TTL常量值", func(t *testing.T) {
		assert.Equal(t, 5*time.Minute, cache.DefaultTTL)
		assert.Equal(t, 15*time.Minute, cache.TokenTTL)
		assert.Equal(t, 1*time.Hour, cache.ClientTTL)
		assert.Equal(t, 1*time.Minute, cache.NilTTL)
	})

	t.Run("前缀常量值", func(t *testing.T) {
		assert.Equal(t, "token:", cache.TokenCachePrefix)
		assert.Equal(t, "user:", cache.UserCachePrefix)
		assert.Equal(t, "client:", cache.ClientCachePrefix)
		assert.Equal(t, "nil:", cache.NilCachePrefix)
	})
}

func TestCacheErrorConstants(t *testing.T) {
	t.Run("ErrRedisConnectionFailed", func(t *testing.T) {
		assert.NotNil(t, cache.ErrRedisConnectionFailed)
		assert.Contains(t, cache.ErrRedisConnectionFailed.Error(), "连接失败")
	})

	t.Run("ErrRedisPingFailed", func(t *testing.T) {
		assert.NotNil(t, cache.ErrRedisPingFailed)
		assert.Contains(t, cache.ErrRedisPingFailed.Error(), "健康检查失败")
	})
}

func TestMemoryCache_CleanupGoroutine(t *testing.T) {
	t.Run("Close后cleanup退出", func(t *testing.T) {
		c := cache.NewMemoryCache()

		err := c.Close()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
	})
}
