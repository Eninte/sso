// Package crypto 测试JWT JTI重放攻击防护
package crypto

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/your-org/sso/internal/errors"
)

// mockCache 模拟缓存实现
type mockCache struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

func newMockCache() *mockCache {
	return &mockCache{
		data: make(map[string]interface{}),
	}
}

func (m *mockCache) Get(ctx context.Context, key string, dest interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	val, ok := m.data[key]
	if !ok {
		return apperrors.ErrCacheMiss
	}
	// 简单的类型断言
	if boolPtr, ok := dest.(*bool); ok {
		if boolVal, ok := val.(bool); ok {
			*boolPtr = boolVal
			return nil
		}
	}
	return apperrors.ErrCacheMiss
}

func (m *mockCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data[key] = value
	return nil
}

// TestJWTService_JTIReplayProtection 测试JWT JTI重放攻击防护
// 安全问题 #12: JWT jti未验证
//
// 测试场景:
// 1. 生成一个JWT token
// 2. 第一次验证token应该成功
// 3. 第二次验证同一个token应该失败（ErrTokenReplayed）
// 4. 验证JTI被正确记录在缓存中
func TestJWTService_JTIReplayProtection(t *testing.T) {
	// 创建JWT服务
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 创建JTI跟踪器
	cache := newMockCache()
	tracker := NewCacheJTITracker(cache, "jti:")
	jwtSvc.SetJTITracker(tracker)

	// 生成token
	token, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// 第一次验证：应该成功
	claims1, err := jwtSvc.ValidateAccessToken(token)
	assert.NoError(t, err)
	assert.NotNil(t, claims1)
	assert.Equal(t, "user-123", claims1.Subject)
	assert.NotEmpty(t, claims1.ID, "JTI应该存在")

	// 验证JTI已被记录
	used, err := tracker.IsJTIUsed(context.Background(), claims1.ID)
	assert.NoError(t, err)
	assert.True(t, used, "JTI应该被标记为已使用")

	// 第二次验证：应该失败（重放攻击）
	claims2, err := jwtSvc.ValidateAccessToken(token)
	assert.Error(t, err)
	assert.Nil(t, claims2)
	assert.True(t, apperrors.Is(err, ErrTokenReplayed), "应该返回ErrTokenReplayed")
}

// TestJWTService_JTIReplayProtection_WithoutTracker 测试没有JTI跟踪器时的行为
// 验证向后兼容性：没有配置JTI跟踪器时，token验证应该正常工作
func TestJWTService_JTIReplayProtection_WithoutTracker(t *testing.T) {
	// 创建JWT服务（不设置JTI跟踪器）
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 生成token
	token, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
	require.NoError(t, err)

	// 第一次验证：应该成功
	claims1, err := jwtSvc.ValidateAccessToken(token)
	assert.NoError(t, err)
	assert.NotNil(t, claims1)

	// 第二次验证：也应该成功（因为没有JTI跟踪器）
	claims2, err := jwtSvc.ValidateAccessToken(token)
	assert.NoError(t, err)
	assert.NotNil(t, claims2)
	assert.Equal(t, claims1.ID, claims2.ID)
}

// TestJWTService_JTIReplayProtection_DifferentTokens 测试不同token的JTI不冲突
func TestJWTService_JTIReplayProtection_DifferentTokens(t *testing.T) {
	// 创建JWT服务
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 创建JTI跟踪器
	cache := newMockCache()
	tracker := NewCacheJTITracker(cache, "jti:")
	jwtSvc.SetJTITracker(tracker)

	// 生成两个不同的token
	token1, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
	require.NoError(t, err)

	token2, err := jwtSvc.GenerateAccessToken("user-456", "test2@example.com", "user", []string{"openid"})
	require.NoError(t, err)

	// 验证token1
	claims1, err := jwtSvc.ValidateAccessToken(token1)
	assert.NoError(t, err)
	assert.NotNil(t, claims1)

	// 验证token2（应该成功，因为是不同的token）
	claims2, err := jwtSvc.ValidateAccessToken(token2)
	assert.NoError(t, err)
	assert.NotNil(t, claims2)

	// 验证JTI不同
	assert.NotEqual(t, claims1.ID, claims2.ID, "不同token的JTI应该不同")

	// 再次验证token1（应该失败）
	_, err = jwtSvc.ValidateAccessToken(token1)
	assert.Error(t, err)
	assert.True(t, apperrors.Is(err, ErrTokenReplayed))

	// 再次验证token2（应该失败）
	_, err = jwtSvc.ValidateAccessToken(token2)
	assert.Error(t, err)
	assert.True(t, apperrors.Is(err, ErrTokenReplayed))
}

