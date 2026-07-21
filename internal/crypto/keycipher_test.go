package crypto_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store/mock"
)

const testKEKHex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

func testKEK(t *testing.T) []byte {
	t.Helper()
	kek, err := crypto.ParseKEK(testKEKHex)
	require.NoError(t, err)
	return kek
}

func TestParseKEK(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid 64 hex", input: testKEKHex, wantErr: false},
		{name: "valid uppercase hex", input: strings.ToUpper(testKEKHex), wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "too short", input: "aabbcc", wantErr: true},
		{name: "too long", input: testKEKHex + "00", wantErr: true},
		{name: "non hex", input: strings.Repeat("zz", 32), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kek, err := crypto.ParseKEK(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, kek)
				return
			}
			require.NoError(t, err)
			assert.Len(t, kek, 32)
		})
	}
}

func TestEncryptDecryptPrivateKey_RoundTrip(t *testing.T) {
	kek := testKEK(t)
	plain := []byte("-----BEGIN PRIVATE KEY-----\nfake-pem\n-----END PRIVATE KEY-----")

	ciphertext, err := crypto.EncryptPrivateKey(kek, plain)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(ciphertext, crypto.KeyCipherPrefixGCM))
	assert.NotContains(t, ciphertext, "BEGIN PRIVATE KEY")

	decrypted, err := crypto.DecryptPrivateKey(kek, ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plain, decrypted)
}

func TestEncryptPrivateKey_UniqueCiphertext(t *testing.T) {
	kek := testKEK(t)
	plain := []byte("same plaintext")

	first, err := crypto.EncryptPrivateKey(kek, plain)
	require.NoError(t, err)
	second, err := crypto.EncryptPrivateKey(kek, plain)
	require.NoError(t, err)
	assert.NotEqual(t, first, second, "each encryption must use a fresh nonce")
}

func TestEncryptPrivateKey_InvalidKEK(t *testing.T) {
	_, err := crypto.EncryptPrivateKey([]byte("short"), []byte("data"))
	assert.Error(t, err)
}

func TestDecryptPrivateKey_Tampered(t *testing.T) {
	kek := testKEK(t)
	ciphertext, err := crypto.EncryptPrivateKey(kek, []byte("secret"))
	require.NoError(t, err)

	// Corrupt the trailing base64 characters (ciphertext body).
	tampered := ciphertext[:len(ciphertext)-2] + "AA"
	_, err = crypto.DecryptPrivateKey(kek, tampered)
	assert.Error(t, err)
}

func TestDecryptPrivateKey_PlaintextPassthrough(t *testing.T) {
	kek := testKEK(t)
	plain := "-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----"
	out, err := crypto.DecryptPrivateKey(kek, plain)
	require.NoError(t, err)
	assert.Equal(t, []byte(plain), out)
}

func TestIsEncryptedPrivateKey(t *testing.T) {
	kek := testKEK(t)
	ciphertext, err := crypto.EncryptPrivateKey(kek, []byte("data"))
	require.NoError(t, err)

	assert.True(t, crypto.IsEncryptedPrivateKey(ciphertext))
	assert.False(t, crypto.IsEncryptedPrivateKey("-----BEGIN PRIVATE KEY-----"))
	assert.False(t, crypto.IsEncryptedPrivateKey(""))
}

// storeKeyPair 生成 RSA 密钥对，私钥以给定 PEM（明文或密文）存入 mock store。
func storeKeyPair(t *testing.T, mockStore *mock.Store, keyID string, privateKeyPEM []byte) {
	t.Helper()
	privateKey, err := crypto.ParsePrivateKey(privateKeyPEM)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, mockStore.StoreKey(ctx, &model.KeyVersion{
		ID:         keyID,
		PublicKey:  crypto.EncodePublicKeyToPEM(&privateKey.PublicKey),
		PrivateKey: privateKeyPEM,
		Status:     model.KeyStatusActive,
		CreatedAt:  time.Now(),
	}))
}

