// Package crypto_test JWT服务单元测试
package crypto_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
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
			token, err := svc.GenerateAccessToken(tt.userID, tt.email, "user", tt.scopes)

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
	validToken, err := svc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
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
	token, err := svc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
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
	token, err := svc1.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
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

// ============================================================================
// 密钥管理测试
// ============================================================================

func TestJWTService_SetActiveKey(t *testing.T) {
	svc := createTestJWTService(t)

	// 生成新的密钥对
	newPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// 设置新的活跃密钥
	newKeyID := "new-key-123"
	err = svc.SetActiveKey(newKeyID, newPrivateKey, &newPrivateKey.PublicKey)
	require.NoError(t, err)

	// 验证活跃密钥ID已更新
	assert.Equal(t, newKeyID, svc.GetActiveKeyID())
}

func TestJWTService_SetActiveKey_InvalidParams(t *testing.T) {
	svc := createTestJWTService(t)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// 空keyID
	err = svc.SetActiveKey("", privateKey, &privateKey.PublicKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "密钥ID")

	// nil privateKey
	err = svc.SetActiveKey("key-123", nil, &privateKey.PublicKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "私钥")

	// nil publicKey
	err = svc.SetActiveKey("key-123", privateKey, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "公钥")
}

func TestJWTService_AddVerificationKey(t *testing.T) {
	svc := createTestJWTService(t)

	// 生成新的公钥
	newPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// 添加验证密钥
	newKeyID := "verify-key-123"
	svc.AddVerificationKey(newKeyID, &newPrivateKey.PublicKey)

	// 验证公钥已添加
	publicKeys := svc.GetPublicKeys()
	assert.Contains(t, publicKeys, newKeyID)
}

func TestJWTService_RemoveKey(t *testing.T) {
	svc := createTestJWTService(t)

	// 先设置一个活跃密钥
	keyID := "test-key-123"
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	err = svc.SetActiveKey(keyID, privateKey, &privateKey.PublicKey)
	require.NoError(t, err)

	// 删除密钥
	svc.RemoveKey(keyID)

	// 验证密钥已删除
	assert.Empty(t, svc.GetActiveKeyID())
	publicKeys := svc.GetPublicKeys()
	assert.NotContains(t, publicKeys, keyID)
}

func TestJWTService_GenerateAccessToken_NoActiveKey(t *testing.T) {
	// 使用NewJWTService但不设置活跃密钥
	svc := crypto.NewJWTService(
		nil, // 没有私钥
		nil, // 没有公钥
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 尝试生成Token应该失败
	_, err := svc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
	assert.ErrorIs(t, err, crypto.ErrNoActiveKey)
}

func TestJWTService_GenerateAccessTokenWithKeyID(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	svc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	keyID := "custom-key-123"
	err = svc.SetActiveKey(keyID, privateKey, &privateKey.PublicKey)
	require.NoError(t, err)

	// 使用指定的keyID生成Token
	token, err := svc.GenerateAccessTokenWithKeyID("user-123", "test@example.com", "user", []string{"openid"}, keyID)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// 验证Token可以被验证
	claims, err := svc.ValidateAccessToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.Subject)
}

func TestJWTService_GetPublicKeys(t *testing.T) {
	svc := createTestJWTService(t)

	// 添加多个公钥
	key1, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	key2, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	svc.AddVerificationKey("key1", &key1.PublicKey)
	svc.AddVerificationKey("key2", &key2.PublicKey)

	publicKeys := svc.GetPublicKeys()
	assert.Len(t, publicKeys, 2)
}

func TestJWTService_GetJWKS(t *testing.T) {
	svc := createTestJWTService(t)

	jwks := svc.GetJWKS()
	assert.NotNil(t, jwks)
	assert.Contains(t, jwks, "keys")
}

// ============================================================================
// 工具函数测试
// ============================================================================

func TestDeriveKeyID(t *testing.T) {
	t.Run("同一密钥多次派生kid恒定", func(t *testing.T) {
		key, err := crypto.GenerateRSAKeyPair(2048)
		require.NoError(t, err)

		id1 := crypto.DeriveKeyID(&key.PublicKey)
		id2 := crypto.DeriveKeyID(&key.PublicKey)
		assert.NotEmpty(t, id1)
		assert.Equal(t, id1, id2, "同一密钥内容应恒定派生同一 kid（跨重启稳定）")
	})

	t.Run("不同密钥派生不同kid", func(t *testing.T) {
		key1, err := crypto.GenerateRSAKeyPair(2048)
		require.NoError(t, err)
		key2, err := crypto.GenerateRSAKeyPair(2048)
		require.NoError(t, err)

		assert.NotEqual(t,
			crypto.DeriveKeyID(&key1.PublicKey),
			crypto.DeriveKeyID(&key2.PublicKey),
			"kid 与密钥内容绑定，不同密钥必须不同 kid")
	})

	t.Run("kid为16字符base64url", func(t *testing.T) {
		key, err := crypto.GenerateRSAKeyPair(2048)
		require.NoError(t, err)

		id := crypto.DeriveKeyID(&key.PublicKey)
		assert.Len(t, id, 16, "kid 应为 thumbprint base64url 的前 16 字符")
		assert.Regexp(t, `^[A-Za-z0-9_-]{16}$`, id, "kid 应为无填充 base64url 字符集")
	})

	t.Run("nil或缺模数返回空", func(t *testing.T) {
		assert.Empty(t, crypto.DeriveKeyID(nil))
		assert.Empty(t, crypto.DeriveKeyID(&rsa.PublicKey{E: 65537}))
	})

	t.Run("RFC7638标准测试向量", func(t *testing.T) {
		// RFC 7638 §3.1 示例 RSA 公钥，其 JWK Thumbprint（SHA-256）为
		// NzbLsXh8uDCcd-6MNwXF4W_7noWXFZAfHkxZsRGC9Xs（base64url，43 字符）
		// 本项目 kid 取其前 16 字符
		nBytes, err := base64.RawURLEncoding.DecodeString("0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n91CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINHaQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw")
		require.NoError(t, err)

		pub := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: 65537}
		assert.Equal(t, "NzbLsXh8uDCcd-6M", crypto.DeriveKeyID(pub),
			"应与 RFC 7638 §3.1 thumbprint 的前 16 字符一致")
	})
}

