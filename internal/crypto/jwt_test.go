// Package crypto_test JWT服务单元测试
package crypto_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/crypto"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestJWTService 创建测试用的JWT服务
func createTestJWTService(t *testing.T) *crypto.JWTService {
	// 生成测试用的RSA密钥对
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

// ============================================================================
// GenerateAccessToken 测试
// ============================================================================

func TestJWTService_GenerateAccessToken(t *testing.T) {
	svc := createTestJWTService(t)

	tests := []struct {
		name   string
		userID string
		email  string
		scopes []string
	}{
		{
			name:   "正常生成",
			userID: "user-123",
			email:  "test@example.com",
			scopes: []string{"openid", "profile", "email"},
		},
		{
			name:   "空scope",
			userID: "user-456",
			email:  "user@test.com",
			scopes: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := svc.GenerateAccessToken(tt.userID, tt.email, tt.scopes)

			require.NoError(t, err)
			assert.NotEmpty(t, token)
			// JWT应该是三段式结构
			assert.Contains(t, token, ".")
		})
	}
}

// ============================================================================
// GenerateRefreshToken 测试
// ============================================================================

func TestJWTService_GenerateRefreshToken(t *testing.T) {
	svc := createTestJWTService(t)

	// 生成多个Refresh Token，验证唯一性
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := svc.GenerateRefreshToken()
		require.NoError(t, err)
		assert.NotEmpty(t, token)
		// 验证唯一性
		assert.False(t, tokens[token], "生成了重复的Refresh Token")
		tokens[token] = true
	}
}

// ============================================================================
// ValidateAccessToken 测试
// ============================================================================

func TestJWTService_ValidateAccessToken(t *testing.T) {
	svc := createTestJWTService(t)

	// 生成有效的Token
	validToken, err := svc.GenerateAccessToken("user-123", "test@example.com", []string{"openid"})
	require.NoError(t, err)

	tests := []struct {
		name      string
		token     string
		wantErr   bool
		errType   error
		checkFunc func(t *testing.T, claims *crypto.AccessTokenClaims)
	}{
		{
			name:    "有效Token",
			token:   validToken,
			wantErr: false,
			checkFunc: func(t *testing.T, claims *crypto.AccessTokenClaims) {
				assert.Equal(t, "user-123", claims.RegisteredClaims.Subject)
				assert.Equal(t, "test@example.com", claims.Email)
				assert.Equal(t, "test-issuer", claims.RegisteredClaims.Issuer)
			},
		},
		{
			name:    "无效Token",
			token:   "invalid.token.here",
			wantErr: true,
			errType: crypto.ErrInvalidToken,
		},
		{
			name:    "空Token",
			token:   "",
			wantErr: true,
			errType: crypto.ErrInvalidToken,
		},
		{
			name:    "篡改的Token",
			token:   validToken + "tampered",
			wantErr: true,
			errType: crypto.ErrInvalidToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := svc.ValidateAccessToken(tt.token)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, claims)
			if tt.checkFunc != nil {
				tt.checkFunc(t, claims)
			}
		})
	}
}

// ============================================================================
// 过期Token测试
// ============================================================================

func TestJWTService_ExpiredToken(t *testing.T) {
	// 创建一个过期时间很短的JWT服务
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	svc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		1*time.Millisecond, // 1毫秒过期
		7*24*time.Hour,
	)

	// 生成Token
	token, err := svc.GenerateAccessToken("user-123", "test@example.com", []string{"openid"})
	require.NoError(t, err)

	// 等待Token过期
	time.Sleep(10 * time.Millisecond)

	// 验证应该失败
	_, err = svc.ValidateAccessToken(token)
	assert.Error(t, err)
	assert.ErrorIs(t, err, crypto.ErrTokenExpired)
}

// ============================================================================
// 公钥获取测试
// ============================================================================

func TestJWTService_GetPublicKey(t *testing.T) {
	svc := createTestJWTService(t)

	pubKey := svc.GetPublicKey()
	assert.NotNil(t, pubKey)
}

// ============================================================================
// Token有效期测试
// ============================================================================

func TestJWTService_GetAccessTokenTTL(t *testing.T) {
	svc := createTestJWTService(t)

	ttl := svc.GetAccessTokenTTL()
	assert.Equal(t, 15*time.Minute, ttl)
}

// ============================================================================
// 算法验证测试
// ============================================================================

func TestJWTService_WrongAlgorithm(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	svc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 使用 HS256 算法生成的 Token（不是 RS256）
	// 这模拟攻击者尝试使用其他算法
	hs256Token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLTEyMyIsImVtYWlsIjoidGVzdEBleGFtcGxlLmNvbSIsInNjb3BlIjpbIm9wZW5pZCJdfQ.invalid-signature"

	_, err = svc.ValidateAccessToken(hs256Token)
	assert.ErrorIs(t, err, crypto.ErrInvalidToken)
}

func TestJWTService_DifferentKeyValidation(t *testing.T) {
	// 用一个密钥生成 Token
	privateKey1, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	svc1 := crypto.NewJWTService(
		privateKey1,
		&privateKey1.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 用另一个密钥验证
	privateKey2, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	svc2 := crypto.NewJWTService(
		privateKey2,
		&privateKey2.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 用 svc1 生成 Token
	token, err := svc1.GenerateAccessToken("user-123", "test@example.com", []string{"openid"})
	require.NoError(t, err)

	// 用 svc2 验证应该失败
	_, err = svc2.ValidateAccessToken(token)
	assert.ErrorIs(t, err, crypto.ErrInvalidToken)
}

// ============================================================================
// nil claims 测试
// ============================================================================

func TestJWTService_ValidateMalformedClaims(t *testing.T) {
	svc := createTestJWTService(t)

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "非JWT格式",
			token: "not-a-jwt-token",
		},
		{
			name:  "部分JWT",
			token: "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0",
		},
		{
			name:  "包含特殊字符",
			token: "eyJ@#$%^&*()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ValidateAccessToken(tt.token)
			assert.ErrorIs(t, err, crypto.ErrInvalidToken)
		})
	}
}
