// Package middleware Panic恢复中间件
// 捕获HTTP处理链中的panic并记录堆栈
package middleware

import (
	"log/slog"
	"net/http"
	"runtime"
)

// ============================================================================
// Panic恢复中间件
// ============================================================================

// Recover 捕获后续HTTP处理链中的panic并返回500响应
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { //nolint:contextcheck // 这是 panic 恢复逻辑，r.Context() 仍可访问（panic 来自下游 handler，非 ctx 取消）
			if recovered := recover(); recovered != nil {
				stack := make([]byte, 64*1024)
				stack = stack[:runtime.Stack(stack, false)]
				// nosec G706 -- slog 结构化日志将值作为参数传递，不受日志注入影响
				slog.Error("HTTP处理发生panic",
					"error", recovered,
					"request_id", GetRequestIDFromContext(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(stack),
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
