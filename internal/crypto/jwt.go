// Package crypto JWT服务
// 负责JWT Token的签发和验证
package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	apperrors "github.com/your-org/sso/internal/errors"
)

// ============================================================================
// JWT相关错误
// ============================================================================

var (
	ErrInvalidToken = apperrors.ErrInvalidToken
	ErrTokenExpired = apperrors.ErrTokenExpired
)

// ============================================================================
// JWTService JWT服务
// ============================================================================

// JWTService JWT服务
// 负责JWT Token的签发和验证
type JWTService struct {
	privateKey      *rsa.PrivateKey // RSA私钥，用于签名
	publicKey       *rsa.PublicKey  // RSA公钥，用于验证
	issuer          string          // 签发者标识
	accessTokenTTL  time.Duration   // Access Token有效期
	refreshTokenTTL time.Duration   // Refresh Token有效期
}

// NewJWTService 创建JWT服务
func NewJWTService(
	privateKey *rsa.PrivateKey,
	publicKey *rsa.PublicKey,
	issuer string,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
) *JWTService {
	return &JWTService{
		privateKey:      privateKey,
		publicKey:       publicKey,
		issuer:          issuer,
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
	}
}

// ============================================================================
// Token声明结构
// ============================================================================

// AccessTokenClaims Access Token声明
// 包含用户身份信息和权限范围
type AccessTokenClaims struct {
	jwt.RegisteredClaims
	Email  string   `json:"email"` // 用户邮箱
	Scopes []string `json:"scope"` // 权限范围
}

// ============================================================================
// Token生成方法
// ============================================================================

// GenerateAccessToken 生成Access Token
// 使用RS256算法签名，包含用户身份信息
func (s *JWTService) GenerateAccessToken(userID, email string, scopes []string) (string, error) {
	now := time.Now()
	claims := AccessTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,                                      // 签发者
			Subject:   userID,                                        // 用户ID
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenTTL)), // 过期时间
			IssuedAt:  jwt.NewNumericDate(now),                       // 签发时间
			NotBefore: jwt.NewNumericDate(now),                       // 生效时间
		},
		Email:  email,
		Scopes: scopes,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.privateKey)
}

// GenerateRefreshToken 生成Refresh Token
// 使用随机字符串，不包含用户信息
// Refresh Token仅用于获取新的Access Token
func (s *JWTService) GenerateRefreshToken() (string, error) {
	// 生成32字节的随机数据
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	// 使用URL安全的Base64编码
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// ============================================================================
// Token验证方法
// ============================================================================

// ValidateAccessToken 验证Access Token
// 验证签名、过期时间和格式
func (s *JWTService) ValidateAccessToken(tokenString string) (*AccessTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AccessTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 严格验证签名算法必须是RS256
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, ErrInvalidToken
		}
		return s.publicKey, nil
	})

	if err != nil {
		// 检查是否为过期错误
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*AccessTokenClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ============================================================================
// 公钥方法
// ============================================================================

// GetPublicKey 获取公钥 (用于JWKS端点)
func (s *JWTService) GetPublicKey() *rsa.PublicKey {
	return s.publicKey
}

// GetAccessTokenTTL 获取Access Token有效期
func (s *JWTService) GetAccessTokenTTL() time.Duration {
	return s.accessTokenTTL
}
