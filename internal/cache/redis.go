// Package cache 缓存实现
// 提供Token和用户信息的缓存功能，减少数据库查询
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	apperrors "github.com/example/sso/internal/errors"
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
	Increment(ctx context.Context, key string) (int, error)
	SetTTL(ctx context.Context, key string, ttl time.Duration) error
	GetTTL(ctx context.Context, key string) (time.Duration, error)
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
	// MFAChallengePrefix MFA 两阶段登录 Challenge 缓存前缀
	// 存储一次性 challenge 令牌（32 字节 hex），TTL 由 config.MFAChallengeTTL 决定
	MFAChallengePrefix = "mfa:challenge:"

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

// UserTokenKey 生成用户维度 token 缓存键前缀
//
// 阶段 2.4：用于按 user_id 维度批量清理 token 缓存
// 替代旧的 DeletePattern("token:*")，避免影响其他用户
//
// 使用方式：
//
//	cache.DeletePattern(ctx, cache.UserTokenKey(userID) + "*")
func UserTokenKey(userID string) string {
	return TokenCachePrefix + "user:" + userID + ":"
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

// MFAChallengeKey 生成 MFA 两阶段登录 Challenge 缓存键
// token 为客户端提交的 mfa_challenge 令牌（32 字节 hex）
func MFAChallengeKey(token string) string {
	return MFAChallengePrefix + token
}

// ============================================================================
// 内存缓存实现
// ============================================================================

// MemoryCache 内存缓存实现
// 使用map存储缓存数据，支持并发安全和自动过期清理
type MemoryCache struct {
	data        map[string]cacheItem
	stopCh      chan struct{} // 用于停止清理goroutine
	onCacheHit  func()        // 缓存命中时的回调
	onCacheMiss func()        // 缓存未命中时的回调
	mu          sync.RWMutex
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

// WithMetrics 设置指标回调函数
func (c *MemoryCache) WithMetrics(onHit, onMiss func()) *MemoryCache {
	c.onCacheHit = onHit
	c.onCacheMiss = onMiss
	return c
}

// Get 获取缓存值
// 如果key不存在或已过期，返回ErrCacheMiss
func (c *MemoryCache) Get(ctx context.Context, key string, dest interface{}) error {
	c.mu.RLock()
	item, exists := c.data[key]
	c.mu.RUnlock()

	if !exists {
		if c.onCacheMiss != nil {
			c.onCacheMiss()
		}
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
		if c.onCacheMiss != nil {
			c.onCacheMiss()
		}
		return ErrCacheMiss
	}

	// 检查是否是空值缓存（缓存穿透防护）
	if len(item.value) == len(nilCacheValue) && string(item.value) == string(nilCacheValue) {
		if c.onCacheMiss != nil {
			c.onCacheMiss()
		}
		return ErrCacheMiss
	}

	if c.onCacheHit != nil {
		c.onCacheHit()
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

// SetWithNilProtection 设置缓存值（带空值保护）
// 如果value为nil，设置空值缓存（使用nilTTL），用于防止缓存穿透
// 如果value不为nil，正常设置缓存（使用ttl）
func (c *MemoryCache) SetWithNilProtection(ctx context.Context, key string, value interface{}, ttl time.Duration, nilTTL time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if value == nil {
		// 设置空值缓存，用于防止缓存穿透
		c.data[key] = cacheItem{
			value:     nilCacheValue,
			expiresAt: time.Now().Add(nilTTL),
		}
		return nil
	}

	// 正常设置缓存
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("序列化缓存值失败: %w", err)
	}

	c.data[key] = cacheItem{
		value:     data,
		expiresAt: time.Now().Add(ttl),
	}

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
// 优化：使用读锁遍历收集需要删除的key，再逐个加锁删除，避免长时间持有写锁
func (c *MemoryCache) DeletePattern(ctx context.Context, pattern string) error {
	// 第一步：使用读锁遍历收集需要删除的key
	c.mu.RLock()
	var keysToDelete []string
	for key := range c.data {
		if matchesPattern(key, pattern) {
			keysToDelete = append(keysToDelete, key)
		}
	}
	c.mu.RUnlock()

	// 第二步：逐个删除收集到的key
	for _, key := range keysToDelete {
		c.mu.Lock()
		delete(c.data, key)
		c.mu.Unlock()
	}
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

// Increment 原子递增计数器
// 返回递增后的值
func (c *MemoryCache) Increment(ctx context.Context, key string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.data[key]
	var count int

	if exists && time.Now().Before(item.expiresAt) {
		// 键存在且未过期，解析当前值
		if err := json.Unmarshal(item.value, &count); err != nil {
			// 如果解析失败，从1开始
			count = 0
		}
	}

	count++
	data, _ := json.Marshal(count)

	// 如果键不存在，设置默认过期时间（1小时）
	expiresAt := time.Now().Add(1 * time.Hour)
	if exists {
		expiresAt = item.expiresAt
	}

	c.data[key] = cacheItem{
		value:     data,
		expiresAt: expiresAt,
	}

	return count, nil
}

// SetTTL 设置键的过期时间
func (c *MemoryCache) SetTTL(ctx context.Context, key string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.data[key]
	if !exists {
		return fmt.Errorf("key not found")
	}

	item.expiresAt = time.Now().Add(ttl)
	c.data[key] = item
	return nil
}

// GetTTL 获取键的剩余过期时间
func (c *MemoryCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.data[key]
	if !exists {
		return 0, fmt.Errorf("key not found")
	}

	ttl := time.Until(item.expiresAt)
	if ttl < 0 {
		return 0, nil
	}
	return ttl, nil
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

// ============================================================================
// Redis缓存实现
// ============================================================================

var (
	ErrRedisConnectionFailed = apperrors.New("ERR_REDIS_CONNECTION_FAILED", "Redis连接失败", 500)
	ErrRedisPingFailed       = apperrors.New("ERR_REDIS_PING_FAILED", "Redis健康检查失败", 500)
)

// RedisCache Redis缓存实现
// 使用go-redis客户端，封装常用缓存操作
type RedisCache struct {
	client      *redis.Client
	onCacheHit  func() // 缓存命中时的回调
	onCacheMiss func() // 缓存未命中时的回调
}

// Client 获取底层的Redis客户端
// 用于需要直接访问Redis的场景（如分布式限流器）
func (c *RedisCache) Client() *redis.Client {
	return c.client
}

// NewRedisCache 创建Redis缓存实例
// host: Redis主机地址 (如 "localhost")
// password: Redis密码 (空字符串表示无需认证)
// db: Redis数据库编号 (0-15)
func NewRedisCache(host, port, password string, db int) (*RedisCache, error) {
	return NewRedisCacheWithPool(host, port, password, db, 10, 5)
}

// NewRedisCacheWithPool 创建Redis缓存实例（支持自定义连接池）
// host: Redis主机地址 (如 "localhost")
// password: Redis密码 (空字符串表示无需认证)
// db: Redis数据库编号 (0-15)
// poolSize: 连接池大小（<=0 时使用默认值 10）
// minIdleConns: 最小空闲连接数（<=0 时使用默认值 5）
func NewRedisCacheWithPool(host, port, password string, db, poolSize, minIdleConns int) (*RedisCache, error) {
	if poolSize <= 0 {
		poolSize = 10
	}
	if minIdleConns <= 0 {
		minIdleConns = 5
	}
	if minIdleConns > poolSize {
		minIdleConns = poolSize
	}

	client := redis.NewClient(&redis.Options{
		Addr:         host + ":" + port,
		Password:     password,
		DB:           db,
		PoolSize:     poolSize,
		MinIdleConns: minIdleConns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisCache{client: client}, nil
}

// WithMetrics 设置指标回调函数
func (c *RedisCache) WithMetrics(onHit, onMiss func()) *RedisCache {
	c.onCacheHit = onHit
	c.onCacheMiss = onMiss
	return c
}

// NewRedisCacheWithOptions 使用自定义选项创建Redis缓存实例
func NewRedisCacheWithOptions(opts *redis.Options) (*RedisCache, error) {
	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisCache{client: client}, nil
}

// Ping 检查Redis连接是否正常
func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Get 获取缓存值
func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	val, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			if c.onCacheMiss != nil {
				c.onCacheMiss()
			}
			return ErrCacheMiss
		}
		return err
	}

	if len(val) == len(nilCacheValue) && string(val) == string(nilCacheValue) {
		if c.onCacheMiss != nil {
			c.onCacheMiss()
		}
		return ErrCacheMiss
	}

	if c.onCacheHit != nil {
		c.onCacheHit()
	}
	return json.Unmarshal(val, dest)
}

// Set 设置缓存值
func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("序列化缓存值失败: %w", err)
	}

	return c.client.Set(ctx, key, data, ttl).Err()
}

// SetWithNilProtection 设置缓存值（带空值保护）
func (c *RedisCache) SetWithNilProtection(ctx context.Context, key string, value interface{}, ttl time.Duration, nilTTL time.Duration) error {
	if value == nil {
		return c.client.Set(ctx, key, nilCacheValue, nilTTL).Err()
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("序列化缓存值失败: %w", err)
	}

	return c.client.Set(ctx, key, data, ttl).Err()
}

// Delete 删除指定key的缓存
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

// DeletePattern 按模式删除缓存
// 使用SCAN命令代替KEYS，避免在大数据集上阻塞Redis
func (c *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	var cursor uint64
	var deletedCount int

	for {
		// 使用SCAN命令，每次扫描100个键
		keys, nextCursor, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("scan keys failed: %w", err)
		}

		// 批量删除扫描到的键
		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("delete keys failed: %w", err)
			}
			deletedCount += len(keys)
		}

		// 检查是否扫描完成
		cursor = nextCursor
		if cursor == 0 {
			break
		}

		// 避免过度占用Redis资源
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}

	_ = deletedCount // 可以用于日志记录
	return nil
}

