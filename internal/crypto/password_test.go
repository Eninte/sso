// Package crypto_test 密码服务单元测试
package crypto_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/crypto"
)

// ============================================================================
// HashPassword 测试
// ============================================================================

func TestPasswordService_HashPassword(t *testing.T) {
	svc := crypto.NewPasswordService(4)

	tests := []struct {
		name     string
		password string
		wantErr  bool
		errType  error
	}{
		{
			name:     "正常密码",
			password: "SecureP@ss123",
			wantErr:  false,
		},
		{
			name:     "最短密码 (8字符)",
			password: "12345678",
			wantErr:  false,
		},
		{
			name:     "空密码",
			password: "",
			wantErr:  true,
			errType:  crypto.ErrPasswordTooShort,
		},
		{
			name:     "过短密码 (7字符)",
			password: "1234567",
			wantErr:  true,
			errType:  crypto.ErrPasswordTooShort,
		},
		{
			name:     "过长密码 (73字符)",
			password: strings.Repeat("a", 73),
			wantErr:  true,
			errType:  crypto.ErrPasswordTooLong,
		},
		{
			name:     "最大长度密码 (72字符)",
			password: strings.Repeat("a", 72),
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := svc.HashPassword(tt.password)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, hash)
			// 哈希值不应该等于原始密码
			assert.NotEqual(t, tt.password, hash)
			// 哈希值应该以 $2a$ (bcrypt标识) 开头
			assert.True(t, strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$"))
		})
	}
}

// ============================================================================
// VerifyPassword 测试
// ============================================================================

func TestPasswordService_VerifyPassword(t *testing.T) {
	svc := crypto.NewPasswordService(4)

	password := "SecureP@ss123"
	hash, err := svc.HashPassword(password)
	require.NoError(t, err)

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "正确密码",
			password: password,
			wantErr:  false,
		},
		{
			name:     "错误密码",
			password: "WrongPassword123",
			wantErr:  true,
		},
		{
			name:     "空密码",
			password: "",
			wantErr:  true,
		},
		{
			name:     "大小写不同",
			password: "securepass123",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.VerifyPassword(hash, tt.password)

			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, crypto.ErrPasswordMismatch)
				return
			}

			assert.NoError(t, err)
		})
	}
}

// ============================================================================
// 密码服务创建测试
// ============================================================================

func TestNewPasswordService_CostNormalization(t *testing.T) {
	tests := []struct {
		name         string
		inputCost    int
		expectedCost int
	}{
		{"过低cost提升到12", 1, 12},
		{"低于12提升到12", 5, 12},
		{"正常cost不变", 12, 12},
		{"13不变", 13, 13},
		{"过高cost被限制到14", 20, 14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crypto.NormalizeBcryptCost(tt.inputCost)
			assert.Equal(t, tt.expectedCost, got)
		})
	}
}

// ============================================================================
// 边界测试
// ============================================================================

func TestPasswordService_UnicodePassword(t *testing.T) {
	svc := crypto.NewPasswordService(4)

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "中文密码",
			password: "密码测试1234",
			wantErr:  false,
		},
		{
			name:     "日文密码",
			password: "パスワード1234",
			wantErr:  false,
		},
		{
			name:     "emoji密码",
			password: "🔐密码1234",
			wantErr:  false,
		},
		{
			name:     "混合Unicode",
			password: "Hello世界こんにちは🔒",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := svc.HashPassword(tt.password)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, hash)

			// 验证密码可以正确匹配
			err = svc.VerifyPassword(hash, tt.password)
			assert.NoError(t, err)
		})
	}
}

func TestPasswordService_VerifyPassword_InvalidHash(t *testing.T) {
	svc := crypto.NewPasswordService(4)

	tests := []struct {
		name           string
		hashedPassword string
		password       string
	}{
		{
			name:           "无效的哈希格式",
			hashedPassword: "invalid-hash-format",
			password:       "TestPassword123",
		},
		{
			name:           "空哈希",
			hashedPassword: "",
			password:       "TestPassword123",
		},
		{
			name:           "非bcrypt哈希",
			hashedPassword: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHQ$hash",
			password:       "TestPassword123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.VerifyPassword(tt.hashedPassword, tt.password)
			assert.ErrorIs(t, err, crypto.ErrPasswordMismatch)
		})
	}
}

func TestPasswordService_SpecialCharacters(t *testing.T) {
	svc := crypto.NewPasswordService(4)

	tests := []struct {
		name     string
		password string
	}{
		{
			name:     "特殊字符",
			password: "P@ss!#$%^&*()_+-=[]{}|;:',.<>?/",
		},
		{
			name:     "空格密码",
			password: "Pass word 123",
		},
		{
			name:     "制表符",
			password: "Pass\tword\n123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := svc.HashPassword(tt.password)
			require.NoError(t, err)
			assert.NotEmpty(t, hash)

			// 验证密码
			err = svc.VerifyPassword(hash, tt.password)
			assert.NoError(t, err)

			// 验证错误密码不匹配
			err = svc.VerifyPassword(hash, tt.password+"wrong")
			assert.ErrorIs(t, err, crypto.ErrPasswordMismatch)
		})
	}
}
