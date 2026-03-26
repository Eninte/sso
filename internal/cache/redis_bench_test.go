// Package cache_test 缓存基准测试
package cache_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/your-org/sso/internal/cache"
)

// ============================================================================
// MemoryCache 基准测试
// ============================================================================

func BenchmarkMemoryCache_Set(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		if err := c.Set(ctx, key, "value", 5*time.Minute); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMemoryCache_Get(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	// 预热缓存
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		c.Set(ctx, key, "value", 5*time.Minute)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%1000)
		var result string
		if err := c.Get(ctx, key, &result); err != nil && err != cache.ErrCacheMiss {
			b.Fatal(err)
		}
	}
}

func BenchmarkMemoryCache_SetGet(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		c.Set(ctx, key, "value", 5*time.Minute)
		var result string
		c.Get(ctx, key, &result)
	}
}

func BenchmarkMemoryCache_Delete(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	// 预热缓存
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		c.Set(ctx, key, "value", 5*time.Minute)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		if err := c.Delete(ctx, key); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMemoryCache_DeletePattern(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 每次迭代创建100个键，然后批量删除
		for j := 0; j < 100; j++ {
			key := fmt.Sprintf("pattern:%d:%d", i, j)
			c.Set(ctx, key, "value", 5*time.Minute)
		}
		c.DeletePattern(ctx, "pattern:*")
	}
}

// ============================================================================
// MemoryCache 并发基准测试
// ============================================================================

func BenchmarkMemoryCache_Parallel(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	// 预热缓存
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		c.Set(ctx, key, "value", 5*time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			if i%2 == 0 {
				var result string
				c.Get(ctx, key, &result)
			} else {
				c.Set(ctx, key, "new-value", 5*time.Minute)
			}
			i++
		}
	})
}

func BenchmarkMemoryCache_Parallel_Read(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	// 预热缓存
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		c.Set(ctx, key, "value", 5*time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			var result string
			c.Get(ctx, key, &result)
			i++
		}
	})
}

func BenchmarkMemoryCache_Parallel_Write(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i)
			c.Set(ctx, key, "value", 5*time.Minute)
			i++
		}
	})
}

// ============================================================================
// SetWithNilProtection 基准测试
// ============================================================================

func BenchmarkMemoryCache_SetWithNilProtection(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		if i%2 == 0 {
			c.SetWithNilProtection(ctx, key, "value", 5*time.Minute, 1*time.Minute)
		} else {
			c.SetWithNilProtection(ctx, key, nil, 5*time.Minute, 1*time.Minute)
		}
	}
}

// ============================================================================
// 大对象基准测试
// ============================================================================

type LargeObject struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Data      []byte    `json:"data"`
	CreatedAt time.Time `json:"created_at"`
}

func BenchmarkMemoryCache_Set_LargeObject(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	obj := LargeObject{
		ID:        "test-id",
		Name:      "Test User",
		Email:     "test@example.com",
		Data:      make([]byte, 1024), // 1KB
		CreatedAt: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		c.Set(ctx, key, obj, 5*time.Minute)
	}
}

func BenchmarkMemoryCache_Get_LargeObject(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	obj := LargeObject{
		ID:        "test-id",
		Name:      "Test User",
		Email:     "test@example.com",
		Data:      make([]byte, 1024),
		CreatedAt: time.Now(),
	}

	// 预热
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		c.Set(ctx, key, obj, 5*time.Minute)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%1000)
		var result LargeObject
		c.Get(ctx, key, &result)
	}
}

// ============================================================================
// 缓存键函数基准测试
// ============================================================================

func BenchmarkTokenKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cache.TokenKey("access-token-123456789")
	}
}

func BenchmarkUserIDKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cache.UserIDKey("user-123456789")
	}
}

func BenchmarkUserEmailKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cache.UserEmailKey("test@example.com")
	}
}

func BenchmarkClientKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cache.ClientKey("client-123456789")
	}
}

// ============================================================================
// 并发安全基准测试
// ============================================================================

func BenchmarkMemoryCache_ConcurrentMixed(b *testing.B) {
	c := cache.NewMemoryCache()
	defer c.Close()
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 10
	opsPerGoroutine := b.N / numGoroutines

	b.ResetTimer()
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				key := fmt.Sprintf("key-%d-%d", id, i)
				switch i % 4 {
				case 0:
					c.Set(ctx, key, "value", 5*time.Minute)
				case 1:
					var result string
					c.Get(ctx, key, &result)
				case 2:
					c.Delete(ctx, key)
				case 3:
					c.SetWithNilProtection(ctx, key, "value", 5*time.Minute, 1*time.Minute)
				}
			}
		}(g)
	}
	wg.Wait()
}
