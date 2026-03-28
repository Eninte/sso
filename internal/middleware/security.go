// Package middleware HTTP中间件
// 提供安全头、认证、限流等中间件功能
package middleware

import (
	"net/http"
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
		// 限制资源加载来源
		w.Header().Set("Content-Security-Policy", "default-src 'self'")

		// 引用策略
		// 控制Referer头的发送
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// 权限策略
		// 禁用不必要的浏览器功能
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// 继续处理请求
		next.ServeHTTP(w, r)
	})
}
