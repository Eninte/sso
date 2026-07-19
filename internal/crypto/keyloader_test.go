// Package crypto_test 密钥加载单元测试
package crypto_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestKeys 创建测试用的密钥文件
func createTestKeys(t *testing.T) (privateKeyPath, publicKeyPath string) {
	t.Helper()

	// 创建临时目录
	tmpDir := t.TempDir()

	// 生成RSA密钥对
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// 编码私钥 (PKCS1格式)
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// 编码私钥 (PKCS8格式)
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	pkcs8PEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})

	// 编码公钥
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	// 写入PKCS1私钥文件 (0600权限)
	pkcs1Path := filepath.Join(tmpDir, "private_pkcs1.pem")
	err = os.WriteFile(pkcs1Path, privateKeyPEM, 0600)
	require.NoError(t, err)

	// 写入PKCS8私钥文件 (0600权限)
	pkcs8Path := filepath.Join(tmpDir, "private_pkcs8.pem")
	err = os.WriteFile(pkcs8Path, pkcs8PEM, 0600)
	require.NoError(t, err)

	// 写入公钥文件 (0600权限)
	pubPath := filepath.Join(tmpDir, "public.pem")
	err = os.WriteFile(pubPath, publicKeyPEM, 0600)
	require.NoError(t, err)

	return pkcs1Path, pubPath
}

// ============================================================================
// LoadPrivateKeyFromFile 测试
// ============================================================================

func TestLoadPrivateKeyFromFile_PKCS1(t *testing.T) {
	privateKeyPath, _ := createTestKeys(t)

	key, err := crypto.LoadPrivateKeyFromFile(privateKeyPath)

	require.NoError(t, err)
	assert.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
}

func TestLoadPrivateKeyFromFile_PKCS8(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()

	// 生成RSA密钥对
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// 编码PKCS8格式
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	pkcs8PEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})

	// 写入文件
	pkcs8Path := filepath.Join(tmpDir, "private_pkcs8.pem")
	err = os.WriteFile(pkcs8Path, pkcs8PEM, 0600)
	require.NoError(t, err)

	key, err := crypto.LoadPrivateKeyFromFile(pkcs8Path)

	require.NoError(t, err)
	assert.NotNil(t, key)
}

func TestLoadPrivateKeyFromFile_FileNotFound(t *testing.T) {
	_, err := crypto.LoadPrivateKeyFromFile("/nonexistent/path/private.pem")

	assert.ErrorIs(t, err, crypto.ErrKeyNotFound)
}

func TestLoadPrivateKeyFromFile_PathTraversal(t *testing.T) {
	// 尝试路径遍历攻击
	_, err := crypto.LoadPrivateKeyFromFile("../../../etc/passwd")

	assert.ErrorIs(t, err, crypto.ErrKeyPathInvalid)
}

func TestLoadPrivateKeyFromFile_InvalidPEM(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "invalid.pem")

	// 写入非PEM内容
	err := os.WriteFile(invalidPath, []byte("not a pem file"), 0600)
	require.NoError(t, err)

	_, err = crypto.LoadPrivateKeyFromFile(invalidPath)

	assert.ErrorIs(t, err, crypto.ErrKeyParseFailed)
}

func TestLoadPrivateKeyFromFile_InvalidKey(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "invalid_key.pem")

	// 写入有效的PEM但内容是无效的密钥
	invalidPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: []byte("invalid key data"),
	})

	err := os.WriteFile(invalidPath, invalidPEM, 0600)
	require.NoError(t, err)

	_, err = crypto.LoadPrivateKeyFromFile(invalidPath)

	assert.ErrorIs(t, err, crypto.ErrKeyParseFailed)
}

func TestLoadPrivateKeyFromFile_WrongKeyType(t *testing.T) {
	tmpDir := t.TempDir()
	wrongKeyPath := filepath.Join(tmpDir, "wrong_key.pem")

	// 生成一个非RSA的PKCS8密钥 (DSA)
	// 使用一个有效的PKCS8结构但不是RSA密钥
	// 这里简化测试，直接测试解析失败的情况
	dsaKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: []byte("not a valid pkcs8 key"),
	})

	err := os.WriteFile(wrongKeyPath, dsaKeyPEM, 0600)
	require.NoError(t, err)

	_, err = crypto.LoadPrivateKeyFromFile(wrongKeyPath)

	// 由于内容无效，应该返回解析失败
	assert.ErrorIs(t, err, crypto.ErrKeyParseFailed)
}

// ============================================================================
// LoadPublicKeyFromFile 测试
// ============================================================================

