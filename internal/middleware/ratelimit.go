// Package middleware 限流中间件
// 使用令牌桶算法实现API限流
package middleware

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ============================================================================
// 受信代理配置
// ============================================================================

// trustedProxyMu 保护受信代理全局状态的读写锁
var trustedProxyMu sync.RWMutex

// trustedProxies 受信代理IP列表
// 仅当请求来自这些IP时，才信任 X-Real-IP 头
var trustedProxies []net.IP

// trustedProxyNets 受信代理CIDR网段
var trustedProxyNets []*net.IPNet

// SetTrustedProxies 设置受信代理IP列表
// 支持单个IP（如 "10.0.0.1"）和CIDR格式（如 "172.16.0.0/12"）
// 空列表表示不信任任何代理（X-Real-IP 将被忽略）
func SetTrustedProxies(proxies []string) {
	trustedProxyMu.Lock()
	defer trustedProxyMu.Unlock()

	trustedProxies = nil
	trustedProxyNets = nil

	for _, p := range proxies {
		if p == "" {
			continue
		}
		// 尝试解析为CIDR
		if _, ipNet, err := net.ParseCIDR(p); err == nil {
			trustedProxyNets = append(trustedProxyNets, ipNet)
			continue
		}
		// 尝试解析为单个IP
		if ip := net.ParseIP(p); ip != nil {
			trustedProxies = append(trustedProxies, ip)
			continue
		}
		slog.Warn("无效的受信代理配置，已忽略", "proxy", p)
	}
}

// isTrustedProxy 检查IP是否为受信代理
func isTrustedProxy(remoteIP net.IP) bool {
	trustedProxyMu.RLock()
	defer trustedProxyMu.RUnlock()

	if remoteIP == nil {
		return false
	}
	for _, ip := range trustedProxies {
		if ip.Equal(remoteIP) {
			return true
		}
	}
	for _, ipNet := range trustedProxyNets {
		if ipNet.Contains(remoteIP) {
			return true
		}
	}
	return false
}

// ============================================================================
// 限流器接口
// ============================================================================

// RateLimitMiddleware 限流中间件接口
// 本地限流器和分布式限流器都实现此接口
type RateLimitMiddleware interface {
	// Middleware 返回限流HTTP中间件
	Middleware(next http.Handler) http.Handler
	// Stop 停止限流器（清理后台资源）
	Stop()
}

// ============================================================================
// RateLimiter 限流器（分片锁优化）
// ============================================================================

// numShards 分片数量（2的幂，便于位运算）
const numShards = 64

// maxClientsPerShard 每个分片的最大客户端数
// 总最大客户端数 = maxClientsPerShard * numShards = 10000 * 64 = 640,000
const maxClientsPerShard = 10000

// RateLimiter 限流器
// 使用令牌桶算法限制每个客户端的请求频率
// 优化：使用分片锁减少高并发下的锁竞争
type RateLimiter struct {
	shards     [numShards]*shard
	limit      int           // 每个时间窗口的请求数
	window     time.Duration // 时间窗口
	done       chan struct{} // 停止cleanup goroutine
	stopOnce   sync.Once     // 确保 Stop 只执行一次
	metricFunc func()        // 限流触发时的指标回调
}

// shard 分片
type shard struct {
	mu      sync.Mutex
	clients map[string]*clientInfo
}

// clientInfo 客户端信息
type clientInfo struct {
	tokens    int       // 当前令牌数
	lastReset time.Time // 上次重置时间
}

// NewRateLimiter 创建限流器
// limit: 每个时间窗口允许的最大请求数
// window: 时间窗口长度
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		limit:  limit,
		window: window,
		done:   make(chan struct{}),
	}
	for i := 0; i < numShards; i++ {
		rl.shards[i] = &shard{clients: make(map[string]*clientInfo)}
	}

	// 启动后台清理goroutine
	go rl.cleanup()

	return rl
}

// getShard 根据IP获取对应的分片
func (rl *RateLimiter) getShard(clientIP string) *shard {
	// 使用简单的哈希分片
	h := uint64(0)
	for i := 0; i < len(clientIP); i++ {
		h = h*31 + uint64(clientIP[i])
	}
	return rl.shards[h&(numShards-1)]
}

// WithMetrics 设置指标回调函数
// 当限流触发时会调用此函数
func (rl *RateLimiter) WithMetrics(metricFunc func()) *RateLimiter {
	rl.metricFunc = metricFunc
	return rl
}

// Stop 停止限流器的后台清理goroutine
// 应在服务关闭时调用，可安全地多次调用
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		close(rl.done)
	})
}

// ============================================================================
// 限流中间件
// ============================================================================

