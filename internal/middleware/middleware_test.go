// Package middleware_test 中间件单元测试
package middleware_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/middleware"
)

// ============================================================================
// SecurityHeaders 测试
// ============================================================================

func TestSecurityHeaders(t *testing.T) {
	// 创建测试处理器
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// 包装中间件
	wrapped := middleware.SecurityHeaders(handler)

	// 创建测试请求
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// 执行请求
	wrapped.ServeHTTP(rec, req)

	// 验证安全头
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "1; mode=block", rec.Header().Get("X-XSS-Protection"))
	assert.Contains(t, rec.Header().Get("Strict-Transport-Security"), "max-age=31536000")
	assert.Equal(t, "default-src 'self'", rec.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
	assert.Contains(t, rec.Header().Get("Permissions-Policy"), "geolocation=()")
}

// ============================================================================
// RateLimiter 测试
// ============================================================================

func TestRateLimiter(t *testing.T) {
	// 创建限流器: 3个请求/分钟 (使用较长的时间窗口避免测试时间问题)
	limiter := middleware.NewRateLimiter(3, time.Minute)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := limiter.Middleware(handler)

	// 发送3个请求应该成功
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "请求 %d 应该成功", i+1)
	}

	// 第4个请求应该被限制
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestRateLimiter_DifferentClients(t *testing.T) {
	// 创建限流器: 1个请求/分钟
	limiter := middleware.NewRateLimiter(1, time.Minute)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := limiter.Middleware(handler)

	// 客户端1的请求
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	rec1 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// 客户端2的请求应该成功（不同IP）
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.2:12345"
	rec2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)

	// 客户端1的第二个请求应该被限制
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.RemoteAddr = "192.168.1.1:12345"
	rec3 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusTooManyRequests, rec3.Code)
}

// ============================================================================
// Logger 测试
// ============================================================================

func TestLogger(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	wrapped := middleware.Logger(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "OK", rec.Body.String())
}

// ============================================================================
// AuthMiddleware 测试
// ============================================================================

