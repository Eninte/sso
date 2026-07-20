// Package cache_test Redis 缓存单元测试（无 build tag，CI 中也会运行）
// 使用 miniredis 避免依赖真实 Redis
package cache_test

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
)

// setupMiniRedisAlways 创建 miniredis 实例，无 build tag 限制，CI 中也运行
func setupMiniRedisAlways(t *testing.T) (*miniredis.Miniredis, *cache.RedisCache) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	opts := &redis.Options{Addr: mr.Addr()}
	rc, err := cache.NewRedisCacheWithOptions(opts)
	require.NoError(t, err)
	t.Cleanup(func() { rc.Close() })

	return mr, rc
}

// TestCacheKeyBuilders_AlwaysRun 测试缓存键构建函数（覆盖 0% 函数）
func TestCacheKeyBuilders_AlwaysRun(t *testing.T) {
	t.Run("UserTokenKey包含用户ID和前缀", func(t *testing.T) {
		key := cache.UserTokenKey("user-123")
		assert.Contains(t, key, "user-123")
		assert.Contains(t, key, "user:")
		assert.True(t, strings.HasSuffix(key, ":"))
	})

	t.Run("MFAChallengeKey包含token和前缀", func(t *testing.T) {
		token := "abc123def456"
		key := cache.MFAChallengeKey(token)
		assert.Contains(t, key, token)
		assert.NotEqual(t, token, key, "应添加前缀")
	})
}

// TestRedisCache_Client_AlwaysRun 测试 Client() 方法
func TestRedisCache_Client_AlwaysRun(t *testing.T) {
	_, rc := setupMiniRedisAlways(t)
	client := rc.Client()
	assert.NotNil(t, client, "Client() 应返回非 nil 的 redis 客户端")
}

// TestRedisCache_NewRedisCache_AlwaysRun 测试 NewRedisCache 构造函数
func TestRedisCache_NewRedisCache_AlwaysRun(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	host, port, err := net.SplitHostPort(mr.Addr())
	require.NoError(t, err)

	rc, err := cache.NewRedisCache(host, port, "", 0)
	require.NoError(t, err)
	t.Cleanup(func() { rc.Close() })
	assert.NotNil(t, rc)
}

// TestRedisCache_NewRedisCache_ConnectionError_AlwaysRun 测试连接失败的情况
func TestRedisCache_NewRedisCache_ConnectionError_AlwaysRun(t *testing.T) {
	// 使用无效端口触发连接失败（127.0.0.1:1 通常是保留端口）
	_, err := cache.NewRedisCache("127.0.0.1", "1", "", 0)
	assert.Error(t, err, "无效端口应返回连接错误")
}

// TestRedisCache_GetAndDelete_AlwaysRun 测试 GetAndDelete 原子操作
func TestRedisCache_GetAndDelete_AlwaysRun(t *testing.T) {
	_, rc := setupMiniRedisAlways(t)
	ctx := context.Background()

	t.Run("key不存在返回ErrCacheMiss", func(t *testing.T) {
		_, err := rc.GetAndDelete(ctx, "nonexistent")
		assert.ErrorIs(t, err, cache.ErrCacheMiss, "key 不存在应返回 ErrCacheMiss")
	})

	t.Run("key存在返回值并删除", func(t *testing.T) {
		info := cache.StateInfoShim{
			Provider:    "google",
			RedirectURI: "http://localhost/cb",
			CreatedAt:   time.Now().UTC().Truncate(time.Second),
		}
		// 直接通过 redis 客户端写入原始 JSON 字节，避免 cache.Set 的二次序列化
		data, err := json.Marshal(info)
		require.NoError(t, err)
		require.NoError(t, rc.Client().Set(ctx, "oauth:state:test-token", data, time.Minute).Err())

		got, err := rc.GetAndDelete(ctx, "oauth:state:test-token")
		require.NoError(t, err)
		assert.Equal(t, "google", got.Provider)
		assert.Equal(t, "http://localhost/cb", got.RedirectURI)

		_, err = rc.GetAndDelete(ctx, "oauth:state:test-token")
		assert.ErrorIs(t, err, cache.ErrCacheMiss, "第二次应返回 ErrCacheMiss")
	})

	t.Run("key值非JSON返回错误", func(t *testing.T) {
		require.NoError(t, rc.Set(ctx, "oauth:state:bad", "not-json", time.Minute))
		_, err := rc.GetAndDelete(ctx, "oauth:state:bad")
		assert.Error(t, err, "非 JSON 值应返回反序列化错误")
	})
}

// TestRedisCache_Increment_AlwaysRun 测试 Redis Increment
func TestRedisCache_Increment_AlwaysRun(t *testing.T) {
	_, rc := setupMiniRedisAlways(t)
	ctx := context.Background()

	v1, err := rc.Increment(ctx, "counter")
	require.NoError(t, err)
	assert.Equal(t, 1, v1)

	v2, err := rc.Increment(ctx, "counter")
	require.NoError(t, err)
	assert.Equal(t, 2, v2)
}

// TestRedisCache_SetTTL_GetTTL_AlwaysRun 测试 Redis SetTTL/GetTTL
func TestRedisCache_SetTTL_GetTTL_AlwaysRun(t *testing.T) {
	_, rc := setupMiniRedisAlways(t)
	ctx := context.Background()

	t.Run("设置TTL并查询", func(t *testing.T) {
		require.NoError(t, rc.Set(ctx, "k", "v", 5*time.Minute))
		ttl, err := rc.GetTTL(ctx, "k")
		require.NoError(t, err)
		assert.True(t, ttl > 0 && ttl <= 5*time.Minute, "TTL 应在 (0, 5m]，实际 %v", ttl)

		require.NoError(t, rc.SetTTL(ctx, "k", 10*time.Second))
		ttl2, err := rc.GetTTL(ctx, "k")
		require.NoError(t, err)
		assert.True(t, ttl2 > 0 && ttl2 <= 10*time.Second, "SetTTL(10s) 后应 <= 10s，实际 %v", ttl2)
	})

	t.Run("不存在的key返回0无错误", func(t *testing.T) {
		// Redis 行为：不存在的 key 的 TTL 返回 -2，代码转换为 0
		ttl, err := rc.GetTTL(ctx, "nonexistent")
		assert.NoError(t, err)
		assert.Equal(t, time.Duration(0), ttl)
	})

	t.Run("不存在的key设置TTL不报错", func(t *testing.T) {
		// Redis 行为：EXPIRE 对不存在的 key 返回 false 但无错误
		err := rc.SetTTL(ctx, "nonexistent", time.Minute)
		assert.NoError(t, err, "不存在的 key 设置 TTL 不会返回错误（Redis 行为）")
	})
}