// Close 关闭Redis连接
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// Increment 原子递增计数器
// 返回递增后的值
func (c *RedisCache) Increment(ctx context.Context, key string) (int, error) {
	val, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	return int(val), nil
}

// SetTTL 设置键的过期时间
func (c *RedisCache) SetTTL(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, key, ttl).Err()
}

// GetTTL 获取键的剩余过期时间
func (c *RedisCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := c.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if ttl < 0 {
		return 0, nil
	}
	return ttl, nil
}

// ============================================================================
// 缓存工厂函数
// ============================================================================

// Option 配置选项
type Option struct {
	RedisHost         string
	RedisPort         string
	RedisPassword     string
	RedisConnTimeout  time.Duration
	RedisDB           int
	RedisPoolSize     int
	RedisMinIdleConns int
	RedisEnable       bool
}

// NewCache 创建缓存实例
// 如果Redis可用则使用Redis，否则回退到内存缓存
func NewCache(opt *Option) (Cache, error) {
	if !opt.RedisEnable {
		return NewMemoryCache(), nil
	}

	redisCache, err := NewRedisCacheWithPool(opt.RedisHost, opt.RedisPort, opt.RedisPassword, opt.RedisDB, opt.RedisPoolSize, opt.RedisMinIdleConns)
	if err != nil {
		return nil, fmt.Errorf("create redis cache failed: %w", err)
	}

	return redisCache, nil
}