// createTestJWTService 创建测试用的JWT服务
func createTestJWTService(t *testing.T) *crypto.JWTService {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	return crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	jwtSvc := createTestJWTService(t)

	// 生成有效的Token
	token, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid", "profile"})
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证上下文中的用户信息
		userID := middleware.GetUserIDFromContext(r.Context())
		email := middleware.GetUserEmailFromContext(r.Context())
		scopes := middleware.GetUserScopesFromContext(r.Context())

		assert.Equal(t, "user-123", userID)
		assert.Equal(t, "test@example.com", email)
		assert.Equal(t, []string{"openid", "profile"}, scopes)

		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware.AuthMiddleware(jwtSvc)(handler)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_MissingAuthorization(t *testing.T) {
	jwtSvc := createTestJWTService(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware.AuthMiddleware(jwtSvc)(handler)

	req := httptest.NewRequest("GET", "/protected", nil)
	// 不设置Authorization头
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "缺少Authorization头")
}

func TestAuthMiddleware_InvalidAuthFormat(t *testing.T) {
	jwtSvc := createTestJWTService(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware.AuthMiddleware(jwtSvc)(handler)

	tests := []struct {
		name   string
		header string
	}{
		{
			name:   "无Bearer前缀",
			header: "invalid-token",
		},
		{
			name:   "错误的前缀",
			header: "Basic dXNlcjpwYXNz",
		},
		{
			name:   "只有Bearer",
			header: "Bearer",
		},
		{
			name:   "空格分隔",
			header: "  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/protected", nil)
			req.Header.Set("Authorization", tt.header)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	jwtSvc := createTestJWTService(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware.AuthMiddleware(jwtSvc)(handler)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "无效或过期的Token")
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	// 创建一个过期时间很短的JWT服务
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		1*time.Millisecond, // 1毫秒过期
		7*24*time.Hour,
	)

	// 生成Token
	token, err := jwtSvc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
	require.NoError(t, err)

	// 等待Token过期
	time.Sleep(10 * time.Millisecond)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware.AuthMiddleware(jwtSvc)(handler)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_DifferentKeyToken(t *testing.T) {
	// 用一个密钥生成Token
	jwtSvc1 := createTestJWTService(t)
	token, err := jwtSvc1.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
	require.NoError(t, err)

	// 用另一个密钥验证
	jwtSvc2 := createTestJWTService(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware.AuthMiddleware(jwtSvc2)(handler)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ============================================================================
// 上下文辅助函数测试
// ============================================================================

func TestGetUserIDFromContext(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() context.Context
		expected string
	}{
		{
			name: "存在用户ID",
			setup: func() context.Context {
				return context.WithValue(context.Background(), middleware.UserIDKey, "user-123")
			},
			expected: "user-123",
		},
		{
			name: "不存在用户ID",
			setup: func() context.Context {
				return context.Background()
			},
			expected: "",
		},
		{
			name: "错误的类型",
			setup: func() context.Context {
				return context.WithValue(context.Background(), middleware.UserIDKey, 12345)
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			result := middleware.GetUserIDFromContext(ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUserEmailFromContext(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() context.Context
		expected string
	}{
		{
			name: "存在邮箱",
			setup: func() context.Context {
				return context.WithValue(context.Background(), middleware.UserEmailKey, "test@example.com")
			},
			expected: "test@example.com",
		},
		{
			name: "不存在邮箱",
			setup: func() context.Context {
				return context.Background()
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			result := middleware.GetUserEmailFromContext(ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUserScopesFromContext(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() context.Context
		expected []string
	}{
		{
			name: "存在scopes",
			setup: func() context.Context {
				return context.WithValue(context.Background(), middleware.UserScopesKey, []string{"openid", "profile"})
			},
			expected: []string{"openid", "profile"},
		},
		{
			name: "不存在scopes",
			setup: func() context.Context {
				return context.Background()
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			result := middleware.GetUserScopesFromContext(ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// CORS 测试
// ============================================================================

func TestCORS(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	corsConfig := middleware.DefaultCORSConfig()
	wrapped := middleware.CORS(corsConfig)(handler)

	// 测试预检请求
	t.Run("OPTIONS预检请求", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/test", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		// OPTIONS 请求返回 204 No Content
		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Origin"), "http://localhost:3000")
	})

	// 测试普通请求
	t.Run("普通GET请求", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Origin"), "http://localhost:3000")
	})
}

func TestCORS_OriginNotAllowed(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	corsConfig := middleware.DefaultCORSConfig()
	wrapped := middleware.CORS(corsConfig)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// 不允许的源不应该有CORS头
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

// ============================================================================
// RateLimiter 补充测试
// ============================================================================

func TestRateLimiter_XRealIP(t *testing.T) {
	limiter := middleware.NewRateLimiter(1, time.Minute)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := limiter.Middleware(handler)

	// 使用 X-Real-IP 头
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Real-IP", "10.0.0.1")
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 同一个 X-Real-IP 应该被限制
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Real-IP", "10.0.0.1")
	req2.RemoteAddr = "192.168.1.2:12345"
	rec2 := httptest.NewRecorder()

	wrapped.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

func TestRateLimiter_InvalidXRealIP(t *testing.T) {
	limiter := middleware.NewRateLimiter(1, time.Minute)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := limiter.Middleware(handler)

	// 无效的 X-Real-IP，应该使用 RemoteAddr
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Real-IP", "invalid-ip")
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimiter_RemoteAddrWithoutPort(t *testing.T) {
	limiter := middleware.NewRateLimiter(1, time.Minute)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := limiter.Middleware(handler)

	// RemoteAddr 没有端口
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1"
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ============================================================================
// RequireAdmin 测试
// ============================================================================

func TestRequireAdmin(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("无角色上下文-返回401", func(t *testing.T) {
		adminMw := middleware.RequireAdmin()
		wrapped := adminMw(handler)

		req := httptest.NewRequest("GET", "/admin", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("普通用户-返回403", func(t *testing.T) {
		adminMw := middleware.RequireAdmin()
		wrapped := adminMw(handler)

		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), middleware.UserRoleKey, "user")
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req.WithContext(ctx))

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("管理员角色-返回200", func(t *testing.T) {
		adminMw := middleware.RequireAdmin()
		wrapped := adminMw(handler)

		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), middleware.UserRoleKey, "admin")
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req.WithContext(ctx))

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// RequireRole 测试
func TestRequireRole(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("角色匹配-返回200", func(t *testing.T) {
		roleMw := middleware.RequireRole("admin", "super_admin")
		wrapped := roleMw(handler)

		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), middleware.UserRoleKey, "admin")
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req.WithContext(ctx))

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("角色不匹配-返回403", func(t *testing.T) {
		roleMw := middleware.RequireRole("admin")
		wrapped := roleMw(handler)

		req := httptest.NewRequest("GET", "/admin", nil)
		ctx := context.WithValue(req.Context(), middleware.UserRoleKey, "user")
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req.WithContext(ctx))

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

// ============================================================================
// GetIsAdminFromContext 测试
// ============================================================================

func TestGetIsAdminFromContext(t *testing.T) {
	t.Run("上下文有管理员标识", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), middleware.IsAdminKey, true)
		assert.True(t, middleware.GetIsAdminFromContext(ctx))
	})

	t.Run("上下文无管理员标识", func(t *testing.T) {
		assert.False(t, middleware.GetIsAdminFromContext(context.Background()))
	})

	t.Run("上下文标识为非bool类型", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), middleware.IsAdminKey, "true")
		assert.False(t, middleware.GetIsAdminFromContext(ctx))
	})
}

// ============================================================================
// Language 中间件测试
// ============================================================================

func TestLanguageMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := middleware.GetLanguageFromContext(r.Context())
		w.Write([]byte(lang))
	})

	t.Run("查询参数优先", func(t *testing.T) {
		wrapped := middleware.Language(handler)

		req := httptest.NewRequest("GET", "/test?lang=en-US", nil)
		req.Header.Set("Accept-Language", "zh-CN")
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, "en-US", rec.Body.String())
	})

	t.Run("Accept-Language头", func(t *testing.T) {
		wrapped := middleware.Language(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, "en-US", rec.Body.String())
	})

	t.Run("默认中文", func(t *testing.T) {
		wrapped := middleware.Language(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, "zh-CN", rec.Body.String())
	})

	t.Run("中文变体规范化", func(t *testing.T) {
		wrapped := middleware.Language(handler)

		req := httptest.NewRequest("GET", "/test?lang=zh-TW", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, "zh-CN", rec.Body.String())
	})

	t.Run("英文变体规范化", func(t *testing.T) {
		wrapped := middleware.Language(handler)

		req := httptest.NewRequest("GET", "/test?lang=en-GB", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, "en-US", rec.Body.String())
	})

	t.Run("Accept-Language复杂格式", func(t *testing.T) {
		wrapped := middleware.Language(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, "zh-CN", rec.Body.String())
	})

	t.Run("未知语言保留原样", func(t *testing.T) {
		wrapped := middleware.Language(handler)

		req := httptest.NewRequest("GET", "/test?lang=ja-JP", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, "ja-jp", rec.Body.String())
	})
}

// ============================================================================
// GetLanguageFromContext 测试
// ============================================================================

func TestGetLanguageFromContext(t *testing.T) {
	t.Run("上下文有语言设置", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), middleware.LanguageKey, "en-US")
		assert.Equal(t, "en-US", middleware.GetLanguageFromContext(ctx))
	})

	t.Run("上下文无语言设置-返回默认中文", func(t *testing.T) {
		assert.Equal(t, "zh-CN", middleware.GetLanguageFromContext(context.Background()))
	})
}
