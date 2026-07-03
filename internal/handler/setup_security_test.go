package handler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ValidateKeyPath 安全测试
// ============================================================================

// TestValidateKeyPath_PrefixBypass 测试白名单前缀匹配绕过防护
func TestValidateKeyPath_PrefixBypass(t *testing.T) {
	// 创建测试目录
	tmpDir := t.TempDir()

	// 设置白名单为测试目录下的keys子目录
	keysDir := filepath.Join(tmpDir, "keys")
	require.NoError(t, os.MkdirAll(keysDir, 0755))

	// 创建恶意目录（前缀匹配但不在白名单内）
	maliciousDir := filepath.Join(tmpDir, "keys_malicious")
	require.NoError(t, os.MkdirAll(maliciousDir, 0755))

	// 设置自定义白名单
	t.Setenv("KEY_PATH_WHITELIST", keysDir)

	t.Run("允许白名单目录中的文件", func(t *testing.T) {
		validPath := filepath.Join(keysDir, "private.pem")
		err := ValidateKeyPath(validPath)
		assert.NoError(t, err, "白名单目录中的文件应该被允许")
	})

	t.Run("允许白名单目录的子目录中的文件", func(t *testing.T) {
		subDir := filepath.Join(keysDir, "subdir")
		require.NoError(t, os.MkdirAll(subDir, 0755))
		validPath := filepath.Join(subDir, "private.pem")
		err := ValidateKeyPath(validPath)
		assert.NoError(t, err, "白名单子目录中的文件应该被允许")
	})

	t.Run("拒绝前缀匹配但不在白名单内的路径", func(t *testing.T) {
		maliciousPath := filepath.Join(maliciousDir, "evil.pem")
		err := ValidateKeyPath(maliciousPath)
		assert.Error(t, err, "前缀匹配但不在白名单内的路径应该被拒绝")
		assert.Contains(t, err.Error(), "path must be within allowed directories")
	})

	t.Run("拒绝白名单目录的兄弟目录", func(t *testing.T) {
		siblingDir := filepath.Join(tmpDir, "keys2")
		require.NoError(t, os.MkdirAll(siblingDir, 0755))
		siblingPath := filepath.Join(siblingDir, "private.pem")
		err := ValidateKeyPath(siblingPath)
		assert.Error(t, err, "白名单目录的兄弟目录应该被拒绝")
	})

	t.Run("允许完全匹配白名单目录的文件", func(t *testing.T) {
		// 测试边界情况：文件直接在白名单目录中
		exactPath := filepath.Join(keysDir, "key.pem")
		err := ValidateKeyPath(exactPath)
		assert.NoError(t, err, "白名单目录中的文件应该被允许")
	})
}

// TestValidateKeyPath_ErrorMessageSafety 测试错误消息不泄露敏感信息
func TestValidateKeyPath_ErrorMessageSafety(t *testing.T) {
	t.Run("空路径错误消息", func(t *testing.T) {
		err := ValidateKeyPath("")
		require.Error(t, err)
		// 错误消息应该是通用的，不泄露内部细节
		assert.Equal(t, "path cannot be empty", err.Error())
	})

	t.Run("相对路径错误消息", func(t *testing.T) {
		err := ValidateKeyPath("relative/path.pem")
		require.Error(t, err)
		assert.Equal(t, "absolute path is required", err.Error())
	})

	t.Run("路径遍历错误消息", func(t *testing.T) {
		err := ValidateKeyPath("/tmp/../etc/passwd")
		require.Error(t, err)
		// filepath.Clean会规范化路径,所以".."会被处理掉
		// 但路径/etc/passwd不在白名单内,所以会返回白名单错误
		assert.Contains(t, err.Error(), "path must be within allowed directories")
	})

	t.Run("白名单外路径错误消息", func(t *testing.T) {
		err := ValidateKeyPath("/unauthorized/path/key.pem")
		require.Error(t, err)
		// 错误消息应该包含白名单信息，但不泄露具体的失败原因
		assert.Contains(t, err.Error(), "path must be within allowed directories")
	})
}

// TestValidateKeyPath_SymlinkBypass 测试符号链接绕过防护
func TestValidateKeyPath_SymlinkBypass(t *testing.T) {
	// 创建测试目录
	tmpDir := t.TempDir()

	// 创建白名单目录
	keysDir := filepath.Join(tmpDir, "keys")
	require.NoError(t, os.MkdirAll(keysDir, 0755))

	// 创建恶意目录
	maliciousDir := filepath.Join(tmpDir, "malicious")
	require.NoError(t, os.MkdirAll(maliciousDir, 0755))

	// 设置自定义白名单
	t.Setenv("KEY_PATH_WHITELIST", keysDir)

	t.Run("拒绝通过符号链接绕过白名单", func(t *testing.T) {
		// 在白名单目录中创建指向恶意目录的符号链接
		symlinkPath := filepath.Join(keysDir, "symlink")
		err := os.Symlink(maliciousDir, symlinkPath)
		if err != nil {
			t.Skip("无法创建符号链接，跳过测试")
		}

		// 尝试访问符号链接指向的文件
		targetPath := filepath.Join(symlinkPath, "evil.pem")
		err = ValidateKeyPath(targetPath)

		// 应该被拒绝，因为真实路径不在白名单内
		assert.Error(t, err, "通过符号链接绕过白名单应该被拒绝")
	})
}

