// Package main SSO服务入口
// 这是单点登录服务的主程序入口，负责初始化配置、
// 连接数据库、注册路由并启动HTTP服务器
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"

	"github.com/your-org/sso/internal/config"
	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/handler"
	"github.com/your-org/sso/internal/logging"
	"github.com/your-org/sso/internal/metrics"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/postgres"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load()
	if err != nil {
		slog.Error("配置加载失败", "error", err)
		os.Exit(1)
	}

	// 2. 初始化日志
	initLogger(cfg.Env)

	slog.Info("SSO服务初始化中...",
		"env", cfg.Env,
		"port", cfg.ServerPort,
	)

	// 3. 连接数据库
	db, err := connectDatabase(cfg)
	if err != nil {
		slog.Error("数据库连接失败", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("数据库连接成功")

	// 4. 初始化存储层
	store := postgres.New(db)

	// 5. 初始化加密服务
	passwordSvc := crypto.NewPasswordService(cfg.BcryptCost)
	privateKey, err := crypto.LoadPrivateKeyFromFile(cfg.JWTPrivateKeyPath)
	if err != nil {
		slog.Error("加载私钥失败", "error", err)
		os.Exit(1)
	}
	publicKey, err := crypto.LoadPublicKeyFromFile(cfg.JWTPublicKeyPath)
	if err != nil {
		slog.Error("加载公钥失败", "error", err)
		os.Exit(1)
	}
	jwtSvc := crypto.NewJWTService(
		privateKey,
		publicKey,
		cfg.JWTIssuer,
		cfg.AccessTokenTTL,
		cfg.RefreshTokenTTL,
	)
	slog.Info("加密服务初始化完成")

	// 6. 初始化邮件服务
	emailConfig := &service.EmailConfig{
		SMTPHost: cfg.SMTPHost,
		SMTPPort: cfg.SMTPPort,
		Username: cfg.SMTPUser,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	}
	emailSvc := service.NewEmailService(emailConfig)

	// 7. 初始化指标服务
	metricsSvc := metrics.NewService()

	// 8. 初始化业务服务
	authSvc := service.NewAuthService(
		store,
		passwordSvc,
		jwtSvc,
		cfg.MaxLoginAttempts,
		cfg.LockoutDuration,
		metricsSvc,
	)
	oauthSvc := service.NewOAuthService(store)
	userSvc := service.NewUserService(store, passwordSvc, emailSvc, cfg.BaseURL())
	auditSvc := service.NewAuditService(store)
	mfaSvc := service.NewMFAService(store)
	adminSvc := service.NewAdminService(store)

	// 8. 初始化第三方登录服务
	socialSvc := service.NewSocialLoginService(store, jwtSvc, cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GitHubClientID, cfg.GitHubClientSecret)
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		slog.Info("Google第三方登录已启用")
	}
	if cfg.GitHubClientID != "" && cfg.GitHubClientSecret != "" {
		slog.Info("GitHub第三方登录已启用")
	}

	// 9. 初始化处理器
	registerHandler := handler.NewRegisterHandler(authSvc)
	loginHandler := handler.NewLoginHandler(authSvc)
	tokenHandler := handler.NewTokenHandler(authSvc, oauthSvc)
	userInfoHandler := handler.NewUserInfoHandler(authSvc)
	authorizeHandler := handler.NewAuthorizeHandler(oauthSvc)
	userHandler := handler.NewUserHandler(userSvc)
	mfaHandler := handler.NewMFAHandler(mfaSvc)
	socialHandler := handler.NewSocialLoginHandler(socialSvc)
	wellKnownHandler := handler.NewWellKnownHandler(cfg.BaseURL(), publicKey)
	metricsHandler := handler.NewMetricsHandler(metricsSvc)
	adminHandler := handler.NewAdminHandler(adminSvc)

	// 11. 创建路由器
	router := mux.NewRouter()

	// 12. 应用中间件
	router.Use(middleware.SecurityHeaders)
	router.Use(middleware.Logger)
	// 添加metrics中间件，收集HTTP请求指标
	router.Use(metricsSvc.HTTPMiddleware)
	// 使用配置中的CORS设置
	corsConfig := &middleware.CORSConfig{
		AllowedOrigins: cfg.GetCORSAllowedOrigins(),
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Requested-With"},
		MaxAge:         86400, // 24小时
	}
	router.Use(middleware.CORS(corsConfig))

	// 添加语言中间件，支持多语言错误消息
	router.Use(middleware.Language)

	// 13. 注册路由
	// 健康检查端点 (不需要认证)
	router.HandleFunc("/health", healthHandler).Methods("GET")

	// Prometheus指标端点
	router.HandleFunc("/metrics", metricsHandler.HandleMetrics).Methods("GET")

	// OIDC Discovery端点 (公开)
	router.HandleFunc("/.well-known/openid-configuration", wellKnownHandler.HandleDiscovery).Methods("GET")
	router.HandleFunc("/.well-known/jwks.json", wellKnownHandler.HandleJWKS).Methods("GET")

	// 第三方登录端点 (公开)
	router.HandleFunc("/auth/providers", socialHandler.HandleProviders).Methods("GET")
	router.HandleFunc("/auth/{provider}", socialHandler.HandleLogin).Methods("GET")
	router.HandleFunc("/auth/{provider}/callback", socialHandler.HandleCallback).Methods("GET")

	// API端点
	api := router.PathPrefix("/api/v1").Subrouter()

	// 公开端点 (不需要认证)
	api.HandleFunc("/register", registerHandler.Handle).Methods("POST")
	api.HandleFunc("/login", loginHandler.Handle).Methods("POST")
	api.HandleFunc("/token", tokenHandler.HandleToken).Methods("POST")
	api.HandleFunc("/token/revoke", tokenHandler.HandleRevoke).Methods("POST")
	api.HandleFunc("/forgot-password", userHandler.HandleForgotPassword).Methods("POST")
	api.HandleFunc("/reset-password", userHandler.HandleResetPassword).Methods("POST")
	api.HandleFunc("/verify-email", userHandler.HandleVerifyEmail).Methods("GET")

	// 受保护的端点 (需要认证)
	protected := api.PathPrefix("").Subrouter()
	protected.Use(middleware.AuthMiddleware(jwtSvc))
	protected.HandleFunc("/userinfo", userInfoHandler.Handle).Methods("GET")
	protected.HandleFunc("/authorize", authorizeHandler.HandleAuthorize).Methods("GET")
	protected.HandleFunc("/authorize/approve", authorizeHandler.HandleApprove).Methods("POST")
	protected.HandleFunc("/verify-email/send", userHandler.HandleSendVerificationEmail).Methods("POST")
	protected.HandleFunc("/change-password", userHandler.HandleChangePassword).Methods("POST")
	protected.HandleFunc("/mfa/setup", mfaHandler.HandleSetupMFA).Methods("POST")
	protected.HandleFunc("/mfa/verify", mfaHandler.HandleVerifyMFA).Methods("POST")
	protected.HandleFunc("/mfa/disable", mfaHandler.HandleDisableMFA).Methods("POST")
	protected.HandleFunc("/mfa/status", mfaHandler.HandleMFAStatus).Methods("GET")

	// 管理员端点 (需要认证 + 管理员权限)
	admin := router.PathPrefix("/admin").Subrouter()
	admin.Use(middleware.AuthMiddleware(jwtSvc))
	admin.Use(middleware.AdminMiddleware(cfg.GetAdminEmails(), cfg.GetAdminDomains()))
	admin.HandleFunc("/health", adminHandler.HandleSystemHealth).Methods("GET")
	admin.HandleFunc("/cleanup", adminHandler.HandleCleanup).Methods("POST")
	admin.HandleFunc("/users", adminHandler.HandleListUsers).Methods("GET")
	admin.HandleFunc("/users/{id}", adminHandler.HandleGetUser).Methods("GET")
	admin.HandleFunc("/users/disable", adminHandler.HandleDisableUser).Methods("POST")
	admin.HandleFunc("/users/enable", adminHandler.HandleEnableUser).Methods("POST")

	// 14. 创建HTTP服务器
	server := &http.Server{
		Addr:         cfg.ServerHost + ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 使用auditSvc记录服务启动
	_ = auditSvc

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

	// 15. 启动服务器
	go func() {
		slog.Info("SSO服务启动成功", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("服务器启动失败", "error", err)
			os.Exit(1)
		}
	}()

	// 16. 等待中断信号
	gracefulShutdown(server)
}

// initLogger 初始化日志配置
func initLogger(env string) {
	logging.InitForEnv(env)
}

// connectDatabase 连接数据库
func connectDatabase(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DatabaseURL())
	if err != nil {
		return nil, err
	}

	// 使用配置的连接池参数
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DBQueryTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return db, nil
}

// healthHandler 健康检查处理器
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"sso","timestamp":"` + time.Now().Format(time.RFC3339) + `"}`))
}

// gracefulShutdown 优雅关闭服务器
func gracefulShutdown(server *http.Server) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit
	slog.Info("收到关闭信号，正在优雅关闭服务器...", "signal", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("服务器关闭失败", "error", err)
		os.Exit(1)
	}

	slog.Info("服务器已成功关闭")
}
