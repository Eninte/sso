// Package crypto 密钥信封加密
// keycipher.go - JWT 轮换私钥的 AES-256-GCM 信封加密（T7 安全修复 H3）
//
// 设计：
//   - KEK（Key Encryption Key）来自环境变量 JWT_KEY_ENCRYPTION_KEY（64 位 hex = 32 字节）
//   - 密文格式：v1:gcm:<base64(nonce|ciphertext)>，nonce 每次随机生成（crypto/rand）
//   - 读取按前缀分派：v1:gcm: → 解密；否则按明文 PEM 处理（过渡兼容存量明文行）
//   - store 层无感知：加解密只在 crypto/service 层进行，DB 中 private_key 为密文或明文
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// KeyCipherPrefixGCM 密文格式前缀：v1:gcm:<base64(nonce|ciphertext)>
const KeyCipherPrefixGCM = "v1:gcm:"

// kekSize KEK 长度（字节）：AES-256 要求 32 字节
const kekSize = 32

// gcmNonceSize AES-GCM 标准 nonce 长度
const gcmNonceSize = 12

// ParseKEK 解析 hex 编码的密钥加密密钥（KEK）
//
// 输入必须为 64 位 hex（32 字节，AES-256）；长度不符或非法 hex 返回英文错误
func ParseKEK(hexKey string) ([]byte, error) {
	if hexKey == "" {
		return nil, fmt.Errorf("JWT_KEY_ENCRYPTION_KEY is empty")
	}
	kek, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("JWT_KEY_ENCRYPTION_KEY must be hex encoded: %w", err)
	}
	if len(kek) != kekSize {
		return nil, fmt.Errorf("JWT_KEY_ENCRYPTION_KEY must be 32 bytes (64 hex chars), got %d bytes", len(kek))
	}
	return kek, nil
}

// IsEncryptedPrivateKey 判断存储的私钥字段是否为信封加密密文
func IsEncryptedPrivateKey(stored string) bool {
	return strings.HasPrefix(stored, KeyCipherPrefixGCM)
}

// EncryptPrivateKey 用 KEK 对私钥 PEM 做 AES-256-GCM 信封加密
//
// 返回 v1:gcm:<base64(nonce|ciphertext)> 格式的密文字符串；
// nonce 每次调用由 crypto/rand 随机生成，相同明文产生不同密文
func EncryptPrivateKey(kek, plaintextPEM []byte) (string, error) {
	if len(kek) != kekSize {
		return "", fmt.Errorf("KEK must be 32 bytes, got %d", len(kek))
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcmNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// nonce|ciphertext 拼接后 base64 编码
	sealed := gcm.Seal(nonce, nonce, plaintextPEM, nil)
	return KeyCipherPrefixGCM + base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptPrivateKey 按前缀分派解密存储的私钥字段
//
//   - v1:gcm: 前缀 → AES-256-GCM 解密，返回明文 PEM
//   - 无前缀 → 按明文 PEM 原样返回（过渡兼容存量明文行）
//
// 密文篡改、格式损坏或 KEK 错误时返回英文错误
func DecryptPrivateKey(kek []byte, stored string) ([]byte, error) {
	if !IsEncryptedPrivateKey(stored) {
		// 明文行（过渡兼容），原样返回
		return []byte(stored), nil
	}

	if len(kek) != kekSize {
		return nil, fmt.Errorf("KEK must be 32 bytes, got %d", len(kek))
	}

	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, KeyCipherPrefixGCM))
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted private key: %w", err)
	}
	if len(payload) < gcmNonceSize+1 {
		return nil, fmt.Errorf("encrypted private key payload too short")
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce, ciphertext := payload[:gcmNonceSize], payload[gcmNonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key (wrong KEK or tampered ciphertext): %w", err)
	}
	return plaintext, nil
}
