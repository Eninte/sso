// Package main SSO服务入口
// 这是单点登录服务的主程序入口，负责初始化配置、
// 连接数据库、注册路由并启动HTTP服务器
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"

	"github.com/your-org/sso/internal/cache"
	"github.com/your-org/sso/internal/config"
	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/handler"
	"github.com/your-org/sso/internal/logging"
	"github.com/your-org/sso/internal/metrics"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/postgres"
)

// Version 服务版本号，通过 -ldflags 注入
var Version = "dev"

// BuildTime 构建时间，通过 -ldflags 注入
var BuildTime = "unknown"

// Services 包含所有服务实例
type Services struct {
	Store    *postgres.Store
	Cache    cache.Cache
	Password *crypto.PasswordService
	JWT      *crypto.JWTService
	Email    *service.EmailService
	Metrics  *metrics.Service
	Token    *service.TokenService
	User     *service.UserService
	Auth     *service.AuthService
	OAuth    *service.OAuthService
	Audit    *service.AuditService
	MFA      *service.MFAService
	Admin    *service.AdminService
	Social   *service.SocialLoginService
}

// main 服务器主入口
// 此函数已重构以降低复杂度，通过提取配置初始化、服务初始化、处理器初始化、服务器启动逻辑
//
// 职责:
//   - 调用initConfig加载配置
//   - 调用initLogger初始化日志
//   - 调用initServices初始化服务
//   - 调用initHandlers初始化处理器和路由
//   - 调用startServer启动HTTP服务器
//
// 重构原因: 原始复杂度为17，通过提取配置、服务、处理器、服务器启动逻辑，降低到<10
// 提取的函数:
//   - initConfig: 加载配置
//   - initLogger: 初始化日志
//   - initServices: 初始化服务
//   - initHandlers: 初始化处理器和路由
//   - startServer: 启动HTTP服务器
func main() {
	// 检查是否为 version 子命令
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("SSO %s (构建时间: %s)\n", Version, BuildTime)
		os.Exit(0)
	}

	// 1. 加载配置
	// 注意：配置向导仅在 initConfig() 失败时启动，
	// 配置加载成功时不会进入 startSetupWizard，因此不存在配置正常时暴露向导的风险
	cfg, err := initConfig()
	if err != nil {
		slog.Warn("配置加载失败，启动配置向导", "error", err)
		startSetupWizard(err)
		return
	}

	// 2. 初始化日志
	initLogger(cfg.Env)

	slog.Info("SSO服务初始化中...",
		"env", cfg.Env,
		"port", cfg.ServerPort,
	)

	// 3. 初始化服务
	svc, db, err := initServices(cfg)
	if err != nil {
		slog.Error("服务初始化失败", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	defer svc.Cache.Close()

	// 9. 初始化处理器和路由
	router, rateLimiter := initHandlers(cfg, svc)

	// 10. 启动服务器
	startServer(cfg, router, rateLimiter, svc)
}

// startServer 启动HTTP服务器
// 此函数从main中提取，用于降低主函数的复杂度
//
// 职责:
//   - 创建HTTP服务器实例
//   - 配置服务器参数（地址、处理器、超时等）
//   - 启动服务器
//   - 处理优雅关闭（SIGINT、SIGTERM信号）
//   - 关闭审计服务、社交登录服务等资源
//
// 参数:
//   - cfg: 配置对象
//   - handler: HTTP处理器（路由）
//   - rateLimiter: 限流中间件
//   - svc: 服务集合
//
// 重构原因: 从main中提取服务器启动逻辑，降低主函数复杂度（17→<10）
func startServer(cfg *config.Config, handler http.Handler, rateLimiter *middleware.RateLimiter, svc *Services) {
	// ==== 创建HTTP服务器 ====
	server := &http.Server{
		Addr:         cfg.ServerHost + ":" + cfg.ServerPort,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ==== 记录服务启动 ====
	svc.Audit.LogSystemStart(context.Background(), Version)

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
		rateLimiter.Stop()
		return
	case sig := <-func() chan os.Signal {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		return quit
	}():
		slog.Info("收到关闭信号，正在优雅关闭服务器...", "signal", sig)
		gracefulShutdown(rateLimiter, server, svc.Audit, svc.Social, cfg.ShutdownTimeout)
	}
}

// initHandlers 初始化HTTP处理器和路由
// 此函数从main中提取，用于降低主函数的复杂度
//
// 职责:
//   - 创建路由器
//   - 注册所有HTTP处理器
//   - 配置中间件（日志、限流、CORS等）
//   - 返回配置好的路由器和限流中间件
//
// 参数:
//   - cfg: 配置对象
//   - svc: 服务集合
//
// 返回:
//   - 配置好的路由器
//   - 限流中间件实例
//
// 重构原因: 从main中提取处理器初始化逻辑，降低主函数复杂度（17→<10）
func initHandlers(cfg *config.Config, svc *Services) (*mux.Router, *middleware.RateLimiter) {
	// ==== 初始化处理器 ====
	registerHandler := handler.NewRegisterHandler(svc.Auth)
	loginHandler := handler.NewLoginHandler(svc.Auth)
	tokenHandler := handler.NewTokenHandler(svc.Auth, svc.OAuth)
	userInfoHandler := handler.NewUserInfoHandler(svc.Store, svc.Cache)
	authorizeHandler := handler.NewAuthorizeHandler(svc.OAuth)
	userHandler := handler.NewUserHandler(svc.User)
	mfaHandler := handler.NewMFAHandler(svc.MFA)
	socialHandler := handler.NewSocialLoginHandler(svc.Social)
	// 使用支持多密钥JWKS的handler
	wellKnownHandler := handler.NewWellKnownHandlerWithJWTService(cfg.BaseURL(), svc.JWT)
	metricsHandler := handler.NewMetricsHandler(svc.Metrics)
	adminHandler := handler.NewAdminHandler(svc.Admin)

	// ==== 创建路由器 ====
	router := mux.NewRouter()

	// ==== 应用中间件 ====
	// 设置受信代理（仅信任这些代理发来的 X-Real-IP 头）
	if cfg.TrustedProxies != "" {
		proxies := strings.Split(cfg.TrustedProxies, ",")
		for i := range proxies {
			proxies[i] = strings.TrimSpace(proxies[i])
		}
		middleware.SetTrustedProxies(proxies)
	}

	// 创建限流器并设置指标回调
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow).
		WithMetrics(func() {
			svc.Metrics.Increment("security_rate_limit_total")
		})

	router.Use(middleware.SecurityHeaders)
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)
	// 添加metrics中间件，收集HTTP请求指标
	router.Use(svc.Metrics.HTTPMiddleware)
	// 限流中间件
	router.Use(rateLimiter.Middleware)
	// 使用配置中的CORS设置
	corsConfig := &middleware.CORSConfig{
		AllowedOrigins: cfg.GetCORSAllowedOrigins(),
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Requested-With"},
		MaxAge:         86400, // 24小时
	}
	
	// 验证CORS配置（生产环境禁止通配符）
	if err := corsConfig.Validate(cfg.Env); err != nil {
		slog.Error("CORS配置验证失败", "error", err)
		os.Exit(1)
	}
	
	router.Use(middleware.CORS(corsConfig))

	// 添加语言中间件，支持多语言错误消息
	router.Use(middleware.Language)

	// ==== 注册路由 ====
	// 健康检查端点 (不需要认证)
	router.HandleFunc("/health", healthHandler).Methods("GET")

	// 初始化面板端点 (无管理员时可用，有管理员时返回404)
	initHandler := handler.NewInitHandler(svc.Store, svc.Password, svc.Cache, svc.Audit, Version, BuildTime)
	router.HandleFunc("/init", initHandler.HandleInitPage).Methods("GET")
	router.HandleFunc("/api/v1/init/status", initHandler.HandleSystemStatus).Methods("GET")
	router.HandleFunc("/api/v1/init/admin", initHandler.HandleCreateAdmin).Methods("POST")
	router.HandleFunc("/api/v1/init/client", initHandler.HandleCreateClient).Methods("POST")

	// Prometheus指标端点 (使用Basic Auth保护)
	router.Handle("/metrics", middleware.BasicAuth(cfg.MetricsUsername, cfg.MetricsPassword)(http.HandlerFunc(metricsHandler.HandleMetrics))).Methods("GET")

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
	protected.Use(middleware.AuthMiddlewareWithMetrics(svc.JWT, svc.Store, svc.Cache, func() {
		svc.Metrics.Increment("security_invalid_token_total")
	}))
	protected.HandleFunc("/userinfo", userInfoHandler.Handle).Methods("GET")
	protected.HandleFunc("/authorize", authorizeHandler.HandleAuthorize).Methods("GET")
	protected.HandleFunc("/authorize/approve", authorizeHandler.HandleApprove).Methods("POST")
	protected.HandleFunc("/verify-email/send", userHandler.HandleSendVerificationEmail).Methods("POST")
	protected.HandleFunc("/change-password", userHandler.HandleChangePassword).Methods("POST")
	protected.HandleFunc("/logout-all", tokenHandler.HandleLogoutAll).Methods("POST")
	protected.HandleFunc("/mfa/setup", mfaHandler.HandleSetupMFA).Methods("POST")
	protected.HandleFunc("/mfa/verify", mfaHandler.HandleVerifyMFA).Methods("POST")
	protected.HandleFunc("/mfa/disable", mfaHandler.HandleDisableMFA).Methods("POST")
	protected.HandleFunc("/mfa/status", mfaHandler.HandleMFAStatus).Methods("GET")

	// 管理员端点 (需要认证 + 管理员角色)
	admin := api.PathPrefix("/admin").Subrouter()
	admin.Use(middleware.AuthMiddlewareWithMetrics(svc.JWT, svc.Store, svc.Cache, func() {
		svc.Metrics.Increment("security_invalid_token_total")
	}))
	admin.Use(middleware.RequireAdmin())
	admin.HandleFunc("/health", adminHandler.HandleSystemHealth).Methods("GET")
	admin.HandleFunc("/cleanup", adminHandler.HandleCleanup).Methods("POST")
	admin.HandleFunc("/users", adminHandler.HandleListUsers).Methods("GET")
	admin.HandleFunc("/users/{id}", adminHandler.HandleGetUser).Methods("GET")
	admin.HandleFunc("/users/{id}/disable", adminHandler.HandleDisableUser).Methods("POST")
	admin.HandleFunc("/users/{id}/enable", adminHandler.HandleEnableUser).Methods("POST")
	admin.HandleFunc("/users/{id}", adminHandler.HandleDeleteUser).Methods("DELETE")
	admin.HandleFunc("/audit-logs", adminHandler.HandleAuditLogs).Methods("GET")

	return router, rateLimiter
}

