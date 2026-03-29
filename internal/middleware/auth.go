// Package middleware 认证中间件
// 提供JWT Token验证、Basic Auth和权限检查功能
package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/your-org/sso/internal/cache"
	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/store"
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

	// RequestIDKey 请求ID上下文键
	RequestIDKey contextKey = "requestID"
)

// ============================================================================
// 认证中间件
// ============================================================================

// AuthMiddleware 认证中间件
// 验证请求中的Bearer Token
// 将验证通过的用户信息添加到请求上下文
func AuthMiddleware(jwtSvc *crypto.JWTService) func(http.Handler) http.Handler {
	return authMiddlewareWithBlacklist(jwtSvc, nil)
}

// AuthMiddlewareWithStore 带数据库检查的认证中间件
// 会检查token是否被撤销
// 注意：此方法每次请求都查询数据库，建议使用AuthMiddlewareWithCache以获得更好性能
func AuthMiddlewareWithStore(jwtSvc *crypto.JWTService, store store.Store) func(http.Handler) http.Handler {
	return authMiddlewareWithBlacklist(jwtSvc, func(token string) bool {
		ctx := context.Background()
		tokenRecord, err := store.GetTokenByAccessToken(ctx, token)
		if err != nil {
			return true
		}
		return tokenRecord.RevokedAt != nil
	})
}

// AuthMiddlewareWithCache 带缓存层的认证中间件
// 使用缓存存储Token撤销状态，减少数据库查询
// 缓存TTL设置为与Access Token TTL一致，确保撤销状态及时更新
func AuthMiddlewareWithCache(jwtSvc *crypto.JWTService, store store.Store, cacheSvc cache.Cache) func(http.Handler) http.Handler {
	return authMiddlewareWithBlacklist(jwtSvc, func(token string) bool {
		ctx := context.Background()
		cacheKey := cache.TokenKey(token)

		var revoked bool
		err := cacheSvc.Get(ctx, cacheKey, &revoked)
		if err == nil {
			return revoked
		}

		if err != nil && !errors.Is(err, cache.ErrCacheMiss) {
			tokenPreview := token
			if len(token) > 8 {
				tokenPreview = token[:8] + "..."
			}
			slog.Error("缓存查询失败", "error", err, "token", tokenPreview)
		}

		tokenRecord, err := store.GetTokenByAccessToken(ctx, token)
		if err != nil {
			_ = cacheSvc.Set(ctx, cacheKey, true, cache.TokenTTL)
			return true
		}

		revoked = tokenRecord.RevokedAt != nil
		_ = cacheSvc.Set(ctx, cacheKey, revoked, cache.TokenTTL)
		return revoked
	})
}

// authMiddlewareWithBlacklist 内部实现
func authMiddlewareWithBlacklist(jwtSvc *crypto.JWTService, blacklistedFunc func(token string) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. 从Authorization头获取Token
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeAdminError(w, http.StatusUnauthorized, "缺少Authorization头")
				return
			}

			// 2. 解析Bearer Token
			// 格式: "Bearer <token>"
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				writeAdminError(w, http.StatusUnauthorized, "无效的Authorization格式")
				return
			}

			token := parts[1]

			// 3. 检查黑名单
			if blacklistedFunc != nil && blacklistedFunc(token) {
				writeAdminError(w, http.StatusUnauthorized, "Token已失效")
				return
			}

			// 4. 验证Token
			claims, err := jwtSvc.ValidateAccessToken(token)
			if err != nil {
				writeAdminError(w, http.StatusUnauthorized, "无效或过期的Token")
				return
			}

			// 5. 将用户信息添加到上下文
			ctx := context.WithValue(r.Context(), UserIDKey, claims.RegisteredClaims.Subject)
			ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
			ctx = context.WithValue(ctx, UserScopesKey, claims.Scopes)
			ctx = context.WithValue(ctx, UserRoleKey, claims.Role)

			// 6. 继续处理请求
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
