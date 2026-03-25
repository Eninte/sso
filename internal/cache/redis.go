// Package cache 缓存实现
// 提供Token和用户信息的缓存功能，减少数据库查询
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	apperrors "github.com/your-org/sso/internal/errors"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

var (
	ErrCacheMiss = apperrors.ErrCacheMiss
)

// ============================================================================
// 缓存接口
// ============================================================================

// Cache 缓存接口
type Cache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	SetWithNilProtection(ctx context.Context, key string, value interface{}, ttl time.Duration, nilTTL time.Duration) error
	Delete(ctx context.Context, key string) error
	DeletePattern(ctx context.Context, pattern string) error
	Close() error
}

// ============================================================================
// 缓存键定义
// ============================================================================

// 缓存键前缀常量
const (
	TokenCachePrefix  = "token:"  // Token缓存前缀
	UserCachePrefix   = "user:"   // 用户缓存前缀
	ClientCachePrefix = "client:" // 客户端缓存前缀
	NilCachePrefix    = "nil:"    // 空值缓存前缀

	DefaultTTL = 5 * time.Minute  // 默认缓存TTL
	TokenTTL   = 15 * time.Minute // Token缓存TTL
	ClientTTL  = 1 * time.Hour    // 客户端缓存TTL
	NilTTL     = 1 * time.Minute  // 空值缓存TTL，用于防止缓存穿透
)

// 缓存穿透防护标记
var nilCacheValue = []byte("NULL")

// TokenKey 生成Token缓存键
func TokenKey(accessToken string) string {
	return TokenCachePrefix + accessToken
}

// UserIDKey 生成用户ID缓存键
func UserIDKey(userID string) string {
	return UserCachePrefix + userID
}

// UserEmailKey 生成用户邮箱缓存键
func UserEmailKey(email string) string {
	return UserCachePrefix + "email:" + email
}

// ClientKey 生成客户端缓存键
func ClientKey(clientID string) string {
	return ClientCachePrefix + clientID
}

// ============================================================================
// 内存缓存实现
// ============================================================================

// MemoryCache 内存缓存实现
// 使用map存储缓存数据，支持并发安全和自动过期清理
type MemoryCache struct {
	mu     sync.RWMutex
	data   map[string]cacheItem
	stopCh chan struct{} // 用于停止清理goroutine
}

// cacheItem 缓存项
type cacheItem struct {
	value     []byte    // 缓存值（JSON序列化）
	expiresAt time.Time // 过期时间
}

// NewMemoryCache 创建内存缓存实例
// 启动后台goroutine定期清理过期缓存
func NewMemoryCache() *MemoryCache {
	cache := &MemoryCache{
		data:   make(map[string]cacheItem),
		stopCh: make(chan struct{}),
	}
	go cache.cleanup()
	return cache
}

// Get 获取缓存值
// 如果key不存在或已过期，返回ErrCacheMiss
func (c *MemoryCache) Get(ctx context.Context, key string, dest interface{}) error {
	c.mu.RLock()
	item, exists := c.data[key]
	c.mu.RUnlock()

	if !exists {
		return ErrCacheMiss
	}

	// 使用双重检查锁修复竞态条件
	if time.Now().After(item.expiresAt) {
		c.mu.Lock()
		// 再次检查，因为可能在等待锁的过程中已经被其他goroutine删除
		if item, exists := c.data[key]; exists && time.Now().After(item.expiresAt) {
			delete(c.data, key)
		}
		c.mu.Unlock()
		return ErrCacheMiss
	}

	// 检查是否是空值缓存（缓存穿透防护）
	if len(item.value) == len(nilCacheValue) && string(item.value) == string(nilCacheValue) {
		return ErrCacheMiss
	}

	return json.Unmarshal(item.value, dest)
}

// Set 设置缓存值
// value会被JSON序列化存储
func (c *MemoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("序列化缓存值失败: %w", err)
	}

	c.mu.Lock()
	c.data[key] = cacheItem{
		value:     data,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()

	return nil
}

// Delete 删除指定key的缓存
func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	delete(c.data, key)
	c.mu.Unlock()
	return nil
}

// DeletePattern 按模式删除缓存
// pattern支持通配符*，如"user:*"删除所有user前缀的缓存
func (c *MemoryCache) DeletePattern(ctx context.Context, pattern string) error {
	c.mu.Lock()
	for key := range c.data {
		if matchesPattern(key, pattern) {
			delete(c.data, key)
		}
	}
	c.mu.Unlock()
	return nil
}

// Close 关闭缓存，停止清理goroutine并清空所有数据
func (c *MemoryCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 发送停止信号给cleanup goroutine
	select {
	case <-c.stopCh:
		// 已经关闭
	default:
		close(c.stopCh)
	}

	c.data = nil
	return nil
}

func (c *MemoryCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.mu.RLock()
			if c.data == nil {
				c.mu.RUnlock()
				return
			}
			c.mu.RUnlock()

			now := time.Now()
			c.mu.Lock()
			for key, item := range c.data {
				if now.After(item.expiresAt) {
					delete(c.data, key)
				}
			}
			c.mu.Unlock()
		}
	}
}

func matchesPattern(str, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		return len(str) >= len(pattern)-1 && str[:len(pattern)-1] == pattern[:len(pattern)-1]
	}

	return str == pattern
}
