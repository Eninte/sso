// Package middleware 认证中间件
// 提供JWT Token验证和权限检查功能
package middleware

import (
	"context"
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

			// 5. 继续处理请求
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ============================================================================
// 管理员权限中间件
// ============================================================================

// AdminMiddleware 管理员权限中间件
// 检查用户是否为管理员（基于邮箱白名单）
func AdminMiddleware(adminEmails []string, adminDomains []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从上下文获取用户邮箱
			email := GetUserEmailFromContext(r.Context())
			if email == "" {
				writeAdminError(w, http.StatusUnauthorized, "未认证")
				return
			}

			// 检查是否为管理员
			if !isAdminUser(email, adminEmails, adminDomains) {
				writeAdminError(w, http.StatusForbidden, "需要管理员权限")
				return
			}

			// 将管理员标识添加到上下文
			ctx := context.WithValue(r.Context(), IsAdminKey, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// isAdminUser 检查用户是否为管理员
func isAdminUser(email string, adminEmails []string, adminDomains []string) bool {
	email = strings.ToLower(email)

	// 检查是否在管理员邮箱白名单中
	for _, adminEmail := range adminEmails {
		if strings.ToLower(adminEmail) == email {
			return true
		}
	}

	// 检查邮箱域名是否在管理员域名白名单中
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		domain := parts[1]
		for _, adminDomain := range adminDomains {
			if strings.ToLower(adminDomain) == domain {
				return true
			}
		}
	}

	return false
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

// GetIsAdminFromContext 从上下文获取管理员标识
func GetIsAdminFromContext(ctx context.Context) bool {
	if isAdmin, ok := ctx.Value(IsAdminKey).(bool); ok {
		return isAdmin
	}
	return false
}
