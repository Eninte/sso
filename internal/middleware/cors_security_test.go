// Package middleware CORS安全测试
package middleware

import (
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
