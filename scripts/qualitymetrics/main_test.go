package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTemp 写入临时文件并返回路径
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestParseCoverageProfile(t *testing.T) {
	t.Run("正常profile计算总覆盖率", func(t *testing.T) {
		path := writeTemp(t, "coverage.out", `mode: set
github.com/example/sso/internal/service/auth.go:10.20,12.3 2 1
github.com/example/sso/internal/service/auth.go:15.20,17.3 3 0
github.com/example/sso/internal/service/oauth.go:20.20,22.3 5 1
`)
		stats, err := parseCoverageProfile(path)
		require.NoError(t, err)
		// (2+5)/(2+3+5) = 70%
		assert.Equal(t, 70.0, stats.percent)
	})

	t.Run("全部未覆盖", func(t *testing.T) {
		path := writeTemp(t, "coverage.out", `mode: set
a.go:1.1,2.2 4 0
`)
		stats, err := parseCoverageProfile(path)
		require.NoError(t, err)
		assert.Equal(t, 0.0, stats.percent)
	})

	t.Run("空profile报错", func(t *testing.T) {
		path := writeTemp(t, "coverage.out", "mode: set\n")
		_, err := parseCoverageProfile(path)
		assert.Error(t, err)
	})

	t.Run("格式异常行报错", func(t *testing.T) {
		path := writeTemp(t, "coverage.out", "mode: set\nbroken-line\n")
		_, err := parseCoverageProfile(path)
		assert.Error(t, err)
	})

	t.Run("文件不存在报错", func(t *testing.T) {
		_, err := parseCoverageProfile(filepath.Join(t.TempDir(), "missing.out"))
		assert.Error(t, err)
	})
}

func TestParseJUnit(t *testing.T) {
	t.Run("testsuites根节点汇总", func(t *testing.T) {
		path := writeTemp(t, "junit.xml", `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="10" failures="1" errors="0" skipped="2">
  <testsuite name="pkgA" tests="6" failures="1" errors="0" skipped="1"></testsuite>
  <testsuite name="pkgB" tests="4" failures="0" errors="0" skipped="1"></testsuite>
</testsuites>`)
		stats, err := parseJUnit(path)
		require.NoError(t, err)
		assert.Equal(t, 10, stats.total)
		assert.Equal(t, 1, stats.failed)
		// 10 - 1失败 - 2跳过
		assert.Equal(t, 7, stats.passed)
	})

	t.Run("单testsuite根节点兼容", func(t *testing.T) {
		path := writeTemp(t, "junit.xml", `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="pkgA" tests="5" failures="0" errors="0" skipped="0"></testsuite>`)
		stats, err := parseJUnit(path)
		require.NoError(t, err)
		assert.Equal(t, 5, stats.total)
		assert.Equal(t, 5, stats.passed)
	})

	t.Run("无skipped属性时按0计", func(t *testing.T) {
		path := writeTemp(t, "junit.xml", `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="3" failures="0">
  <testsuite name="pkgA" tests="3" failures="0"></testsuite>
</testsuites>`)
		stats, err := parseJUnit(path)
		require.NoError(t, err)
		assert.Equal(t, 3, stats.total)
		assert.Equal(t, 3, stats.passed)
	})

	t.Run("文件不存在报错", func(t *testing.T) {
		_, err := parseJUnit(filepath.Join(t.TempDir(), "missing.xml"))
		assert.Error(t, err)
	})
}
