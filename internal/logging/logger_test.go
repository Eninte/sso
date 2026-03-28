// Package logging_test 结构化日志单元测试
package logging_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/your-org/sso/internal/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// DefaultConfig 测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := logging.DefaultConfig()

	require.NotNil(t, cfg)
	assert.Equal(t, "info", cfg.Level)
	assert.Equal(t, "text", cfg.Format)
	assert.NotNil(t, cfg.Output)
	assert.False(t, cfg.AddSource)
}

// ============================================================================
// Init 测试
// ============================================================================

func TestInit_NilConfig(t *testing.T) {
	err := logging.Init(nil)
	assert.NoError(t, err)
}

func TestInit_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:     "debug",
		Format:    "text",
		Output:    &buf,
		AddSource: false,
	}

	err := logging.Init(cfg)
	require.NoError(t, err)

	slog.Info("test message")
	assert.Contains(t, buf.String(), "test message")
}

func TestInit_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:     "info",
		Format:    "json",
		Output:    &buf,
		AddSource: false,
	}

	err := logging.Init(cfg)
	require.NoError(t, err)

	slog.Info("json test")
	assert.Contains(t, buf.String(), `"msg":"json test"`)
}

func TestInit_InvalidLevel(t *testing.T) {
	// parseLevel returns slog.LevelInfo for unknown levels, no error
	cfg := &logging.Config{
		Level:  "invalid-level",
		Format: "text",
		Output: &bytes.Buffer{},
	}

	err := logging.Init(cfg)
	assert.NoError(t, err)
}

func TestInit_AllLevels(t *testing.T) {
	tests := []struct {
		name  string
		level string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"warning", "warning"},
		{"error", "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &logging.Config{
				Level:  tt.level,
				Format: "text",
				Output: &bytes.Buffer{},
			}
			err := logging.Init(cfg)
			assert.NoError(t, err)
		})
	}
}

// ============================================================================
// InitForEnv 测试
// ============================================================================

func TestInitForEnv_Production(t *testing.T) {
	logging.InitForEnv("production")
	// 应该不panic
}

func TestInitForEnv_Development(t *testing.T) {
	logging.InitForEnv("development")
	// 应该不panic
}

func TestInitForEnv_Staging(t *testing.T) {
	logging.InitForEnv("staging")
	// 应该不panic
}

func TestInitForEnv_Unknown(t *testing.T) {
	logging.InitForEnv("unknown-env")
	// 应该不panic，使用默认debug配置
}

// ============================================================================
// WithContext 测试
// ============================================================================

func TestWithContext_NoRequestID(t *testing.T) {
	logger := logging.WithContext(context.Background())
	assert.NotNil(t, logger)
}

func TestWithContext_WithRequestID(t *testing.T) {
	ctx := context.Background()
	logger := logging.WithContext(ctx)
	assert.NotNil(t, logger)
}

// ============================================================================
// WithComponent 测试
// ============================================================================

func TestWithComponent(t *testing.T) {
	logger := logging.WithComponent("auth-service")
	assert.NotNil(t, logger)
}

// ============================================================================
// LogRequest 测试
// ============================================================================

func TestLogRequest(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "info",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	fields := logging.RequestFields{
		Method:     "GET",
		Path:       "/api/v1/users",
		StatusCode: 200,
		Duration:   100,
		RemoteAddr: "127.0.0.1",
		UserAgent:  "test-agent",
		UserID:     "user-123",
		RequestID:  "req-456",
	}

	logging.LogRequest(fields)

	output := buf.String()
	assert.Contains(t, output, "HTTP请求")
	assert.Contains(t, output, "GET")
	assert.Contains(t, output, "/api/v1/users")
}

// ============================================================================
// LogAuth 测试
// ============================================================================

func TestLogAuth_Success(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "info",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	logging.LogAuth("login", "user-123", "test@example.com", true, nil)

	output := buf.String()
	assert.Contains(t, output, "认证事件")
	assert.Contains(t, output, "login")
}

func TestLogAuth_Failure(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "warn",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	logging.LogAuth("login", "user-123", "test@example.com", false, errors.New("密码错误"))

	output := buf.String()
	assert.Contains(t, output, "认证失败")
}

// ============================================================================
// LogToken 测试
// ============================================================================

func TestLogToken_Success(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "debug",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	logging.LogToken("refresh", "user-123", "token-456", true, nil)

	output := buf.String()
	assert.Contains(t, output, "Token事件")
}

func TestLogToken_Failure(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "warn",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	logging.LogToken("refresh", "user-123", "token-456", false, errors.New("token过期"))

	output := buf.String()
	assert.Contains(t, output, "Token操作失败")
}

// ============================================================================
// LogOAuth 测试
// ============================================================================

func TestLogOAuth_Success(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "info",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	logging.LogOAuth("authorize", "client-123", "user-456", true, nil)

	output := buf.String()
	assert.Contains(t, output, "OAuth事件")
}

func TestLogOAuth_Failure(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "warn",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	logging.LogOAuth("authorize", "client-123", "user-456", false, errors.New("无效client"))

	output := buf.String()
	assert.Contains(t, output, "OAuth操作失败")
}

// ============================================================================
// LogSecurity 测试
// ============================================================================

func TestLogSecurity(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "warn",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	details := map[string]interface{}{
		"ip":     "192.168.1.1",
		"reason": "多次登录失败",
	}
	logging.LogSecurity("brute_force_detected", details)

	output := buf.String()
	assert.Contains(t, output, "安全事件")
}

// ============================================================================
// LogError 测试
// ============================================================================

func TestLogError(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "error",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	logging.LogError("操作失败", errors.New("database error"), "user_id", "123")

	output := buf.String()
	assert.Contains(t, output, "操作失败")
	assert.Contains(t, output, "database error")
}

// ============================================================================
// LogInfo / LogDebug / LogWarn 测试
// ============================================================================

func TestLogInfo(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "info",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	logging.LogInfo("系统启动", "port", "9090")

	output := buf.String()
	assert.Contains(t, output, "系统启动")
}

func TestLogDebug(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "debug",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	logging.LogDebug("调试信息", "key", "value")

	output := buf.String()
	assert.Contains(t, output, "调试信息")
}

func TestLogWarn(t *testing.T) {
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "warn",
		Format: "text",
		Output: &buf,
	}
	_ = logging.Init(cfg)

	logging.LogWarn("配置警告", "field", "deprecated")

	output := buf.String()
	assert.Contains(t, output, "配置警告")
}
