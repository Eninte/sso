// Package captcha 验证码服务测试
package captcha

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
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

// ============================================================================
// Verify 剩余 TTL 测试（修复：错误猜测不延长验证码生命周期）
// ============================================================================

func TestService_Verify_RemainingTTL(t *testing.T) {
	// 使用短 TTL 验证错误猜测不会重置为完整 TTL
	cacheSvc := newTestCache(t)
	svc := NewService(cacheSvc, true, 2*time.Second)
	ctx := context.Background()

	t.Run("错误猜测不延长验证码生命周期", func(t *testing.T) {
		c, err := svc.Generate(ctx)
		require.NoError(t, err)

		// 获取正确答案
		var data captchaData
		err = cacheSvc.Get(ctx, CaptchaCachePrefix+c.ID, &data)
		require.NoError(t, err)

		// 立即提交错误答案（TTL 几乎未消耗）
		ok, err := svc.Verify(ctx, c.ID, "wrong")
		assert.NoError(t, err)
		assert.False(t, ok)

		// 等待超过原始 TTL（2秒）后，验证码应已过期
		// 修复前：错误猜测会重置 TTL 为完整 2 秒，此时验证码仍存在
		// 修复后：错误猜测使用剩余 TTL，验证码应在约 2 秒后过期
		time.Sleep(2200 * time.Millisecond)

		ok, err = svc.Verify(ctx, c.ID, data.Answer)
		assert.NoError(t, err)
		assert.False(t, ok, "验证码应在原始 TTL 后过期，不被错误猜测延长")
	})

	t.Run("错误猜测后正确答案在剩余 TTL 内仍可验证", func(t *testing.T) {
		c, err := svc.Generate(ctx)
		require.NoError(t, err)

		var data captchaData
		err = cacheSvc.Get(ctx, CaptchaCachePrefix+c.ID, &data)
		require.NoError(t, err)

		// 提交一次错误答案
		ok, err := svc.Verify(ctx, c.ID, "wrong")
		assert.NoError(t, err)
		assert.False(t, ok)

		// 在剩余 TTL 内用正确答案仍可通过
		ok, err = svc.Verify(ctx, c.ID, data.Answer)
		assert.NoError(t, err)
		assert.True(t, ok, "剩余 TTL 内正确答案应验证通过")
	})

	t.Run("接近过期时错误猜测导致立即过期", func(t *testing.T) {
		c, err := svc.Generate(ctx)
		require.NoError(t, err)

		// 等待接近 TTL 过期
		time.Sleep(1800 * time.Millisecond)

		// 此时提交错误答案，剩余 TTL 很短
		ok, err := svc.Verify(ctx, c.ID, "wrong")
		assert.NoError(t, err)
		assert.False(t, ok)

		// 再等待很短时间，验证码应已过期
		time.Sleep(300 * time.Millisecond)

		var data captchaData
		err = cacheSvc.Get(ctx, CaptchaCachePrefix+c.ID, &data)
		assert.Error(t, err, "接近过期时错误猜测后验证码应很快过期")
	})
}

// ============================================================================
// RecordFailure 滑动窗口行为测试（修复：注释与实现一致性验证）
// ============================================================================

