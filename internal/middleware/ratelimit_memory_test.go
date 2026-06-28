// Package middleware 限流器内存泄漏测试
package middleware

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRateLimiter_MaxClientsPerShard 测试每个分片的最大客户端数限制
func TestRateLimiter_MaxClientsPerShard(t *testing.T) {
	rl := NewRateLimiter(100, 1*time.Minute)
	defer rl.Stop()

	// 获取第一个分片
	shard := rl.shards[0]

	// 直接向分片添加客户端，超过最大限制
	// 注意：由于IP地址范围限制，我们需要生成足够多的唯一IP
	shard.mu.Lock()
	for i := 0; i < maxClientsPerShard+100; i++ {
		ip := fmt.Sprintf("192.168.%d.%d", i/256, i%256)
		shard.clients[ip] = &clientInfo{
			tokens:    100,
			lastReset: time.Now(),
		}
	}
	initialCount := len(shard.clients)
	shard.mu.Unlock()

	assert.Greater(t, initialCount, maxClientsPerShard, "应该超过最大客户端数")

	// 尝试添加新客户端（应该触发清理）
	// 使用一个会映射到同一分片的IP
	testIP := "test-ip-0"
	// 确保映射到分片0
	for i := 0; i < 1000; i++ {
		testIP = fmt.Sprintf("test-ip-%d", i)
		if rl.getShard(testIP) == shard {
			break
		}
	}

	// 第一次调用应该被拒绝（因为超过限制且所有客户端都是活跃的）
	allowed := rl.Allow(testIP)
	assert.False(t, allowed, "超过最大客户端数时应该拒绝新客户端")

	// 等待时间窗口过期
	time.Sleep(1*time.Minute + 100*time.Millisecond)

	// 现在应该可以添加新客户端（因为旧客户端已过期）
	allowed = rl.Allow(testIP)
	assert.True(t, allowed, "过期客户端清理后应该允许新客户端")

	// 检查分片大小是否在限制内
	shard.mu.Lock()
	finalCount := len(shard.clients)
	shard.mu.Unlock()

	assert.LessOrEqual(t, finalCount, maxClientsPerShard, "清理后应该在最大客户端数限制内")
}

// TestRateLimiter_CleanupExpiredClients 测试过期客户端清理
func TestRateLimiter_CleanupExpiredClients(t *testing.T) {
	rl := NewRateLimiter(100, 100*time.Millisecond)
	defer rl.Stop()

	// 添加一些客户端
	for i := 0; i < 10; i++ {
		ip := fmt.Sprintf("192.168.1.%d", i)
		rl.Allow(ip)
	}

	// 等待时间窗口过期
	time.Sleep(150 * time.Millisecond)

	// 触发清理（通过添加新客户端）
	rl.Allow("192.168.1.100")

	// 等待后台清理运行
	time.Sleep(150 * time.Millisecond)

	// 检查是否清理了过期客户端
	totalClients := 0
	for i := 0; i < numShards; i++ {
		rl.shards[i].mu.Lock()
		totalClients += len(rl.shards[i].clients)
		rl.shards[i].mu.Unlock()
	}

	// 应该只剩下最近添加的客户端
	assert.LessOrEqual(t, totalClients, 2, "过期客户端应该被清理")
}

// TestRateLimiter_EvictOldestClients 测试驱逐最旧客户端
func TestRateLimiter_EvictOldestClients(t *testing.T) {
	rl := NewRateLimiter(100, 1*time.Minute)
	defer rl.Stop()

	shard := rl.shards[0]

	// 添加客户端，时间戳递增
	now := time.Now()
	shard.mu.Lock()
	for i := 0; i < 100; i++ {
		ip := fmt.Sprintf("192.168.1.%d", i)
		shard.clients[ip] = &clientInfo{
			tokens:    100,
			lastReset: now.Add(time.Duration(i) * time.Second),
		}
	}
	shard.mu.Unlock()

	// 驱逐最旧的50个客户端
	shard.mu.Lock()
	rl.evictOldestClients(shard, 50)
	remainingCount := len(shard.clients)
	shard.mu.Unlock()

	assert.Equal(t, 50, remainingCount, "应该保留50个最新的客户端")

	// 验证保留的是最新的客户端
	shard.mu.Lock()
	for i := 0; i < 50; i++ {
		ip := fmt.Sprintf("192.168.1.%d", i)
		_, exists := shard.clients[ip]
		assert.False(t, exists, "最旧的客户端应该被删除")
	}
	for i := 50; i < 100; i++ {
		ip := fmt.Sprintf("192.168.1.%d", i)
		_, exists := shard.clients[ip]
		assert.True(t, exists, "最新的客户端应该被保留")
	}
	shard.mu.Unlock()
}

