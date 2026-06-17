// Package crypto JTI跟踪器实现
// 用于防止JWT重放攻击
package crypto

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// CacheInterface 缓存接口（最小化依赖）
// 避免直接依赖cache包，保持crypto包的独立性
type CacheInterface interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Increment(ctx context.Context, key string) (int, error)
	SetTTL(ctx context.Context, key string, ttl time.Duration) error
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

// CheckAndMarkUsed 原子性检查并标记JTI为已使用
// 使用缓存的Increment操作实现原子性，防止TOCTOU竞态
// 返回true表示JTI已被使用过（重放攻击），false表示首次使用
func (t *CacheJTITracker) CheckAndMarkUsed(ctx context.Context, jti string, ttl time.Duration) (bool, error) {
	key := t.prefix + jti

	// Increment是原子操作（Redis INCR / 内存缓存mutex保护）
	// 返回值为1表示首次设置，>1表示已存在
	count, err := t.cache.Increment(ctx, key)
	if err != nil {
		return false, fmt.Errorf("JTI原子检查失败: %w", err)
	}

	// 首次使用，设置过期时间
	if count == 1 {
		// 兜底：确保ttl为正数，避免Redis EXPIRE立即删除key导致重放检测失效
		if ttl <= 0 {
			ttl = time.Minute
		}
		if err := t.cache.SetTTL(ctx, key, ttl); err != nil {
			// SetTTL失败：Redis INCR创建的key无TTL，不清理会永久驻留
			slog.Warn("JTI SetTTL失败，key可能无过期时间", "jti", jti, "err", err)
		}
		return false, nil
	}

	// count > 1，JTI已被使用过，判定为重放攻击
	return true, nil
}

// IsJTIUsed 检查JTI是否已被使用
func (t *CacheJTITracker) IsJTIUsed(ctx context.Context, jti string) (bool, error) {
	key := t.prefix + jti
	var count int
	err := t.cache.Get(ctx, key, &count)
	if err != nil {
		// 缓存未命中表示JTI未被使用
		return false, nil
	}
	return count > 0, nil
}

// MarkJTIUsed 标记JTI为已使用
// ttl: JTI的有效期，应该与token的有效期一致
func (t *CacheJTITracker) MarkJTIUsed(ctx context.Context, jti string, ttl time.Duration) error {
	key := t.prefix + jti
	if err := t.cache.Set(ctx, key, 1, ttl); err != nil {
		return fmt.Errorf("标记JTI为已使用失败: %w", err)
	}
	return nil
}
