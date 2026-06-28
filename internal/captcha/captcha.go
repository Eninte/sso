// Package captcha 验证码服务
// 提供数学运算验证码的生成和验证功能，用于防止自动化攻击
package captcha

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/example/sso/internal/cache"
)

// ============================================================================
// 常量定义
// ============================================================================

const (
	// 缓存键前缀
	CaptchaCachePrefix     = "captcha:"
	CaptchaFailCountPrefix = "captcha:fail:" // 失败计数前缀

	// 默认验证码TTL
	DefaultCaptchaTTL = 5 * time.Minute

	// 验证码最大验证次数
	MaxVerifyAttempts = 3

	// 默认触发阈值（连续失败N次后要求验证码）
	DefaultFailThreshold = 3

	// 默认失败计数窗口
	DefaultFailWindow = 15 * time.Minute
)

// ============================================================================
// 验证码类型
// ============================================================================

// Type 验证码类型
type Type string

const (
	TypeMath Type = "math" // 数学运算
)

// ============================================================================
// 验证码数据结构
// ============================================================================

// Captcha 验证码数据
type Captcha struct {
	ID       string `json:"id"`       // 验证码唯一ID
	Type     Type   `json:"type"`     // 验证码类型
	Question string `json:"question"` // 验证码问题（如 "3 + 5 = ?"）
	TTL      int    `json:"ttl"`      // 有效期（秒）
}

// captchaData 存储在缓存中的验证码数据
type captchaData struct {
	Answer    string `json:"answer"`     // 正确答案
	Attempts  int    `json:"attempts"`   // 已验证次数
	Type      Type   `json:"type"`       // 验证码类型
	CreatedAt int64  `json:"created_at"` // 创建时间戳
}

// ============================================================================
// Service 验证码服务
// ============================================================================

// Service 验证码服务
type Service struct {
	cache         cache.Cache
	ttl           time.Duration
	enabled       bool
	failThreshold int           // 连续失败N次后触发验证码
	failWindow    time.Duration // 失败计数窗口
}

// NewService 创建验证码服务
func NewService(cacheSvc cache.Cache, enabled bool, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = DefaultCaptchaTTL
	}
	return &Service{
		cache:         cacheSvc,
		ttl:           ttl,
		enabled:       enabled,
		failThreshold: DefaultFailThreshold,
		failWindow:    DefaultFailWindow,
	}
}

// NewServiceWithAdaptive 创建带自适应触发配置的验证码服务
func NewServiceWithAdaptive(cacheSvc cache.Cache, enabled bool, ttl time.Duration, failThreshold int, failWindow time.Duration) *Service {
	svc := NewService(cacheSvc, enabled, ttl)
	if failThreshold > 0 {
		svc.failThreshold = failThreshold
	}
	if failWindow > 0 {
		svc.failWindow = failWindow
	}
	return svc
}

// IsEnabled 检查验证码是否启用
func (s *Service) IsEnabled() bool {
	return s.enabled
}

// Generate 生成验证码
func (s *Service) Generate(ctx context.Context) (*Captcha, error) {
	if !s.enabled {
		return nil, fmt.Errorf("captcha service is disabled")
	}

	// 生成唯一ID
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generate captcha id: %w", err)
	}

	// 生成数学题
	question, answer, err := generateMathQuestion()
	if err != nil {
		return nil, fmt.Errorf("generate math question: %w", err)
	}

	// 存储到缓存
	data := &captchaData{
		Answer:    answer,
		Attempts:  0,
		Type:      TypeMath,
		CreatedAt: time.Now().Unix(),
	}

	cacheKey := CaptchaCachePrefix + id
	if err := s.cache.Set(ctx, cacheKey, data, s.ttl); err != nil {
		return nil, fmt.Errorf("store captcha data: %w", err)
	}

	return &Captcha{
		ID:       id,
		Type:     TypeMath,
		Question: question,
		TTL:      int(s.ttl.Seconds()),
	}, nil
}

