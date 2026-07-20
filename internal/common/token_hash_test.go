// Package common 公共工具包
// token_hash_test.go - Token 哈希工具测试
package common

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashToken(t *testing.T) {
	t.Run("空字符串返回空", func(t *testing.T) {
		assert.Equal(t, "", HashToken(""))
	})

	t.Run("固定输入产生固定输出", func(t *testing.T) {
		token := "eyJhbGciOiJIUzI1NiJ9.payload.signature"
		hash1 := HashToken(token)
		hash2 := HashToken(token)
		assert.Equal(t, hash1, hash2, "相同 token 应产生相同 hash")
	})

	t.Run("hash 长度为 64（hex 编码 SHA-256）", func(t *testing.T) {
		hash := HashToken("any-token")
		assert.Equal(t, 64, len(hash), "SHA-256 hex 编码应为 64 字符")
	})

	t.Run("与标准 SHA-256 计算一致", func(t *testing.T) {
		token := "test-token-123"
		h := sha256.Sum256([]byte(token))
		expected := hex.EncodeToString(h[:])
		assert.Equal(t, expected, HashToken(token))
	})

	t.Run("不同 token 产生不同 hash", func(t *testing.T) {
		hash1 := HashToken("token-A")
		hash2 := HashToken("token-B")
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("长 token 也能正常哈希", func(t *testing.T) {
		// 模拟 RS256 JWT（2048-bit RSA）的 token 长度
		longToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWUsImlhdCI6MTUxNjIzOTAyMn0.NHVaYe26MbtOYhSKkoKYdFVomDmNxk5wQ7AU5T9Dw0"
		hash := HashToken(longToken)
		assert.Equal(t, 64, len(hash))
		assert.NotEmpty(t, hash)
	})
}
