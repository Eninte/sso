// Package main SSO服务入口
// 仅负责解析 version 子命令并将控制权交给 app 组合根
package main

import (
	"fmt"
	"os"

	"github.com/example/sso/internal/app"
)

// Version 服务版本号，通过 -ldflags 注入
var Version = "dev"

// BuildTime 构建时间，通过 -ldflags 注入
var BuildTime = "unknown"

func main() {
	// version 子命令：打印版本信息后退出
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("SSO %s (构建时间: %s)\n", Version, BuildTime)
		os.Exit(0)
	}

	// 交由组合根完成配置加载、服务装配与服务器启动
	app.Run(Version, BuildTime)
}