// Verify 验证验证码
// 返回 (是否验证成功, 错误)
// 验证成功或超过最大尝试次数后，验证码将被删除
func (s *Service) Verify(ctx context.Context, id, answer string) (bool, error) {
	if !s.enabled {
		return true, nil // 未启用时直接通过
	}

	if id == "" || answer == "" {
		return false, nil
	}

	// 去除答案首尾空白，避免用户输入 " 8" 导致验证失败
	answer = strings.TrimSpace(answer)

	cacheKey := CaptchaCachePrefix + id

	// 从缓存获取验证码数据
	var data captchaData
	if err := s.cache.Get(ctx, cacheKey, &data); err != nil {
		// 缓存未命中 = 验证码不存在或已过期
		return false, nil
	}

	// 检查尝试次数
	data.Attempts++
	// 防御性检查：正常流程下 Attempts 不会超过 MaxVerifyAttempts
	// （达到阈值时下方 >= MaxVerifyAttempts 分支会删除验证码），
	// 但若 cache.Delete 失败，验证码残留缓存中，此检查作为安全网防止无限尝试
	if data.Attempts > MaxVerifyAttempts {
		_ = s.cache.Delete(ctx, cacheKey)
		return false, nil
	}

	// 验证答案
	if data.Answer == answer {
		// 验证成功，删除验证码（一次性使用）
		_ = s.cache.Delete(ctx, cacheKey)
		return true, nil
	}

	// 验证失败，更新尝试次数
	if data.Attempts >= MaxVerifyAttempts {
		// 已达最大尝试次数，删除验证码
		_ = s.cache.Delete(ctx, cacheKey)
	} else {
		// 更新缓存中的尝试次数，使用剩余TTL避免错误猜测延长验证码生命周期
		remainingTTL := s.ttl - time.Since(time.Unix(data.CreatedAt, 0))
		if remainingTTL <= 0 {
			// 已过期，删除验证码
			_ = s.cache.Delete(ctx, cacheKey)
			return false, nil
		}
		_ = s.cache.Set(ctx, cacheKey, &data, remainingTTL)
	}

	return false, nil
}

// ============================================================================
// 自适应触发：基于失败次数决定是否需要验证码
// ============================================================================

// ShouldRequireCaptcha 判断指定标识（如IP）是否需要验证码
// 当该标识在失败窗口内的失败次数达到阈值时返回 true
func (s *Service) ShouldRequireCaptcha(ctx context.Context, key string) bool {
	if !s.enabled {
		return false
	}

	cacheKey := CaptchaFailCountPrefix + key
	var count int
	if err := s.cache.Get(ctx, cacheKey, &count); err != nil {
		return false // 无记录 = 不需要
	}

	return count >= s.failThreshold
}

// RecordFailure 记录一次失败（按标识，如IP）
// 每次失败都递增计数并重置TTL为完整的failWindow，实现滑动窗口语义：
// 持续失败的标识其计数器永不过期，直到停止失败超过failWindow后自动清除。
//
// 注意：此方法使用 Get→Set 两步操作，非原子递增。
// 在极端并发场景下（同一IP同时大量请求），可能丢失少量计数。
// 这在防机器人场景下可接受——攻击者无法利用此窗口降低触发阈值。
// 若后续需要严格原子递增，需扩展 cache.Cache 接口添加 Increment 方法。
func (s *Service) RecordFailure(ctx context.Context, key string) {
	if !s.enabled {
		return
	}

	cacheKey := CaptchaFailCountPrefix + key
	var count int
	if err := s.cache.Get(ctx, cacheKey, &count); err != nil {
		// 首次失败
		count = 1
	} else {
		count++
	}

	_ = s.cache.Set(ctx, cacheKey, count, s.failWindow)
}

// ClearFailures 清除指定标识的失败计数（登录成功后调用）
func (s *Service) ClearFailures(ctx context.Context, key string) {
	if !s.enabled {
		return
	}

	cacheKey := CaptchaFailCountPrefix + key
	_ = s.cache.Delete(ctx, cacheKey)
}

// FailThreshold 返回当前触发阈值
func (s *Service) FailThreshold() int {
	return s.failThreshold
}

// ============================================================================
// 数学题生成
// ============================================================================

// generateMathQuestion 生成数学运算题
// 返回问题文本、答案字符串、错误
func generateMathQuestion() (string, string, error) {
	// 随机选择运算类型: 0=加法, 1=减法, 2=乘法
	opType, err := rand.Int(rand.Reader, big.NewInt(3))
	if err != nil {
		return "", "", err
	}

	var a, b, result int
	var opStr string

	switch opType.Int64() {
	case 0: // 加法
		a, err = randInt(1, 50)
		if err != nil {
			return "", "", err
		}
		b, err = randInt(1, 49)
		if err != nil {
			return "", "", err
		}
		result = a + b
		opStr = "+"
	case 1: // 减法（确保结果非负）
		a, err = randInt(10, 99)
		if err != nil {
			return "", "", err
		}
		b, err = randInt(1, a)
		if err != nil {
			return "", "", err
		}
		result = a - b
		opStr = "-"
	case 2: // 乘法（小数字）
		a, err = randInt(2, 9)
		if err != nil {
			return "", "", err
		}
		b, err = randInt(2, 9)
		if err != nil {
			return "", "", err
		}
		result = a * b
		opStr = "×"
	}

	question := fmt.Sprintf("%d %s %d = ?", a, opStr, b)
	answer := fmt.Sprintf("%d", result)

	return question, answer, nil
}

// ============================================================================
// 辅助函数
// ============================================================================

// generateID 生成验证码唯一ID
func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// randInt 生成 [minVal, maxVal] 范围内的随机整数
func randInt(minVal, maxVal int) (int, error) {
	if minVal > maxVal {
		return 0, fmt.Errorf("invalid range: min(%d) > max(%d)", minVal, maxVal)
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(maxVal-minVal+1)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()) + minVal, nil
}
