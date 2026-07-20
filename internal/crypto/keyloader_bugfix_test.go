// Package crypto_test bugfix探索性测试
// 这些测试用于验证bug condition，在未修复代码上应该失败
package crypto_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
)

// skipOnWindowsOrContainer 跳过 Windows 和容器环境（阶段 D 预存问题修复）
// Windows 不支持 Unix 权限位，权限检查测试无法验证；
// 容器环境权限检查被禁用。
func skipOnWindowsOrContainer(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows 不支持 Unix 权限位，跳过权限检查测试")
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		t.Skip("Skipping permission test in Docker container")
	}
}

// ============================================================================
// Task 1.1: Bug Condition Exploration - Key Permission Check Logic
// ============================================================================

// TestBugCondition_PublicKeyWithPrivateInPath tests that a public key file
// with "private" in the path is incorrectly rejected due to heuristic-based
// key type detection.
//
// **Property 1: Bug Condition** - Heuristic Key Type Detection Failures
// **CRITICAL**: This test MUST FAIL on unfixed code - failure confirms the bug exists
// **DO NOT attempt to fix the test or the code when it fails**
// **NOTE**: This test encodes the expected behavior - it will validate the fix when it passes
//
// Bug Condition: Public key with "private" in filename → incorrectly rejected with 0077 mask
// Expected Behavior: Public key should be accepted with 0022 mask (no group/other write)
func TestBugCondition_PublicKeyWithPrivateInPath(t *testing.T) {
	skipOnWindowsOrContainer(t)
	t.Setenv("STRICT_KEY_PERMISSIONS", "true")

	tmpDir := t.TempDir()

	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Encode PUBLIC key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	// Write public key to file with "private" in the path
	// This should be accepted (it's a public key) but unfixed code rejects it
	publicKeyPath := filepath.Join(tmpDir, "backup_private_key.pub")
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0644) // Public key permissions
	require.NoError(t, err)

	// Expected: Should succeed (it's a public key with correct permissions)
	// Unfixed code: Will fail because filename contains "private"
	key, err := crypto.LoadPublicKeyFromFile(publicKeyPath)

	// This assertion encodes the EXPECTED behavior (after fix)
	assert.NoError(t, err, "Public key with 'private' in path should be accepted based on content, not filename")
	assert.NotNil(t, key, "Public key should be loaded successfully")
}

// TestBugCondition_PrivateKeyWithoutPrivateInName tests that a private key file
// without "private" in the filename and with non-standard permissions is incorrectly
// accepted due to heuristic-based key type detection.
//
// Bug Condition: Private key without "private" in name + 0640 permissions → incorrectly accepted
// Expected Behavior: Private key should be rejected with 0640 permissions (group read not allowed)
func TestBugCondition_PrivateKeyWithoutPrivateInName(t *testing.T) {
	skipOnWindowsOrContainer(t)
	t.Setenv("STRICT_KEY_PERMISSIONS", "true")

	tmpDir := t.TempDir()

	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Encode PRIVATE key
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// Write private key to file without "private" in the name
	// Use 0640 permissions (owner rw, group r, other none)
	privateKeyPath := filepath.Join(tmpDir, "jwt_signing.pem")
	err = os.WriteFile(privateKeyPath, privateKeyPEM, 0640)
	require.NoError(t, err)

	// Expected: Should fail (private key with group read permission)
	// Unfixed code: May succeed because filename doesn't contain "private"
	_, err = crypto.LoadPrivateKeyFromFile(privateKeyPath)

	// This assertion encodes the EXPECTED behavior (after fix)
	assert.ErrorIs(t, err, crypto.ErrKeyPermissionOpen, "Private key with 0640 permissions should be rejected (group read not allowed)")
}

// TestBugCondition_ContentVsFilenameMismatch tests that permission checks
// use PEM content to determine key type, not filename patterns.
//
// Bug Condition: Public key content in file named "private.pem" → permission check uses filename
// Expected Behavior: Permission check should use PEM content (PUBLIC KEY type)
func TestBugCondition_ContentVsFilenameMismatch(t *testing.T) {
	skipOnWindowsOrContainer(t)
	t.Setenv("STRICT_KEY_PERMISSIONS", "true")

	tmpDir := t.TempDir()

	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Encode PUBLIC key (not private!)
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY", // This is the authoritative key type
		Bytes: publicKeyBytes,
	})

	// Write PUBLIC key content to file named "private.pem"
	// Use 0644 permissions (appropriate for public key)
	keyPath := filepath.Join(tmpDir, "private.pem")
	err = os.WriteFile(keyPath, publicKeyPEM, 0644)
	require.NoError(t, err)

	// Expected: Should succeed (it's a public key with correct permissions)
	// Unfixed code: Will fail because filename is "private.pem"
	key, err := crypto.LoadPublicKeyFromFile(keyPath)

	// This assertion encodes the EXPECTED behavior (after fix)
	assert.NoError(t, err, "Public key content should be accepted regardless of filename")
	assert.NotNil(t, key, "Public key should be loaded successfully based on PEM content")
}

// TestBugCondition_CorruptedPEMReturnsParseError tests that corrupted PEM
// files return parse errors, not permission errors.
//
// Bug Condition: Invalid PEM content → may return permission error
// Expected Behavior: Should return ErrKeyParseFailed
func TestBugCondition_CorruptedPEMReturnsParseError(t *testing.T) {
	skipOnWindowsOrContainer(t)
	t.Setenv("STRICT_KEY_PERMISSIONS", "true")

	tmpDir := t.TempDir()

	// Write corrupted PEM content
	corruptedPath := filepath.Join(tmpDir, "corrupted.pem")
	err := os.WriteFile(corruptedPath, []byte("-----BEGIN RSA PRIVATE KEY-----\ninvalid base64 content\n-----END RSA PRIVATE KEY-----"), 0600)
	require.NoError(t, err)

	// Expected: Should return parse error, not permission error
	_, err = crypto.LoadPrivateKeyFromFile(corruptedPath)

	// This assertion encodes the EXPECTED behavior (after fix)
	assert.ErrorIs(t, err, crypto.ErrKeyParseFailed, "Corrupted PEM should return parse error, not permission error")
}
