// Package middleware 日志中间件
// 记录HTTP请求日志
package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

const slowRequestThreshold = 500 * time.Millisecond

// ============================================================================
// 请求日志记录器
// ============================================================================

// responseWriter 包装http.ResponseWriter以捕获状态码
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader 写入响应头
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// ============================================================================
// 日志中间件
// ============================================================================

// Logger 日志中间件
// 记录每个HTTP请求的详细信息
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 包装ResponseWriter以捕获状态码
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// 处理请求
		next.ServeHTTP(wrapped, r)

		// 记录请求日志
		// slog 会自动转义用户输入，防止日志注入
		duration := time.Since(start)
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", duration.String(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"request_id", GetRequestIDFromContext(r.Context()),
		}
		if duration > slowRequestThreshold {
			slog.Warn("HTTP慢请求", attrs...) // #nosec G706 -- slog会自动转义用户输入，防止日志注入
			return
		}
		slog.Info("HTTP请求", attrs...) // #nosec G706 -- slog会自动转义用户输入，防止日志注入
	})
}
