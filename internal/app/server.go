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
func startServer(
	cfg *config.Config,
	handler http.Handler,
	rateLimiters []middleware.RateLimitMiddleware,
	svc *Services,
	version string,
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

	// ==== 启动服务器 ====
	errChan := make(chan error, 1)
	go func() {
		slog.Info("SSO服务启动成功", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("服务器启动失败", "error", err)
			errChan <- err
		}
	}()

	// ==== 等待中断信号或启动错误 ====
	select {
	case err := <-errChan:
		slog.Error("服务器启动错误，正在关闭", "error", err)
		for _, rl := range rateLimiters {
			rl.Stop()
		}
		return
	case sig := <-func() chan os.Signal {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		return quit
	}():
		slog.Info("收到关闭信号，正在优雅关闭服务器...", "signal", sig)
		gracefulShutdown(rateLimiters, server, svc.Audit, svc.Social, svc.MFA, cfg.ShutdownTimeout)
	}
}

// gracefulShutdown 优雅关闭服务器
// 顺序: 停止限流器 → 关闭HTTP服务器（等待在途请求完成）→ 关闭MFA → 关闭社交登录 → 关闭审计服务
//
// 注意：必须先 server.Shutdown 停止接收新请求并等待在途请求完成，
// 再关闭审计等服务。否则在途请求若调用 auditSvc.Log() 会触发
// panic: send on closed channel。
func gracefulShutdown(
	rateLimiters []middleware.RateLimitMiddleware,
	server *http.Server,
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
