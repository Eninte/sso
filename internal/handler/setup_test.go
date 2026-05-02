// Package handler 配置向导单元测试
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ValidateKeyPath 测试
// ============================================================================

func TestValidateKeyPath(t *testing.T) {
	t.Run("有效路径", func(t *testing.T) {
		validPaths := []string{
			"/app/keys/private.pem",
			"/keys/public.pem",
			"/etc/sso/keys/jwt.key",
		}

		for _, path := range validPaths {
			err := ValidateKeyPath(path)
			assert.NoError(t, err, "路径应该有效: %s", path)
		}
	})

	t.Run("空路径", func(t *testing.T) {
		err := ValidateKeyPath("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "路径不能为空")
	})

	t.Run("相对路径", func(t *testing.T) {
		err := ValidateKeyPath("keys/private.pem")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "必须使用绝对路径")
	})

	t.Run("不在白名单内的路径", func(t *testing.T) {
		err := ValidateKeyPath("/tmp/keys/private.pem")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "路径必须在允许的目录内")
	})

	t.Run("路径遍历攻击", func(t *testing.T) {
		err := ValidateKeyPath("/app/keys/../../../etc/passwd")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "路径必须在允许的目录内")
	})

	t.Run("白名单目录本身", func(t *testing.T) {
		err := ValidateKeyPath("/app/keys")
		assert.NoError(t, err)
	})
}

// ============================================================================
// getKeyPathWhitelist 测试
// ============================================================================

func TestGetKeyPathWhitelist(t *testing.T) {
	t.Run("使用默认白名单", func(t *testing.T) {
		// 清除环境变量
		os.Unsetenv("KEY_PATH_WHITELIST")

		whitelist := getKeyPathWhitelist()

		// 默认包含3个固定路径 + 当前工作目录/keys
		assert.GreaterOrEqual(t, len(whitelist), 3)
		assert.Contains(t, whitelist, "/app/keys")
		assert.Contains(t, whitelist, "/keys")
		assert.Contains(t, whitelist, "/etc/sso/keys")
	})

	t.Run("自定义单个路径", func(t *testing.T) {
		os.Setenv("KEY_PATH_WHITELIST", "/custom/keys")
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		whitelist := getKeyPathWhitelist()

		assert.Len(t, whitelist, 1)
		assert.Contains(t, whitelist, "/custom/keys")
	})

	t.Run("自定义多个路径", func(t *testing.T) {
		os.Setenv("KEY_PATH_WHITELIST", "/custom/keys,/opt/sso/keys,/var/lib/sso")
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		whitelist := getKeyPathWhitelist()

		assert.Len(t, whitelist, 3)
		assert.Contains(t, whitelist, "/custom/keys")
		assert.Contains(t, whitelist, "/opt/sso/keys")
		assert.Contains(t, whitelist, "/var/lib/sso")
	})

	t.Run("自定义路径包含空格", func(t *testing.T) {
		os.Setenv("KEY_PATH_WHITELIST", " /custom/keys , /opt/sso/keys ")
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		whitelist := getKeyPathWhitelist()

		assert.Len(t, whitelist, 2)
		assert.Contains(t, whitelist, "/custom/keys")
		assert.Contains(t, whitelist, "/opt/sso/keys")
	})

	t.Run("自定义路径包含相对路径", func(t *testing.T) {
		os.Setenv("KEY_PATH_WHITELIST", "/custom/keys,relative/path,/opt/sso/keys")
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		whitelist := getKeyPathWhitelist()

		// 相对路径应该被过滤掉
		assert.Len(t, whitelist, 2)
		assert.Contains(t, whitelist, "/custom/keys")
		assert.Contains(t, whitelist, "/opt/sso/keys")
		assert.NotContains(t, whitelist, "relative/path")
	})

	t.Run("自定义路径全部无效", func(t *testing.T) {
		os.Setenv("KEY_PATH_WHITELIST", "relative/path,another/relative")
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		whitelist := getKeyPathWhitelist()

		// 应该回退到默认值（3个固定路径 + 当前工作目录/keys）
		assert.GreaterOrEqual(t, len(whitelist), 3)
		assert.Contains(t, whitelist, "/app/keys")
		assert.Contains(t, whitelist, "/keys")
		assert.Contains(t, whitelist, "/etc/sso/keys")
	})

	t.Run("空环境变量", func(t *testing.T) {
		os.Setenv("KEY_PATH_WHITELIST", "")
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		whitelist := getKeyPathWhitelist()

		// 应该使用默认值（3个固定路径 + 当前工作目录/keys）
		assert.GreaterOrEqual(t, len(whitelist), 3)
		assert.Contains(t, whitelist, "/app/keys")
	})

	t.Run("只有逗号的环境变量", func(t *testing.T) {
		os.Setenv("KEY_PATH_WHITELIST", ",,,")
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		whitelist := getKeyPathWhitelist()

		// 应该回退到默认值（3个固定路径 + 当前工作目录/keys）
		assert.GreaterOrEqual(t, len(whitelist), 3)
		assert.Contains(t, whitelist, "/app/keys")
	})
}