// initConfig 加载和验证配置
// 此函数从main中提取，用于降低主函数的复杂度
//
// 职责:
//   - 从环境变量加载配置
//   - 验证配置的有效性
//   - 返回验证后的配置或错误
//
// 返回:
//   - 如果成功，返回验证后的配置
//   - 如果失败，返回错误
//
// 重构原因: 从main中提取配置初始化逻辑，降低主函数复杂度（17→<10）
func initConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// initLogger 初始化日志系统
// 此函数从main中提取，用于降低主函数的复杂度
//
// 职责:
//   - 根据环境设置日志级别
//   - 配置日志格式
//   - 初始化全局日志记录器
//
// 参数:
//   - env: 运行环境（development或production）
//
// 重构原因: 从main中提取日志初始化逻辑，降低主函数复杂度（17→<10）
func initLogger(env string) {
	logging.InitForEnv(env)
}

// initCache 初始化缓存服务
// 此函数从main中提取，用于降低主函数的复杂度
//
// 职责:
//   - 根据配置创建缓存实例
//   - 验证缓存连接
//   - 返回缓存服务或错误
//
// 参数:
//   - cfg: 配置对象
//
// 返回:
//   - 如果成功，返回缓存服务实例
//   - 如果失败，返回错误
//
// 重构原因: 从main中提取缓存初始化逻辑，降低主函数复杂度（17→<10）
func initCache(cfg *config.Config, metricsSvc *metrics.Service) (cache.Cache, error) {
	opt := &cache.Option{
		RedisEnable:   cfg.RedisEnable,
		RedisHost:     cfg.RedisHost,
		RedisPort:     cfg.RedisPort,
		RedisPassword: cfg.RedisPassword,
		RedisDB:       cfg.RedisDB,
		RedisPoolSize: cfg.RedisPoolSize,
	}
	cacheSvc, err := cache.NewCacheWithFallback(opt)
	if err != nil {
		return nil, err
	}

	// 设置缓存指标回调
	setMetrics := func(hitsFunc, missesFunc func()) {
		if mc, ok := cacheSvc.(*cache.MemoryCache); ok {
			mc.WithMetrics(hitsFunc, missesFunc)
		} else if rc, ok := cacheSvc.(*cache.RedisCache); ok {
			rc.WithMetrics(hitsFunc, missesFunc)
		}
	}
	setMetrics(
		func() { metricsSvc.Increment("cache_hits_total") },
		func() { metricsSvc.Increment("cache_misses_total") },
	)

	return cacheSvc, nil
}