func TestService_RecordFailure_SlidingWindow(t *testing.T) {
	cacheSvc := newTestCache(t)
	svc := NewServiceWithAdaptive(cacheSvc, true, 5*time.Minute, 3, 15*time.Minute)
	ctx := context.Background()

	t.Run("每次失败都重置 TTL（滑动窗口语义）", func(t *testing.T) {
		ip := "192.168.1.1"

		// 第1次失败
		svc.RecordFailure(ctx, ip)
		assert.False(t, svc.ShouldRequireCaptcha(ctx, ip))

		// 第2次失败 - TTL 被重置为完整 failWindow
		svc.RecordFailure(ctx, ip)
		assert.False(t, svc.ShouldRequireCaptcha(ctx, ip))

		// 第3次失败 - 达到阈值
		svc.RecordFailure(ctx, ip)
		assert.True(t, svc.ShouldRequireCaptcha(ctx, ip))
	})

	t.Run("不同标识的失败计数相互独立", func(t *testing.T) {
		ipA := "10.0.0.1"
		ipB := "10.0.0.2"

		// IP-A 失败3次
		for i := 0; i < 3; i++ {
			svc.RecordFailure(ctx, ipA)
		}
		assert.True(t, svc.ShouldRequireCaptcha(ctx, ipA))

		// IP-B 无记录
		assert.False(t, svc.ShouldRequireCaptcha(ctx, ipB))

		// 清除 IP-A 不影响 IP-B
		svc.ClearFailures(ctx, ipA)
		assert.False(t, svc.ShouldRequireCaptcha(ctx, ipA))
		assert.False(t, svc.ShouldRequireCaptcha(ctx, ipB))
	})

	t.Run("禁用时 RecordFailure 不生效", func(t *testing.T) {
		disabledCache := newTestCache(t)
		disabledSvc := NewServiceWithAdaptive(disabledCache, false, 5*time.Minute, 3, 15*time.Minute)

		for i := 0; i < 10; i++ {
			disabledSvc.RecordFailure(ctx, "1.2.3.4")
		}
		assert.False(t, disabledSvc.ShouldRequireCaptcha(ctx, "1.2.3.4"))
	})

	t.Run("ClearFailures 后计数从零开始", func(t *testing.T) {
		ip := "172.16.0.1"

		// 失败3次达到阈值
		for i := 0; i < 3; i++ {
			svc.RecordFailure(ctx, ip)
		}
		assert.True(t, svc.ShouldRequireCaptcha(ctx, ip))

		// 清除后重新计数
		svc.ClearFailures(ctx, ip)
		assert.False(t, svc.ShouldRequireCaptcha(ctx, ip))

		// 再次失败2次，仍未达阈值
		svc.RecordFailure(ctx, ip)
		svc.RecordFailure(ctx, ip)
		assert.False(t, svc.ShouldRequireCaptcha(ctx, ip))

		// 第3次失败达到阈值
		svc.RecordFailure(ctx, ip)
		assert.True(t, svc.ShouldRequireCaptcha(ctx, ip))
	})
}

// ============================================================================
// FailThreshold 测试
// ============================================================================

func TestService_FailThreshold(t *testing.T) {
	cacheSvc := cache.NewMemoryCache()
	defer cacheSvc.Close()

	t.Run("默认构造_返回DefaultFailThreshold", func(t *testing.T) {
		svc := NewService(cacheSvc, true, 5*time.Minute)
		assert.Equal(t, DefaultFailThreshold, svc.FailThreshold())
	})

	t.Run("自适应构造_返回自定义阈值", func(t *testing.T) {
		svc := NewServiceWithAdaptive(cacheSvc, true, 5*time.Minute, 5, 15*time.Minute)
		assert.Equal(t, 5, svc.FailThreshold())
	})

	t.Run("自适应构造_阈值0回退默认值", func(t *testing.T) {
		// failThreshold <= 0 时不覆盖默认值
		svc := NewServiceWithAdaptive(cacheSvc, true, 5*time.Minute, 0, 15*time.Minute)
		assert.Equal(t, DefaultFailThreshold, svc.FailThreshold())
	})

	t.Run("自适应构造_负数阈值回退默认值", func(t *testing.T) {
		svc := NewServiceWithAdaptive(cacheSvc, true, 5*time.Minute, -1, 15*time.Minute)
		assert.Equal(t, DefaultFailThreshold, svc.FailThreshold())
	})

	t.Run("阈值与ShouldRequireCaptcha联动", func(t *testing.T) {
		// 阈值=2：失败1次不应触发，2次应触发
		svc := NewServiceWithAdaptive(cacheSvc, true, 5*time.Minute, 2, 15*time.Minute)
		assert.Equal(t, 2, svc.FailThreshold())

		ctx := context.Background()
		ip := "192.168.1.1"

		svc.RecordFailure(ctx, ip)
		assert.False(t, svc.ShouldRequireCaptcha(ctx, ip), "失败1次未达阈值2")

		svc.RecordFailure(ctx, ip)
		assert.True(t, svc.ShouldRequireCaptcha(ctx, ip), "失败2次达到阈值2")
	})
}