// ============================================================================
// ValidateKeyPath 与自定义白名单集成测试
// ============================================================================

func TestValidateKeyPath_WithCustomWhitelist(t *testing.T) {
	t.Run("自定义白名单路径验证", func(t *testing.T) {
		os.Setenv("KEY_PATH_WHITELIST", "/custom/keys")
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		// 自定义路径应该有效
		err := ValidateKeyPath("/custom/keys/private.pem")
		assert.NoError(t, err)

		// 默认路径应该无效
		err = ValidateKeyPath("/app/keys/private.pem")
		assert.Error(t, err)
	})

	t.Run("多个自定义白名单路径", func(t *testing.T) {
		os.Setenv("KEY_PATH_WHITELIST", "/custom/keys,/opt/sso/keys")
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		// 两个路径都应该有效
		err := ValidateKeyPath("/custom/keys/private.pem")
		assert.NoError(t, err)

		err = ValidateKeyPath("/opt/sso/keys/public.pem")
		assert.NoError(t, err)

		// 其他路径应该无效
		err = ValidateKeyPath("/tmp/keys/private.pem")
		assert.Error(t, err)
	})
}

// ============================================================================
// NewSetupHandler 测试
// ============================================================================

func TestNewSetupHandler(t *testing.T) {
	t.Run("成功创建handler", func(t *testing.T) {
		handler := NewSetupHandler(".env", "1.0.0")

		assert.NotNil(t, handler)
		assert.Equal(t, ".env", handler.envPath)
		assert.Equal(t, "1.0.0", handler.version)

		// 验证令牌已生成
		token := handler.GetSetupToken()
		assert.NotEmpty(t, token)
		assert.Len(t, token, 64) // 32字节的hex编码
	})

	t.Run("每次创建的令牌不同", func(t *testing.T) {
		handler1 := NewSetupHandler(".env", "1.0.0")
		handler2 := NewSetupHandler(".env", "1.0.0")

		token1 := handler1.GetSetupToken()
		token2 := handler2.GetSetupToken()

		assert.NotEqual(t, token1, token2)
	})
}

// ============================================================================
// GetSetupToken 测试
// ============================================================================

func TestSetupHandler_GetSetupToken(t *testing.T) {
	t.Run("获取令牌", func(t *testing.T) {
		handler := NewSetupHandler(".env", "1.0.0")

		token := handler.GetSetupToken()
		assert.NotEmpty(t, token)
		assert.Len(t, token, 64)
	})
}

// ============================================================================
// openDB 测试
// ============================================================================

func TestOpenDB(t *testing.T) {
	t.Run("无效DSN", func(t *testing.T) {
		db, err := openDB("invalid-dsn")

		// 可能返回错误，或者返回db但连接失败
		if err == nil && db != nil {
			db.Close()
		}
		// 这个测试主要确保函数不会panic
		assert.True(t, true)
	})

	t.Run("连接参数设置", func(t *testing.T) {
		// 使用无效DSN，但我们只是测试函数不会panic
		db, err := openDB("postgres://invalid")
		if err == nil && db != nil {
			// 验证连接池设置
			stats := db.Stats()
			assert.Equal(t, 1, stats.MaxOpenConnections)
			db.Close()
		}
		assert.True(t, true)
	})
}

// ============================================================================
// newRedisClient 测试
// ============================================================================

