// Package service TOTP 验证逻辑测试
package service

import (
	"context"
	"encoding/base32"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/example/sso/internal/store/mock"
)

// TestFindMatchingTimeStep_ValidCode 测试当前时间窗口的有效 TOTP 码
func TestFindMatchingTimeStep_ValidCode(t *testing.T) {
	// 构造一个 base32 编码的密钥（与 findMatchingTimeStep 内部解码流程一致）
	rawSecret := []byte("JBSWY3DPEHPK3PXP")
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawSecret)

	// 生成当前时间窗口的有效码（基于原始字节）
	baseTimeStep := uint64(time.Now().Unix() / 30)
	validCode := generateHOTP(rawSecret, baseTimeStep)

	t.Run("当前时间窗口_验证通过", func(t *testing.T) {
		ts, ok := findMatchingTimeStep(secret, validCode)
		assert.True(t, ok)
		assert.Equal(t, baseTimeStep, ts)
	})
}

// TestFindMatchingTimeStep_AdjacentWindow 测试相邻时间窗口（-1, +1）容差
func TestFindMatchingTimeStep_AdjacentWindow(t *testing.T) {
	rawSecret := []byte("test-secret-key-1234")
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawSecret)

	baseTimeStep := uint64(time.Now().Unix() / 30)

	t.Run("前一时间窗口_验证通过", func(t *testing.T) {
		if baseTimeStep == 0 {
			t.Skip("时间步为0，无法测试前驱窗口")
		}
		code := generateHOTP(rawSecret, baseTimeStep-1)
		ts, ok := findMatchingTimeStep(secret, code)
		assert.True(t, ok)
		assert.Equal(t, baseTimeStep-1, ts)
	})

	t.Run("后一时间窗口_验证通过", func(t *testing.T) {
		code := generateHOTP(rawSecret, baseTimeStep+1)
		ts, ok := findMatchingTimeStep(secret, code)
		assert.True(t, ok)
		assert.Equal(t, baseTimeStep+1, ts)
	})
}

// TestFindMatchingTimeStep_InvalidInputs 测试无效输入
func TestFindMatchingTimeStep_InvalidInputs(t *testing.T) {
	rawSecret := []byte("JBSWY3DPEHPK3PXP")
	validSecret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawSecret)

	t.Run("无效base32密钥_返回false", func(t *testing.T) {
		// 包含非 base32 字符
		_, ok := findMatchingTimeStep("!!!invalid!!!", "123456")
		assert.False(t, ok)
	})

	t.Run("空密钥_返回false", func(t *testing.T) {
		_, ok := findMatchingTimeStep("", "123456")
		assert.False(t, ok)
	})

	t.Run("错误验证码_返回false", func(t *testing.T) {
		_, ok := findMatchingTimeStep(validSecret, "000000")
		assert.False(t, ok)
	})

	t.Run("空验证码_返回false", func(t *testing.T) {
		_, ok := findMatchingTimeStep(validSecret, "")
		assert.False(t, ok)
	})
}

// TestFindMatchingTimeStep_SecretNormalization 测试密钥大小写/空格归一化
func TestFindMatchingTimeStep_SecretNormalization(t *testing.T) {
	// 用 base32 编码一个密钥，findMatchingTimeStep 内部会解码
	rawSecret := []byte("test-secret-key")
	encodedSecret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawSecret)

	// 小写 + 前后空格，应被 ToUpper/TrimSpace 归一化后等效
	secretLower := " " + strings.ToLower(encodedSecret) + " "

	baseTimeStep := uint64(time.Now().Unix() / 30)
	validCode := generateHOTP(rawSecret, baseTimeStep)

	_, ok := findMatchingTimeStep(secretLower, validCode)
	assert.True(t, ok, "小写+空格的密钥应被归一化后验证通过")
}

// TestFindMatchingTimeStep_FarFutureCode 测试超出容差窗口的码应失败
func TestFindMatchingTimeStep_FarFutureCode(t *testing.T) {
	rawSecret := []byte("JBSWY3DPEHPK3PXP")
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawSecret)

	baseTimeStep := uint64(time.Now().Unix() / 30)
	// 偏移 +5 个时间步（150秒后），超出 ±1 窗口容差
	farCode := generateHOTP(rawSecret, baseTimeStep+5)

	_, ok := findMatchingTimeStep(secret, farCode)
	assert.False(t, ok, "超出容差窗口的验证码应失败")
}

// TestValidateTOTPWithReplayProtection_ReplayAttack 测试重放保护
// 同一 (userID, code, timeStep) 第二次验证应失败
func TestValidateTOTPWithReplayProtection_ReplayAttack(t *testing.T) {
	store := mock.New()
	svc := NewMFAService(store)
	defer svc.Close()

	rawSecret := []byte("JBSWY3DPEHPK3PXP")
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawSecret)
	userID := "user-replay-test"

	baseTimeStep := uint64(time.Now().Unix() / 30)
	validCode := generateHOTP(rawSecret, baseTimeStep)

	t.Run("首次验证_通过", func(t *testing.T) {
		assert.True(t, svc.validateTOTPWithReplayProtection(context.Background(), userID, secret, validCode))
	})

	t.Run("同一码重复使用_拒绝", func(t *testing.T) {
		// 同一时间步的同一码应被拒绝（重放保护）
		assert.False(t, svc.validateTOTPWithReplayProtection(context.Background(), userID, secret, validCode))
	})
}

// TestValidateTOTPWithReplayProtection_InvalidCode 测试无效码不触发重放记录
func TestValidateTOTPWithReplayProtection_InvalidCode(t *testing.T) {
	store := mock.New()
	svc := NewMFAService(store)
	defer svc.Close()

	rawSecret := []byte("JBSWY3DPEHPK3PXP")
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawSecret)
	userID := "user-invalid-test"

	t.Run("无效码_返回false", func(t *testing.T) {
		assert.False(t, svc.validateTOTPWithReplayProtection(context.Background(), userID, secret, "000000"))
	})

	t.Run("无效码后仍可验证有效码", func(t *testing.T) {
		baseTimeStep := uint64(time.Now().Unix() / 30)
		validCode := generateHOTP(rawSecret, baseTimeStep)
		assert.True(t, svc.validateTOTPWithReplayProtection(context.Background(), userID, secret, validCode))
	})
}
