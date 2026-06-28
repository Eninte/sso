// Package logging_test 日志脱敏单元测试
package logging_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
