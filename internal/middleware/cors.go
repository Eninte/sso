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
				return fmt.Errorf("生产环境禁止使用CORS通配符(*)，请配置具体的域名")
			}
			// 检查是否包含localhost（生产环境不应该允许）
			if strings.Contains(strings.ToLower(origin), "localhost") || strings.Contains(origin, "127.0.0.1") {
				return fmt.Errorf("生产环境禁止使用localhost或127.0.0.1作为CORS源")
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

			// 检查Origin是否在允许列表中
			if origin != "" && isOriginAllowed(origin, config.AllowedOrigins) {
				// 设置CORS响应头
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
				w.Header().Set("Access-Control-Allow-Credentials", "true")
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

// isOriginAllowed 检查Origin是否在允许列表中
func isOriginAllowed(origin string, allowedOrigins []string) bool {
	for _, allowed := range allowedOrigins {
		// 通配符匹配
		if allowed == "*" {
			return true
		}
		// 精确匹配
		if allowed == origin {
			return true
		}
		// 子域名匹配 (例如 *.example.com)
		if strings.HasPrefix(allowed, "*.") {
			domain := allowed[2:]
			if strings.HasSuffix(origin, "."+domain) || origin == "https://"+domain {
				return true
			}
		}
	}
	return false
}
