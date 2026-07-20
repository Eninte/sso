// Package app 初始化面板的独立 loopback HTTP 服务器
//
// 安全设计：
// 初始化路由 (/init, /api/v1/init/*) 仅注册在此服务器的路由器上，
// 该服务器强制监听 127.0.0.1（loopback），与公网主路由完全隔离。
//
// 解决的问题：
// 原实现中初始化路由注册在公网主路由上，仅靠 isLocalRequest(r.RemoteAddr)
// 判断访问来源；反向代理（Nginx/Caddy）在本机转发时，所有公网请求的
// RemoteAddr 都会变成 127.0.0.1，导致保护失效，攻击者可远程创建
// 首个管理员账户并获取 OAuth Client Secret。
//
// 防御层次：
//  1. 网络层：服务器只绑定 loopback，公网流量物理不可达
//  2. 应用层：handler 内 isLocalRequest 二次校验（防御 loopback 转发）
//  3. 配置层：INIT_ENABLED=false 可永久关闭初始化面板
//  4. 数据库层：CreateAdmin 使用 advisory_xact_lock 防止并发竞态
package app

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/config"
	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/auditutil"
)

// InitServer 初始化面板的独立 loopback HTTP 服务器
type InitServer struct {
	server      *http.Server
	rateLimiter middleware.RateLimitMiddleware
}

// NewInitServer 创建初始化面板服务器
// 仅当 cfg.InitEnabled == true 时返回非 nil 服务器，否则返回 nil
//
// 参数：
//   - cfg: 配置（必须已通过 validate 校验）
//   - storeSvc: store 实例
//   - passwordSvc: 密码服务
//   - cacheSvc: 缓存服务
//   - auditSvc: 审计服务
//   - version, buildTime: 版本信息
func NewInitServer(
	cfg *config.Config,
	storeSvc store.Store,
	passwordSvc *crypto.PasswordService,
	cacheSvc cache.Cache,
	auditSvc auditutil.AuditService,
	version, buildTime string,
) *InitServer {
	if !cfg.InitEnabled {
		slog.Info("初始化面板已禁用（INIT_ENABLED=false）")
		return nil
	}

	// 再次校验监听地址必须为 loopback（防御配置加载后篡改）
	if !isLoopbackAddr(cfg.InitListenAddr) {
		slog.Error("初始化面板监听地址非 loopback，拒绝启动", "addr", cfg.InitListenAddr)
		return nil
	}

	initHandler := handler.NewInitHandler(
		storeSvc,
		passwordSvc,
		cacheSvc,
		auditSvc,
		version,
		buildTime,
		cfg.InitEnabled,
	)

	router := mux.NewRouter()
	// 安全中间件链：与主路由保持一致的安全头、请求 ID、recover、日志
	router.Use(middleware.SecurityHeaders)
	router.Use(middleware.RequestID)
	router.Use(middleware.Recover)
	router.Use(middleware.Logger)
	// 初始化面板限流：10 请求/分钟，防止暴力攻击
	initRateLimiter := middleware.NewRateLimiter(10, time.Minute)
	router.Use(initRateLimiter.Middleware)

	router.HandleFunc("/init", initHandler.HandleInitPage).Methods("GET")
	router.HandleFunc("/api/v1/init/status", initHandler.HandleSystemStatus).Methods("GET")
	router.HandleFunc("/api/v1/init/admin", initHandler.HandleCreateAdmin).Methods("POST")
	router.HandleFunc("/api/v1/init/client", initHandler.HandleCreateClient).Methods("POST")
	// 根路径重定向到 /init
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/init", http.StatusFound)
	})

	srv := &http.Server{
		Addr:         cfg.InitListenAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("初始化面板服务器已配置", "addr", cfg.InitListenAddr,
		"warning", "初始化完成后应设置 INIT_ENABLED=false 永久关闭")

	return &InitServer{
		server:      srv,
		rateLimiter: initRateLimiter,
	}
}

// Start 启动初始化面板服务器（非阻塞，在 goroutine 中运行）
// 启动失败通过返回的 error 通道通知调用方
// 返回 nil 表示服务器未启用（INIT_ENABLED=false 或监听地址非 loopback）；
// 调用方对 nil 通道的 select 会阻塞，等效于"永不发送错误"
func (s *InitServer) Start() <-chan error {
	if s == nil || s.server == nil {
		return nil
	}

	errChan := make(chan error, 1)
	go func() {
		slog.Info("初始化面板服务启动", "address", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("初始化面板服务启动失败", "error", err, "address", s.server.Addr)
			errChan <- err
			return
		}
		close(errChan)
	}()

	return errChan
}

// Shutdown 优雅关闭初始化面板服务器
// 顺序：先停止限流器（停止接收新限流检查），再关闭 HTTP 服务器（等待在途请求）
func (s *InitServer) Shutdown(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	if s.rateLimiter != nil {
		s.rateLimiter.Stop()
	}
	slog.Info("初始化面板服务正在关闭...")
	return s.server.Shutdown(ctx)
}

// isLoopbackAddr 校验地址是否绑定到 loopback
// 接受 127.0.0.1:port、[::1]:port、localhost:port 三种格式
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	switch host {
	case "127.0.0.1", "::1", "localhost":
		return true
	default:
		return false
	}
}
