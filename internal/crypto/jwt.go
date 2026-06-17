// Package crypto JWT服务
// 负责JWT Token的签发和验证
package crypto

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
)

var (
	ErrInvalidToken  = apperrors.ErrInvalidToken
	ErrTokenExpired  = apperrors.ErrTokenExpired
	ErrNoActiveKey   = apperrors.ErrNoActiveKey
	ErrTokenReplayed = apperrors.New("ERR_TOKEN_REPLAYED", "Token已被使用，可能是重放攻击", 401)
)

// JTITracker JTI跟踪接口
// 用于防止JWT重放攻击
type JTITracker interface {
	// IsJTIUsed 检查JTI是否已被使用
	IsJTIUsed(ctx context.Context, jti string) (bool, error)
	// MarkJTIUsed 标记JTI为已使用
	// ttl: JTI的有效期，应该与token的有效期一致
	MarkJTIUsed(ctx context.Context, jti string, ttl time.Duration) error
	// CheckAndMarkUsed 原子性检查并标记JTI为已使用
	// 返回true表示JTI已被使用过（重放攻击），false表示首次使用
	CheckAndMarkUsed(ctx context.Context, jti string, ttl time.Duration) (bool, error)
}

type JWTService struct {
	mu              sync.RWMutex
	privateKey      *rsa.PrivateKey
	publicKey       *rsa.PublicKey
	keys            map[string]*rsa.PrivateKey
	publicKeys      map[string]*rsa.PublicKey
	activeKeyID     string
	keyStore        store.KeyStore
	jtiTracker      JTITracker // JTI跟踪器（可选）
	issuer          string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

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
		keys:            make(map[string]*rsa.PrivateKey),
		publicKeys:      make(map[string]*rsa.PublicKey),
		issuer:          issuer,
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
	}
}

func NewJWTServiceWithKeyStore(
	keyStore store.KeyStore,
	issuer string,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
) *JWTService {
	return &JWTService{
		keys:            make(map[string]*rsa.PrivateKey),
		publicKeys:      make(map[string]*rsa.PublicKey),
		keyStore:        keyStore,
		issuer:          issuer,
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
	}
}

// SetJTITracker 设置JTI跟踪器
// 用于防止JWT重放攻击
func (s *JWTService) SetJTITracker(tracker JTITracker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jtiTracker = tracker
}

func (s *JWTService) SetActiveKey(keyID string, privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey) error {
	if keyID == "" {
		return apperrors.ErrKeyIDEmpty
	}
	if privateKey == nil {
		return apperrors.ErrPrivateKeyNil
	}
	if publicKey == nil {
		return apperrors.ErrPublicKeyNil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[keyID] = privateKey
	s.publicKeys[keyID] = publicKey
	s.activeKeyID = keyID
	s.privateKey = privateKey
	s.publicKey = publicKey
	return nil
}

func (s *JWTService) AddVerificationKey(keyID string, publicKey *rsa.PublicKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publicKeys[keyID] = publicKey
}

func (s *JWTService) RemoveKey(keyID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, keyID)
	delete(s.publicKeys, keyID)
	if s.activeKeyID == keyID {
		s.activeKeyID = ""
	}
}

func (s *JWTService) GetActiveKeyID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeKeyID
}

func (s *JWTService) LoadKeysFromStore(ctx context.Context) error {
	if s.keyStore == nil {
		return nil
	}

	keys, err := s.keyStore.ListActiveKeys(ctx)
	if err != nil {
		return fmt.Errorf("failed to load keys from store: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, keyVersion := range keys {
		if !keyVersion.CanVerify() {
			continue
		}

		pubKey, err := ParsePublicKey(keyVersion.PublicKey)
		if err != nil {
			continue
		}

		s.publicKeys[keyVersion.ID] = pubKey

		if keyVersion.IsActive() && len(keyVersion.PrivateKey) > 0 {
			privKey, err := ParsePrivateKey(keyVersion.PrivateKey)
			if err == nil {
				s.keys[keyVersion.ID] = privKey
				s.activeKeyID = keyVersion.ID
				s.privateKey = privKey
				s.publicKey = pubKey
			}
		}
	}

	return nil
}

type AccessTokenClaims struct {
	jwt.RegisteredClaims
	KeyID  string   `json:"kid,omitempty"`
	Email  string   `json:"email"`
	Scopes []string `json:"scope"`
	Role   string   `json:"role,omitempty"`
}

// GenerateAccessToken 生成访问令牌
// 使用当前活跃密钥签名
func (s *JWTService) GenerateAccessToken(userID, email, role string, scopes []string) (string, error) {
	s.mu.RLock()
	activeKeyID := s.activeKeyID
	s.mu.RUnlock()
	return s.GenerateAccessTokenWithKeyID(userID, email, role, scopes, activeKeyID)
}

// GenerateAccessTokenWithKeyID 使用指定密钥生成访问令牌
// userID: 用户唯一标识
// email: 用户邮箱
// role: 用户角色
// scopes: 授权范围
// keyID: 指定的密钥ID，为空时使用活跃密钥
func (s *JWTService) GenerateAccessTokenWithKeyID(userID, email, role string, scopes []string, keyID string) (string, error) {
	s.mu.RLock()
	var privateKey *rsa.PrivateKey
	if keyID != "" {
		var ok bool
		privateKey, ok = s.keys[keyID]
		if !ok {
			privateKey = s.privateKey
			keyID = s.activeKeyID
		}
	} else {
		privateKey = s.privateKey
		keyID = s.activeKeyID
	}
	issuer := s.issuer
	accessTokenTTL := s.accessTokenTTL
	s.mu.RUnlock()

	if privateKey == nil {
		return "", ErrNoActiveKey
	}

	// 生成唯一的jti（JWT ID）确保token唯一性
	jtiBytes := make([]byte, 16)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", fmt.Errorf("生成jti失败: %w", err)
	}
	jti := base64.URLEncoding.EncodeToString(jtiBytes)

	now := time.Now()
	claims := AccessTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Issuer:    issuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
		KeyID:  keyID,
		Email:  email,
		Scopes: scopes,
		Role:   role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if keyID != "" {
		token.Header["kid"] = keyID
	}
	return token.SignedString(privateKey)
}