func TestLoadPublicKeyFromFile_Valid(t *testing.T) {
	_, publicKeyPath := createTestKeys(t)

	key, err := crypto.LoadPublicKeyFromFile(publicKeyPath)

	require.NoError(t, err)
	assert.NotNil(t, key)
}

func TestLoadPublicKeyFromFile_FileNotFound(t *testing.T) {
	_, err := crypto.LoadPublicKeyFromFile("/nonexistent/path/public.pem")

	assert.ErrorIs(t, err, crypto.ErrKeyNotFound)
}

func TestLoadPublicKeyFromFile_PathTraversal(t *testing.T) {
	_, err := crypto.LoadPublicKeyFromFile("../../etc/passwd")

	assert.ErrorIs(t, err, crypto.ErrKeyPathInvalid)
}

func TestLoadPublicKeyFromFile_InvalidPEM(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "invalid_pub.pem")

	err := os.WriteFile(invalidPath, []byte("not a pem file"), 0600)
	require.NoError(t, err)

	_, err = crypto.LoadPublicKeyFromFile(invalidPath)

	assert.ErrorIs(t, err, crypto.ErrKeyParseFailed)
}

func TestLoadPublicKeyFromFile_InvalidKey(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "invalid_pub.pem")

	invalidPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: []byte("invalid key data"),
	})

	err := os.WriteFile(invalidPath, invalidPEM, 0600)
	require.NoError(t, err)

	_, err = crypto.LoadPublicKeyFromFile(invalidPath)

	assert.ErrorIs(t, err, crypto.ErrKeyParseFailed)
}

// ============================================================================
// 权限检查测试
// ============================================================================

func TestLoadPrivateKeyFromFile_PermissionTooOpen(t *testing.T) {
	t.Setenv("STRICT_KEY_PERMISSIONS", "true")
	tmpDir := t.TempDir()

	// 生成密钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// 写入文件，权限过于宽松 (0644)
	keyPath := filepath.Join(tmpDir, "open_key.pem")
	err = os.WriteFile(keyPath, privateKeyPEM, 0644)
	require.NoError(t, err)

	_, err = crypto.LoadPrivateKeyFromFile(keyPath)

	assert.ErrorIs(t, err, crypto.ErrKeyPermissionOpen)
}

func TestLoadPublicKeyFromFile_PermissionTooOpen(t *testing.T) {
	t.Setenv("STRICT_KEY_PERMISSIONS", "true")
	tmpDir := t.TempDir()

	// 生成密钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	// 写入文件，权限 0644 对公钥是合适的（所有者读写，组和其他用户只读）
	keyPath := filepath.Join(tmpDir, "open_pub.pem")
	err = os.WriteFile(keyPath, publicKeyPEM, 0644)
	require.NoError(t, err)

	// 公钥文件权限 0644 应该被接受（不允许组和其他用户写入即可）
	_, err = crypto.LoadPublicKeyFromFile(keyPath)
	assert.NoError(t, err, "Public key with 0644 permissions should be accepted")

	// 测试真正过于宽松的权限：0666（所有人可写）
	keyPath2 := filepath.Join(tmpDir, "too_open_pub.pem")
	err = os.WriteFile(keyPath2, publicKeyPEM, 0666)
	require.NoError(t, err)
	// os.WriteFile 受 umask 影响（CI 环境 umask=022 会将 0666 降为 0644），
	// 显式 Chmod 确保文件权限精确为 0666，以正确测试权限检查逻辑
	require.NoError(t, os.Chmod(keyPath2, 0666))

	_, err = crypto.LoadPublicKeyFromFile(keyPath2)
	assert.ErrorIs(t, err, crypto.ErrKeyPermissionOpen, "Public key with 0666 permissions should be rejected")
}

// ============================================================================
// ParsePublicKey 测试
// ============================================================================

func TestParsePublicKey_PKIX(t *testing.T) {
	// 生成RSA密钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// 编码为PKIX格式 (PUBLIC KEY)
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	key, err := crypto.ParsePublicKey(publicKeyPEM)

	require.NoError(t, err)
	assert.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
}

func TestParsePublicKey_PKCS1(t *testing.T) {
	// 生成RSA密钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// 编码为PKCS1格式 (RSA PUBLIC KEY)
	publicKeyBytes := x509.MarshalPKCS1PublicKey(&privateKey.PublicKey)
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	key, err := crypto.ParsePublicKey(publicKeyPEM)

	require.NoError(t, err)
	assert.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
}

func TestParsePublicKey_InvalidPEM(t *testing.T) {
	_, err := crypto.ParsePublicKey([]byte("not a pem"))

	assert.ErrorIs(t, err, crypto.ErrKeyParseFailed)
}

