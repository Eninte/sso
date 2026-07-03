// Package testutil 测试辅助函数单元测试
//
// 覆盖 envInt、envDuration、LoadConnConfig、RedisAddr 等纯函数
package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// envInt 测试
//
// 注意：操作环境变量的测试不使用 t.Parallel()，因为 t.Setenv 与 t.Parallel 不兼容，
// 且环境变量是进程级全局状态，并行操作同一变量会导致竞态。
// ============================================================================

func TestEnvInt_DefaultValue(t *testing.T) {
	t.Setenv("TEST_ENV_INT_KEY", "")
	got := envInt("TEST_ENV_INT_KEY", 42)
	assert.Equal(t, 42, got)
}

func TestEnvInt_ValidValue(t *testing.T) {
	t.Setenv("TEST_ENV_INT_KEY", "100")

	got := envInt("TEST_ENV_INT_KEY", 42)
	assert.Equal(t, 100, got)
}

func TestEnvInt_InvalidValue(t *testing.T) {
	t.Setenv("TEST_ENV_INT_KEY", "not-a-number")

	got := envInt("TEST_ENV_INT_KEY", 42)
	assert.Equal(t, 42, got, "无效值应返回默认值")
}

func TestEnvInt_EmptyValue(t *testing.T) {
	t.Setenv("TEST_ENV_INT_KEY", "")

	got := envInt("TEST_ENV_INT_KEY", 42)
	assert.Equal(t, 42, got, "空值应返回默认值")
}

// ============================================================================
// envDuration 测试
// ============================================================================

func TestEnvDuration_DefaultValue(t *testing.T) {
	t.Setenv("TEST_ENV_DUR_KEY", "")
	got := envDuration("TEST_ENV_DUR_KEY", 5*time.Second)
	assert.Equal(t, 5*time.Second, got)
}

func TestEnvDuration_ValidValue(t *testing.T) {
	t.Setenv("TEST_ENV_DUR_KEY", "10s")

	got := envDuration("TEST_ENV_DUR_KEY", 5*time.Second)
	assert.Equal(t, 10*time.Second, got)
}

func TestEnvDuration_InvalidValue(t *testing.T) {
	t.Setenv("TEST_ENV_DUR_KEY", "not-a-duration")

	got := envDuration("TEST_ENV_DUR_KEY", 5*time.Second)
	assert.Equal(t, 5*time.Second, got, "无效值应返回默认值")
}

func TestEnvDuration_EmptyValue(t *testing.T) {
	t.Setenv("TEST_ENV_DUR_KEY", "")

	got := envDuration("TEST_ENV_DUR_KEY", 5*time.Second)
	assert.Equal(t, 5*time.Second, got, "空值应返回默认值")
}

// ============================================================================
// LoadConnConfig 测试
// ============================================================================

func TestLoadConnConfig_Defaults(t *testing.T) {
	// 清除所有相关环境变量（设为空等价于未设置，因为 envInt/envDuration 对空值返回默认值）
	t.Setenv("TEST_CONN_MAX_RETRIES", "")
	t.Setenv("TEST_CONN_BASE_DELAY", "")
	t.Setenv("TEST_CONN_TIMEOUT", "")

	cfg := LoadConnConfig()

	assert.Equal(t, 3, cfg.MaxRetries, "默认 MaxRetries 应为 3")
	assert.Equal(t, 500*time.Millisecond, cfg.BaseDelay, "默认 BaseDelay 应为 500ms")
	assert.Equal(t, 30*time.Second, cfg.Timeout, "默认 Timeout 应为 30s")
}

func TestLoadConnConfig_CustomValues(t *testing.T) {
	t.Setenv("TEST_CONN_MAX_RETRIES", "5")
	t.Setenv("TEST_CONN_BASE_DELAY", "1s")
	t.Setenv("TEST_CONN_TIMEOUT", "60s")

	cfg := LoadConnConfig()

	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 1*time.Second, cfg.BaseDelay)
	assert.Equal(t, 60*time.Second, cfg.Timeout)
}

// ============================================================================
// ConnConfig.RetryConfig 测试
// ============================================================================

func TestConnConfig_RetryConfig(t *testing.T) {
	t.Parallel()

	cfg := ConnConfig{
		MaxRetries: 10,
		BaseDelay:  200 * time.Millisecond,
	}

	rc := cfg.RetryConfig()

	assert.Equal(t, 10, rc.MaxRetries)
	assert.Equal(t, 200*time.Millisecond, rc.BaseDelay)
	assert.Equal(t, 5*time.Second, rc.MaxDelay, "MaxDelay 应固定为 5s")
	assert.Equal(t, 0.25, rc.JitterFactor, "JitterFactor 应固定为 0.25")
}

// ============================================================================
// RedisAddr 测试
// ============================================================================

func TestRedisAddr_NotSet(t *testing.T) {
	t.Setenv("REDIS_TEST_ADDR", "")
	got := RedisAddr()
	assert.Equal(t, "", got)
}

func TestRedisAddr_Set(t *testing.T) {
	t.Setenv("REDIS_TEST_ADDR", "localhost:6379")

	got := RedisAddr()
	assert.Equal(t, "localhost:6379", got)
}
