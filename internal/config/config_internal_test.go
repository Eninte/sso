// Package config 内部测试（访问未导出函数 escapeEnvValue）
package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// escapeEnvValue 测试
// ============================================================================

func TestEscapeEnvValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "纯字母数字_不转义",
			input: "localhost",
			want:  "localhost",
		},
		{
			name:  "普通密码_不转义",
			input: "Abc123456!",
			want:  "Abc123456!",
		},
		{
			name:  "包含空格_加引号",
			input: "value with space",
			want:  `"value with space"`,
		},
		{
			name:  "包含双引号_转义并加引号",
			input: `with"quote`,
			want:  `"with\"quote"`,
		},
		{
			name:  "包含反斜杠_转义并加引号",
			input: `back\slash`,
			want:  `"back\\slash"`,
		},
		{
			name:  "包含美元符_转义并加引号",
			input: "dollar$sign",
			want:  `"dollar\$sign"`,
		},
		{
			name:  "包含井号_加引号",
			input: "hash#mark",
			want:  `"hash#mark"`,
		},
		{
			name:  "包含换行_转义并加引号",
			input: "line1\nline2",
			want:  `"line1\nline2"`,
		},
		{
			name:  "包含回车_转义并加引号",
			input: "line1\rline2",
			want:  `"line1\rline2"`,
		},
		{
			name:  "空字符串_不转义",
			input: "",
			want:  "",
		},
		{
			name:  "多特殊字符组合",
			input: `a "b" $c\n#d e`,
			want:  `"a \"b\" \$c\\n#d e"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeEnvValue(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ============================================================================
// WriteEnvFile 测试
// ============================================================================

func TestWriteEnvFile(t *testing.T) {
	t.Run("写入有序键_内容按预定顺序", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")

		values := map[string]string{
			"DB_PASSWORD": "secret123",
			"DB_HOST":     "localhost",
			"SERVER_PORT": "8080",
		}
		require.NoError(t, WriteEnvFile(path, values))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		text := string(content)

		// SERVER_PORT 应在 DB_HOST 之前（按 order 列表）
		idxPort := strings.Index(text, "SERVER_PORT")
		idxHost := strings.Index(text, "DB_HOST")
		idxPwd := strings.Index(text, "DB_PASSWORD")
		require.True(t, idxPort >= 0 && idxHost > 0 && idxPwd > 0)
		assert.Less(t, idxPort, idxHost, "SERVER_PORT 应排在 DB_HOST 前")
		assert.Less(t, idxHost, idxPwd, "DB_HOST 应排在 DB_PASSWORD 前")
	})

	t.Run("值含特殊字符_被转义", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")

		values := map[string]string{
			"SMTP_PASSWORD": `p@ss "word" $var`,
		}
		require.NoError(t, WriteEnvFile(path, values))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		// 应被双引号包裹并转义内部字符
		assert.Contains(t, string(content), `SMTP_PASSWORD="p@ss \"word\" \$var"`)
	})

	t.Run("包含非顺序键_追加到末尾", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")

		values := map[string]string{
			"CUSTOM_KEY":     "custom",
			"SERVER_HOST":    "0.0.0.0",
			"ANOTHER_CUSTOM": "val",
		}
		require.NoError(t, WriteEnvFile(path, values))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		text := string(content)

		// 顺序键在前
		idxHost := strings.Index(text, "SERVER_HOST")
		idxCustom := strings.Index(text, "CUSTOM_KEY")
		idxAnother := strings.Index(text, "ANOTHER_CUSTOM")
		require.True(t, idxHost >= 0 && idxCustom > 0 && idxAnother > 0)
		assert.Less(t, idxHost, idxCustom, "顺序键应在自定义键前")
		assert.Less(t, idxHost, idxAnother, "顺序键应在自定义键前")
	})

	t.Run("文件权限0600", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")

		require.NoError(t, WriteEnvFile(path, map[string]string{"K": "v"}))

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "文件权限应为 0600")
	})

	t.Run("空values_写入空文件仅含换行", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")

		require.NoError(t, WriteEnvFile(path, map[string]string{}))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "\n", string(content), "空配置应只含结尾换行")
	})

	t.Run("原子写入_临时文件被清理", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")

		require.NoError(t, WriteEnvFile(path, map[string]string{"K": "v"}))

		// .tmp 临时文件应已不存在（Rename 后被移走）
		_, err := os.Stat(path + ".tmp")
		assert.True(t, os.IsNotExist(err), "临时文件应被清理")
	})

	t.Run("覆盖已存在文件", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")

		// 先写入旧内容
		require.NoError(t, os.WriteFile(path, []byte("OLD=value\n"), 0600))
		// 再用 WriteEnvFile 覆盖
		require.NoError(t, WriteEnvFile(path, map[string]string{"NEW": "value"}))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(content), "NEW=value")
		assert.NotContains(t, string(content), "OLD=value", "旧内容应被覆盖")
	})

	t.Run("无效路径_返回错误", func(t *testing.T) {
		err := WriteEnvFile("/nonexistent/dir/.env", map[string]string{"K": "v"})
		assert.Error(t, err, "无效路径应返回错误")
	})
}

// ============================================================================
// GetTrustedProxies 测试
// ============================================================================

func TestGetTrustedProxies(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "空字符串_返回nil", input: "", want: nil},
		{name: "单个IP", input: "127.0.0.1", want: []string{"127.0.0.1"}},
		{name: "多个IP_逗号分隔", input: "127.0.0.1, 10.0.0.1, 172.16.0.1", want: []string{"127.0.0.1", "10.0.0.1", "172.16.0.1"}},
		{name: "含空段_被过滤", input: "127.0.0.1,, 10.0.0.1", want: []string{"127.0.0.1", "10.0.0.1"}},
		{name: "仅空格_返回空切片", input: "   ", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{TrustedProxies: tt.input}
			got := c.GetTrustedProxies()
			assert.Equal(t, tt.want, got)
		})
	}
}

// ============================================================================
// GetJWTTransitionPubKeyPaths 测试
// ============================================================================

func TestGetJWTTransitionPubKeyPaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "空字符串_返回nil", input: "", want: nil},
		{name: "单个路径", input: "/keys/old.pub", want: []string{"/keys/old.pub"}},
		{
			name:  "多个路径_逗号分隔",
			input: "/keys/old.pub, /keys/older.pub",
			want:  []string{"/keys/old.pub", "/keys/older.pub"},
		},
		{name: "含空段_被过滤", input: "/keys/old.pub,, /keys/older.pub", want: []string{"/keys/old.pub", "/keys/older.pub"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{JWTTransitionPubKeyPaths: tt.input}
			got := c.GetJWTTransitionPubKeyPaths()
			assert.Equal(t, tt.want, got)
		})
	}
}

// 辅助：排序字符串切片（用于断言无序场景）
func sortedSlice(s []string) []string {
	cp := make([]string, len(s))
	copy(cp, s)
	sort.Strings(cp)
	return cp
}

var _ = sortedSlice // 保留以备未来断言使用
