// Package crypto_test JWT密钥过期验证测试
package crypto_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/crypto"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
)

// mockKeyStore 模拟密钥存储
type mockKeyStore struct {
	keys map[string]*model.KeyVersion
}

func newMockKeyStore() *mockKeyStore {
	return &mockKeyStore{
		keys: make(map[string]*model.KeyVersion),
	}
}

func (m *mockKeyStore) StoreKey(ctx context.Context, key *model.KeyVersion) error {
	m.keys[key.ID] = key
	return nil
}

func (m *mockKeyStore) GetActiveKey(ctx context.Context) (*model.KeyVersion, error) {
	for _, key := range m.keys {
		if key.Status == model.KeyStatusActive {
			return key, nil
		}
	}
	return nil, apperrors.ErrKeyNotFound
}

func (m *mockKeyStore) GetKeyByID(ctx context.Context, keyID string) (*model.KeyVersion, error) {
	if key, ok := m.keys[keyID]; ok {
		return key, nil
	}
	return nil, apperrors.ErrKeyNotFound
}

func (m *mockKeyStore) ListActiveKeys(ctx context.Context) ([]*model.KeyVersion, error) {
	var keys []*model.KeyVersion
	for _, key := range m.keys {
		if key.Status == model.KeyStatusActive || key.Status == model.KeyStatusDeprecated {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (m *mockKeyStore) ListAllKeys(ctx context.Context) ([]*model.KeyVersion, error) {
	var keys []*model.KeyVersion
	for _, key := range m.keys {
		keys = append(keys, key)
	}
	return keys, nil
}

func (m *mockKeyStore) DeprecateKey(ctx context.Context, keyID string, expiresAt time.Time) error {
	if key, ok := m.keys[keyID]; ok {
		key.Status = model.KeyStatusDeprecated
		key.ExpiresAt = &expiresAt
		return nil
	}
	return apperrors.ErrKeyNotFound
}

func (m *mockKeyStore) RevokeKey(ctx context.Context, keyID string) error {
	if key, ok := m.keys[keyID]; ok {
		key.Status = model.KeyStatusRevoked
		return nil
	}
	return apperrors.ErrKeyNotFound
}

func (m *mockKeyStore) DeleteKey(ctx context.Context, keyID string) error {
	if _, ok := m.keys[keyID]; ok {
		delete(m.keys, keyID)
		return nil
	}
	return apperrors.ErrKeyNotFound
}

func (m *mockKeyStore) AddKey(keyVersion *model.KeyVersion) {
	m.keys[keyVersion.ID] = keyVersion
}

// TestJWTService_ValidateAccessToken_KeyExpiration 测试密钥过期验证
func TestJWTService_ValidateAccessToken_KeyExpiration(t *testing.T) {
	// 生成测试密钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	keyStore := newMockKeyStore()

	// 创建JWT服务
	svc := crypto.NewJWTServiceWithKeyStore(
		keyStore,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)

	// 设置活跃密钥
	keyID := "test-key-1"
	err = svc.SetActiveKey(keyID, privateKey, &privateKey.PublicKey)
	require.NoError(t, err)

	t.Run("密钥未过期_验证成功", func(t *testing.T) {
		// 添加未过期的密钥到keyStore
		expiresAt := time.Now().Add(24 * time.Hour)
		keyStore.AddKey(&model.KeyVersion{
			ID:         keyID,
			PublicKey:  crypto.EncodePublicKeyToPEM(&privateKey.PublicKey),
			PrivateKey: crypto.EncodePrivateKeyToPEM(privateKey),
			Status:     model.KeyStatusActive,
			CreatedAt:  time.Now(),
			ExpiresAt:  &expiresAt,
		})

		// 生成token
		token, err := svc.GenerateAccessToken("user-123", "test@example.com", "user", []string{"read", "write"})
		require.NoError(t, err)

		// 验证token（应该成功）
		claims, err := svc.ValidateAccessToken(token)
		assert.NoError(t, err)
		assert.NotNil(t, claims)
		assert.Equal(t, "user-123", claims.Subject)
	})

	t.Run("密钥已过期_验证失败", func(t *testing.T) {
		// 生成新密钥
		expiredPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		expiredKeyID := "expired-key"
		err = svc.SetActiveKey(expiredKeyID, expiredPrivateKey, &expiredPrivateKey.PublicKey)
		require.NoError(t, err)

		// 添加已过期的密钥到keyStore
		expiresAt := time.Now().Add(-1 * time.Hour) // 1小时前过期
		keyStore.AddKey(&model.KeyVersion{
			ID:         expiredKeyID,
			PublicKey:  crypto.EncodePublicKeyToPEM(&expiredPrivateKey.PublicKey),
			PrivateKey: crypto.EncodePrivateKeyToPEM(expiredPrivateKey),
			Status:     model.KeyStatusActive,
			CreatedAt:  time.Now().Add(-25 * time.Hour),
			ExpiresAt:  &expiresAt,
		})

		// 生成token
		token, err := svc.GenerateAccessToken("user-456", "test2@example.com", "user", []string{"read"})
		require.NoError(t, err)

		// 验证token（应该失败，因为密钥已过期）
		claims, err := svc.ValidateAccessToken(token)
		assert.Error(t, err)
		assert.Nil(t, claims)
		assert.True(t, apperrors.Is(err, apperrors.ErrKeyExpired))
	})

	t.Run("密钥已撤销_验证失败", func(t *testing.T) {
		// 生成新密钥
		revokedPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		revokedKeyID := "revoked-key"
		err = svc.SetActiveKey(revokedKeyID, revokedPrivateKey, &revokedPrivateKey.PublicKey)
		require.NoError(t, err)

		// 添加已撤销的密钥到keyStore
		keyStore.AddKey(&model.KeyVersion{
			ID:         revokedKeyID,
			PublicKey:  crypto.EncodePublicKeyToPEM(&revokedPrivateKey.PublicKey),
			PrivateKey: crypto.EncodePrivateKeyToPEM(revokedPrivateKey),
			Status:     model.KeyStatusRevoked, // 已撤销
			CreatedAt:  time.Now().Add(-1 * time.Hour),
			ExpiresAt:  nil,
		})

		// 生成token
		token, err := svc.GenerateAccessToken("user-789", "test3@example.com", "user", []string{"read"})
		require.NoError(t, err)

		// 验证token（应该失败，因为密钥已撤销）
		claims, err := svc.ValidateAccessToken(token)
		assert.Error(t, err)
		assert.Nil(t, claims)
		assert.True(t, apperrors.Is(err, apperrors.ErrKeyExpired))
	})

	t.Run("密钥已弃用但未过期_验证成功", func(t *testing.T) {
		// 生成新密钥
		deprecatedPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		deprecatedKeyID := "deprecated-key"
		err = svc.SetActiveKey(deprecatedKeyID, deprecatedPrivateKey, &deprecatedPrivateKey.PublicKey)
		require.NoError(t, err)

		// 添加已弃用但未过期的密钥到keyStore
		expiresAt := time.Now().Add(24 * time.Hour)
		keyStore.AddKey(&model.KeyVersion{
			ID:         deprecatedKeyID,
			PublicKey:  crypto.EncodePublicKeyToPEM(&deprecatedPrivateKey.PublicKey),
			PrivateKey: crypto.EncodePrivateKeyToPEM(deprecatedPrivateKey),
			Status:     model.KeyStatusDeprecated, // 已弃用但仍可验证
			CreatedAt:  time.Now().Add(-10 * 24 * time.Hour),
			ExpiresAt:  &expiresAt,
		})

		// 生成token
		token, err := svc.GenerateAccessToken("user-101", "test4@example.com", "admin", []string{"read", "write", "admin"})
		require.NoError(t, err)

		// 验证token（应该成功，因为已弃用的密钥仍可用于验证）
		claims, err := svc.ValidateAccessToken(token)
		assert.NoError(t, err)
		assert.NotNil(t, claims)
		assert.Equal(t, "user-101", claims.Subject)
		assert.Equal(t, "admin", claims.Role)
	})

	t.Run("无keyStore_跳过过期检查", func(t *testing.T) {
		// 创建没有keyStore的JWT服务
		svcNoStore := crypto.NewJWTService(
			privateKey,
			&privateKey.PublicKey,
			"test-issuer",
			15*time.Minute,
			7*24*time.Hour,
		)

		// 生成token
		token, err := svcNoStore.GenerateAccessToken("user-202", "test5@example.com", "user", []string{"read"})
		require.NoError(t, err)

		// 验证token（应该成功，因为没有keyStore不会检查过期）
		claims, err := svcNoStore.ValidateAccessToken(token)
		assert.NoError(t, err)
		assert.NotNil(t, claims)
		assert.Equal(t, "user-202", claims.Subject)
	})

	t.Run("密钥不存在_验证失败", func(t *testing.T) {
		// 生成新密钥
		unknownPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		unknownKeyID := "unknown-key"
		err = svc.SetActiveKey(unknownKeyID, unknownPrivateKey, &unknownPrivateKey.PublicKey)
		require.NoError(t, err)

		// 不添加密钥到keyStore

		// 生成token
		token, err := svc.GenerateAccessToken("user-303", "test6@example.com", "user", []string{"read"})
		require.NoError(t, err)

		// 验证token（应该失败，因为keyStore中找不到密钥）
		claims, err := svc.ValidateAccessToken(token)
		assert.Error(t, err)
		assert.Nil(t, claims)
		assert.True(t, apperrors.Is(err, apperrors.ErrInvalidToken))
	})
}
