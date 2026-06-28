// Package cache_test 缓存优化单元测试
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

// ============================================================================
// MemoryCache.DeletePattern 优化测试
// ============================================================================

func TestMemoryCache_DeletePattern_Optimized(t *testing.T) {
	mc := cache.NewMemoryCache()
	ctx := context.Background()

	// 设置多个key
	_ = mc.Set(ctx, "user:1", "value1", 5*time.Minute)
	_ = mc.Set(ctx, "user:2", "value2", 5*time.Minute)
	_ = mc.Set(ctx, "user:3", "value3", 5*time.Minute)
	_ = mc.Set(ctx, "session:1", "session1", 5*time.Minute)

	// 删除user:*模式
	err := mc.DeletePattern(ctx, "user:*")
	require.NoError(t, err)

	// 验证user:*被删除
	var val string
	assert.Error(t, mc.Get(ctx, "user:1", &val))
	assert.Error(t, mc.Get(ctx, "user:2", &val))
	assert.Error(t, mc.Get(ctx, "user:3", &val))

	// 验证session:*未被删除
	assert.NoError(t, mc.Get(ctx, "session:1", &val))
	assert.Equal(t, "session1", val)
}

func TestMemoryCache_DeletePattern_Concurrent(t *testing.T) {
	mc := cache.NewMemoryCache()
	ctx := context.Background()

	// 设置大量key
	for i := 0; i < 100; i++ {
		_ = mc.Set(ctx, fmt.Sprintf("key:%d", i), fmt.Sprintf("value%d", i), 5*time.Minute)
	}

	// 并发执行DeletePattern
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mc.DeletePattern(ctx, "key:*")
		}()
	}
	wg.Wait()

	// 验证所有key被删除
	var val string
	for i := 0; i < 100; i++ {
		assert.Error(t, mc.Get(ctx, fmt.Sprintf("key:%d", i), &val))
	}
}

// ============================================================================
// SingleflightCache 测试
// ============================================================================

func TestSingleflightCache_Do(t *testing.T) {
	baseCache := cache.NewMemoryCache()
	sf := cache.NewSingleflightCache(baseCache)
	ctx := context.Background()

	callCount := 0

	// 第一次调用 - 应该执行load函数
	value, err := sf.Do(ctx, "test-key", 5*time.Minute, func(context.Context) (interface{}, error) {
		callCount++
		return "test-value", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "test-value", value)
	assert.Equal(t, 1, callCount)

	// 第二次调用 - 应该从缓存获取，不执行load函数
	value, err = sf.Do(ctx, "test-key", 5*time.Minute, func(context.Context) (interface{}, error) {
		callCount++
		return "new-value", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "test-value", value) // 仍然是旧值
	assert.Equal(t, 1, callCount)        // load函数只调用了一次
}

func TestSingleflightCache_Do_Concurrent(t *testing.T) {
	baseCache := cache.NewMemoryCache()
	sf := cache.NewSingleflightCache(baseCache)
	ctx := context.Background()

	callCount := 0
	var mu sync.Mutex

	// 并发调用同一个key，应该只有一个执行load函数
	var wg sync.WaitGroup
	results := make([]interface{}, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			value, err := sf.Do(ctx, "concurrent-key", 5*time.Minute, func(context.Context) (interface{}, error) {
				mu.Lock()
				callCount++
				mu.Unlock()
				time.Sleep(50 * time.Millisecond) // 模拟慢查询
				return fmt.Sprintf("result-%d", idx), nil
			})
			if err == nil {
				results[idx] = value
			}
		}(i)
	}
	wg.Wait()

	// load函数应该只被调用1次
	assert.Equal(t, 1, callCount)

	// 所有结果应该相同（都是第一个完成的结果）
	firstResult := results[0]
	for _, r := range results {
		assert.Equal(t, firstResult, r)
	}
}

func TestSingleflightCache_Do_Error(t *testing.T) {
	baseCache := cache.NewMemoryCache()
	sf := cache.NewSingleflightCache(baseCache)
	ctx := context.Background()

	// load函数返回错误
	_, err := sf.Do(ctx, "error-key", 5*time.Minute, func(context.Context) (interface{}, error) {
		return nil, fmt.Errorf("load failed")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load failed")
}

func TestSingleflightCache_Do_DifferentKeys(t *testing.T) {
	baseCache := cache.NewMemoryCache()
	sf := cache.NewSingleflightCache(baseCache)
	ctx := context.Background()

	callCount := 0

	// 不同key应该独立执行
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key-%d", i)
		_, _ = sf.Do(ctx, key, 5*time.Minute, func(context.Context) (interface{}, error) {
			callCount++
			return key, nil
		})
	}

	assert.Equal(t, 5, callCount)
}