// GenerateRefreshToken 生成刷新令牌
// 使用密码学安全的随机数生成器生成32字节的随机令牌
func (s *JWTService) GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// ValidateAccessToken 验证访问令牌并返回Claims
// 验证签名、过期时间、算法、密钥过期状态和JTI重放
// 返回解析后的Claims或错误
func (s *JWTService) ValidateAccessToken(tokenString string) (*AccessTokenClaims, error) {
	var usedKeyID string
	token, err := jwt.ParseWithClaims(tokenString, &AccessTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, ErrInvalidToken
		}

		s.mu.RLock()
		defer s.mu.RUnlock()

		kid, _ := token.Header["kid"].(string)
		if kid != "" {
			if pubKey, ok := s.publicKeys[kid]; ok {
				usedKeyID = kid
				return pubKey, nil
			}
		}

		if s.publicKey != nil {
			usedKeyID = s.activeKeyID
			return s.publicKey, nil
		}

		return nil, ErrInvalidToken
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*AccessTokenClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	// 检查密钥是否已过期（如果配置了keyStore）
	if s.keyStore != nil && usedKeyID != "" {
		ctx := context.Background()
		keyVersion, err := s.keyStore.GetKeyByID(ctx, usedKeyID)
		if err != nil {
			// 如果无法获取密钥信息，为了安全起见拒绝token
			return nil, ErrInvalidToken
		}

		// 使用KeyVersion的CanVerify方法检查密钥是否可用
		// 该方法会检查密钥状态和过期时间
		if !keyVersion.CanVerify() {
			return nil, apperrors.ErrKeyExpired
		}
	}

	// 检查JTI是否已被使用（防止重放攻击）
	s.mu.RLock()
	tracker := s.jtiTracker
	s.mu.RUnlock()

	if tracker != nil && claims.ID != "" {
		ctx := context.Background()
		ttl := time.Until(claims.ExpiresAt.Time)

		// 使用原子操作检查并标记JTI，防止TOCTOU竞态
		replayed, err := tracker.CheckAndMarkUsed(ctx, claims.ID, ttl)
		if err != nil {
			// JTI检查失败时不拒绝token，避免缓存故障导致服务不可用
			return claims, nil
		}
		if replayed {
			return nil, ErrTokenReplayed
		}
	}

	return claims, nil
}

func (s *JWTService) GetPublicKey() *rsa.PublicKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.publicKey
}

func (s *JWTService) GetPublicKeys() map[string]*rsa.PublicKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// 返回副本以防止外部修改
	result := make(map[string]*rsa.PublicKey, len(s.publicKeys))
	for k, v := range s.publicKeys {
		result[k] = v
	}
	return result
}

func (s *JWTService) GetAccessTokenTTL() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accessTokenTTL
}

func (s *JWTService) GetJWKS() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]map[string]interface{}, 0, len(s.publicKeys))
	for kid, pubKey := range s.publicKeys {
		keys = append(keys, map[string]interface{}{
			"kid": kid,
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}),
		})
	}
	return map[string]interface{}{
		"keys": keys,
	}
}

func GenerateKeyID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func GenerateRSAKeyPair(bits int) (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, bits)
}

func EncodePrivateKeyToPEM(key *rsa.PrivateKey) []byte {
	return EncodePrivateKeyToPKCS8PEM(key)
}

func EncodePrivateKeyToPKCS1PEM(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

func EncodePrivateKeyToPKCS8PEM(key *rsa.PrivateKey) []byte {
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})
}

func EncodePublicKeyToPEM(key *rsa.PublicKey) []byte {
	return EncodePublicKeyToPKIXPEM(key)
}

func EncodePublicKeyToPKIXPEM(key *rsa.PublicKey) []byte {
	der, _ := x509.MarshalPKIXPublicKey(key)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	})
}

func CreateKeyVersion(privateKey *rsa.PrivateKey) (*model.KeyVersion, error) {
	keyID, err := GenerateKeyID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key ID: %w", err)
	}

	return &model.KeyVersion{
		ID:         keyID,
		PublicKey:  EncodePublicKeyToPEM(&privateKey.PublicKey),
		PrivateKey: EncodePrivateKeyToPEM(privateKey),
		Status:     model.KeyStatusActive,
		CreatedAt:  time.Now(),
	}, nil
}
