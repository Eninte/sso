// Package middleware 认证中间件
// 提供JWT Token验证、Basic Auth和权限检查功能
package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/your-org/sso/internal/crypto"
)

// ============================================================================
// 上下文键
// ============================================================================

// contextKey 上下文键类型
// 使用自定义类型避免键冲突
type contextKey string

const (
	// UserIDKey 用户ID上下文键
	UserIDKey contextKey = "userID"

	// UserEmailKey 用户邮箱上下文键
	UserEmailKey contextKey = "userEmail"

	// UserScopesKey 用户权限范围上下文键
	UserScopesKey contextKey = "userScopes"

	// UserRoleKey 用户角色上下文键
	UserRoleKey contextKey = "userRole"

	// IsAdminKey 管理员标识上下文键
	IsAdminKey contextKey = "isAdmin"
)

// ============================================================================
// 认证中间件
// ============================================================================

// AuthMiddleware 认证中间件
// 验证请求中的Bearer Token
// 将验证通过的用户信息添加到请求上下文
func AuthMiddleware(jwtSvc *crypto.JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. 从Authorization头获取Token
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"缺少Authorization头"}`, http.StatusUnauthorized)
				return
			}

			// 2. 解析Bearer Token
			// 格式: "Bearer <token>"
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"error":"无效的Authorization格式"}`, http.StatusUnauthorized)
				return
			}

			// 3. 验证Token
			claims, err := jwtSvc.ValidateAccessToken(parts[1])
			if err != nil {
				http.Error(w, `{"error":"无效或过期的Token"}`, http.StatusUnauthorized)
				return
			}

			// 4. 将用户信息添加到上下文
			ctx := context.WithValue(r.Context(), UserIDKey, claims.RegisteredClaims.Subject)
			ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
			ctx = context.WithValue(ctx, UserScopesKey, claims.Scopes)
			ctx = context.WithValue(ctx, UserRoleKey, claims.Role)

			// 5. 继续处理请求
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ============================================================================
// 基于角色的权限中间件
// ============================================================================

// RequireRole 要求特定角色的中间件
// 检查用户JWT中的角色是否在允许的角色列表中
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从上下文获取用户角色
			userRole := GetUserRoleFromContext(r.Context())
			if userRole == "" {
				writeAdminError(w, http.StatusUnauthorized, "未认证")
				return
			}

			// 检查角色是否匹配
			for _, role := range roles {
				if userRole == role {
					next.ServeHTTP(w, r)
					return
				}
			}

			writeAdminError(w, http.StatusForbidden, "需要管理员权限")
		})
	}
}

// RequireAdmin 要求管理员角色的中间件（便捷函数）
func RequireAdmin() func(http.Handler) http.Handler {
	return RequireRole("admin")
}

// writeAdminError 写入管理员权限错误响应
func writeAdminError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// ============================================================================
// 上下文辅助函数
// ============================================================================

// GetUserIDFromContext 从上下文获取用户ID
func GetUserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

// GetUserEmailFromContext 从上下文获取用户邮箱
func GetUserEmailFromContext(ctx context.Context) string {
	if email, ok := ctx.Value(UserEmailKey).(string); ok {
		return email
	}
	return ""
}

// GetUserScopesFromContext 从上下文获取用户权限范围
func GetUserScopesFromContext(ctx context.Context) []string {
	if scopes, ok := ctx.Value(UserScopesKey).([]string); ok {
		return scopes
	}
	return nil
}

// GetUserRoleFromContext 从上下文获取用户角色
func GetUserRoleFromContext(ctx context.Context) string {
	if role, ok := ctx.Value(UserRoleKey).(string); ok {
		return role
	}
	return ""
}

// GetIsAdminFromContext 从上下文获取管理员标识
func GetIsAdminFromContext(ctx context.Context) bool {
	if isAdmin, ok := ctx.Value(IsAdminKey).(bool); ok {
		return isAdmin
	}
	return false
}

// ============================================================================
// Basic Auth中间件
// ============================================================================

// BasicAuth Basic Auth认证中间件
// 使用恒定时间比较防止时序攻击
func BasicAuth(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if username == "" || password == "" {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="Metrics"`)
				http.Error(w, "未授权", http.StatusUnauthorized)
				return
			}

			const prefix = "Basic "
			if !strings.HasPrefix(authHeader, prefix) {
				http.Error(w, "无效的认证头格式", http.StatusUnauthorized)
				return
			}

			encoded := authHeader[len(prefix):]
			decoded, err := base64Decode(encoded)
			if err != nil {
				http.Error(w, "无效的编码格式", http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(decoded, ":", 2)
			if len(parts) != 2 {
				http.Error(w, "无效的凭据格式", http.StatusUnauthorized)
				return
			}

			usernameMatch := subtle.ConstantTimeCompare([]byte(parts[0]), []byte(username)) == 1
			passwordMatch := subtle.ConstantTimeCompare([]byte(parts[1]), []byte(password)) == 1

			if !usernameMatch || !passwordMatch {
				w.Header().Set("WWW-Authenticate", `Basic realm="Metrics"`)
				http.Error(w, "未授权", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// base64Decode 解码base64字符串
func base64Decode(s string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
