// Package middleware 限流中间件
// 使用令牌桶算法实现API限流
package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

// ============================================================================
// RateLimiter 限流器（分片锁优化）
// ============================================================================

// numShards 分片数量（2的幂，便于位运算）
const numShards = 64

// RateLimiter 限流器
// 使用令牌桶算法限制每个客户端的请求频率
// 优化：使用分片锁减少高并发下的锁竞争
type RateLimiter struct {
	shards     [numShards]*shard
	limit      int           // 每个时间窗口的请求数
	window     time.Duration // 时间窗口
	done       chan struct{} // 停止cleanup goroutine
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
// 应在服务关闭时调用
func (rl *RateLimiter) Stop() {
	close(rl.done)
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
		clientIP := getClientIP(r)

		// 检查是否超过限制
		if !rl.Allow(clientIP) {
			// 记录限流指标
			if rl.metricFunc != nil {
				rl.metricFunc()
			}
			w.Header().Set("Retry-After", rl.window.String())
			writeError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			return
		}

		// 继续处理请求
		next.ServeHTTP(w, r)
	})
}

// getClientIP 获取客户端真实IP
// 优先使用 X-Real-IP，其次 RemoteAddr
// 不信任 X-Forwarded-For (可被伪造)
func getClientIP(r *http.Request) string {
	// 优先使用 X-Real-IP (通常由反向代理设置)
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		if parsedIP := net.ParseIP(ip); parsedIP != nil {
			return ip
		}
	}

	// 使用 RemoteAddr (最可靠)
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return ip
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

// cleanup 定期清理过期的客户端记录
// 防止内存泄漏
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window * 2)
	defer ticker.Stop()

	for {
		select {
		case <-rl.done:
			return
		case <-ticker.C:
			now := time.Now()
			for i := 0; i < numShards; i++ {
				rl.shards[i].mu.Lock()
				for ip, client := range rl.shards[i].clients {
					// 清理超过2个时间窗口未活动的客户端
					if now.Sub(client.lastReset) >= rl.window*2 {
						delete(rl.shards[i].clients, ip)
					}
				}
				rl.shards[i].mu.Unlock()
			}
		}
	}
}
