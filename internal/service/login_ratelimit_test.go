// Package service 登录限流测试
package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/example/sso/internal/cache"
)

// TestLoginRateLimiter_CheckAndRecord 测试登录限流检查与计数
func TestLoginRateLimiter_CheckAndRecord(t *testing.T) {
	ctx := context.Background()
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	limiter := NewLoginRateLimiter(memCache)
	ip := "192.168.1.1"

	t.Run("首次尝试_允许且剩余为limit-1", func(t *testing.T) {
		allowed, remaining, err := limiter.CheckAndRecord(ctx, ip)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, LoginRateLimitMax-1, remaining)
	})

	t.Run("连续尝试至上限_最后一次允许", func(t *testing.T) {
		// 重新用新 IP 避免与上一子测试累积
		ip2 := "10.0.0.1"
		for i := 1; i <= LoginRateLimitMax; i++ {
			allowed, remaining, err := limiter.CheckAndRecord(ctx, ip2)
			assert.NoError(t, err)
			assert.True(t, allowed, "第%d次应允许", i)
			assert.Equal(t, LoginRateLimitMax-i, remaining)
		}
	})

	t.Run("超过上限_拒绝", func(t *testing.T) {
		ip3 := "10.0.0.2"
		// 用满额度
		for i := 0; i < LoginRateLimitMax; i++ {
			limiter.CheckAndRecord(ctx, ip3)
		}
		// 第 limit+1 次应拒绝
		allowed, remaining, err := limiter.CheckAndRecord(ctx, ip3)
		assert.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 0, remaining)
	})

	t.Run("不同IP独立计数", func(t *testing.T) {
		ipA := "172.16.0.1"
		ipB := "172.16.0.2"
		// ipA 用掉 3 次
		for i := 0; i < 3; i++ {
			limiter.CheckAndRecord(ctx, ipA)
		}
		// ipB 仍应是首次
		allowed, remaining, err := limiter.CheckAndRecord(ctx, ipB)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, LoginRateLimitMax-1, remaining)
	})
}

// TestLoginRateLimiter_EdgeCases 测试边界场景
func TestLoginRateLimiter_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("nil缓存_始终允许", func(t *testing.T) {
		limiter := NewLoginRateLimiter(nil)
		allowed, remaining, err := limiter.CheckAndRecord(ctx, "1.2.3.4")
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, LoginRateLimitMax, remaining)
	})

	t.Run("空IP_始终允许", func(t *testing.T) {
		memCache := cache.NewMemoryCache()
		defer memCache.Close()
		limiter := NewLoginRateLimiter(memCache)

		allowed, remaining, err := limiter.CheckAndRecord(ctx, "")
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, LoginRateLimitMax, remaining)
	})
}

// TestFormatLoginRateLimitError 测试限流错误消息格式化
func TestFormatLoginRateLimitError(t *testing.T) {
	t.Run("不足1分钟_通用提示", func(t *testing.T) {
		msg := FormatLoginRateLimitError(30 * time.Second)
		assert.Equal(t, "登录尝试过于频繁，请稍后再试", msg)
	})

	t.Run("0时长_通用提示", func(t *testing.T) {
		msg := FormatLoginRateLimitError(0)
		assert.Equal(t, "登录尝试过于频繁，请稍后再试", msg)
	})

	t.Run("正好1分钟_显示分钟数", func(t *testing.T) {
		msg := FormatLoginRateLimitError(1 * time.Minute)
		assert.Equal(t, "登录尝试过于频繁，请在 1 分钟后再试", msg)
	})

	t.Run("多分钟_显示分钟数", func(t *testing.T) {
		msg := FormatLoginRateLimitError(10 * time.Minute)
		assert.Equal(t, "登录尝试过于频繁，请在 10 分钟后再试", msg)
	})
}

// TestNewLoginRateLimiter 测试构造函数默认值
func TestNewLoginRateLimiter(t *testing.T) {
	memCache := cache.NewMemoryCache()
	defer memCache.Close()

	limiter := NewLoginRateLimiter(memCache)
	assert.NotNil(t, limiter)
	assert.Equal(t, LoginRateLimitMax, limiter.limit)
	assert.Equal(t, LoginRateLimitWindow, limiter.window)
	assert.Same(t, memCache, limiter.cache)
}
