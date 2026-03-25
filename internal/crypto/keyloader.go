// Package crypto 密钥加载工具
// 提供密钥文件加载和解析功能
package crypto

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apperrors "github.com/your-org/sso/internal/errors"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

var (
	ErrKeyNotFound       = apperrors.ErrKeyNotFound
	ErrKeyPathInvalid    = apperrors.ErrKeyPathInvalid
	ErrKeyParseFailed    = apperrors.ErrKeyParseFailed
	ErrKeyPermissionOpen = apperrors.ErrKeyPermissionOpen
	ErrKeyTooShort       = apperrors.ErrKeyTooShort
)

// ============================================================================
// 密钥加载函数
// ============================================================================

// loadKeyFromFile 从文件加载密钥（通用函数）
// 验证路径安全性，读取文件内容，调用相应的解析函数
func loadKeyFromFile(path string, parseFunc func([]byte) (interface{}, error)) (interface{}, error) {
	if err := validateKeyPath(path); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrKeyNotFound, path)
		}
		return nil, fmt.Errorf("读取密钥文件失败: %w", err)
	}

	return parseFunc(data)
}

// LoadPrivateKeyFromFile 从文件加载RSA私钥
func LoadPrivateKeyFromFile(path string) (*rsa.PrivateKey, error) {
	key, err := loadKeyFromFile(path, func(data []byte) (interface{}, error) {
		return ParsePrivateKey(data)
	})
	if err != nil {
		return nil, err
	}
	return key.(*rsa.PrivateKey), nil
}

// LoadPublicKeyFromFile 从文件加载RSA公钥
func LoadPublicKeyFromFile(path string) (*rsa.PublicKey, error) {
	key, err := loadKeyFromFile(path, func(data []byte) (interface{}, error) {
		return ParsePublicKey(data)
	})
	if err != nil {
		return nil, err
	}
	return key.(*rsa.PublicKey), nil
}

// ParsePrivateKey 解析PEM格式的私钥
func ParsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	key, err := parseRSAKey(data, map[string]func([]byte) (interface{}, error){
		"RSA PRIVATE KEY": func(b []byte) (interface{}, error) {
			return x509.ParsePKCS1PrivateKey(b)
		},
		"PRIVATE KEY": func(b []byte) (interface{}, error) {
			return x509.ParsePKCS8PrivateKey(b)
		},
	})
	if err != nil {
		return nil, err
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, ErrKeyParseFailed
	}

	return rsaKey, nil
}

// ParsePublicKey 解析PEM格式的公钥
func ParsePublicKey(data []byte) (*rsa.PublicKey, error) {
	key, err := parseRSAKey(data, map[string]func([]byte) (interface{}, error){
		"RSA PUBLIC KEY": func(b []byte) (interface{}, error) {
			return x509.ParsePKCS1PublicKey(b)
		},
		"PUBLIC KEY": func(b []byte) (interface{}, error) {
			return x509.ParsePKIXPublicKey(b)
		},
	})
	if err != nil {
		return nil, err
	}

	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, ErrKeyParseFailed
	}

	return rsaKey, nil
}

// parseRSAKey 通用RSA密钥解析函数
func parseRSAKey(data []byte, parsers map[string]func([]byte) (interface{}, error)) (interface{}, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, ErrKeyParseFailed
	}

	parser, exists := parsers[block.Type]
	if !exists {
		return nil, ErrKeyParseFailed
	}

	key, err := parser(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyParseFailed, err)
	}

	// 验证密钥大小（最小256字节 = 2048位）
	switch k := key.(type) {
	case *rsa.PrivateKey:
		if k.Size() < 256 {
			return nil, ErrKeyTooShort
		}
	case *rsa.PublicKey:
		if k.Size() < 256 {
			return nil, ErrKeyTooShort
		}
	}

	return key, nil
}

// validateKeyPath 验证密钥文件路径的安全性
func validateKeyPath(path string) error {
	if path == "" {
		return ErrKeyPathInvalid
	}

	if strings.Contains(path, "..") {
		return ErrKeyPathInvalid
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrKeyNotFound, path)
		}
		return err
	}

	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		return fmt.Errorf("%w: %o", ErrKeyPermissionOpen, perm)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return ErrKeyPathInvalid
	}

	// 允许的路径前缀
	allowedPrefixes := []string{"/etc/sso/", "/keys/", "/home/"}

	// 生产环境不允许/tmp/路径
	env := os.Getenv("SERVER_ENV")
	if env != "production" {
		allowedPrefixes = append(allowedPrefixes, "/tmp/")
	}

	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(absPath, prefix) {
			return nil
		}
	}

	return ErrKeyPathInvalid
}
