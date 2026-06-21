// Package captcha 验证码服务测试
package captcha

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/cache"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// newTestCache 创建测试用内存缓存
func newTestCache(t *testing.T) cache.Cache {
	t.Helper()
	c := cache.NewMemoryCache()
	t.Cleanup(func() { c.Close() })
	return c
}

// ============================================================================
// Service 生成测试
// ============================================================================

func TestService_Generate(t *testing.T) {
	cacheSvc := newTestCache(t)
	svc := NewService(cacheSvc, true, 5*time.Minute)

	c, err := svc.Generate(context.Background())
	require.NoError(t, err)

	assert.NotEmpty(t, c.ID, "captcha ID should not be empty")
	assert.Equal(t, TypeMath, c.Type, "default type should be math")
	assert.NotEmpty(t, c.Question, "question should not be empty")
	assert.Equal(t, 300, c.TTL, "TTL should be 300 seconds")
}

func TestService_Generate_Disabled(t *testing.T) {
	cacheSvc := newTestCache(t)
	svc := NewService(cacheSvc, false, 5*time.Minute)

	_, err := svc.Generate(context.Background())
	assert.Error(t, err, "should error when captcha is disabled")
}

func TestService_IsEnabled(t *testing.T) {
	cacheSvc := newTestCache(t)

	svcEnabled := NewService(cacheSvc, true, 5*time.Minute)
	assert.True(t, svcEnabled.IsEnabled())

	svcDisabled := NewService(cacheSvc, false, 5*time.Minute)
	assert.False(t, svcDisabled.IsEnabled())
}

// ============================================================================
// Service 验证测试
// ============================================================================

func TestService_Verify_Success(t *testing.T) {
	cacheSvc := newTestCache(t)
	svc := NewService(cacheSvc, true, 5*time.Minute)

	// 生成验证码
	c, err := svc.Generate(context.Background())
	require.NoError(t, err)

	// 从缓存中获取正确答案
	var data captchaData
	err = cacheSvc.Get(context.Background(), CaptchaCachePrefix+c.ID, &data)
	require.NoError(t, err)

	// 验证正确答案
	ok, err := svc.Verify(context.Background(), c.ID, data.Answer)
	assert.NoError(t, err)
	assert.True(t, ok, "correct answer should verify successfully")

	// 验证码应被删除（一次性使用）
	ok, err = svc.Verify(context.Background(), c.ID, data.Answer)
	assert.NoError(t, err)
	assert.False(t, ok, "used captcha should not verify again")
}

func TestService_Verify_WrongAnswer(t *testing.T) {
	cacheSvc := newTestCache(t)
	svc := NewService(cacheSvc, true, 5*time.Minute)

	c, err := svc.Generate(context.Background())
	require.NoError(t, err)

	// 验证错误答案
	ok, err := svc.Verify(context.Background(), c.ID, "wrong_answer")
	assert.NoError(t, err)
	assert.False(t, ok, "wrong answer should not verify")
}

func TestService_Verify_TrimsAnswer(t *testing.T) {
	cacheSvc := newTestCache(t)
	svc := NewService(cacheSvc, true, 5*time.Minute)

	c, err := svc.Generate(context.Background())
	require.NoError(t, err)

	var data captchaData
	err = cacheSvc.Get(context.Background(), CaptchaCachePrefix+c.ID, &data)
	require.NoError(t, err)

	// 带空格的答案也应该验证通过
	ok, err := svc.Verify(context.Background(), c.ID, " "+data.Answer+" ")
	assert.NoError(t, err)
	assert.True(t, ok, "answer with surrounding whitespace should verify")
}

func TestService_Verify_MaxAttempts(t *testing.T) {
	cacheSvc := newTestCache(t)
	svc := NewService(cacheSvc, true, 5*time.Minute)

	c, err := svc.Generate(context.Background())
	require.NoError(t, err)

	// 连续3次错误答案（达到最大尝试次数）
	for i := 0; i < MaxVerifyAttempts; i++ {
		ok, err := svc.Verify(context.Background(), c.ID, "wrong")
		assert.NoError(t, err)
		assert.False(t, ok)
	}

	// 获取正确答案 - 验证码应已被删除
	var data captchaData
	err = cacheSvc.Get(context.Background(), CaptchaCachePrefix+c.ID, &data)
	assert.Error(t, err, "captcha should be deleted after max attempts")
}

func TestService_Verify_EmptyInput(t *testing.T) {
	cacheSvc := newTestCache(t)
	svc := NewService(cacheSvc, true, 5*time.Minute)

	// 空ID
	ok, err := svc.Verify(context.Background(), "", "answer")
	assert.NoError(t, err)
	assert.False(t, ok)

	// 空答案
	ok, err = svc.Verify(context.Background(), "some-id", "")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestService_Verify_Disabled(t *testing.T) {
	cacheSvc := newTestCache(t)
	svc := NewService(cacheSvc, false, 5*time.Minute)

	// 禁用时应直接通过
	ok, err := svc.Verify(context.Background(), "", "")
	assert.NoError(t, err)
	assert.True(t, ok, "disabled captcha should always pass")
}

func TestService_Verify_NonexistentID(t *testing.T) {
	cacheSvc := newTestCache(t)
	svc := NewService(cacheSvc, true, 5*time.Minute)

	ok, err := svc.Verify(context.Background(), "nonexistent-id", "answer")
	assert.NoError(t, err)
	assert.False(t, ok, "nonexistent captcha should not verify")
}

// ============================================================================
// 数学题生成测试
// ============================================================================

func TestGenerateMathQuestion(t *testing.T) {
	// 多次生成，确保不panic且答案正确
	for i := 0; i < 100; i++ {
		question, answer, err := generateMathQuestion()
		require.NoError(t, err)
		assert.NotEmpty(t, question)
		assert.NotEmpty(t, answer)

		// 验证答案为数字字符串
		for _, ch := range answer {
			assert.True(t, ch >= '0' && ch <= '9', "answer should be numeric, got: %s", answer)
		}
	}
}

func TestGenerateMathQuestion_Operators(t *testing.T) {
	// 确保三种运算都能生成
	foundPlus, foundMinus, foundMul := false, false, false

	for i := 0; i < 300; i++ {
		question, _, err := generateMathQuestion()
		require.NoError(t, err)

		switch {
		case containsStr(question, "+"):
			foundPlus = true
		case containsStr(question, "-"):
			foundMinus = true
		case containsStr(question, "×"):
			foundMul = true
		}
	}

	assert.True(t, foundPlus, "should generate addition questions")
	assert.True(t, foundMinus, "should generate subtraction questions")
	assert.True(t, foundMul, "should generate multiplication questions")
}

// ============================================================================
// 辅助函数
// ============================================================================

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// 默认TTL测试
// ============================================================================

func TestNewService_DefaultTTL(t *testing.T) {
	cacheSvc := newTestCache(t)

	// TTL为0时应使用默认值
	svc := NewService(cacheSvc, true, 0)
	assert.Equal(t, DefaultCaptchaTTL, svc.ttl)
}

// ============================================================================
// randInt 测试
// ============================================================================

func TestRandInt(t *testing.T) {
	for i := 0; i < 100; i++ {
		n, err := randInt(1, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, n, 1)
		assert.LessOrEqual(t, n, 10)
	}
}

func TestRandInt_InvalidRange(t *testing.T) {
	_, err := randInt(10, 1)
	assert.Error(t, err)
}

func TestRandInt_SameMinMax(t *testing.T) {
	n, err := randInt(5, 5)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
}
