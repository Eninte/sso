// Package logging_test 日志脱敏单元测试
package logging_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/your-org/sso/internal/logging"
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
			name:     "标准邮箱",
			email:    "user@example.com",
			expected: "use***@example.com",
		},
		{
			name:     "短用户名",
			email:    "ab@example.com",
			expected: "ab***@example.com",
		},
		{
			name:     "单字符用户名",
			email:    "a@example.com",
			expected: "a***@example.com",
		},
		{
			name:     "长用户名",
			email:    "john.doe@example.com",
			expected: "joh***@example.com",
		},
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
			name:     "多级域名",
			email:    "user@mail.example.com",
			expected: "use***@mail.example.com",
		},
		{
			name:     "带点的用户名",
			email:    "first.last@example.com",
			expected: "fir***@example.com",
		},
		{
			name:     "带加号的用户名",
			email:    "user+tag@example.com",
			expected: "use***@example.com",
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
			name:     "长Token",
			token:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected: "eyJhbGci...",
		},
		{
			name:     "正好8字符",
			token:    "12345678",
			expected: "***",
		},
		{
			name:     "短Token",
			token:    "abc",
			expected: "***",
		},
		{
			name:     "空Token",
			token:    "",
			expected: "***",
		},
		{
			name:     "9字符Token",
			token:    "123456789",
			expected: "12345678...",
		},
		{
			name:     "JWT格式",
			token:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
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
			name:     "空手机号",
			phone:    "",
			expected: "",
		},
		{
			name:     "非手机号格式",
			phone:    "123456",
			expected: "123456",
		},
		{
			name:     "带国家代码",
			phone:    "+8613812345678",
			expected: "+8613812345678",
		},
		{
			name:     "座机号码",
			phone:    "01012345678",
			expected: "01012345678",
		},
		{
			name:     "非数字字符",
			phone:    "138-1234-5678",
			expected: "138-1234-5678",
		},
		{
			name:     "11位但非1开头",
			phone:    "23812345678",
			expected: "23812345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logging.SanitizePhone(tt.phone)
			assert.Equal(t, tt.expected, result)
		})
	}
}
