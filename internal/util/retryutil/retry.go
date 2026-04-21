// Package retryutil 提供通用的重试机制
// 包含指数退避算法、抖动计算等可重用逻辑
package retryutil

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"time"
)

// ============================================================================
// 类型定义
// ============================================================================

// RetryConfig 重试配置
// 定义重试行为的各项参数
type RetryConfig struct {
	// MaxRetries 最大重试次数（默认3）
	MaxRetries int

	// BaseDelay 基础延迟时间（默认100ms）
	BaseDelay time.Duration

	// MaxDelay 最大延迟上限（默认5s）
	MaxDelay time.Duration

	// JitterFactor 抖动因子（默认0.25，即25%）
	// 抖动范围为延迟时间的0%到JitterFactor%
	JitterFactor float64
}

// RetryableFunc 可重试的函数类型
// 函数接收context.Context参数，返回error
// 如果返回nil表示操作成功，否则表示操作失败需要重试
type RetryableFunc func(ctx context.Context) error

// ============================================================================
// 默认配置
// ============================================================================

// DefaultRetryConfig 返回默认重试配置
// 默认值：
//   - MaxRetries: 3次
//   - BaseDelay: 100毫秒
//   - MaxDelay: 5秒
//   - JitterFactor: 0.25（25%）
//
// 这些默认值满足需求1.3、1.4、1.5的要求
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		BaseDelay:    100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		JitterFactor: 0.25,
	}
}

// ============================================================================
// 重试执行
// ============================================================================

// ExponentialBackoffRetry 执行带指数退避的重试
// 使用指数退避算法计算重试延迟，在基础延迟上添加随机抖动
//
// 算法公式:
//
//	delay = min(baseDelay * 2^attempt, maxDelay)
//	jitter = random(0, delay * jitterFactor)
//	actualDelay = delay + jitter
//
// 参数:
//   - ctx: 上下文，用于取消重试
//   - fn: 可重试的函数
//   - config: 重试配置
//
// 返回:
//   - 如果fn在重试次数内成功，返回nil
//   - 如果达到最大重试次数仍失败，返回最后一次的错误
//   - 如果上下文被取消，返回context.Cause(ctx)
//
// 行为:
//   - 第一次执行不延迟，立即执行
//   - 每次失败后计算延迟时间并等待
//   - 记录每次重试的尝试次数、延迟时间和错误信息到日志
//   - 支持上下文取消，可随时中断重试
//
// 示例:
//
//	err := retryutil.ExponentialBackoffRetry(ctx, func(ctx context.Context) error {
//	    return s.store.RevokeToken(ctx, tokenID)
//	}, retryutil.DefaultRetryConfig())
func ExponentialBackoffRetry(
	ctx context.Context,
	fn RetryableFunc,
	config RetryConfig,
) error {
	var lastErr error

	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		// 检查上下文是否已取消
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 执行操作
		err := fn(ctx)
		if err == nil {
			// 操作成功，立即返回
			return nil
		}

		lastErr = err

		// 如果是最后一次尝试，不再延迟
		if attempt == config.MaxRetries-1 {
			break
		}

		// 计算延迟时间
		delay := calculateDelay(attempt, config)

		// 记录重试日志
		slog.Warn("operation failed, retrying",
			"attempt", attempt+1,
			"max_retries", config.MaxRetries,
			"delay", delay,
			"error", err,
		)

		// 等待延迟时间或上下文取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// 继续下一次重试
		}
	}

	// 所有重试都失败，返回包装后的错误
	return fmt.Errorf("operation failed after %d retries: %w", config.MaxRetries, lastErr)
}

// ============================================================================
// 内部辅助函数
// ============================================================================

// calculateDelay 计算重试延迟时间
// 使用指数退避算法，并添加随机抖动
func calculateDelay(attempt int, config RetryConfig) time.Duration {
	// 计算指数退避延迟: baseDelay * 2^attempt
	// 使用位移运算避免浮点数运算
	exponentialDelay := config.BaseDelay * time.Duration(1<<attempt)

	// 限制最大延迟
	if exponentialDelay > config.MaxDelay {
		exponentialDelay = config.MaxDelay
	}

	// 计算抖动: random(0, delay * jitterFactor)
	jitter := calculateJitter(exponentialDelay, config.JitterFactor)

	return exponentialDelay + jitter
}

// calculateJitter 计算随机抖动
// 抖动范围为0到delay*jitterFactor
func calculateJitter(delay time.Duration, jitterFactor float64) time.Duration {
	if jitterFactor <= 0 {
		return 0
	}

	// 计算最大抖动值
	maxJitter := float64(delay) * jitterFactor
	maxJitterInt := int64(maxJitter)

	// 如果maxJitter小于1，返回0（避免panic）
	if maxJitterInt < 1 {
		return 0
	}

	// 生成随机数 [0, maxJitter)
	// 使用crypto/rand生成安全的随机数
	randomValue, err := rand.Int(rand.Reader, big.NewInt(maxJitterInt))
	if err != nil {
		// 如果随机数生成失败，返回0（无抖动）
		return 0
	}

	return time.Duration(randomValue.Int64())
}