func TestParsePublicKey_InvalidDER(t *testing.T) {
	invalidPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: []byte("invalid der data"),
	})

	_, err := crypto.ParsePublicKey(invalidPEM)

	assert.ErrorIs(t, err, crypto.ErrKeyParseFailed)
}

func TestParsePublicKey_PKCS1_InvalidDER(t *testing.T) {
	invalidPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: []byte("invalid der data"),
	})

	_, err := crypto.ParsePublicKey(invalidPEM)

	assert.ErrorIs(t, err, crypto.ErrKeyParseFailed)
}

func TestParsePublicKey_1024bit_Rejected(t *testing.T) {
	// 1024位密钥应被拒绝
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	_, err = crypto.ParsePublicKey(publicKeyPEM)

	assert.ErrorIs(t, err, crypto.ErrKeyTooShort)
}

// ============================================================================
// ParsePrivateKey 测试
// ============================================================================

func TestParsePrivateKey_PKCS1(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	key, err := crypto.ParsePrivateKey(privateKeyPEM)

	require.NoError(t, err)
	assert.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
}

func TestParsePrivateKey_PKCS8(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	pkcs8PEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})

	key, err := crypto.ParsePrivateKey(pkcs8PEM)

	require.NoError(t, err)
	assert.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
}

func TestParsePrivateKey_1024bit_Rejected(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	_, err = crypto.ParsePrivateKey(privateKeyPEM)

	assert.ErrorIs(t, err, crypto.ErrKeyTooShort)
}

// ============================================================================
// 空路径测试
// ============================================================================

func TestLoadPrivateKeyFromFile_EmptyPath(t *testing.T) {
	_, err := crypto.LoadPrivateKeyFromFile("")

	assert.ErrorIs(t, err, crypto.ErrKeyPathInvalid)
}

func TestLoadPublicKeyFromFile_EmptyPath(t *testing.T) {
	_, err := crypto.LoadPublicKeyFromFile("")

	assert.ErrorIs(t, err, crypto.ErrKeyPathInvalid)
}

// ============================================================================
// LoadKeysForRotation 测试
// ============================================================================

func TestLoadKeysForRotation_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// 生成密钥对
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	// 写入私钥文件
	privateKeyPath := filepath.Join(tmpDir, "private.pem")
	err = os.WriteFile(privateKeyPath, privateKeyPEM, 0600)
	require.NoError(t, err)

	// 写入公钥文件
	publicKeyPath := filepath.Join(tmpDir, "public.pem")
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0600)
	require.NoError(t, err)

	// 加载密钥轮换服务
	svc, err := crypto.LoadKeysForRotation(
		privateKeyPath,
		publicKeyPath,
		nil, // 无轮换公钥
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	require.NoError(t, err)
	assert.NotNil(t, svc)

	// 验证可以生成和验证Token
	token, err := svc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"openid"})
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := svc.ValidateAccessToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.Subject)
}

func TestLoadKeysForRotation_WithRotationKeys(t *testing.T) {
	tmpDir := t.TempDir()

	// 生成主密钥对
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	// 写入主密钥文件
	privateKeyPath := filepath.Join(tmpDir, "private.pem")
	err = os.WriteFile(privateKeyPath, privateKeyPEM, 0600)
	require.NoError(t, err)

	publicKeyPath := filepath.Join(tmpDir, "public.pem")
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0600)
	require.NoError(t, err)

	// 生成轮换密钥对
	rotPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	rotPublicKeyBytes, err := x509.MarshalPKIXPublicKey(&rotPrivateKey.PublicKey)
	require.NoError(t, err)
	rotPublicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: rotPublicKeyBytes,
	})

	// 写入轮换公钥文件
	rotPublicKeyPath := filepath.Join(tmpDir, "rot_public.pem")
	err = os.WriteFile(rotPublicKeyPath, rotPublicKeyPEM, 0600)
	require.NoError(t, err)

	// 加载密钥轮换服务（带轮换公钥）
	svc, err := crypto.LoadKeysForRotation(
		privateKeyPath,
		publicKeyPath,
		[]string{rotPublicKeyPath, ""}, // 包含一个空路径应被忽略
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestLoadKeysForRotation_PrivateKeyNotFound(t *testing.T) {
	_, err := crypto.LoadKeysForRotation(
		"/nonexistent/private.pem",
		"/nonexistent/public.pem",
		nil,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "加载签名私钥失败")
}

func TestLoadKeysForRotation_PublicKeyNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// 只创建私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	privateKeyPath := filepath.Join(tmpDir, "private.pem")
	err = os.WriteFile(privateKeyPath, privateKeyPEM, 0600)
	require.NoError(t, err)

	_, err = crypto.LoadKeysForRotation(
		privateKeyPath,
		"/nonexistent/public.pem",
		nil,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "加载签名公钥失败")
}
