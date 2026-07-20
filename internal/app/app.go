// Package app 进程入口编排
// Run 串联配置加载、日志初始化、服务装配、路由装配、服务器启动
package app

import (
	"log/slog"
	"os"

	"github.com/example/sso/internal/config"
	"github.com/example/sso/internal/logging"
)

// Run 启动 SSO 服务
// version/buildTime 通过 -ldflags 注入，用于审计日志与初始化面板展示
// 流程: 加载配置（失败则启动配置向导）→ 初始化日志 → 初始化服务 → 装配路由 → 启动服务器
func Run(version, buildTime string) {
	// 1. 加载配置
	// 注意：配置向导仅在 initConfig() 失败时启动，
	// 配置加载成功时不会进入 startSetupWizard，因此不存在配置正常时暴露向导的风险
	cfg, err := initConfig()
	if err != nil {
		slog.Warn("配置加载失败，启动配置向导", "error", err)
		startSetupWizard(err, version)
		return
	}

	// 2. 初始化日志
	initLogger(cfg.Env)

	slog.Info("SSO服务初始化中...",
		"env", cfg.Env,
		"port", cfg.ServerPort,
	)

	// 3. 初始化服务
	svc, db, err := initServices(cfg, version, buildTime)
	if err != nil {
		slog.Error("服务初始化失败", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	defer svc.Cache.Close()

	// 4. 初始化处理器和路由
	router, rateLimiters, err := initHandlers(cfg, svc, version, buildTime)
	if err != nil {
		slog.Error("路由初始化失败", "error", err)
		// 显式释放已初始化的资源，os.Exit 不会执行 defer
		_ = db.Close()
		_ = svc.Cache.Close()
		os.Exit(1)
	}

	// 4.1 初始化面板服务器（loopback 隔离）
	// 仅当 INIT_ENABLED=true 时创建；否则返回 nil，跳过启动
	initServer := NewInitServer(cfg, svc.Store, svc.Password, svc.Cache, svc.Audit, version, buildTime)

	// 5. 启动服务器
	startServer(cfg, router, rateLimiters, svc, version, initServer)
}

// initConfig 加载和验证配置
func initConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// initLogger 初始化日志系统
func initLogger(env string) {
	logging.InitForEnv(env)
}
