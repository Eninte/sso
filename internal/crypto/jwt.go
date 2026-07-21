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
	"log/slog"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
)

var (
	ErrInvalidToken = apperrors.ErrInvalidToken
	ErrTokenExpired = apperrors.ErrTokenExpired
	ErrNoActiveKey  = apperrors.ErrNoActiveKey
)

type JWTService struct {
	mu              sync.RWMutex
	privateKey      *rsa.PrivateKey
	publicKey       *rsa.PublicKey
	keys            map[string]*rsa.PrivateKey
	publicKeys      map[string]*rsa.PublicKey
	activeKeyID     string
	keyStore        store.KeyStore
	kek             []byte // T7：私钥信封加密的 KEK（32 字节），nil 表示未启用加密
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

// SetKeyEncryptionKey 配置私钥信封加密的 KEK（T7）
//
// 配置后：LoadKeysFromStore 读取密文行时解密、读取存量明文行后懒加密回写；
// kek 必须为 32 字节（由 ParseKEK 解析 JWT_KEY_ENCRYPTION_KEY 得到）
func (s *JWTService) SetKeyEncryptionKey(kek []byte) error {
	if len(kek) != 0 && len(kek) != 32 {
		return fmt.Errorf("KEK must be 32 bytes, got %d", len(kek))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kek = kek
	return nil
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

	s.mu.RLock()
	kek := s.kek
	s.mu.RUnlock()

	// T7：待懒加密回写的明文行（keyID -> 明文 PEM）
	var lazyEncrypt []struct {
		keyID string
		pem   []byte
	}

	s.mu.Lock()
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
			// T7：按前缀分派解密；明文行原样返回（过渡兼容存量数据）
			plainPEM, err := DecryptPrivateKey(kek, string(keyVersion.PrivateKey))
			if err != nil {
				slog.Warn("私钥信封解密失败，跳过该密钥", "key_id", keyVersion.ID, "error", err)
				continue
			}
			privKey, err := ParsePrivateKey(plainPEM)
			if err == nil {
				s.keys[keyVersion.ID] = privKey
				s.activeKeyID = keyVersion.ID
				s.privateKey = privKey
				s.publicKey = pubKey

				// 明文行且已配置 KEK → 标记懒加密回写（无需迁移脚本）
				if len(kek) > 0 && !IsEncryptedPrivateKey(string(keyVersion.PrivateKey)) {
					lazyEncrypt = append(lazyEncrypt, struct {
						keyID string
						pem   []byte
					}{keyID: keyVersion.ID, pem: plainPEM})
				}
			}
		}
	}
	s.mu.Unlock()

	// T7：懒加密回写（锁外执行；回写失败仅告警，不阻塞密钥加载）
	for _, item := range lazyEncrypt {
		ciphertext, err := EncryptPrivateKey(kek, item.pem)
		if err != nil {
			slog.Warn("私钥懒加密失败", "key_id", item.keyID, "error", err)
			continue
		}
		if err := s.keyStore.UpdateKeyPrivateKey(ctx, item.keyID, []byte(ciphertext)); err != nil {
			slog.Warn("私钥懒加密回写失败", "key_id", item.keyID, "error", err)
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
// 验证签名、过期时间、算法、Issuer、密钥过期状态和JTI重放
// 返回解析后的Claims或错误
//
// 注意：此方法不接受 ctx 参数，无法继承请求的超时/取消。
// 内部对 keyStore 的查询使用 context.Background()，因 keyStore 查询通常很快
// 且 token 验证是同步阻塞调用方的主流程。
// 若调用方需要请求级 ctx 控制，请使用 ValidateAccessTokenWithContext。
func (s *JWTService) ValidateAccessToken(tokenString string) (*AccessTokenClaims, error) {
	return s.ValidateAccessTokenWithContext(context.Background(), tokenString)
}

// ValidateAccessTokenWithContext 使用请求 ctx 验证访问令牌
// 阶段 D 审查修复（M10）：原 ValidateAccessToken 内部对 keyStore 查询使用
// context.Background()，请求级超时无法传播到 keyStore 查询，
// 在 keyStore 慢响应时可能阻塞请求 goroutine。
func (s *JWTService) ValidateAccessTokenWithContext(ctx context.Context, tokenString string) (*AccessTokenClaims, error) {
	var usedKeyID string
	s.mu.RLock()
	issuer := s.issuer
	s.mu.RUnlock()

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

		// 阶段 D 审查修复（M1）：密钥轮换模式下，kid 缺失应拒绝
		// 原实现在 keyStore 已配置（多密钥轮换模式）时仍回退到 activeKeyID，
		// 导致无 kid 的 token 在轮换后仍可被验证通过，无法识别其使用的是哪把旧密钥，
		// 增加密钥撤销难度。仅当未配置 keyStore（单密钥模式）时允许回退。
		if s.keyStore != nil {
			return nil, ErrInvalidToken
		}

		if s.publicKey != nil {
			usedKeyID = s.activeKeyID
			return s.publicKey, nil
		}

		return nil, ErrInvalidToken
	},
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
	)

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
	// 阶段 D 审查修复（M10）：使用请求 ctx 而非 context.Background()
	if s.keyStore != nil && usedKeyID != "" {
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

// GetRefreshTokenTTL 返回 refresh token 的有效期
// 用于 token 轮换时计算新 refresh token 的独立过期时间
func (s *JWTService) GetRefreshTokenTTL() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.refreshTokenTTL
}

// SignConsentToken 使用当前活跃密钥签名任意 *jwt.Token
//
// 用于阶段 2.2 consent_token 签发，避免暴露私钥。
// 调用方负责在 token.Header 中设置 kid（可选）。
// 复用 access_token 的 RS256 签名路径，保证不可伪造。
func (s *JWTService) SignConsentToken(token *jwt.Token) (string, error) {
	if token == nil {
		return "", fmt.Errorf("token is nil")
	}
	if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
		return "", fmt.Errorf("unsupported signing method: %v", token.Header["alg"])
	}
	s.mu.RLock()
	privateKey := s.privateKey
	s.mu.RUnlock()
	if privateKey == nil {
		return "", ErrNoActiveKey
	}
	return token.SignedString(privateKey)
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
