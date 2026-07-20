package model_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/example/sso/internal/model"
)

// ============================================================================
// scope.go 测试
// 阶段 D 审查修复（覆盖率）：补充 model 包 scope 工具函数的覆盖率
// ============================================================================

func TestIsSupportedScope(t *testing.T) {
	t.Run("支持的scope返回true", func(t *testing.T) {
		assert.True(t, model.IsSupportedScope(model.ScopeOpenID))
		assert.True(t, model.IsSupportedScope(model.ScopeProfile))
		assert.True(t, model.IsSupportedScope(model.ScopeEmail))
		assert.True(t, model.IsSupportedScope(model.ScopeOfflineAccess))
	})

	t.Run("不支持的scope返回false", func(t *testing.T) {
		assert.False(t, model.IsSupportedScope("custom-scope"))
		assert.False(t, model.IsSupportedScope(""))
		assert.False(t, model.IsSupportedScope("openid email")) // 不能包含空格
	})
}

func TestNormalizeScopes(t *testing.T) {
	t.Run("nil返回空切片", func(t *testing.T) {
		result := model.NormalizeScopes(nil)
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("空切片返回空切片", func(t *testing.T) {
		result := model.NormalizeScopes([]string{})
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("去除空字符串", func(t *testing.T) {
		result := model.NormalizeScopes([]string{"openid", "", "email"})
		assert.Equal(t, []string{"openid", "email"}, result)
	})

	t.Run("去除重复项保留首次出现顺序", func(t *testing.T) {
		result := model.NormalizeScopes([]string{"openid", "email", "openid", "profile", "email"})
		assert.Equal(t, []string{"openid", "email", "profile"}, result)
	})

	t.Run("全空字符串返回空切片", func(t *testing.T) {
		result := model.NormalizeScopes([]string{"", "", ""})
		assert.Empty(t, result)
	})
}

func TestIsScopesSubset(t *testing.T) {
	t.Run("空请求是任何集合的子集", func(t *testing.T) {
		assert.True(t, model.IsScopesSubset(nil, []string{"openid"}))
		assert.True(t, model.IsScopesSubset([]string{}, []string{"openid"}))
	})

	t.Run("真子集返回true", func(t *testing.T) {
		allowed := []string{"openid", "email", "profile"}
		assert.True(t, model.IsScopesSubset([]string{"openid", "email"}, allowed))
		assert.True(t, model.IsScopesSubset([]string{"openid"}, allowed))
	})

	t.Run("包含未授权scope返回false", func(t *testing.T) {
		allowed := []string{"openid", "email"}
		assert.False(t, model.IsScopesSubset([]string{"openid", "offline_access"}, allowed))
	})

	t.Run("相等集合返回true", func(t *testing.T) {
		allowed := []string{"openid", "email"}
		assert.True(t, model.IsScopesSubset(allowed, allowed))
	})
}

// ============================================================================
// social_account.go 测试
// ============================================================================

func TestIsSupportedProvider(t *testing.T) {
	t.Run("支持的provider返回true", func(t *testing.T) {
		assert.True(t, model.IsSupportedProvider(model.ProviderGoogle))
		assert.True(t, model.IsSupportedProvider(model.ProviderGitHub))
	})

	t.Run("不支持的provider返回false", func(t *testing.T) {
		assert.False(t, model.IsSupportedProvider("facebook"))
		assert.False(t, model.IsSupportedProvider(""))
		assert.False(t, model.IsSupportedProvider("google-oauth")) // 不能包含后缀
	})
}

func TestProviderMetadataFromJSON(t *testing.T) {
	t.Run("空输入返回nil", func(t *testing.T) {
		assert.Nil(t, model.ProviderMetadataFromJSON(nil))
		assert.Nil(t, model.ProviderMetadataFromJSON([]byte{}))
	})

	t.Run("合法JSON返回map", func(t *testing.T) {
		raw := []byte(`{"raw_id":"123","avatar":"https://example.com/a.png"}`)
		m := model.ProviderMetadataFromJSON(raw)
		assert.Equal(t, "123", m["raw_id"])
		assert.Equal(t, "https://example.com/a.png", m["avatar"])
	})

	t.Run("非法JSON返回nil", func(t *testing.T) {
		raw := []byte(`not a json`)
		assert.Nil(t, model.ProviderMetadataFromJSON(raw))
	})
}

func TestProviderMetadataToJSON(t *testing.T) {
	t.Run("nil输入返回nil", func(t *testing.T) {
		assert.Nil(t, model.ProviderMetadataToJSON(nil))
	})

	t.Run("空map返回非nil", func(t *testing.T) {
		// 空 map 不是 nil，应该正常序列化为 "{}"
		result := model.ProviderMetadataToJSON(map[string]string{})
		assert.Equal(t, "{}", string(result))
	})

	t.Run("正常序列化", func(t *testing.T) {
		m := map[string]string{"raw_id": "123", "avatar": "https://example.com/a.png"}
		result := model.ProviderMetadataToJSON(m)
		// 反序列化验证
		var parsed map[string]string
		err := json.Unmarshal(result, &parsed)
		assert.NoError(t, err)
		assert.Equal(t, "123", parsed["raw_id"])
		assert.Equal(t, "https://example.com/a.png", parsed["avatar"])
	})
}