// generateKeyPEM 生成 RSA 密钥并返回明文 PEM 私钥。
func generateKeyPEM(t *testing.T) []byte {
	t.Helper()
	privateKey, err := crypto.GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	return crypto.EncodePrivateKeyToPEM(privateKey)
}

// storeEncryptedKeyPair 生成 RSA 密钥对，私钥加密后存入 mock store。
func storeEncryptedKeyPair(t *testing.T, mockStore *mock.Store, keyID string, kek []byte) {
	t.Helper()
	plainPEM := generateKeyPEM(t)
	ciphertext, err := crypto.EncryptPrivateKey(kek, plainPEM)
	require.NoError(t, err)

	privateKey, err := crypto.ParsePrivateKey(plainPEM)
	require.NoError(t, err)
	require.NoError(t, mockStore.StoreKey(context.Background(), &model.KeyVersion{
		ID:         keyID,
		PublicKey:  crypto.EncodePublicKeyToPEM(&privateKey.PublicKey),
		PrivateKey: []byte(ciphertext),
		Status:     model.KeyStatusActive,
		CreatedAt:  time.Now(),
	}))
}

// TestLoadKeysFromStore_LazyEncryption verifies plaintext keys are lazily
// encrypted back to the store when a KEK is configured.
func TestLoadKeysFromStore_LazyEncryption(t *testing.T) {
	mockStore := mock.New()
	storeKeyPair(t, mockStore, "lazy-key-1", generateKeyPEM(t))

	svc := crypto.NewJWTServiceWithKeyStore(mockStore, "test", 5*time.Minute, time.Hour)
	require.NoError(t, svc.SetKeyEncryptionKey(testKEK(t)))
	require.NoError(t, svc.LoadKeysFromStore(context.Background()))

	// Plaintext key must have been re-encrypted in the store.
	stored, err := mockStore.GetKeyByID(context.Background(), "lazy-key-1")
	require.NoError(t, err)
	assert.True(t, crypto.IsEncryptedPrivateKey(string(stored.PrivateKey)), "expected lazy encryption to persist ciphertext")
	assert.NotContains(t, string(stored.PrivateKey), "BEGIN PRIVATE KEY")

	// And the service must still hold the usable key.
	assert.Equal(t, "lazy-key-1", svc.GetActiveKeyID())
}

// TestLoadKeysFromStore_DecryptsCiphertext verifies encrypted keys are
// decrypted transparently at load time.
func TestLoadKeysFromStore_DecryptsCiphertext(t *testing.T) {
	kek := testKEK(t)
	mockStore := mock.New()
	storeEncryptedKeyPair(t, mockStore, "enc-key-1", kek)

	svc := crypto.NewJWTServiceWithKeyStore(mockStore, "test", 5*time.Minute, time.Hour)
	require.NoError(t, svc.SetKeyEncryptionKey(kek))
	require.NoError(t, svc.LoadKeysFromStore(context.Background()))
	assert.Equal(t, "enc-key-1", svc.GetActiveKeyID())

	// Store content must remain ciphertext (no rewrite needed).
	stored, err := mockStore.GetKeyByID(context.Background(), "enc-key-1")
	require.NoError(t, err)
	assert.True(t, crypto.IsEncryptedPrivateKey(string(stored.PrivateKey)))
}

// TestLoadKeysFromStore_WrongKEKSkipsKey verifies keys that fail decryption
// are skipped with a warning instead of breaking startup.
func TestLoadKeysFromStore_WrongKEKSkipsKey(t *testing.T) {
	otherKEK, err := crypto.ParseKEK("ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100")
	require.NoError(t, err)

	mockStore := mock.New()
	storeEncryptedKeyPair(t, mockStore, "bad-key-1", otherKEK)

	svc := crypto.NewJWTServiceWithKeyStore(mockStore, "test", 5*time.Minute, time.Hour)
	require.NoError(t, svc.SetKeyEncryptionKey(testKEK(t)))
	require.NoError(t, svc.LoadKeysFromStore(context.Background()))
	assert.Empty(t, svc.GetActiveKeyID(), "undecryptable key must be skipped")
}
