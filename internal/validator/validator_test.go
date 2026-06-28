// Package validator_test 验证器单元测试
package validator_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/example/sso/internal/validator"
)

// ============================================================================
// ValidateEmail 测试
// ============================================================================

func TestValidateEmail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		email   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "有效邮箱",
			email:   "user@example.com",
			wantErr: false,
		},
		{
			name:    "带+号的邮箱",
			email:   "user+tag@example.com",
			wantErr: false,
		},
		{
			name:    "带子域名的邮箱",
			email:   "user@mail.example.com",
			wantErr: false,
		},
		{
			name:    "空邮箱",
			email:   "",
			wantErr: true,
			errMsg:  "邮箱地址不能为空",
		},
		{
			name:    "无效格式 - 无@",
			email:   "userexample.com",
			wantErr: true,
			errMsg:  "邮箱地址格式无效",
		},
		{
			name:    "无效格式 - 无域名",
			email:   "user@",
			wantErr: true,
			errMsg:  "邮箱地址格式无效",
		},
		{
			name:    "无效格式 - 无用户名",
			email:   "@example.com",
			wantErr: true,
			errMsg:  "邮箱地址格式无效",
		},
		{
			name:    "带空格的邮箱",
			email:   " user@example.com ",
			wantErr: false, // 应该trim空格
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validator.ValidateEmail(tt.email)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			assert.NoError(t, err)
		})
	}
}

// ============================================================================
// ValidatePassword 测试
// ============================================================================

func TestValidatePassword(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errType  error
		errMsg   string
	}{
		{
			name:     "有效密码 - 完整复杂度",
			password: "SecureP@ss123",
			wantErr:  false,
		},
		{
			name:     "有效密码 - 最低复杂度",
			password: "Pass123!",
			wantErr:  false,
		},
		{
			name:     "空密码",
			password: "",
			wantErr:  true,
			errType:  validator.ErrPasswordRequired,
		},
		{
			name:     "过短密码",
			password: "Pa1!",
			wantErr:  true,
			errType:  validator.ErrPasswordTooShort,
		},
		{
			name:     "过长密码",
			password: "SecureP@ss123456789012345678901234567890123456789012345678901234567890123", // 73字符
			wantErr:  true,
			errType:  validator.ErrPasswordTooLong,
		},
		{
			name:     "缺少大写字母",
			password: "securepass123!",
			wantErr:  true,
			errType:  validator.ErrPasswordNoUppercase,
		},
		{
			name:     "缺少小写字母",
			password: "SECUREPASS123!",
			wantErr:  true,
			errType:  validator.ErrPasswordNoLowercase,
		},
		{
			name:     "缺少数字",
			password: "SecurePass!",
			wantErr:  true,
			errType:  validator.ErrPasswordNoDigit,
		},
		{
			name:     "缺少特殊字符",
			password: "SecurePass123",
			wantErr:  true,
			errType:  validator.ErrPasswordNoSpecial,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validator.ValidatePassword(tt.password)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			assert.NoError(t, err)
		})
	}
}

// ============================================================================
// ValidatePasswordSimple 测试
// ============================================================================

func TestValidatePasswordSimple(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errType  error
	}{
		{
			name:     "有效密码 - 仅长度检查",
			password: "simplepassword",
			wantErr:  false,
		},
		{
			name:     "空密码",
			password: "",
			wantErr:  true,
			errType:  validator.ErrPasswordRequired,
		},
		{
			name:     "过短密码",
			password: "1234567",
			wantErr:  true,
			errType:  validator.ErrPasswordTooShort,
		},
		{
			name:     "过长密码",
			password: "1234567890123456789012345678901234567890123456789012345678901234567890123",
			wantErr:  true,
			errType:  validator.ErrPasswordTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validator.ValidatePasswordSimple(tt.password)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				return
			}

			assert.NoError(t, err)
		})
	}
}

// ============================================================================
// ValidateRegisterRequest 测试
// ============================================================================

func TestValidateRegisterRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		email    string
		password string
		wantErr  bool
	}{
		{
			name:     "有效请求",
			email:    "user@example.com",
			password: "SecureP@ss123",
			wantErr:  false,
		},
		{
			name:     "无效邮箱",
			email:    "invalid",
			password: "SecureP@ss123",
			wantErr:  true,
		},
		{
			name:     "无效密码",
			email:    "user@example.com",
			password: "123",
			wantErr:  true,
		},
		{
			name:     "都无效",
			email:    "",
			password: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validator.ValidateRegisterRequest(tt.email, tt.password)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}

// ============================================================================
// ValidateLoginRequest 测试
// ============================================================================

func TestValidateLoginRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		email    string
		password string
		wantErr  bool
	}{
		{
			name:     "有效请求",
			email:    "user@example.com",
			password: "anypassword",
			wantErr:  false,
		},
		{
			name:     "无效邮箱",
			email:    "invalid",
			password: "anypassword",
			wantErr:  true,
		},
		{
			name:     "空密码",
			email:    "user@example.com",
			password: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validator.ValidateLoginRequest(tt.email, tt.password)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}