func TestNewRedisClient(t *testing.T) {
	t.Run("创建客户端", func(t *testing.T) {
		client := newRedisClient("localhost:6379", "password", 0)

		assert.NotNil(t, client)
		assert.Equal(t, "localhost:6379", client.Options().Addr)
		assert.Equal(t, "password", client.Options().Password)
		assert.Equal(t, 0, client.Options().DB)

		client.Close()
	})

	t.Run("不同参数", func(t *testing.T) {
		client := newRedisClient("redis:6379", "secret", 5)

		assert.NotNil(t, client)
		assert.Equal(t, "redis:6379", client.Options().Addr)
		assert.Equal(t, "secret", client.Options().Password)
		assert.Equal(t, 5, client.Options().DB)

		client.Close()
	})
}

// ============================================================================
// HandleSetupGenerateKeys 测试
// ============================================================================

func TestSetupHandler_HandleSetupGenerateKeys(t *testing.T) {
	handler := NewSetupHandler(".env", "1.0.0")

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/generate-keys", bytes.NewBufferString("invalid-json"))
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()

		handler.HandleSetupGenerateKeys(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("路径验证失败", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"private_path": "relative/path/private.pem",
			"public_path":  "/app/keys/public.pem",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/generate-keys", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()

		handler.HandleSetupGenerateKeys(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("成功生成密钥", func(t *testing.T) {
		tmpDir := t.TempDir()

		os.Setenv("KEY_PATH_WHITELIST", tmpDir)
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		privatePath := filepath.Join(tmpDir, "private.pem")
		publicPath := filepath.Join(tmpDir, "public.pem")

		reqBody := map[string]interface{}{
			"private_path": privatePath,
			"public_path":  publicPath,
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/generate-keys", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()

		handler.HandleSetupGenerateKeys(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		assert.FileExists(t, privatePath)
		assert.FileExists(t, publicPath)

		privInfo, err := os.Stat(privatePath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), privInfo.Mode().Perm())

		pubInfo, err := os.Stat(publicPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0644), pubInfo.Mode().Perm())

		privContent, err := os.ReadFile(privatePath)
		require.NoError(t, err)
		assert.Contains(t, string(privContent), "BEGIN RSA PRIVATE KEY")

		pubContent, err := os.ReadFile(publicPath)
		require.NoError(t, err)
		assert.Contains(t, string(pubContent), "BEGIN PUBLIC KEY")
	})

	t.Run("非本地访问被拒绝", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/generate-keys", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		handler.HandleSetupGenerateKeys(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestValidateKeyPath_EdgeCases(t *testing.T) {
	t.Run("路径包含点号", func(t *testing.T) {
		err := ValidateKeyPath("/app/keys/my.key.pem")
		assert.NoError(t, err)
	})

	t.Run("路径包含多个斜杠", func(t *testing.T) {
		err := ValidateKeyPath("/app//keys///private.pem")
		// filepath.Clean 会清理多余的斜杠
		assert.NoError(t, err)
	})

	t.Run("路径以斜杠结尾", func(t *testing.T) {
		err := ValidateKeyPath("/app/keys/")
		// filepath.Clean 会移除尾部斜杠
		assert.NoError(t, err)
	})

	t.Run("非常长的路径", func(t *testing.T) {
		// 创建一个长但不超过系统限制的路径（在白名单内）
		// Linux文件名限制通常是255字节，路径限制是4096字节
		longPath := "/app/keys/" + strings.Repeat("a", 200) + ".pem"
		err := ValidateKeyPath(longPath)
		// 长路径只要在白名单内就应该有效
		assert.NoError(t, err)
	})
}

func TestGetKeyPathWhitelist_EdgeCases(t *testing.T) {
	t.Run("环境变量包含重复路径", func(t *testing.T) {
		os.Setenv("KEY_PATH_WHITELIST", "/custom/keys,/custom/keys,/opt/sso")
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		whitelist := getKeyPathWhitelist()

		// 应该包含重复的路径（不去重）
		assert.Len(t, whitelist, 3)
	})

	t.Run("环境变量包含特殊字符", func(t *testing.T) {
		os.Setenv("KEY_PATH_WHITELIST", "/custom/keys with spaces,/opt/sso-keys")
		defer os.Unsetenv("KEY_PATH_WHITELIST")

		whitelist := getKeyPathWhitelist()

		// 路径中的空格应该被保留
		assert.Contains(t, whitelist, "/custom/keys with spaces")
		assert.Contains(t, whitelist, "/opt/sso-keys")
	})
}
