// Package main SSO服务入口测试
package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestVersionVariables 验证版本变量默认值
// 默认值在未通过 -ldflags 注入时生效，此处捕获默认值被意外修改的回归
func TestVersionVariables(t *testing.T) {
	assert.Equal(t, "dev", Version, "默认 Version 应为 'dev'")
	assert.Equal(t, "unknown", BuildTime, "默认 BuildTime 应为 'unknown'")
}

// TestMain_VersionSubcommand 测试 version 子命令输出
// 通过子进程执行 main()，验证版本信息输出格式
func TestMain_VersionSubcommand(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过子进程测试：-short 模式")
	}

	// 编译当前包为临时二进制
	binary, err := os.CreateTemp("", "sso-test-*")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(binary.Name())

	// 编译
	cmd := exec.Command("go", "build", "-o", binary.Name(), ".")
	cmd.Dir = "."
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译失败: %v\n%s", err, output)
	}

	// 执行 version 子命令
	cmd = exec.Command(binary.Name(), "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("执行 version 子命令失败: %v", err)
	}

	outputStr := strings.TrimSpace(string(output))

	// 验证输出包含 "SSO" 前缀
	if !strings.HasPrefix(outputStr, "SSO ") {
		t.Errorf("version 输出应以 'SSO ' 开头，实际: %s", outputStr)
	}

	// 验证输出包含版本号
	if !strings.Contains(outputStr, Version) {
		t.Errorf("version 输出应包含版本号 %q，实际: %s", Version, outputStr)
	}
}
