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
	"time"

	"github.com/golang-jwt/jwt/v5"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
)

var (
	ErrInvalidToken = apperrors.ErrInvalidToken
	ErrTokenExpired = apperrors.ErrTokenExpired
	ErrNoActiveKey  = errors.New("no active key available")
)

type JWTService struct {
	privateKey      *rsa.PrivateKey
	publicKey       *rsa.PublicKey
	keys            map[string]*rsa.PrivateKey
	publicKeys      map[string]*rsa.PublicKey
	activeKeyID     string
	keyStore        store.KeyStore
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

func (s *JWTService) SetActiveKey(keyID string, privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey) {
	s.keys[keyID] = privateKey
	s.publicKeys[keyID] = publicKey
	s.activeKeyID = keyID
	s.privateKey = privateKey
	s.publicKey = publicKey
}

func (s *JWTService) AddVerificationKey(keyID string, publicKey *rsa.PublicKey) {
	s.publicKeys[keyID] = publicKey
}

func (s *JWTService) RemoveKey(keyID string) {
	delete(s.keys, keyID)
	delete(s.publicKeys, keyID)
	if s.activeKeyID == keyID {
		s.activeKeyID = ""
	}
}

func (s *JWTService) GetActiveKeyID() string {
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
}

func (s *JWTService) GenerateAccessToken(userID, email string, scopes []string) (string, error) {
	return s.GenerateAccessTokenWithKeyID(userID, email, scopes, s.activeKeyID)
}

func (s *JWTService) GenerateAccessTokenWithKeyID(userID, email string, scopes []string, keyID string) (string, error) {
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

	if privateKey == nil {
		return "", ErrNoActiveKey
	}

	now := time.Now()
	claims := AccessTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
		KeyID:  keyID,
		Email:  email,
		Scopes: scopes,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if keyID != "" {
		token.Header["kid"] = keyID
	}
	return token.SignedString(privateKey)
}

func (s *JWTService) GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func (s *JWTService) ValidateAccessToken(tokenString string) (*AccessTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AccessTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, ErrInvalidToken
		}

		kid, _ := token.Header["kid"].(string)
		if kid != "" {
			if pubKey, ok := s.publicKeys[kid]; ok {
				return pubKey, nil
			}
		}

		if s.publicKey != nil {
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

	return claims, nil
}

func (s *JWTService) GetPublicKey() *rsa.PublicKey {
	return s.publicKey
}

func (s *JWTService) GetPublicKeys() map[string]*rsa.PublicKey {
	return s.publicKeys
}

func (s *JWTService) GetAccessTokenTTL() time.Duration {
	return s.accessTokenTTL
}

func (s *JWTService) GetJWKS() map[string]interface{} {
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