// TestRateLimiter_ConcurrentAccess 测试并发访问下的内存管理
func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(10, 100*time.Millisecond)
	defer rl.Stop()

	// 并发添加大量客户端
	var wg sync.WaitGroup
	numGoroutines := 100
	requestsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				ip := fmt.Sprintf("192.168.%d.%d", goroutineID, j)
				rl.Allow(ip)
			}
		}(i)
	}

	wg.Wait()

	// 检查总客户端数
	totalClients := 0
	for i := 0; i < numShards; i++ {
		rl.shards[i].mu.Lock()
		totalClients += len(rl.shards[i].clients)
		rl.shards[i].mu.Unlock()
	}

	// 应该不超过总最大限制
	maxTotalClients := maxClientsPerShard * numShards
	assert.LessOrEqual(t, totalClients, maxTotalClients, "总客户端数不应超过限制")

	// 等待清理
	time.Sleep(250 * time.Millisecond)

	// 再次检查
	totalClientsAfterCleanup := 0
	for i := 0; i < numShards; i++ {
		rl.shards[i].mu.Lock()
		totalClientsAfterCleanup += len(rl.shards[i].clients)
		rl.shards[i].mu.Unlock()
	}

	// 清理后应该减少
	assert.Less(t, totalClientsAfterCleanup, totalClients, "清理后客户端数应该减少")
}

// TestRateLimiter_MemoryBound 测试内存使用是否有界
func TestRateLimiter_MemoryBound(t *testing.T) {
	rl := NewRateLimiter(100, 50*time.Millisecond)
	defer rl.Stop()

	// 持续添加新客户端
	for round := 0; round < 5; round++ {
		for i := 0; i < 1000; i++ {
			ip := fmt.Sprintf("192.168.%d.%d", round, i)
			rl.Allow(ip)
		}

		// 等待清理
		time.Sleep(100 * time.Millisecond)
	}

	// 检查最终客户端数
	totalClients := 0
	for i := 0; i < numShards; i++ {
		rl.shards[i].mu.Lock()
		totalClients += len(rl.shards[i].clients)
		rl.shards[i].mu.Unlock()
	}

	// 应该远小于总添加的客户端数（5000）
	assert.Less(t, totalClients, 2000, "内存使用应该有界，不会无限增长")
}

// TestRateLimiter_CleanupFrequency 测试清理频率
func TestRateLimiter_CleanupFrequency(t *testing.T) {
	window := 100 * time.Millisecond
	rl := NewRateLimiter(100, window)
	defer rl.Stop()

	// 添加一些客户端
	for i := 0; i < 50; i++ {
		ip := fmt.Sprintf("192.168.1.%d", i)
		rl.Allow(ip)
	}

	initialCount := 0
	for i := 0; i < numShards; i++ {
		rl.shards[i].mu.Lock()
		initialCount += len(rl.shards[i].clients)
		rl.shards[i].mu.Unlock()
	}

	// 等待1个时间窗口（清理应该运行）
	time.Sleep(window + 50*time.Millisecond)

	// 检查是否清理了过期客户端
	countAfterOneWindow := 0
	for i := 0; i < numShards; i++ {
		rl.shards[i].mu.Lock()
		countAfterOneWindow += len(rl.shards[i].clients)
		rl.shards[i].mu.Unlock()
	}

	// 等待2个时间窗口（应该清理更多）
	time.Sleep(window)

	countAfterTwoWindows := 0
	for i := 0; i < numShards; i++ {
		rl.shards[i].mu.Lock()
		countAfterTwoWindows += len(rl.shards[i].clients)
		rl.shards[i].mu.Unlock()
	}

	assert.LessOrEqual(t, countAfterTwoWindows, countAfterOneWindow, "随着时间推移，客户端数应该减少")
	assert.LessOrEqual(t, countAfterTwoWindows, initialCount, "最终客户端数应该少于初始数")
}
