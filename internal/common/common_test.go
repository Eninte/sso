// Package common_test 公共工具包单元测试
package common_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/example/sso/internal/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// GenerateRandomString 测试
// ============================================================================

func TestGenerateRandomString_ReturnsCorrectLength(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"length 1", 1},
		{"length 16", 16},
		{"length 32", 32},
		{"length 64", 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := common.GenerateRandomString(tt.length)
			require.NoError(t, err)

			expectedLen := base64.RawURLEncoding.EncodedLen(tt.length)
			assert.Len(t, result, expectedLen)
		})
	}
}

func TestGenerateRandomString_NotEmpty(t *testing.T) {
	result, err := common.GenerateRandomString(32)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestGenerateRandomString_Randomness(t *testing.T) {
	results := make(map[string]bool)
	for i := 0; i < 100; i++ {
		result, err := common.GenerateRandomString(32)
		require.NoError(t, err)
		assert.False(t, results[result], "duplicate random string generated")
		results[result] = true
	}
}

func TestGenerateRandomString_URLSafe(t *testing.T) {
	// base64.RawURLEncoding 使用 - 和 _ 而不是 + 和 /
	for i := 0; i < 50; i++ {
		result, err := common.GenerateRandomString(32)
		require.NoError(t, err)

		// 不应包含标准 base64 的特殊字符
		assert.NotContains(t, result, "+")
		assert.NotContains(t, result, "/")
		assert.NotContains(t, result, "=")
	}
}

// ============================================================================
// GenerateToken 测试
// ============================================================================

func TestGenerateToken_NotEmpty(t *testing.T) {
	token, err := common.GenerateToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestGenerateToken_ConsistentLength(t *testing.T) {
	// GenerateToken 使用 32 字节，base64.URLEncoding 编码后固定为 44 字符
	for i := 0; i < 20; i++ {
		token, err := common.GenerateToken()
		require.NoError(t, err)
		assert.Len(t, token, 44)
	}
}

func TestGenerateToken_Randomness(t *testing.T) {
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := common.GenerateToken()
		require.NoError(t, err)
		assert.False(t, tokens[token], "duplicate token generated")
		tokens[token] = true
	}
}

func TestGenerateToken_URLSafe(t *testing.T) {
	for i := 0; i < 50; i++ {
		token, err := common.GenerateToken()
		require.NoError(t, err)

		// URLEncoding 使用 - 和 _ ，可能会有 = 填充
		assert.NotContains(t, token, "+")
		assert.NotContains(t, token, "/")
	}
}

// ============================================================================
// NormalizeLanguage 测试
// ============================================================================

func TestNormalizeLanguage_Chinese(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"zh", "zh-CN"},
		{"zh-CN", "zh-CN"},
		{"zh-TW", "zh-CN"},
		{"ZH", "zh-CN"},
		{"Zh-cn", "zh-CN"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, common.NormalizeLanguage(tt.input))
		})
	}
}

func TestNormalizeLanguage_English(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"en", "en-US"},
		{"en-US", "en-US"},
		{"en-GB", "en-US"},
		{"EN", "en-US"},
		{"En-us", "en-US"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, common.NormalizeLanguage(tt.input))
		})
	}
}

func TestNormalizeLanguage_AcceptLanguage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"full Accept-Language", "en-US,en;q=0.9,zh-CN;q=0.8", "en-US"},
		{"Chinese first", "zh-CN,zh;q=0.9,en;q=0.8", "zh-CN"},
		{"with quality", "en;q=0.8", "en-US"},
		{"multiple with quality", "fr-FR,fr;q=0.9,en-US;q=0.8,en;q=0.7", "fr-fr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := common.NormalizeLanguage(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeLanguage_OtherLanguages(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"fr", "fr"},
		{"de", "de"},
		{"ja", "ja"},
		{"ko", "ko"},
		{"fr-FR", "fr-fr"},
		{"de-DE", "de-de"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, common.NormalizeLanguage(tt.input))
		})
	}
}

func TestNormalizeLanguage_Whitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{" en-US ", "en-US"},
		{"  zh-CN  ", "zh-CN"},
		{"\ten\t", "en-US"},
	}

	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.input, " ", "_"), func(t *testing.T) {
			assert.Equal(t, tt.expected, common.NormalizeLanguage(tt.input))
		})
	}
}

func TestNormalizeLanguage_Empty(t *testing.T) {
	assert.Equal(t, "", common.NormalizeLanguage(""))
}

func TestNormalizeLanguage_CaseInsensitive(t *testing.T) {
	assert.Equal(t, "en-US", common.NormalizeLanguage("EN"))
	assert.Equal(t, "zh-CN", common.NormalizeLanguage("ZH"))
	assert.Equal(t, "en-US", common.NormalizeLanguage("En"))
	assert.Equal(t, "zh-CN", common.NormalizeLanguage("Zh"))
}
