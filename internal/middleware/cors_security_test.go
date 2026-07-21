// Package middleware CORS安全测试
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCORSConfig_Validate_Production 测试生产环境CORS配置验证
func TestCORSConfig_Validate_Production(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		env            string
		expectError    bool
		errorContains  string
	}{
		{
			name:           "生产环境_通配符_应该失败",
			allowedOrigins: []string{"*"},
			env:            "production",
			expectError:    true,
			errorContains:  "wildcard",
		},
		{
			name:           "生产环境_包含通配符_应该失败",
			allowedOrigins: []string{"https://example.com", "*"},
			env:            "production",
			expectError:    true,
			errorContains:  "wildcard",
		},
		{
			name:           "生产环境_localhost_应该失败",
			allowedOrigins: []string{"http://localhost:3000"},
			env:            "production",
			expectError:    true,
			errorContains:  "localhost",
		},
		{
			name:           "生产环境_127.0.0.1_应该失败",
			allowedOrigins: []string{"http://127.0.0.1:3000"},
			env:            "production",
			expectError:    true,
			errorContains:  "127.0.0.1",
		},
		{
			name:           "生产环境_混合localhost_应该失败",
			allowedOrigins: []string{"https://example.com", "http://localhost:3000"},
			env:            "production",
			expectError:    true,
			errorContains:  "localhost",
		},
		{
			name:           "生产环境_合法域名_应该成功",
			allowedOrigins: []string{"https://example.com"},
			env:            "production",
			expectError:    false,
		},
		{
			name:           "生产环境_多个合法域名_应该成功",
			allowedOrigins: []string{"https://example.com", "https://app.example.com"},
			env:            "production",
			expectError:    false,
		},
		{
			name:           "生产环境_子域名通配符_应该成功",
			allowedOrigins: []string{"*.example.com"},
			env:            "production",
			expectError:    false,
		},
		{
			name:           "开发环境_通配符_应该成功",
			allowedOrigins: []string{"*"},
			env:            "development",
			expectError:    false,
		},
		{
			name:           "开发环境_localhost_应该成功",
			allowedOrigins: []string{"http://localhost:3000"},
			env:            "development",
			expectError:    false,
		},
		{
			name:           "开发环境_127.0.0.1_应该成功",
			allowedOrigins: []string{"http://127.0.0.1:3000"},
			env:            "development",
			expectError:    false,
		},
		{
			name:           "空环境_默认为开发_应该成功",
			allowedOrigins: []string{"*"},
			env:            "",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &CORSConfig{
				AllowedOrigins: tt.allowedOrigins,
				AllowedMethods: []string{"GET", "POST"},
				AllowedHeaders: []string{"Content-Type"},
				MaxAge:         3600,
			}

			err := config.Validate(tt.env)

			if tt.expectError {
				assert.Error(t, err, "应该返回错误")
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains, "错误消息应该包含关键词")
				}
			} else {
				assert.NoError(t, err, "不应该返回错误")
			}
		})
	}
}