// initServices 初始化所有服务
// 包含数据库连接、Redis连接、服务实例化
// 返回Services结构体、数据库连接和错误
// initServices 初始化所有业务服务
// 此函数从main中提取，用于降低主函数的复杂度
//
// 职责:
//   - 连接数据库
//   - 初始化缓存
//   - 创建所有服务实例（认证、用户、审计等）
//   - 返回服务集合或错误
//
// 参数:
//   - cfg: 配置对象
//
// 返回:
//   - 如果成功，返回服务集合、数据库连接、nil
//   - 如果失败，返回nil、nil、错误
//
// 重构原因: 从main中提取服务初始化逻辑，降低主函数复杂度（17→<10）
func initServices(cfg *config.Config) (*Services, *sql.DB, error) {
	// ==== 连接数据库 ====
	db, err := connectDatabase(cfg)
	if err != nil {
		return nil, nil, err
	}
	slog.Info("数据库连接成功")

	// ==== 初始化存储层 ====
	store := postgres.New(db)

	// ==== 设置MFA恢复码HMAC密钥 ====
	if cfg.MFARecoveryHMACKey == "" {
		if cfg.Env == "production" {
			slog.Error("MFA_RECOVERY_HMAC_KEY未设置，生产环境必须配置")
			return nil, nil, fmt.Errorf("MFA_RECOVERY_HMAC_KEY is required in production")
		}
		slog.Warn("MFA_RECOVERY_HMAC_KEY未设置（开发环境允许，生产环境会拒绝启动）")
	}
	postgres.SetMFARecoveryHMACKey(cfg.MFARecoveryHMACKey)

	// ==== 初始化指标服务 ====
	metricsSvc := metrics.NewService()

	// ==== 初始化缓存层 ====
	cacheSvc, err := initCache(cfg, metricsSvc)
	if err != nil {
		slog.Warn("缓存初始化失败，使用内存缓存", "error", err)
		cacheSvc = cache.NewMemoryCache().WithMetrics(
			func() { metricsSvc.Increment("cache_hits_total") },
			func() { metricsSvc.Increment("cache_misses_total") },
		)
	}
	slog.Info("缓存层初始化完成")

	// ==== 初始化加密服务 ====
	passwordSvc := crypto.NewPasswordService(crypto.NormalizeBcryptCost(cfg.BcryptCost))
	var jwtSvc *crypto.JWTService

	// 根据是否启用密钥轮换选择不同的初始化方式
	if cfg.KeyRotationEnabled && cfg.JWTTransitionPubKeyPaths != "" {
		// 使用密钥轮换模式
		transitionPubKeyPaths := cfg.GetJWTTransitionPubKeyPaths()
		jwtSvc, err = crypto.LoadKeysForRotation(
			cfg.JWTPrivateKeyPath,
			cfg.JWTPublicKeyPath,
			transitionPubKeyPaths,
			cfg.JWTIssuer,
			cfg.AccessTokenTTL,
			cfg.RefreshTokenTTL,
		)
		if err != nil {
			_ = db.Close() // #nosec G104 -- 关闭数据库连接时的错误可以忽略，主要错误已返回
			return nil, nil, err
		}
		slog.Info("密钥轮换模式已启用",
			"transition_keys_count", len(transitionPubKeyPaths),
		)
	} else {
		// 标准模式（单密钥）
		privateKey, err := crypto.LoadPrivateKeyFromFile(cfg.JWTPrivateKeyPath)
		if err != nil {
			_ = db.Close() // #nosec G104 -- 关闭数据库连接时的错误可以忽略，主要错误已返回
			return nil, nil, err
		}
		publicKey, err := crypto.LoadPublicKeyFromFile(cfg.JWTPublicKeyPath)
		if err != nil {
			_ = db.Close() // #nosec G104 -- 关闭数据库连接时的错误可以忽略，主要错误已返回
			return nil, nil, err
		}
		jwtSvc = crypto.NewJWTService(
			privateKey,
			publicKey,
			cfg.JWTIssuer,
			cfg.AccessTokenTTL,
			cfg.RefreshTokenTTL,
		)
	}

	slog.Info("加密服务初始化完成")

	// ==== 接入JTI防重放跟踪器 ====
	jtiTracker := crypto.NewCacheJTITracker(cacheSvc, "jti:")
	jwtSvc.SetJTITracker(jtiTracker)
	slog.Info("JTI防重放跟踪器已启用")

	// ==== 初始化邮件服务 ====
	emailConfig := &service.EmailConfig{
		SMTPHost: cfg.SMTPHost,
		SMTPPort: cfg.SMTPPort,
		Username: cfg.SMTPUser,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	}
	emailSvc, err := service.NewEmailService(emailConfig)
	if err != nil {
		slog.Error("初始化邮件服务失败", "error", err)
		return nil, nil, fmt.Errorf("初始化邮件服务失败: %w", err)
	}

	// ==== 初始化Token生成服务 ====
	tokenSvc := service.NewTokenService(jwtSvc, store)

	// ==== 初始化业务服务（带缓存） ====
	userSvc := service.NewUserService(store, passwordSvc, emailSvc, cfg.BaseURL()).
		WithEmailRateLimit(service.NewEmailRateLimiter(cacheSvc))
	authSvc := service.NewAuthServiceWithOptions(
		store,
		passwordSvc,
		jwtSvc,
		cfg.MaxLoginAttempts,
		cfg.LockoutDuration,
		service.WithCache(cacheSvc),
		service.WithMetrics(metricsSvc),
		service.WithUserService(userSvc),
	)
	oauthSvc := service.NewOAuthServiceWithCache(store, cacheSvc, tokenSvc)
	auditSvc := service.NewAuditService(store)
	mfaSvc := service.NewMFAService(store)
	// 设置MFA恢复码HMAC密钥（与数据库层使用相同密钥）
	if cfg.MFARecoveryHMACKey != "" {
		mfaSvc.SetHMACKey([]byte(cfg.MFARecoveryHMACKey))
	}
	adminSvc := service.NewAdminServiceWithVersion(store, cacheSvc, Version, BuildTime)

	// ==== 初始化第三方登录服务 ====
	socialSvc := service.NewSocialLoginService(store, jwtSvc, cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GitHubClientID, cfg.GitHubClientSecret)
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		slog.Info("Google第三方登录已启用")
	}
	if cfg.GitHubClientID != "" && cfg.GitHubClientSecret != "" {
		slog.Info("GitHub第三方登录已启用")
	}

	// ==== 返回所有服务 ====
	return &Services{
		Store:    store,
		Cache:    cacheSvc,
		Password: passwordSvc,
		JWT:      jwtSvc,
		Email:    emailSvc,
		Metrics:  metricsSvc,
		Token:    tokenSvc,
		User:     userSvc,
		Auth:     authSvc,
		OAuth:    oauthSvc,
		Audit:    auditSvc,
		MFA:      mfaSvc,
		Admin:    adminSvc,
		Social:   socialSvc,
	}, db, nil
}

