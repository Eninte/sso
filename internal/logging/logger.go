// Package logging 结构化日志
// 提供统一的日志接口和配置
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/example/sso/internal/middleware"
)

// ============================================================================
// 日志配置
// ============================================================================

// Config 日志配置
type Config struct {
	Level     string    // 日志级别: debug, info, warn, error
	Format    string    // 日志格式: text, json
	Output    io.Writer // 输出目标
	AddSource bool      // 是否添加源码位置
}

// DefaultConfig 默认日志配置
func DefaultConfig() *Config {
	return &Config{
		Level:     "info",
		Format:    "text",
		Output:    os.Stdout,
		AddSource: false,
	}
}

// ============================================================================
// 日志初始化
// ============================================================================

// Init 初始化日志系统
func Init(cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 解析日志级别
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return err
	}

	// 创建handler
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}

	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(cfg.Output, opts)
	default:
		handler = slog.NewTextHandler(cfg.Output, opts)
	}

	// 设置默认logger
	slog.SetDefault(slog.New(handler))

	return nil
}

// InitForEnv 根据环境初始化日志
func InitForEnv(env string) {
	cfg := DefaultConfig()

	switch env {
	case "production":
		cfg.Level = "info"
		cfg.Format = "json"
		cfg.AddSource = false
	case "staging":
		cfg.Level = "info"
		cfg.Format = "json"
		cfg.AddSource = false
	case "development":
		cfg.Level = "debug"
		cfg.Format = "text"
		cfg.AddSource = true
	default:
		cfg.Level = "debug"
		cfg.Format = "text"
		cfg.AddSource = true
	}

	_ = Init(cfg)
}

// parseLevel 解析日志级别
func parseLevel(level string) (slog.Level, error) {
	switch level {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, nil
	}
}

// ============================================================================
// 日志辅助函数
// ============================================================================

// WithContext 创建带上下文的日志记录器
func WithContext(ctx context.Context) *slog.Logger {
	logger := slog.Default()

	if requestID := middleware.GetRequestIDFromContext(ctx); requestID != "" {
		logger = logger.With("request_id", requestID)
	}

	return logger
}

// WithComponent 创建带组件名称的日志记录器
func WithComponent(component string) *slog.Logger {
	return slog.Default().With("component", component)
}

// ============================================================================
// 请求日志字段
// ============================================================================

// RequestFields HTTP请求日志字段
type RequestFields struct {
	Method     string        `json:"method"`
	Path       string        `json:"path"`
	StatusCode int           `json:"status_code"`
	Duration   time.Duration `json:"duration"`
	RemoteAddr string        `json:"remote_addr"`
	UserAgent  string        `json:"user_agent,omitempty"`
	UserID     string        `json:"user_id,omitempty"`
	RequestID  string        `json:"request_id,omitempty"`
}

// LogRequest 记录HTTP请求日志
func LogRequest(fields RequestFields) {
	slog.Info("HTTP请求",
		"method", fields.Method,
		"path", fields.Path,
		"status", fields.StatusCode,
		"duration", fields.Duration.String(),
		"remote_addr", fields.RemoteAddr,
		"user_agent", fields.UserAgent,
		"user_id", fields.UserID,
		"request_id", fields.RequestID,
	)
}

// ============================================================================
// 业务日志函数
// ============================================================================

// LogAuth 认证相关日志
func LogAuth(event string, userID string, email string, success bool, err error) {
	attrs := []any{
		"event", event,
		"user_id", userID,
		"email", SanitizeEmail(email),
		"success", success,
	}
	if err != nil {
		attrs = append(attrs, "error", err.Error())
	}

	if success {
		slog.Info("认证事件", attrs...)
	} else {
		slog.Warn("认证失败", attrs...)
	}
}

// LogToken Token相关日志
func LogToken(event string, userID string, tokenID string, success bool, err error) {
	attrs := []any{
		"event", event,
		"user_id", userID,
		"token_id", tokenID,
		"success", success,
	}
	if err != nil {
		attrs = append(attrs, "error", err.Error())
	}

	if success {
		slog.Debug("Token事件", attrs...)
	} else {
		slog.Warn("Token操作失败", attrs...)
	}
}

// LogOAuth OAuth相关日志
func LogOAuth(event string, clientID string, userID string, success bool, err error) {
	attrs := []any{
		"event", event,
		"client_id", clientID,
		"user_id", userID,
		"success", success,
	}
	if err != nil {
		attrs = append(attrs, "error", err.Error())
	}

	if success {
		slog.Info("OAuth事件", attrs...)
	} else {
		slog.Warn("OAuth操作失败", attrs...)
	}
}

// LogSecurity 安全相关日志
func LogSecurity(event string, details map[string]interface{}) {
	attrs := []any{"event", event}
	for k, v := range details {
		attrs = append(attrs, k, v)
	}
	slog.Warn("安全事件", attrs...)
}

// LogError 错误日志
func LogError(msg string, err error, attrs ...any) {
	attrs = append(attrs, "error", err.Error())
	slog.Error(msg, attrs...)
}

// LogInfo 信息日志
func LogInfo(msg string, attrs ...any) {
	slog.Info(msg, attrs...)
}

// LogDebug 调试日志
func LogDebug(msg string, attrs ...any) {
	slog.Debug(msg, attrs...)
}

// LogWarn 警告日志
func LogWarn(msg string, attrs ...any) {
	slog.Warn(msg, attrs...)
}
