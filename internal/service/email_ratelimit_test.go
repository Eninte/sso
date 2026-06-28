// Package service 邮件限流测试
package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/example/sso/internal/cache"
)

// TestEmailRateLimiter_CheckLimit 测试邮件限流检查
func TestEmailRateLimiter_CheckLimit(t *testing.T) {
	ctx := context.Background()
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	rateLimiter := NewEmailRateLimiter(memCache)

	email := "test@example.com"

	t.Run("首次发送_允许", func(t *testing.T) {
		allowed, remaining, err := rateLimiter.CheckLimit(ctx, email)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 4, remaining) // 5-1=4
	})

	t.Run("连续发送5次_第5次允许", func(t *testing.T) {
		// 重置
		rateLimiter.Reset(ctx, email)

		for i := 1; i <= 5; i++ {
			allowed, remaining, err := rateLimiter.CheckLimit(ctx, email)
			assert.NoError(t, err)
			assert.True(t, allowed, "第%d次应该允许", i)
			assert.Equal(t, 5-i, remaining)
		}
	})

	t.Run("第6次发送_拒绝", func(t *testing.T) {
		// 重置并发送5次
		rateLimiter.Reset(ctx, email)
		for i := 0; i < 5; i++ {
			rateLimiter.CheckLimit(ctx, email)
		}

		// 第6次应该被拒绝
		allowed, remaining, err := rateLimiter.CheckLimit(ctx, email)
		assert.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 0, remaining)
	})

	t.Run("不同邮箱独立计数", func(t *testing.T) {
		email1 := "user1@example.com"
		email2 := "user2@example.com"

		rateLimiter.Reset(ctx, email1)
		rateLimiter.Reset(ctx, email2)

		// email1发送3次
		for i := 0; i < 3; i++ {
			rateLimiter.CheckLimit(ctx, email1)
		}

		// email2应该还能发送5次
		allowed, remaining, err := rateLimiter.CheckLimit(ctx, email2)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 4, remaining)
	})
}

// TestEmailRateLimiter_GetRemaining 测试获取剩余次数
func TestEmailRateLimiter_GetRemaining(t *testing.T) {
	ctx := context.Background()
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	rateLimiter := NewEmailRateLimiter(memCache)
	email := "test@example.com"

	t.Run("未发送过_返回最大值", func(t *testing.T) {
		remaining, err := rateLimiter.GetRemaining(ctx, email)
		assert.NoError(t, err)
		assert.Equal(t, EmailRateLimitMax, remaining)
	})

	t.Run("发送2次后_返回3", func(t *testing.T) {
		rateLimiter.Reset(ctx, email)
		rateLimiter.CheckLimit(ctx, email)
		rateLimiter.CheckLimit(ctx, email)

		remaining, err := rateLimiter.GetRemaining(ctx, email)
		assert.NoError(t, err)
		assert.Equal(t, 3, remaining)
	})

	t.Run("超过限制后_返回0", func(t *testing.T) {
		rateLimiter.Reset(ctx, email)
		for i := 0; i < 6; i++ {
			rateLimiter.CheckLimit(ctx, email)
		}

		remaining, err := rateLimiter.GetRemaining(ctx, email)
		assert.NoError(t, err)
		assert.Equal(t, 0, remaining)
	})
}

// TestEmailRateLimiter_GetTTL 测试获取TTL
func TestEmailRateLimiter_GetTTL(t *testing.T) {
	ctx := context.Background()
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	rateLimiter := NewEmailRateLimiter(memCache)
	email := "test@example.com"

	t.Run("发送后有TTL", func(t *testing.T) {
		rateLimiter.Reset(ctx, email)
		rateLimiter.CheckLimit(ctx, email)

		ttl, err := rateLimiter.GetTTL(ctx, email)
		assert.NoError(t, err)
		assert.Greater(t, ttl, time.Duration(0))
		assert.LessOrEqual(t, ttl, EmailRateLimitWindow)
	})
}

// TestEmailRateLimiter_Reset 测试重置限流
func TestEmailRateLimiter_Reset(t *testing.T) {
	ctx := context.Background()
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	rateLimiter := NewEmailRateLimiter(memCache)
	email := "test@example.com"

	t.Run("重置后可以重新发送", func(t *testing.T) {
		// 发送5次达到限制
		for i := 0; i < 5; i++ {
			rateLimiter.CheckLimit(ctx, email)
		}

		// 验证已达到限制
		allowed, _, _ := rateLimiter.CheckLimit(ctx, email)
		assert.False(t, allowed)

		// 重置
		err := rateLimiter.Reset(ctx, email)
		assert.NoError(t, err)

		// 重置后应该可以发送
		allowed, remaining, err := rateLimiter.CheckLimit(ctx, email)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 4, remaining)
	})
}

// TestEmailRateLimiter_NilCache 测试无缓存时的行为
func TestEmailRateLimiter_NilCache(t *testing.T) {
	ctx := context.Background()
	rateLimiter := NewEmailRateLimiter(nil)

	email := "test@example.com"

	t.Run("无缓存时总是允许", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			allowed, remaining, err := rateLimiter.CheckLimit(ctx, email)
			assert.NoError(t, err)
			assert.True(t, allowed)
			assert.Equal(t, EmailRateLimitMax, remaining)
		}
	})
}

// TestFormatRateLimitError 测试错误消息格式化
func TestFormatRateLimitError(t *testing.T) {
	tests := []struct {
		name     string
		ttl      time.Duration
		expected string
	}{
		{
			name:     "小于1分钟",
			ttl:      30 * time.Second,
			expected: "发送邮件过于频繁，请稍后再试",
		},
		{
			name:     "5分钟",
			ttl:      5 * time.Minute,
			expected: "发送邮件过于频繁，请在 5 分钟后再试",
		},
		{
			name:     "30分钟",
			ttl:      30 * time.Minute,
			expected: "发送邮件过于频繁，请在 30 分钟后再试",
		},
		{
			name:     "1小时",
			ttl:      60 * time.Minute,
			expected: "发送邮件过于频繁，请在 60 分钟后再试",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatRateLimitError(tt.ttl)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestEmailRateLimiter_Concurrent 测试并发场景
func TestEmailRateLimiter_Concurrent(t *testing.T) {
	ctx := context.Background()
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	rateLimiter := NewEmailRateLimiter(memCache)
	email := "concurrent@example.com"

	t.Run("并发请求正确计数", func(t *testing.T) {
		rateLimiter.Reset(ctx, email)

		// 并发发送10个请求
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				rateLimiter.CheckLimit(ctx, email)
				done <- true
			}()
		}

		// 等待所有请求完成
		for i := 0; i < 10; i++ {
			<-done
		}

		// 验证计数正确（应该超过限制）
		allowed, _, _ := rateLimiter.CheckLimit(ctx, email)
		assert.False(t, allowed, "并发请求后应该超过限制")
	})
}