// TestCORSConfig_Validate_EdgeCases 测试边界情况
func TestCORSConfig_Validate_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		env            string
		expectError    bool
	}{
		{
			name:           "空origins列表_应该成功",
			allowedOrigins: []string{},
			env:            "production",
			expectError:    false,
		},
		{
			name:           "nil_origins_应该成功",
			allowedOrigins: nil,
			env:            "production",
			expectError:    false,
		},
		{
			name:           "大小写混合localhost_应该失败",
			allowedOrigins: []string{"http://LocalHost:3000"},
			env:            "production",
			expectError:    true,
		},
		{
			name:           "HTTPS_localhost_应该失败",
			allowedOrigins: []string{"https://localhost:8443"},
			env:            "production",
			expectError:    true,
		},
		{
			name:           "域名包含localhost子串_应该失败",
			allowedOrigins: []string{"https://localhost.example.com"},
			env:            "production",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &CORSConfig{
				AllowedOrigins: tt.allowedOrigins,
				AllowedMethods: []string{"GET", "POST"},
				AllowedHeaders: []string{"Content-Type"},
				MaxAge:         3600,
			}

			err := config.Validate(tt.env)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCORSConfig_Validate_RealWorldScenarios 测试真实场景
func TestCORSConfig_Validate_RealWorldScenarios(t *testing.T) {
	t.Run("典型生产配置_应该成功", func(t *testing.T) {
		config := &CORSConfig{
			AllowedOrigins: []string{
				"https://app.example.com",
				"https://admin.example.com",
				"https://api.example.com",
			},
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
			MaxAge:         86400,
		}

		err := config.Validate("production")
		assert.NoError(t, err)
	})

	t.Run("典型开发配置_应该成功", func(t *testing.T) {
		config := &CORSConfig{
			AllowedOrigins: []string{
				"http://localhost:3000",
				"http://localhost:3001",
				"http://127.0.0.1:3000",
			},
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
			MaxAge:         86400,
		}

		err := config.Validate("development")
		assert.NoError(t, err)
	})

	t.Run("错误的生产配置_混合localhost和生产域名_应该失败", func(t *testing.T) {
		config := &CORSConfig{
			AllowedOrigins: []string{
				"https://app.example.com",
				"http://localhost:3000", // 错误：生产环境不应该有localhost
			},
			AllowedMethods: []string{"GET", "POST"},
			AllowedHeaders: []string{"Content-Type"},
			MaxAge:         86400,
		}

		err := config.Validate("production")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "localhost")
	})
}

// ============================================================================
// T8：CORS credentials 策略收紧测试（M1）
// ============================================================================

// newCORSRequest 构造带 Origin 的测试请求并经由 CORS 中间件处理
func newCORSRequest(t *testing.T, config *CORSConfig, method, origin string) *httptest.ResponseRecorder {
	t.Helper()
	handler := CORS(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(method, "/api/v1/userinfo", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

// TestCORS_CredentialsPolicy 验证 Allow-Credentials 仅在精确匹配时发送
func TestCORS_CredentialsPolicy(t *testing.T) {
	baseConfig := func(origins ...string) *CORSConfig {
		return &CORSConfig{
			AllowedOrigins: origins,
			AllowedMethods: []string{"GET", "POST", "OPTIONS"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
			MaxAge:         3600,
		}
	}

	t.Run("精确匹配_有credentials且ACAO回显", func(t *testing.T) {
		recorder := newCORSRequest(t, baseConfig("https://app.example.com"), http.MethodGet, "https://app.example.com")

		assert.Equal(t, "https://app.example.com", recorder.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", recorder.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("通配符星号_有ACAO无credentials", func(t *testing.T) {
		recorder := newCORSRequest(t, baseConfig("*"), http.MethodGet, "https://anything.example.com")

		assert.Equal(t, "https://anything.example.com", recorder.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"),
			"通配符命中不得发送 Allow-Credentials")
	})

	t.Run("子域通配后缀命中_有ACAO无credentials", func(t *testing.T) {
		recorder := newCORSRequest(t, baseConfig("*.example.com"), http.MethodGet, "https://api.example.com")

		assert.Equal(t, "https://api.example.com", recorder.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"),
			"子域通配命中不得发送 Allow-Credentials")
	})

	t.Run("子域通配裸域命中_有ACAO无credentials", func(t *testing.T) {
		recorder := newCORSRequest(t, baseConfig("*.example.com"), http.MethodGet, "https://example.com")

		assert.Equal(t, "https://example.com", recorder.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("混合列表_精确项仍发送credentials", func(t *testing.T) {
		config := baseConfig("*.example.com", "https://api.example.com")
		recorder := newCORSRequest(t, config, http.MethodGet, "https://api.example.com")

		assert.Equal(t, "https://api.example.com", recorder.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", recorder.Header().Get("Access-Control-Allow-Credentials"),
			"origin 同时命中精确项时应视为精确匹配")
	})

	t.Run("混合列表_仅通配项命中_无credentials", func(t *testing.T) {
		config := baseConfig("*.example.com", "https://api.example.com")
		recorder := newCORSRequest(t, config, http.MethodGet, "https://web.example.com")

		assert.Equal(t, "https://web.example.com", recorder.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("预检请求_通配命中_无credentials", func(t *testing.T) {
		recorder := newCORSRequest(t, baseConfig("*"), http.MethodOptions, "https://anything.example.com")

		assert.Equal(t, http.StatusNoContent, recorder.Code)
		assert.Equal(t, "https://anything.example.com", recorder.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	})
}

// TestCORS_VaryOrigin 验证响应始终携带 Vary: Origin
func TestCORS_VaryOrigin(t *testing.T) {
	config := &CORSConfig{
		AllowedOrigins: []string{"https://app.example.com"},
		AllowedMethods: []string{"GET", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type"},
		MaxAge:         3600,
	}

	t.Run("允许的origin_含Vary", func(t *testing.T) {
		recorder := newCORSRequest(t, config, http.MethodGet, "https://app.example.com")
		assert.Contains(t, recorder.Header().Values("Vary"), "Origin")
	})

	t.Run("不允许的origin_含Vary", func(t *testing.T) {
		recorder := newCORSRequest(t, config, http.MethodGet, "https://evil.example.com")
		assert.Contains(t, recorder.Header().Values("Vary"), "Origin",
			"不允许的 origin 也需 Vary 以防缓存污染")
	})

	t.Run("无origin_含Vary", func(t *testing.T) {
		recorder := newCORSRequest(t, config, http.MethodGet, "")
		assert.Contains(t, recorder.Header().Values("Vary"), "Origin")
	})

	t.Run("预检请求_含Vary", func(t *testing.T) {
		recorder := newCORSRequest(t, config, http.MethodOptions, "https://app.example.com")
		assert.Contains(t, recorder.Header().Values("Vary"), "Origin")
	})
}

// TestCORS_DisallowedOrigin 验证不允许的 origin 不携带任何 CORS 头
func TestCORS_DisallowedOrigin(t *testing.T) {
	config := &CORSConfig{
		AllowedOrigins: []string{"https://app.example.com", "*.example.org"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:         3600,
	}

	t.Run("普通请求_无CORS头", func(t *testing.T) {
		recorder := newCORSRequest(t, config, http.MethodGet, "https://evil.example.com")

		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Methods"))
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Headers"))
		assert.Equal(t, http.StatusOK, recorder.Code, "非预检请求应继续放行由后续处理器响应")
	})

	t.Run("子域后缀欺骗_无CORS头", func(t *testing.T) {
		// evil-example.org 不是 *.example.org 的子域
		recorder := newCORSRequest(t, config, http.MethodGet, "https://evil-example.org")
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("预检请求_无CORS头", func(t *testing.T) {
		recorder := newCORSRequest(t, config, http.MethodOptions, "https://evil.example.com")

		assert.Equal(t, http.StatusNoContent, recorder.Code)
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	})
}
