// Package middleware 语言中间件
// 提供请求语言检测和上下文存储
package middleware

import (
	"context"
	"net/http"

	"github.com/your-org/sso/internal/common"
)

// ============================================================================
// 上下文键
// ============================================================================

// 注意: contextKey 类型在 auth.go 中定义

const (
	// LanguageKey 语言上下文键
	LanguageKey contextKey = "language"
)

// ============================================================================
// 语言中间件
// ============================================================================

// Language 语言中间件
// 从请求头或查询参数中获取语言设置，存入上下文
func Language(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := detectLanguage(r)
		ctx := context.WithValue(r.Context(), LanguageKey, lang)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// detectLanguage 检测请求语言
// 优先级: 查询参数 > Accept-Language头 > 默认值
func detectLanguage(r *http.Request) string {
	// 1. 从查询参数获取
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return common.NormalizeLanguage(lang)
	}

	// 2. 从 Accept-Language 头获取
	if acceptLang := r.Header.Get("Accept-Language"); acceptLang != "" {
		return common.NormalizeLanguage(acceptLang)
	}

	// 3. 默认中文
	return "zh-CN"
}

// ============================================================================
// 上下文辅助函数
// ============================================================================

// GetLanguageFromContext 从上下文获取语言
func GetLanguageFromContext(ctx context.Context) string {
	if lang, ok := ctx.Value(LanguageKey).(string); ok {
		return lang
	}
	return "zh-CN"
}
