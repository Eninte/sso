// Package middleware CORS中间件
// 处理跨域资源共享
package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ============================================================================
// CORS配置
// ============================================================================

// CORSConfig CORS配置
type CORSConfig struct {
	AllowedOrigins []string // 允许的源
	AllowedMethods []string // 允许的HTTP方法
	AllowedHeaders []string // 允许的请求头
	MaxAge         int      // 预检请求缓存时间 (秒)
}

// Validate 验证CORS配置
// 在生产环境下禁止使用通配符，防止CSRF攻击
func (c *CORSConfig) Validate(env string) error {
	if env == "production" {
		for _, origin := range c.AllowedOrigins {
			if origin == "*" {
				return fmt.Errorf("CORS wildcard '*' is forbidden in production, please configure specific domains")
			}
			// 检查是否包含localhost（生产环境不应该允许）
			if strings.Contains(strings.ToLower(origin), "localhost") || strings.Contains(origin, "127.0.0.1") {
				return fmt.Errorf("localhost or 127.0.0.1 is forbidden as a CORS origin in production")
			}
		}
	}
	return nil
}

// DefaultCORSConfig 默认CORS配置
func DefaultCORSConfig() *CORSConfig {
	return &CORSConfig{
		AllowedOrigins: []string{"http://localhost:3000"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Requested-With", "X-Captcha-Id", "X-Captcha-Answer"},
		MaxAge:         86400, // 24小时
	}
}

// ============================================================================
// CORS中间件
// ============================================================================

// CORS CORS中间件
// 处理跨域请求
func CORS(config *CORSConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultCORSConfig()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// 响应随 Origin 变化，通知缓存按 Origin 区分（Vary: Origin）
			addVaryOrigin(w.Header())

			// 检查Origin是否在允许列表中
			// T8（M1）：仅精确匹配具体 origin 时发送 Allow-Credentials；
			// 通配形式（* 或 *.suffix）允许跨域但不携带凭据
			if origin != "" {
				allowed, exact := matchOrigin(origin, config.AllowedOrigins)
				if allowed {
					// 设置CORS响应头
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
					if exact {
						w.Header().Set("Access-Control-Allow-Credentials", "true")
					}
				}
			}

			// 处理预检请求 (OPTIONS)
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// 继续处理请求
			next.ServeHTTP(w, r)
		})
	}
}

// addVaryOrigin 在 Vary 响应头中追加 Origin（已存在时不重复添加）
func addVaryOrigin(h http.Header) {
	for _, v := range h.Values("Vary") {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "Origin") {
				return
			}
		}
	}
	h.Add("Vary", "Origin")
}

// matchOrigin 检查Origin是否在允许列表中
// 返回 allowed（是否允许跨域）与 exact（是否精确匹配具体 origin）
// 通配符 * 与 *.suffix 后缀匹配均为非精确匹配；精确匹配优先于通配匹配
func matchOrigin(origin string, allowedOrigins []string) (allowed, exact bool) {
	wildcardAllowed := false
	for _, allowedOrigin := range allowedOrigins {
		// 精确匹配（最高优先级，立即返回）
		if allowedOrigin == origin {
			return true, true
		}
		// 通配符匹配
		if allowedOrigin == "*" {
			wildcardAllowed = true
			continue
		}
		// 子域名匹配 (例如 *.example.com)
		if strings.HasPrefix(allowedOrigin, "*.") {
			domain := allowedOrigin[2:]
			if strings.HasSuffix(origin, "."+domain) || origin == "https://"+domain {
				wildcardAllowed = true
			}
		}
	}
	return wildcardAllowed, false
}
