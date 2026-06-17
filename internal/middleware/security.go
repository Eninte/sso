// Package middleware HTTP中间件
// 提供安全头、认证、限流等中间件功能
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
)

const (
	// cspNonceKey CSP nonce上下文键
	cspNonceKey contextKey = "cspNonce"
)

// SecurityHeaders 安全头中间件
// 添加OWASP推荐的安全HTTP头
// 参考: https://owasp.org/www-project-secure-headers/
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 防止点击劫持 (Clickjacking)
		// DENY: 完全禁止页面被嵌入iframe
		w.Header().Set("X-Frame-Options", "DENY")

		// 防止MIME类型嗅探
		// 浏览器将严格遵循Content-Type头
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// 严格传输安全 (HSTS)
		// 强制浏览器使用HTTPS
		// max-age: 1年
		// includeSubDomains: 包含所有子域名
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// 内容安全策略 (CSP)
		// 使用nonce支持安全的内联脚本
		nonce := generateCSPNonce()
		csp := fmt.Sprintf(
			"default-src 'self'; script-src 'self' 'nonce-%s'; style-src 'self' 'nonce-%s'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'",
			nonce, nonce,
		)
		w.Header().Set("Content-Security-Policy", csp)

		// 引用策略
		// 控制Referer头的发送
		w.Header().Set("Referrer-Policy", "no-referrer")

		// 权限策略
		// 禁用不必要的浏览器功能
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// 将nonce添加到上下文，供模板使用
		ctx := context.WithValue(r.Context(), cspNonceKey, nonce)

		// 继续处理请求
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateCSPNonce 生成CSP nonce
func generateCSPNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// GetCSPNonce 从上下文获取CSP nonce
func GetCSPNonce(ctx context.Context) string {
	if nonce, ok := ctx.Value(cspNonceKey).(string); ok {
		return nonce
	}
	return ""
}
