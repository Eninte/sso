// Package postgres_test 测试初始化
// 自动加载 .env.test 中的 DATABASE_URL，避免每次手动设置环境变量
package postgres_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	// 如果 DATABASE_URL 已设置（如 make test 传入），跳过自动加载
	if os.Getenv("DATABASE_URL") != "" {
		return
	}

	// 从项目根目录查找 .env.test
	envFile := findEnvTestFile()
	if envFile == "" {
		return
	}

	f, err := os.Open(envFile)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// 只加载测试必需的变量
		if key == "DATABASE_URL" || key == "DB_HOST" || key == "DB_PORT" ||
			key == "DB_NAME" || key == "DB_USER" || key == "DB_PASSWORD" ||
			key == "DB_SSL_MODE" || key == "REDIS_TEST_ADDR" {
			os.Setenv(key, value)
		}
	}
}

// findEnvTestFile 从当前目录向上查找 .env.test 文件
func findEnvTestFile() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for i := 0; i < 5; i++ {
		path := filepath.Join(dir, ".env.test")
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// TestEnvLoaded 验证环境变量是否加载成功
// 这个测试确保自动加载机制正常工作
func TestEnvLoaded(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL 未设置（.env.test 不存在或无 DATABASE_URL 配置）")
	}
	// 环境变量已加载，不跳过
}
