// Package config_test bugfix探索性测试
// 这些测试用于验证bug condition，在未修复代码上应该失败
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/config"
)

// ============================================================================
// Task 1.2: Bug Condition Exploration - GetEnvPath Fallback Logic
// ============================================================================

// TestBugCondition_GetEnvPathFallback tests that GetEnvPath returns a non-existent
// path instead of falling back to /app/.env when the .env file doesn't exist in cwd.
//
// **Property 1: Bug Condition** - Incorrect File Existence Check
// **CRITICAL**: This test MUST FAIL on unfixed code - failure confirms the bug exists
// **DO NOT attempt to fix the test or the code when it fails**
// **NOTE**: This test encodes the expected behavior - it will validate the fix when it passes
//
// Bug Condition: ENV_FILE_PATH not set, .env doesn't exist in cwd → returns non-existent cwd path
// Expected Behavior: Should return "/app/.env" (default fallback)
func TestBugCondition_GetEnvPathFallback(t *testing.T) {
	originalCwd, err := os.Getwd()
	require.NoError(t, err)

	// CI/CD 修复：使用 t.Setenv 自动恢复，避免环境变量泄漏到后续测试
	// 用空字符串等同于 unset（GetEnvPath 内部检查 envPath != ""）
	t.Setenv("ENV_FILE_PATH", "")
	t.Cleanup(func() { os.Chdir(originalCwd) })

	// Create a temporary directory without .env file
	tmpDir := t.TempDir()

	// Change to temporary directory
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Verify .env does not exist in current directory
	cwdEnvPath := filepath.Join(tmpDir, ".env")
	_, err = os.Stat(cwdEnvPath)
	require.True(t, os.IsNotExist(err), ".env should not exist in test directory")

	// Call GetEnvPath
	result := config.GetEnvPath()

	// Expected: Should return "/app/.env" (default fallback)
	// Unfixed code: Returns non-existent cwd path
	assert.Equal(t, "/app/.env", result, "GetEnvPath should return default /app/.env when cwd .env doesn't exist")

	// Additional verification: the returned path should be the default, not the cwd path
	assert.NotEqual(t, cwdEnvPath, result, "GetEnvPath should not return non-existent cwd path")
}

// TestBugCondition_GetEnvPathWithExistingFile tests that GetEnvPath correctly
// returns the cwd path when .env exists (this should pass on unfixed code).
//
// This test establishes baseline behavior that should be preserved.
func TestBugCondition_GetEnvPathWithExistingFile(t *testing.T) {
	originalCwd, err := os.Getwd()
	require.NoError(t, err)

	// CI/CD 修复：使用 t.Setenv 自动恢复
	t.Setenv("ENV_FILE_PATH", "")
	t.Cleanup(func() { os.Chdir(originalCwd) })

	// Create a temporary directory with .env file
	tmpDir := t.TempDir()

	// Create .env file
	envPath := filepath.Join(tmpDir, ".env")
	err = os.WriteFile(envPath, []byte("TEST=value"), 0600)
	require.NoError(t, err)

	// Change to temporary directory
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Call GetEnvPath
	result := config.GetEnvPath()

	// Expected: Should return cwd .env path (existing file)
	// This should pass on both unfixed and fixed code
	assert.Equal(t, envPath, result, "GetEnvPath should return cwd .env when it exists")
}

// TestBugCondition_GetEnvPathPriority tests that ENV_FILE_PATH environment
// variable takes priority (this should pass on unfixed code).
//
// This test establishes baseline behavior that should be preserved.
func TestBugCondition_GetEnvPathPriority(t *testing.T) {
	// CI/CD 修复：使用 t.Setenv 自动恢复
	customPath := "/custom/path/.env"
	t.Setenv("ENV_FILE_PATH", customPath)

	// Call GetEnvPath
	result := config.GetEnvPath()

	// Expected: Should return custom path (highest priority)
	// This should pass on both unfixed and fixed code
	assert.Equal(t, customPath, result, "GetEnvPath should return ENV_FILE_PATH when set")
}
