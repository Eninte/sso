// Package crypto JTI跟踪器实现
// 用于防止JWT重放攻击
package crypto

import (
	"context"
	"fmt"
	"time"
)

// CacheInterface 缓存接口（最小化依赖）
// 避免直接依赖cache包，保持crypto包的独立性
type CacheInterface interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
}

// CacheJTITracker 基于缓存的JTI跟踪器
// 使用Redis或内存缓存存储已使用的JTI
type CacheJTITracker struct {
	cache  CacheInterface
	prefix string // 缓存键前缀
}

// NewCacheJTITracker 创建基于缓存的JTI跟踪器
// cache: 缓存实现（Redis或内存缓存）
// prefix: 缓存键前缀，默认为"jti:"
func NewCacheJTITracker(cache CacheInterface, prefix string) *CacheJTITracker {
	if prefix == "" {
		prefix = "jti:"
	}
	return &CacheJTITracker{
		cache:  cache,
		prefix: prefix,
	}
}

// IsJTIUsed 检查JTI是否已被使用
func (t *CacheJTITracker) IsJTIUsed(ctx context.Context, jti string) (bool, error) {
	key := t.prefix + jti
	var used bool
	err := t.cache.Get(ctx, key, &used)
	if err != nil {
		// 缓存未命中表示JTI未被使用
		return false, nil
	}
	return used, nil
}

// MarkJTIUsed 标记JTI为已使用
// ttl: JTI的有效期，应该与token的有效期一致
func (t *CacheJTITracker) MarkJTIUsed(ctx context.Context, jti string, ttl time.Duration) error {
	key := t.prefix + jti
	if err := t.cache.Set(ctx, key, true, ttl); err != nil {
		return fmt.Errorf("标记JTI为已使用失败: %w", err)
	}
	return nil
}