// TestValidateKeyPath_EdgeCases_Security 测试边界情况（安全相关）
func TestValidateKeyPath_EdgeCases_Security(t *testing.T) {
	tmpDir := t.TempDir()
	keysDir := filepath.Join(tmpDir, "keys")
	require.NoError(t, os.MkdirAll(keysDir, 0755))
	t.Setenv("KEY_PATH_WHITELIST", keysDir)

	t.Run("路径末尾有斜杠", func(t *testing.T) {
		pathWithSlash := filepath.Join(keysDir, "key.pem") + string(filepath.Separator)
		err := ValidateKeyPath(pathWithSlash)
		// filepath.Clean 会移除末尾斜杠，应该被接受
		assert.NoError(t, err)
	})

	t.Run("路径中有多个连续斜杠", func(t *testing.T) {
		pathWithDoubleSlash := filepath.Join(keysDir, "subdir") + string(filepath.Separator) + string(filepath.Separator) + "key.pem"
		err := ValidateKeyPath(pathWithDoubleSlash)
		// filepath.Clean 会规范化路径，应该被接受
		assert.NoError(t, err)
	})

	t.Run("白名单目录本身", func(t *testing.T) {
		// 测试白名单目录本身（不是其中的文件）
		err := ValidateKeyPath(keysDir)
		// 应该被接受，因为 dir == allowedDir
		assert.NoError(t, err)
	})
}

// TestGetKeyPathWhitelist_Security 测试白名单获取逻辑（安全相关）
func TestGetKeyPathWhitelist_Security(t *testing.T) {
	t.Run("默认白名单", func(t *testing.T) {
		// 清除环境变量
		t.Setenv("KEY_PATH_WHITELIST", "")

		whitelist := getKeyPathWhitelist()

		// 应该包含默认目录
		assert.Contains(t, whitelist, "/app/keys")
		assert.Contains(t, whitelist, "/keys")
		assert.Contains(t, whitelist, "/etc/sso/keys")

		// 应该包含当前工作目录的keys子目录
		cwd, _ := os.Getwd()
		if cwd != "" {
			cwdKeys := filepath.Join(cwd, "keys")
			assert.Contains(t, whitelist, cwdKeys)
		}
	})

	t.Run("自定义白名单", func(t *testing.T) {
		customDirs := "/custom/keys,/another/path"
		t.Setenv("KEY_PATH_WHITELIST", customDirs)

		whitelist := getKeyPathWhitelist()

		// 应该只包含自定义目录
		assert.Contains(t, whitelist, "/custom/keys")
		assert.Contains(t, whitelist, "/another/path")
		assert.Len(t, whitelist, 2)
	})

	t.Run("自定义白名单带空格", func(t *testing.T) {
		customDirs := " /custom/keys , /another/path "
		t.Setenv("KEY_PATH_WHITELIST", customDirs)

		whitelist := getKeyPathWhitelist()

		// 应该正确处理空格
		assert.Contains(t, whitelist, "/custom/keys")
		assert.Contains(t, whitelist, "/another/path")
	})

	t.Run("自定义白名单包含相对路径", func(t *testing.T) {
		customDirs := "/custom/keys,relative/path"
		t.Setenv("KEY_PATH_WHITELIST", customDirs)

		whitelist := getKeyPathWhitelist()

		// 相对路径应该被忽略
		assert.Contains(t, whitelist, "/custom/keys")
		assert.NotContains(t, whitelist, "relative/path")
	})

	t.Run("自定义白名单为空或无效时回退到默认值", func(t *testing.T) {
		t.Setenv("KEY_PATH_WHITELIST", "relative/path,another/relative")

		whitelist := getKeyPathWhitelist()

		// 应该回退到默认白名单
		assert.Contains(t, whitelist, "/app/keys")
		assert.Contains(t, whitelist, "/keys")
		assert.Contains(t, whitelist, "/etc/sso/keys")
	})
}

// BenchmarkValidateKeyPath 性能基准测试
func BenchmarkValidateKeyPath(b *testing.B) {
	tmpDir := b.TempDir()
	keysDir := filepath.Join(tmpDir, "keys")
	_ = os.MkdirAll(keysDir, 0755)
	b.Setenv("KEY_PATH_WHITELIST", keysDir)

	validPath := filepath.Join(keysDir, "private.pem")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateKeyPath(validPath)
	}
}

// TestValidateKeyPath_ConcurrentAccess 测试并发访问安全性
func TestValidateKeyPath_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	keysDir := filepath.Join(tmpDir, "keys")
	require.NoError(t, os.MkdirAll(keysDir, 0755))
	t.Setenv("KEY_PATH_WHITELIST", keysDir)

	validPath := filepath.Join(keysDir, "private.pem")

	// 并发调用 ValidateKeyPath
	const numGoroutines = 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			err := ValidateKeyPath(validPath)
			assert.NoError(t, err)
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// TestValidateKeyPath_RealWorldScenarios 测试真实场景
func TestValidateKeyPath_RealWorldScenarios(t *testing.T) {
	t.Run("Docker环境路径", func(t *testing.T) {
		t.Setenv("KEY_PATH_WHITELIST", "/app/keys")

		dockerPath := "/app/keys/jwt/private.pem"
		err := ValidateKeyPath(dockerPath)
		assert.NoError(t, err, "Docker环境路径应该被接受")
	})

	t.Run("开发环境路径", func(t *testing.T) {
		cwd, _ := os.Getwd()
		if cwd == "" {
			t.Skip("无法获取当前工作目录")
		}

		t.Setenv("KEY_PATH_WHITELIST", "")

		devPath := filepath.Join(cwd, "keys", "private.pem")
		err := ValidateKeyPath(devPath)
		assert.NoError(t, err, "开发环境路径应该被接受")
	})

	t.Run("生产环境路径", func(t *testing.T) {
		t.Setenv("KEY_PATH_WHITELIST", "/etc/sso/keys")

		prodPath := "/etc/sso/keys/production/private.pem"
		err := ValidateKeyPath(prodPath)
		assert.NoError(t, err, "生产环境路径应该被接受")
	})
}
