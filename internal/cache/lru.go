// Package cache LRU缓存实现
// 提供基于内存的LRU缓存，用于减少Redis网络调用
package cache

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// ============================================================================
// LRU缓存
// ============================================================================

// LRUCache 基于内存的LRU缓存实现
type LRUCache struct {
	cache     map[string]*list.Element
	evictList *list.List
	mu        sync.RWMutex
	capacity  int
}

// cacheEntry 缓存条目
type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
	key       string
}

// NewLRUCache 创建LRU缓存
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity:  capacity,
		cache:     make(map[string]*list.Element),
		evictList: list.New(),
	}
}

// Get 获取缓存值
func (c *LRUCache) Get(ctx context.Context, key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.cache[key]
	if !ok {
		return nil, false
	}

	entry := elem.Value.(*cacheEntry)

	// 检查是否过期
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		c.removeElement(elem)
		return nil, false
	}

	// 移动到链表头部（最近使用）
	c.evictList.MoveToFront(elem)
	return entry.value, true
}

// Set 设置缓存值
func (c *LRUCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已存在
	if elem, ok := c.cache[key]; ok {
		// 更新值并移动到头部
		entry := elem.Value.(*cacheEntry)
		entry.value = value
		if ttl > 0 {
			entry.expiresAt = time.Now().Add(ttl)
		}
		c.evictList.MoveToFront(elem)
		return
	}

	// 如果容量已满，移除最久未使用的条目
	if c.evictList.Len() >= c.capacity {
		c.removeOldest()
	}

	// 添加新条目
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	entry := &cacheEntry{
		key:       key,
		value:     value,
		expiresAt: expiresAt,
	}

	elem := c.evictList.PushFront(entry)
	c.cache[key] = elem
}

// Delete 删除缓存值
func (c *LRUCache) Delete(ctx context.Context, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.removeElement(elem)
	}
}

// Len 返回缓存条目数量
func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// removeOldest 移除最久未使用的条目
func (c *LRUCache) removeOldest() {
	elem := c.evictList.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement 移除指定元素
func (c *LRUCache) removeElement(elem *list.Element) {
	c.evictList.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(c.cache, entry.key)
}

// Clear 清空缓存
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*list.Element)
	c.evictList.Init()
}