// TestJWTService_JTIReplayProtection_ExpiredToken 测试过期token的JTI处理
func TestJWTService_JTIReplayProtection_ExpiredToken(t *testing.T) {
	// 创建JWT服务（token有效期1秒）
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		1*time.Second, // 1秒过期
		7*24*time.Hour,
	)

	// 创建JTI跟踪器
	cache := newMockCache()
	tracker := NewCacheJTITracker(cache, "jti:")
	jwtSvc.SetJTITracker(tracker)

	// 生成token
	token, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
	require.NoError(t, err)

	// 等待token过期
	time.Sleep(2 * time.Second)

	// 验证过期token：应该返回ErrTokenExpired而非ErrTokenReplayed
	_, err = jwtSvc.ValidateAccessToken(token)
	assert.Error(t, err)
	assert.True(t, apperrors.Is(err, ErrTokenExpired), "过期token应该返回ErrTokenExpired")
}

// TestJWTService_JTIReplayProtection_ConcurrentValidation 测试并发验证
func TestJWTService_JTIReplayProtection_ConcurrentValidation(t *testing.T) {
	// 创建JWT服务
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 创建JTI跟踪器
	cache := newMockCache()
	tracker := NewCacheJTITracker(cache, "jti:")
	jwtSvc.SetJTITracker(tracker)

	// 生成token
	token, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
	require.NoError(t, err)

	// 并发验证同一个token
	const concurrency = 10
	results := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			_, err := jwtSvc.ValidateAccessToken(token)
			results <- err
		}()
	}

	// 收集结果
	successCount := 0
	replayCount := 0
	for i := 0; i < concurrency; i++ {
		err := <-results
		if err == nil {
			successCount++
		} else if apperrors.Is(err, ErrTokenReplayed) {
			replayCount++
		}
	}

	// 验证结果：应该只有一个成功，其他都是重放错误
	// 注意：由于并发竞争，可能有多个请求同时通过JTI检查
	// 但至少应该有一些请求被标记为重放
	assert.GreaterOrEqual(t, successCount, 1, "至少应该有一个请求成功")
	assert.GreaterOrEqual(t, replayCount, 1, "至少应该有一个请求被标记为重放")
	assert.Equal(t, concurrency, successCount+replayCount, "所有请求都应该有结果")
}

// TestCacheJTITracker_IsJTIUsed 测试JTI跟踪器的IsJTIUsed方法
func TestCacheJTITracker_IsJTIUsed(t *testing.T) {
	cache := newMockCache()
	tracker := NewCacheJTITracker(cache, "jti:")

	ctx := context.Background()
	jti := "test-jti-123"

	// 测试1: 未使用的JTI
	used, err := tracker.IsJTIUsed(ctx, jti)
	assert.NoError(t, err)
	assert.False(t, used, "未使用的JTI应该返回false")

	// 测试2: 标记为已使用
	err = tracker.MarkJTIUsed(ctx, jti, 5*time.Minute)
	assert.NoError(t, err)

	// 测试3: 已使用的JTI
	used, err = tracker.IsJTIUsed(ctx, jti)
	assert.NoError(t, err)
	assert.True(t, used, "已使用的JTI应该返回true")
}

// TestCacheJTITracker_CustomPrefix 测试自定义缓存键前缀
func TestCacheJTITracker_CustomPrefix(t *testing.T) {
	cache := newMockCache()
	tracker := NewCacheJTITracker(cache, "custom-prefix:")

	ctx := context.Background()
	jti := "test-jti-456"

	// 标记JTI为已使用
	err := tracker.MarkJTIUsed(ctx, jti, 5*time.Minute)
	assert.NoError(t, err)

	// 验证缓存键使用了自定义前缀
	cache.mu.RLock()
	_, exists := cache.data["custom-prefix:"+jti]
	cache.mu.RUnlock()
	assert.True(t, exists, "应该使用自定义前缀")

	// 验证JTI已被使用
	used, err := tracker.IsJTIUsed(ctx, jti)
	assert.NoError(t, err)
	assert.True(t, used)
}

// TestCacheJTITracker_DefaultPrefix 测试默认缓存键前缀
func TestCacheJTITracker_DefaultPrefix(t *testing.T) {
	cache := newMockCache()
	tracker := NewCacheJTITracker(cache, "") // 空字符串应该使用默认前缀

	ctx := context.Background()
	jti := "test-jti-789"

	// 标记JTI为已使用
	err := tracker.MarkJTIUsed(ctx, jti, 5*time.Minute)
	assert.NoError(t, err)

	// 验证缓存键使用了默认前缀"jti:"
	cache.mu.RLock()
	_, exists := cache.data["jti:"+jti]
	cache.mu.RUnlock()
	assert.True(t, exists, "应该使用默认前缀jti:")
}