// writeError 写入JSON错误响应
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// Middleware 限流中间件
// 检查客户端请求频率，超过限制返回429
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 获取客户端标识
		clientIP := GetClientIP(r)

		// 检查是否超过限制
		if !rl.Allow(clientIP) {
			// 记录限流指标
			if rl.metricFunc != nil {
				rl.metricFunc()
			}
			w.Header().Set("Retry-After", strconv.Itoa(int(rl.window.Seconds())))
			writeError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			return
		}

		// 继续处理请求
		next.ServeHTTP(w, r)
	})
}

// GetClientIP 获取客户端真实IP
// 仅当请求来自受信代理时才信任 X-Real-IP 头
// 否则直接使用 RemoteAddr（最可靠的来源）
func GetClientIP(r *http.Request) string {
	// 解析 RemoteAddr（最可靠的来源）
	remoteAddr := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}

	remoteIP := net.ParseIP(remoteAddr)
	if remoteIP == nil {
		// 无法解析IP时直接返回原始值
		return r.RemoteAddr
	}

	// 仅当请求来自受信代理时，才信任 X-Real-IP 头
	if isTrustedProxy(remoteIP) {
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			if parsedIP := net.ParseIP(ip); parsedIP != nil {
				return ip
			}
		}
	}

	return remoteAddr
}

// Allow 检查是否允许请求
// 返回true表示允许，false表示超过限制
func (rl *RateLimiter) Allow(clientIP string) bool {
	// limit <= 0 表示禁用限流
	if rl.limit <= 0 {
		return true
	}

	shard := rl.getShard(clientIP)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	now := time.Now()
	client, exists := shard.clients[clientIP]

	if !exists {
		// 检查是否超过最大客户端数
		if len(shard.clients) >= maxClientsPerShard {
			// 尝试清理过期客户端
			rl.cleanupExpiredClients(shard, now)

			// 如果仍然超过限制，拒绝新客户端（防止内存耗尽）
			if len(shard.clients) >= maxClientsPerShard {
				return false
			}
		}

		// 新客户端，创建记录
		shard.clients[clientIP] = &clientInfo{
			tokens:    rl.limit - 1,
			lastReset: now,
		}
		return true
	}

	// 检查是否需要重置令牌桶
	if now.Sub(client.lastReset) >= rl.window {
		client.tokens = rl.limit - 1
		client.lastReset = now
		return true
	}

	// 检查是否有剩余令牌
	if client.tokens > 0 {
		client.tokens--
		return true
	}

	// 超过限制
	return false
}

// cleanupExpiredClients 清理过期的客户端记录（内部方法，调用者需持有锁）
func (rl *RateLimiter) cleanupExpiredClients(shard *shard, now time.Time) {
	for ip, client := range shard.clients {
		// 清理超过1个时间窗口未活动的客户端（更积极的清理策略）
		if now.Sub(client.lastReset) >= rl.window {
			delete(shard.clients, ip)
		}
	}
}

// cleanup 定期清理过期的客户端记录
// 防止内存泄漏
func (rl *RateLimiter) cleanup() {
	// 改为1倍时间窗口，更频繁地清理
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()

	for {
		select {
		case <-rl.done:
			return
		case <-ticker.C:
			now := time.Now()
			for i := 0; i < numShards; i++ {
				rl.shards[i].mu.Lock()

				// 检查是否超过最大客户端数
				if len(rl.shards[i].clients) > maxClientsPerShard {
					// 清理所有过期客户端（1个时间窗口）
					rl.cleanupExpiredClients(rl.shards[i], now)

					// 如果仍然超过限制，清理最旧的客户端
					if len(rl.shards[i].clients) > maxClientsPerShard {
						rl.evictOldestClients(rl.shards[i], maxClientsPerShard)
					}
				} else {
					// 正常清理：清理超过2个时间窗口未活动的客户端
					for ip, client := range rl.shards[i].clients {
						if now.Sub(client.lastReset) >= rl.window*2 {
							delete(rl.shards[i].clients, ip)
						}
					}
				}

				rl.shards[i].mu.Unlock()
			}
		}
	}
}

// evictOldestClients 驱逐最旧的客户端（内部方法，调用者需持有锁）
func (rl *RateLimiter) evictOldestClients(shard *shard, maxClients int) {
	// 按lastReset排序，删除最旧的客户端
	type clientEntry struct {
		ip        string
		lastReset time.Time
	}

	entries := make([]clientEntry, 0, len(shard.clients))
	for ip, client := range shard.clients {
		entries = append(entries, clientEntry{ip, client.lastReset})
	}

	// 按时间排序（最旧的在前）
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].lastReset.After(entries[j].lastReset) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// 删除最旧的客户端
	toDelete := len(entries) - maxClients
	for i := 0; i < toDelete; i++ {
		delete(shard.clients, entries[i].ip)
	}
}
