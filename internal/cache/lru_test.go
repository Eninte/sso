// Package cache LRU缓存测试
package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
)

func TestLRUCache_Basic(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(3)

	t.Run("设置和获取", func(t *testing.T) {
		lru.Set(ctx, "key1", "value1", 0)

		value, ok := lru.Get(ctx, "key1")
		require.True(t, ok)
		assert.Equal(t, "value1", value)
	})

	t.Run("获取不存在的键", func(t *testing.T) {
		_, ok := lru.Get(ctx, "nonexistent")
		assert.False(t, ok)
	})

	t.Run("更新已存在的键", func(t *testing.T) {
		lru.Set(ctx, "key1", "new_value", 0)

		value, ok := lru.Get(ctx, "key1")
		require.True(t, ok)
		assert.Equal(t, "new_value", value)
	})
}

func TestLRUCache_Eviction(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(2)

	// 添加3个条目，但容量只有2
	lru.Set(ctx, "key1", "value1", 0)
	lru.Set(ctx, "key2", "value2", 0)
	lru.Set(ctx, "key3", "value3", 0)

	// key1 应该被驱逐
	_, ok := lru.Get(ctx, "key1")
	assert.False(t, ok)

	// key2 和 key3 应该还在
	value, ok := lru.Get(ctx, "key2")
	require.True(t, ok)
	assert.Equal(t, "value2", value)

	value, ok = lru.Get(ctx, "key3")
	require.True(t, ok)
	assert.Equal(t, "value3", value)
}

func TestLRUCache_LRU_Eviction(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(2)

	lru.Set(ctx, "key1", "value1", 0)
	lru.Set(ctx, "key2", "value2", 0)

	// 访问 key1，使其成为最近使用的
	_, ok := lru.Get(ctx, "key1")
	require.True(t, ok)

	// 添加 key3，应该驱逐 key2（最久未使用）
	lru.Set(ctx, "key3", "value3", 0)

	// key1 应该还在
	value, ok := lru.Get(ctx, "key1")
	require.True(t, ok)
	assert.Equal(t, "value1", value)

	// key2 应该被驱逐
	_, ok = lru.Get(ctx, "key2")
	assert.False(t, ok)

	// key3 应该存在
	value, ok = lru.Get(ctx, "key3")
	require.True(t, ok)
	assert.Equal(t, "value3", value)
}

func TestLRUCache_TTL(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(10)

	// 设置一个100ms过期的条目
	lru.Set(ctx, "key1", "value1", 100*time.Millisecond)

	// 立即获取应该成功
	value, ok := lru.Get(ctx, "key1")
	require.True(t, ok)
	assert.Equal(t, "value1", value)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 获取应该失败
	_, ok = lru.Get(ctx, "key1")
	assert.False(t, ok)
}

func TestLRUCache_Delete(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(10)

	lru.Set(ctx, "key1", "value1", 0)

	// 删除前应该存在
	_, ok := lru.Get(ctx, "key1")
	require.True(t, ok)

	// 删除
	lru.Delete(ctx, "key1")

	// 删除后应该不存在
	_, ok = lru.Get(ctx, "key1")
	assert.False(t, ok)
}

func TestLRUCache_Len(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(10)

	assert.Equal(t, 0, lru.Len())

	lru.Set(ctx, "key1", "value1", 0)
	assert.Equal(t, 1, lru.Len())

	lru.Set(ctx, "key2", "value2", 0)
	assert.Equal(t, 2, lru.Len())

	lru.Delete(ctx, "key1")
	assert.Equal(t, 1, lru.Len())
}

func TestLRUCache_Clear(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(10)

	lru.Set(ctx, "key1", "value1", 0)
	lru.Set(ctx, "key2", "value2", 0)

	assert.Equal(t, 2, lru.Len())

	lru.Clear()

	assert.Equal(t, 0, lru.Len())
	_, ok := lru.Get(ctx, "key1")
	assert.False(t, ok)
	_, ok = lru.Get(ctx, "key2")
	assert.False(t, ok)
}

func TestLRUCache_Concurrent(t *testing.T) {
	ctx := context.Background()
	lru := cache.NewLRUCache(100)

	done := make(chan bool, 10)

	// 启动10个goroutine并发访问
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := string(rune('a' + id))
				lru.Set(ctx, key, id, 0)
				lru.Get(ctx, key)
			}
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证缓存正常工作
	assert.LessOrEqual(t, lru.Len(), 100)
}