// NewCacheWithFallback 创建带降级功能的缓存实例
// Redis连接失败时自动使用内存缓存
func NewCacheWithFallback(opt *Option) (Cache, error) {
	if !opt.RedisEnable {
		slog.Info("using memory cache mode")
		return NewMemoryCache(), nil
	}

	redisCache, err := NewRedisCacheWithPool(opt.RedisHost, opt.RedisPort, opt.RedisPassword, opt.RedisDB, opt.RedisPoolSize, opt.RedisMinIdleConns)
	if err != nil {
		slog.Warn("redis connection failed, fallback to memory cache", "error", err)
		return NewMemoryCache(), nil
	}

	slog.Info("redis cache enabled")
	return redisCache, nil
}

// ============================================================================
// SingleflightCache 防缓存击穿包装
// ============================================================================

// SingleflightCache 使用singleflight防止缓存击穿
// 当多个并发请求同时请求同一个不存在的key时，只有一个请求会查询数据库
// 其他请求等待结果，避免缓存击穿
//
// 使用方式:
//
//	sf := cache.NewSingleflightCache(baseCache)
//	value, err := sf.Do(ctx, key, 5*time.Minute, func(ctx context.Context) (interface{}, error) {
//	    return store.GetByID(ctx, userID)
//	})
type SingleflightCache struct {
	cache Cache
	sf    singleflight.Group
}

// NewSingleflightCache 创建带防缓存击穿的缓存包装
func NewSingleflightCache(cache Cache) *SingleflightCache {
	return &SingleflightCache{cache: cache}
}

// Do 执行查询并缓存结果（防缓存击穿）
// 如果多个请求同时查询同一个key，只有一个会执行load函数
// 其他请求等待结果并复用
//
// 参数:
//   - ctx: 上下文
//   - key: 缓存键
//   - ttl: 缓存有效期
//   - load: 缓存未命中时的加载函数
//
// 返回:
//   - 查询结果
//   - 错误信息（如果load函数返回错误）
func (sf *SingleflightCache) Do(ctx context.Context, key string, ttl time.Duration, load func(context.Context) (interface{}, error)) (interface{}, error) {
	v, err, _ := sf.sf.Do(key, func() (interface{}, error) {
		// 再次检查缓存（可能已被其他请求设置）
		var dest interface{}
		if err := sf.cache.Get(ctx, key, &dest); err == nil {
			return dest, nil
		}

		// 缓存未命中，执行查询，将上下文传递给加载函数
		value, err := load(ctx)
		if err != nil {
			return nil, err
		}

		// 写入缓存
		_ = sf.cache.Set(ctx, key, value, ttl)
		return value, nil
	})
	return v, err
}
