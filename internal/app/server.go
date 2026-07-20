// Package app HTTP 服务器启动与优雅关闭
package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/sso/internal/config"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/service"
)

// startServer 启动HTTP服务器并处理优雅关闭
// 职责: 创建HTTP服务器、启动监听、等待中断信号、优雅关闭资源
// initServer 为初始化面板的独立 loopback 服务器，可为 nil（INIT_ENABLED=false 时）
func startServer(
	cfg *config.Config,
	handler http.Handler,
	rateLimiters []middleware.RateLimitMiddleware,
	svc *Services,
	version string,
	initServer *InitServer,
) {
	// ==== 创建HTTP服务器 ====
	server := &http.Server{
		Addr:         cfg.ServerHost + ":" + cfg.ServerPort,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ==== 记录服务启动 ====
	svc.Audit.LogSystemStart(context.Background(), version)

	slog.Info("SSO服务初始化完成",
		"endpoints", []string{
			"/health",
			"/metrics",
			"/.well-known/openid-configuration",
			"/api/v1/*",
			"/auth/*",
			"/admin/*",
		},
	)

	// ==== 启动主服务器 ====
	errChan := make(chan error, 1)
	go func() {
		slog.Info("SSO服务启动成功", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("服务器启动失败", "error", err)
			errChan <- err
		}
	}()

	// ==== 启动初始化面板服务器（loopback 隔离） ====
	// initServer 可能为 nil（INIT_ENABLED=false 时）；Start() 在该情况下返回 nil，
	// 对 nil 通道的 select 永远阻塞，等效于"永不发送错误"，不会误触发
	initErrChan := initServer.Start()

	// ==== 信号通道（提前创建以便两个 case 共用） ====
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// ==== 等待中断信号或启动错误 ====
	select {
	case err := <-errChan:
		slog.Error("主服务器启动错误，正在关闭", "error", err)
		for _, rl := range rateLimiters {
			rl.Stop()
		}
		// 同步关闭初始化面板服务器，避免资源泄漏
		if initServer != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := initServer.Shutdown(shutdownCtx); err != nil {
				slog.Error("初始化面板服务器关闭失败", "error", err)
			}
		}
		return
	case err := <-initErrChan:
		// initErrChan 为 nil 时此 case 永远阻塞；仅在 init 服务器实际启动失败时触发
		if err != nil {
			slog.Error("初始化面板服务器启动错误，正在关闭主服务器", "error", err)
			for _, rl := range rateLimiters {
				rl.Stop()
			}
			shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				slog.Error("主服务器关闭失败", "error", err)
			}
			return
		}
		// 通道已关闭且无错误：理论不应到达（ListenAndServe 阻塞直到关闭）
		// 防御性处理：继续等待信号
		sig := <-sigChan
		slog.Info("收到关闭信号，正在优雅关闭服务器...", "signal", sig)
		gracefulShutdown(rateLimiters, server, initServer, svc.Audit, svc.Social, svc.MFA, cfg.ShutdownTimeout)
	case sig := <-sigChan:
		slog.Info("收到关闭信号，正在优雅关闭服务器...", "signal", sig)
		gracefulShutdown(rateLimiters, server, initServer, svc.Audit, svc.Social, svc.MFA, cfg.ShutdownTimeout)
	}
}

// gracefulShutdown 优雅关闭服务器
// 顺序: 停止限流器 → 关闭HTTP服务器（等待在途请求完成）→ 关闭初始化面板服务器 →
//
//	关闭MFA → 关闭社交登录 → 关闭审计服务
//
// 注意：必须先 server.Shutdown 停止接收新请求并等待在途请求完成，
// 再关闭审计等服务。否则在途请求若调用 auditSvc.Log() 会触发
// panic: send on closed channel。
func gracefulShutdown(
	rateLimiters []middleware.RateLimitMiddleware,
	server *http.Server,
	initServer *InitServer,
	auditSvc *service.AuditService,
	socialSvc *service.SocialLoginService,
	mfaSvc *service.MFAService,
	timeout time.Duration,
) {
	// 停止所有限流器（停止接收新的限流检查）
	for _, rl := range rateLimiters {
		rl.Stop()
	}

	// 先关闭HTTP服务器：停止接收新请求，等待在途请求完成
	// 此期间审计服务仍可用，确保关闭过程中的请求审计日志正常写入
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("服务器关闭失败", "error", err)
		// 即使 Shutdown 失败也继续关闭后台服务，避免资源泄漏
	}

	// 关闭初始化面板服务器（与主服务器同步关闭，避免遗留 loopback 监听）
	if initServer != nil {
		if err := initServer.Shutdown(ctx); err != nil {
			slog.Error("初始化面板服务器关闭失败", "error", err)
		} else {
			slog.Info("初始化面板服务器已关闭")
		}
	}

	// 关闭MFA服务（停止TOTP清理goroutine）
	if mfaSvc != nil {
		mfaSvc.Close()
		slog.Info("MFA服务已关闭")
	}

	// 关闭社交登录服务（停止清理goroutine）
	if socialSvc != nil {
		socialSvc.Close()
		slog.Info("社交登录服务已关闭")
	}

	// 最后关闭审计服务：此时所有请求已处理完毕，可安全关闭日志通道
	if auditSvc != nil {
		auditSvc.Close()
		slog.Info("审计服务已关闭")
	}

	slog.Info("服务器已成功关闭")
}