func TestGenerateRSAKeyPair(t *testing.T) {
	key, err := crypto.GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	assert.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
}

func TestEncodePrivateKeyToPEM(t *testing.T) {
	key, err := crypto.GenerateRSAKeyPair(2048)
	require.NoError(t, err)

	pemData := crypto.EncodePrivateKeyToPEM(key)
	assert.NotEmpty(t, pemData)
	assert.Contains(t, string(pemData), "BEGIN PRIVATE KEY")
}

func TestEncodePublicKeyToPEM(t *testing.T) {
	key, err := crypto.GenerateRSAKeyPair(2048)
	require.NoError(t, err)

	pemData := crypto.EncodePublicKeyToPEM(&key.PublicKey)
	assert.NotEmpty(t, pemData)
	assert.Contains(t, string(pemData), "BEGIN PUBLIC KEY")
}

func TestCreateKeyVersion(t *testing.T) {
	key, err := crypto.GenerateRSAKeyPair(2048)
	require.NoError(t, err)

	keyVersion, err := crypto.CreateKeyVersion(key)
	require.NoError(t, err)
	assert.NotNil(t, keyVersion)
	assert.NotEmpty(t, keyVersion.ID)
	assert.NotEmpty(t, keyVersion.PublicKey)
	assert.NotEmpty(t, keyVersion.PrivateKey)

	// T16：ID 从公钥内容派生，与 DeriveKeyID 一致且幂等
	assert.Equal(t, crypto.DeriveKeyID(&key.PublicKey), keyVersion.ID,
		"KeyVersion.ID 应为公钥内容的 RFC 7638 thumbprint 派生值")
	keyVersion2, err := crypto.CreateKeyVersion(key)
	require.NoError(t, err)
	assert.Equal(t, keyVersion.ID, keyVersion2.ID,
		"同一密钥重复创建版本应得到同一 ID（幂等）")
}

// ============================================================================
// EncodePrivateKeyToPKCS1PEM 测试
// ============================================================================

func TestEncodePrivateKeyToPKCS1PEM(t *testing.T) {
	key, err := crypto.GenerateRSAKeyPair(2048)
	require.NoError(t, err)

	pemData := crypto.EncodePrivateKeyToPKCS1PEM(key)
	assert.NotEmpty(t, pemData)
	assert.Contains(t, string(pemData), "BEGIN RSA PRIVATE KEY")
}

// ============================================================================
// NewJWTServiceWithKeyStore 测试
// ============================================================================

func TestNewJWTServiceWithKeyStore(t *testing.T) {
	t.Run("创建带KeyStore的JWT服务", func(t *testing.T) {
		svc := crypto.NewJWTServiceWithKeyStore(
			nil, // keyStore为nil
			"test-issuer",
			15*time.Minute,
			7*24*time.Hour,
		)
		assert.NotNil(t, svc)
	})
}

// ============================================================================
// LoadKeysFromStore 测试
// ============================================================================

func TestJWTService_LoadKeysFromStore(t *testing.T) {
	t.Run("keyStore为nil时正常返回", func(t *testing.T) {
		svc := crypto.NewJWTServiceWithKeyStore(
			nil,
			"test-issuer",
			15*time.Minute,
			7*24*time.Hour,
		)

		err := svc.LoadKeysFromStore(context.Background())
		assert.NoError(t, err)
	})
}

// ============================================================================
// Issuer 校验测试
// ============================================================================

func TestJWTService_ValidateAccessToken_RejectsWrongIssuer(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// 用 issuer-A 生成 token
	svcA := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"issuer-A",
		15*time.Minute,
		7*24*time.Hour,
	)
	token, err := svcA.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
	require.NoError(t, err)

	// 用 issuer-B 验证，应被拒绝（同一密钥但 issuer 不同）
	svcB := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"issuer-B",
		15*time.Minute,
		7*24*time.Hour,
	)
	_, err = svcB.ValidateAccessToken(token)
	assert.ErrorIs(t, err, crypto.ErrInvalidToken)
}

func TestJWTService_ValidateAccessToken_AcceptsCorrectIssuer(t *testing.T) {
	svc := createTestJWTService(t) // issuer = "test-issuer"
	token, err := svc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
	require.NoError(t, err)

	claims, err := svc.ValidateAccessToken(token)
	require.NoError(t, err)
	assert.Equal(t, "test-issuer", claims.RegisteredClaims.Issuer)
}
