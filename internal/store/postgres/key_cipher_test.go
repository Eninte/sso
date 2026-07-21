//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/model"
)

// T7 集成测试固定 KEK（64 位 hex = 32 字节 AES-256）
const testKeyCipherKEKHex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

func testKeyCipherKEK(t *testing.T) []byte {
	t.Helper()
	kek, err := crypto.ParseKEK(testKeyCipherKEKHex)
	require.NoError(t, err)
	return kek
}

// generateTestKeyVersion 生成 RSA 密钥对并构造 KeyVersion（明文 PEM）
func generateTestKeyVersion(t *testing.T, keyID string) *model.KeyVersion {
	t.Helper()
	privateKey, err := crypto.GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	return &model.KeyVersion{
		ID:         keyID,
		PublicKey:  crypto.EncodePublicKeyToPEM(&privateKey.PublicKey),
		PrivateKey: crypto.EncodePrivateKeyToPEM(privateKey),
		Status:     model.KeyStatusActive,
		CreatedAt:  time.Now(),
	}
}

// readStoredPrivateKey 直连 DB 读取 key_versions.private_key 原始内容
func readStoredPrivateKey(t *testing.T, db *sql.DB, keyID string) string {
	t.Helper()
	var stored []byte
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT private_key FROM key_versions WHERE id = $1", keyID).Scan(&stored))
	return string(stored)
}

// TestKeyCipher_StoreEncryptedKey 验证密文私钥落库后可通过 JWTService 透明解密加载
func TestKeyCipher_StoreEncryptedKey(t *testing.T) {
	store, db := setupTestStore(t)
	ctx := context.Background()
	keyID := "test-cipher-key-1"
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM key_versions WHERE id LIKE 'test-cipher-%'")
		db.Close()
	})

	kek := testKeyCipherKEK(t)
	keyVersion := generateTestKeyVersion(t, keyID)

	// service 侧加密后落库（与 keyrotation.RotateKey 行为一致）
	ciphertext, err := crypto.EncryptPrivateKey(kek, keyVersion.PrivateKey)
	require.NoError(t, err)
	keyVersion.PrivateKey = []byte(ciphertext)
	require.NoError(t, store.StoreKey(ctx, keyVersion))

	// DB 中必须为密文，且不含 PEM 明文标记
	stored := readStoredPrivateKey(t, db, keyID)
	assert.True(t, crypto.IsEncryptedPrivateKey(stored), "DB 中的私钥必须为 v1:gcm: 密文")
	assert.NotContains(t, stored, "BEGIN PRIVATE KEY")

	// JWTService 配置 KEK 后应能透明解密加载
	svc := crypto.NewJWTServiceWithKeyStore(store, "test", 5*time.Minute, time.Hour)
	require.NoError(t, svc.SetKeyEncryptionKey(kek))
	require.NoError(t, svc.LoadKeysFromStore(ctx))
	assert.Equal(t, keyID, svc.GetActiveKeyID())

	// 加载后 DB 内容应保持密文（无需回写）
	assert.True(t, crypto.IsEncryptedPrivateKey(readStoredPrivateKey(t, db, keyID)))
}

// TestKeyCipher_LazyEncryptsPlaintextKey 验证存量明文私钥在加载时被懒加密回写
func TestKeyCipher_LazyEncryptsPlaintextKey(t *testing.T) {
	store, db := setupTestStore(t)
	ctx := context.Background()
	keyID := "test-cipher-key-2"
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM key_versions WHERE id LIKE 'test-cipher-%'")
		db.Close()
	})

	// 直接落库明文 PEM（模拟 KEK 启用前的存量数据）
	keyVersion := generateTestKeyVersion(t, keyID)
	require.NoError(t, store.StoreKey(ctx, keyVersion))
	assert.Contains(t, readStoredPrivateKey(t, db, keyID), "BEGIN PRIVATE KEY")

	// 配置 KEK 加载：明文行应被解密使用并懒加密回写
	svc := crypto.NewJWTServiceWithKeyStore(store, "test", 5*time.Minute, time.Hour)
	require.NoError(t, svc.SetKeyEncryptionKey(testKeyCipherKEK(t)))
	require.NoError(t, svc.LoadKeysFromStore(ctx))
	assert.Equal(t, keyID, svc.GetActiveKeyID())

	stored := readStoredPrivateKey(t, db, keyID)
	assert.True(t, crypto.IsEncryptedPrivateKey(stored), "存量明文私钥应被懒加密回写为 v1:gcm: 密文")
	assert.NotContains(t, stored, "BEGIN PRIVATE KEY")

	// 回写后的密文可用同一 KEK 解密出原 PEM
	decrypted, err := crypto.DecryptPrivateKey(testKeyCipherKEK(t), stored)
	require.NoError(t, err)
	assert.Contains(t, string(decrypted), "BEGIN PRIVATE KEY")
}

// TestKeyCipher_UpdateKeyPrivateKey 验证 UpdateKeyPrivateKey 的存储语义
func TestKeyCipher_UpdateKeyPrivateKey(t *testing.T) {
	store, db := setupTestStore(t)
	ctx := context.Background()
	keyID := "test-cipher-key-3"
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM key_versions WHERE id LIKE 'test-cipher-%'")
		db.Close()
	})

	keyVersion := generateTestKeyVersion(t, keyID)
	require.NoError(t, store.StoreKey(ctx, keyVersion))

	newMaterial := []byte("v1:gcm:fakematerial")
	require.NoError(t, store.UpdateKeyPrivateKey(ctx, keyID, newMaterial))
	assert.Equal(t, string(newMaterial), readStoredPrivateKey(t, db, keyID))

	// 不存在的 keyID 应返回 ErrNotFound
	err := store.UpdateKeyPrivateKey(ctx, "test-cipher-nonexistent", newMaterial)
	require.Error(t, err)
}