// connectDatabase 连接数据库
// 此函数从initServices中提取，用于降低函数的复杂度
//
// 职责:
//   - 根据配置创建数据库连接
//   - 验证连接有效性
//   - 执行数据库迁移
//   - 返回数据库连接或错误
//
// 参数:
//   - cfg: 配置对象
//
// 返回:
//   - 如果成功，返回数据库连接
//   - 如果失败，返回错误
//
// 重构原因: 从initServices中提取数据库连接逻辑，提高代码可读性
func connectDatabase(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DatabaseURL())
	if err != nil {
		return nil, err
	}

	// 使用配置的连接池参数
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	if cfg.DBConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.DBConnMaxIdleTime)
	}

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

// startSetupWizard 启动配置向导HTTP服务
// 当config.Load()失败时调用，启动轻量HTTP服务显示配置向导
// 阻塞直到收到配置写入后通过syscall.Exec重启进程
func startSetupWizard(loadErr error) {
	envPath := config.GetEnvPath()
	setupHandler := handler.NewSetupHandler(envPath, Version)

	router := mux.NewRouter()
	router.Use(middleware.SecurityHeaders)
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)

	// 配置向导限流：10请求/分钟，防止暴力攻击
	setupRateLimiter := middleware.NewRateLimiter(10, time.Minute)
	defer setupRateLimiter.Stop()
	router.Use(setupRateLimiter.Middleware)

	router.HandleFunc("/setup", setupHandler.HandleSetupPage).Methods("GET")
	router.HandleFunc("/api/v1/setup/save", setupHandler.HandleSetupSave).Methods("POST")
	router.HandleFunc("/api/v1/setup/test-db", setupHandler.HandleSetupTestDB).Methods("POST")
	router.HandleFunc("/api/v1/setup/test-redis", setupHandler.HandleSetupTestRedis).Methods("POST")
	router.HandleFunc("/api/v1/setup/generate-keys", setupHandler.HandleSetupGenerateKeys).Methods("POST")

	// 根路径重定向到/setup
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/setup", http.StatusFound)
	})

	addr := os.Getenv("SERVER_HOST") + ":" + os.Getenv("SERVER_PORT")
	if addr == ":" {
		addr = "127.0.0.1:9090"
	}
	slog.Info("配置向导启动", "address", addr, "config_error", loadErr.Error())

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		slog.Error("配置向导服务启动失败", "error", err)
		os.Exit(1)
	}
}

// gracefulShutdown 优雅关闭服务器
func gracefulShutdown(rateLimiter *middleware.RateLimiter, server *http.Server, auditSvc *service.AuditService, socialSvc *service.SocialLoginService, timeout time.Duration) {
	// 停止限流器
	rateLimiter.Stop()

	// 关闭审计服务（确保所有日志写入完成）
	if auditSvc != nil {
		auditSvc.Close()
		slog.Info("审计服务已关闭")
	}

	// 关闭社交登录服务（停止清理goroutine）
	if socialSvc != nil {
		socialSvc.Close()
		slog.Info("社交登录服务已关闭")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("服务器关闭失败", "error", err)
		return
	}

	slog.Info("服务器已成功关闭")
}
