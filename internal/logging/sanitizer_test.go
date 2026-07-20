// Package logging_test 日志脱敏单元测试
package logging_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/logging"
)

// ============================================================================
// SanitizeEmail 测试
// ============================================================================

func TestSanitizeEmail(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{
			name:     "空邮箱",
			email:    "",
			expected: "",
		},
		{
			name:     "无@符号",
			email:    "invalid-email",
			expected: "invalid-email",
		},
		{
			name:     "只有@符号",
			email:    "@",
			expected: "@",
		},
		{
			name:     "单字符用户名",
			email:    "a@example.com",
			expected: "a***@example.com",
		},
		{
			name:     "两字符用户名",
			email:    "ab@example.com",
			expected: "ab***@example.com",
		},
		{
			name:     "三字符用户名",
			email:    "abc@example.com",
			expected: "abc***@example.com",
		},
		{
			name:     "四字符用户名",
			email:    "user@example.com",
			expected: "u***@example.com",
		},
		{
			name:     "长用户名",
			email:    "john.doe@example.com",
			expected: "j***@example.com",
		},
		{
			name:     "多级域名",
			email:    "admin@mail.example.com",
			expected: "a***@mail.example.com",
		},
		{
			name:     "带加号的用户名",
			email:    "user+tag@example.com",
			expected: "u***@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logging.SanitizeEmail(tt.email)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// SanitizeToken 测试
// ============================================================================

func TestSanitizeToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "空Token",
			token:    "",
			expected: "***",
		},
		{
			name:     "短Token",
			token:    "abc",
			expected: "***",
		},
		{
			name:     "正好8字符",
			token:    "12345678",
			expected: "***",
		},
		{
			name:     "9字符Token",
			token:    "123456789",
			expected: "12345678...",
		},
		{
			name:     "长Token",
			token:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected: "eyJhbGci...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logging.SanitizeToken(tt.token)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// SanitizePhone 测试
// ============================================================================

func TestSanitizePhone(t *testing.T) {
	tests := []struct {
		name     string
		phone    string
		expected string
	}{
		{
			name:     "空手机号",
			phone:    "",
			expected: "",
		},
		{
			name:     "标准手机号",
			phone:    "13812345678",
			expected: "138****5678",
		},
		{
			name:     "其他运营商",
			phone:    "18612345678",
			expected: "186****5678",
		},
		{
			name:     "非手机号格式（太短）",
			phone:    "123456",
			expected: "123456",
		},
		{
			name:     "非手机号格式（太长）",
			phone:    "138123456789",
			expected: "138123456789",
		},
		{
			name:     "带国家代码",
			phone:    "+8613812345678",
			expected: "+8613812345678",
		},
		{
			name:     "座机号码（不以1开头）",
			phone:    "01012345678",
			expected: "01012345678",
		},
		{
			name:     "非数字字符",
			phone:    "138-1234-5678",
			expected: "138-1234-5678",
		},
		{
			name:     "无效号段（第二位为0）",
			phone:    "10012345678",
			expected: "10012345678",
		},
		{
			name:     "无效号段（第二位为1）",
			phone:    "11012345678",
			expected: "11012345678",
		},
		{
			name:     "无效号段（第二位为2）",
			phone:    "12012345678",
			expected: "12012345678",
		},
		{
			name:     "有效号段（第二位为3）",
			phone:    "13012345678",
			expected: "130****5678",
		},
		{
			name:     "有效号段（第二位为9）",
			phone:    "19012345678",
			expected: "190****5678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logging.SanitizePhone(tt.phone)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// SanitizeSecret 测试
// ============================================================================

func TestSanitizeSecret(t *testing.T) {
	// 永远返回固定掩码，避免泄露长度信息
	assert.Equal(t, "***", logging.SanitizeSecret())
}

// ============================================================================
// SanitizeDBURL 测试
// ============================================================================

func TestSanitizeDBURL(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		expected string
	}{
		{
			name:     "空DSN",
			dsn:      "",
			expected: "",
		},
		{
			name:     "标准DSN带密码",
			dsn:      "postgres://user:secret@localhost:5432/mydb?sslmode=require",
			expected: "postgres://user:***@localhost:5432/mydb?sslmode=require",
		},
		{
			name:     "DSN无密码",
			dsn:      "postgres://user@localhost:5432/mydb",
			expected: "postgres://user@localhost:5432/mydb",
		},
		{
			name:     "DSN带特殊字符密码（不含@符号）",
			dsn:      "postgres://admin:p%40ssw0rd!@host:5432/db",
			expected: "postgres://admin:***@host:5432/db",
		},
		{
			name: "纯字符串无法解析为URL",
			dsn:  "some random error message",
			// 阶段 D 修复（H5）：非 DSN 字符串原样返回，保留错误上下文
			expected: "some random error message",
		},
		{
			name: "key=value格式含password",
			dsn:  "password=secret user=admin",
			// sanitizeKeyValueDSN 识别 password= 子串，仅脱敏值
			expected: "password=*** user=admin",
		},
		{
			name: "key=value格式无password",
			dsn:  "host=localhost user=admin dbname=sso",
			// 不含 password 字段，原样返回
			expected: "host=localhost user=admin dbname=sso",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logging.SanitizeDBURL(tt.dsn)
			assert.Equal(t, tt.expected, result)
			// 关键断言：结果中不包含原始密码
			if tt.dsn != "" && result != "***" && result != "" {
				assert.NotContains(t, result, "secret",
					"DSN 脱敏后不应包含明文密码")
			}
		})
	}
}

// ============================================================================
// isSensitiveKey 测试（通过 SanitizeValue 行为间接验证）
// ============================================================================

func TestIsSensitiveKey(t *testing.T) {
	// 通过 SanitizeValue 的返回值间接验证 isSensitiveKey 行为
	// 敏感字段返回 "***"（或 token 类前缀），非敏感字段返回原值

	t.Run("敏感字段被脱敏", func(t *testing.T) {
		sensitiveKeys := []string{
			"password", "Password", "password_confirm",
			"oldPassword", "secret", "client_secret",
			"api_key", "private_key", "hmac_key",
			"access_token", "refresh_token", "token",
			"database_url", "dsn", "connection_string",
		}
		for _, key := range sensitiveKeys {
			result := logging.SanitizeValue(key, "value-to-sanitize")
			resultStr, ok := result.(string)
			require.True(t, ok, "key=%s 应返回字符串", key)
			assert.NotEqual(t, "value-to-sanitize", resultStr,
				"key=%s 应被识别为敏感字段并脱敏", key)
		}
	})

	t.Run("安全字段不脱敏", func(t *testing.T) {
		safeKeys := []string{
			"token_id", "token_type", "token_ttl",
			"access_token_ttl", "refresh_token_ttl",
			"refresh_token_length", "token_length",
			"token_prefix", "token_preview",
			"password_hash", "password_cost",
			"password_policy", "password_length",
			"secret_length",
		}
		for _, key := range safeKeys {
			result := logging.SanitizeValue(key, "safe-value")
			assert.Equal(t, "safe-value", result,
				"key=%s 应识别为安全字段，不脱敏", key)
		}
	})

	t.Run("普通字段不脱敏", func(t *testing.T) {
		normalKeys := []string{
			"user_id", "email", "ip_address",
			"client_id", "user_agent", "",
		}
		for _, key := range normalKeys {
			result := logging.SanitizeValue(key, "normal-value")
			assert.Equal(t, "normal-value", result,
				"key=%s 应识别为普通字段，不脱敏", key)
		}
	})
}

// ============================================================================
// SanitizeValue 测试
// ============================================================================

func TestSanitizeValue(t *testing.T) {
	t.Run("敏感字符串字段被脱敏", func(t *testing.T) {
		tests := []struct {
			key string
			val string
		}{
			{"password", "MySecret123!"},
			{"old_password", "OldPass456"},
			{"secret", "api-secret-value"},
			{"client_secret", "oauth-client-secret"},
			{"api_key", "AKIA...secret"},
			{"private_key", "-----BEGIN RSA PRIVATE KEY-----\n..."},
		}
		for _, tt := range tests {
			result := logging.SanitizeValue(tt.key, tt.val)
			// 敏感字符串字段应被完全隐藏
			assert.Equal(t, "***", result, "key=%s 应被脱敏", tt.key)
		}
	})

	t.Run("token类字段保留前缀", func(t *testing.T) {
		token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature"
		tests := []struct {
			key string
			val string
		}{
			{"access_token", token},
			{"refresh_token", token},
			{"token", token},
			{"auth_token", token},
		}
		for _, tt := range tests {
			result := logging.SanitizeValue(tt.key, tt.val)
			resultStr, ok := result.(string)
			require.True(t, ok)
			assert.Equal(t, "eyJhbGci...", resultStr, "key=%s 应保留前8位", tt.key)
		}
	})

	t.Run("database_url字段用SanitizeDBURL脱敏", func(t *testing.T) {
		dsn := "postgres://user:secret@localhost:5432/mydb"
		result := logging.SanitizeValue("database_url", dsn)
		resultStr, ok := result.(string)
		require.True(t, ok)
		assert.Equal(t, "postgres://user:***@localhost:5432/mydb", resultStr)
	})

	t.Run("非敏感字段保持原值", func(t *testing.T) {
		assert.Equal(t, "user@example.com", logging.SanitizeValue("email", "user@example.com"))
		assert.Equal(t, "192.168.1.1", logging.SanitizeValue("ip_address", "192.168.1.1"))
		assert.Equal(t, "client-123", logging.SanitizeValue("client_id", "client-123"))
	})

	t.Run("安全字段即使包含敏感词也不脱敏", func(t *testing.T) {
		assert.Equal(t, "tok-abc-123", logging.SanitizeValue("token_id", "tok-abc-123"))
		assert.Equal(t, "Bearer", logging.SanitizeValue("token_type", "Bearer"))
		assert.Equal(t, 3600, logging.SanitizeValue("access_token_ttl", 3600))
	})

	t.Run("非字符串敏感字段不脱敏", func(t *testing.T) {
		// 数值类型不脱敏
		assert.Equal(t, 42, logging.SanitizeValue("password_count", 42))
		assert.Equal(t, true, logging.SanitizeValue("secret_enabled", true))
	})

	t.Run("字符串指针类型", func(t *testing.T) {
		secret := "MySecret123!"
		result := logging.SanitizeValue("password", &secret)
		assert.Equal(t, "***", result)

		// nil 指针
		var nilPtr *string
		result = logging.SanitizeValue("password", nilPtr)
		assert.Nil(t, result)
	})
}

// ============================================================================
// LogSecurity 自动脱敏测试
// ============================================================================

func TestLogSecurity_AutoSanitization(t *testing.T) {
	// 测试 LogSecurity 调用 SanitizeValue 自动脱敏敏感字段
	var buf bytes.Buffer
	cfg := &logging.Config{
		Level:  "warn",
		Format: "text",
		Output: &buf,
	}
	require.NoError(t, logging.Init(cfg))

	details := map[string]interface{}{
		"ip":            "192.168.1.1",
		"reason":        "suspicious activity",
		"password":      "MySecret123!",
		"access_token":  "eyJhbGciOiJIUzI1NiJ9.payload.sig",
		"client_secret": "oauth-secret-value",
		"database_url":  "postgres://user:secret@host:5432/db",
		"token_id":      "tok-abc-123", // 安全字段，不脱敏
		"user_id":       "user-456",
	}
	logging.LogSecurity("test_event", details)

	output := buf.String()
	// 敏感字段应被脱敏
	assert.Contains(t, output, "***")
	// 不应包含明文敏感数据
	assert.NotContains(t, output, "MySecret123!")
	assert.NotContains(t, output, "oauth-secret-value")
	assert.NotContains(t, output, "postgres://user:secret@")
	// token 只显示前 8 位
	assert.Contains(t, output, "eyJhbGci...")
	assert.NotContains(t, output, "payload.sig")
	// 非敏感字段保留原值
	assert.Contains(t, output, "192.168.1.1")
	assert.Contains(t, output, "suspicious activity")
	assert.Contains(t, output, "tok-abc-123")
	assert.Contains(t, output, "user-456")
}
