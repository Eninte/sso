// Package app 路由与处理器装配
package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/config"
	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/metrics"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/store"
)

// initHandlers 初始化HTTP处理器和路由
// 职责: 创建路由器、注册所有HTTP处理器、配置中间件
// CORS 配置校验失败时返回 error，由调用方决定退出方式
func initHandlers(cfg *config.Config, svc *Services, version, buildTime string) (http.Handler, []middleware.RateLimitMiddleware, error) {
	// ==== 初始化处理器 ====
	registerHandler := handler.NewRegisterHandler(svc.Auth, svc.Captcha)
	loginHandler := handler.NewLoginHandler(svc.Auth, svc.Captcha)
	tokenHandler := handler.NewTokenHandler(svc.Auth, svc.OAuth)
	userInfoHandler := handler.NewUserInfoHandler(svc.Store, svc.Cache)
	authorizeHandler := handler.NewAuthorizeHandler(svc.OAuth)
	userHandler := handler.NewUserHandler(svc.User, svc.Captcha)
	mfaHandler := handler.NewMFAHandler(svc.MFA)
	socialHandler := handler.NewSocialLoginHandler(svc.Social).
		WithStateBinding(cfg.SocialStateCookieBinding, []byte(cfg.MFARecoveryHMACKey), cfg.Env == "production")
	// 使用支持多密钥JWKS的handler
	wellKnownHandler := handler.NewWellKnownHandlerWithJWTService(cfg.BaseURL(), svc.JWT)
	metricsHandler := handler.NewMetricsHandler(svc.Metrics)
	adminHandler := handler.NewAdminHandler(svc.Admin)
	captchaHandler := handler.NewCaptchaHandler(svc.Captcha)
	// API 文档调试前端（Scalar + OpenAPI），仅管理员可访问
	apiDocsHandler := handler.NewAPIDocsHandler(cfg.BaseURL(), version)

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

	// 创建限流器：Redis可用时使用分布式限流器，否则使用本地限流器
	// T10（方案 B）：分布式限流器统一启用内存降级——Redis 故障时当次请求
	// 降级为进程内内存限流（全局与敏感端点一致，降级期间限额仍生效）
	var rateLimiter middleware.RateLimitMiddleware
	if rc, ok := svc.Cache.(*cache.RedisCache); ok {
		rateLimiter = middleware.NewDistributedRateLimiter(
			rc.Client(), cfg.RateLimitRequests, cfg.RateLimitWindow, "ratelimit:global",
		).WithMemoryFallback()
	} else {
		rateLimiter = middleware.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)
	}
	// 阶段 4：注入指标回调
	// - 限流触发：security_rate_limit_total
	// - fail-open 放行（Redis 错误）：security_ratelimit_error_total
	injectRateLimitMetrics(rateLimiter, svc.Metrics)

	router.Use(middleware.SecurityHeaders)
	router.Use(middleware.RequestID)
	router.Use(middleware.Recover)
	router.Use(middleware.Logger)
	// 添加metrics中间件，收集HTTP请求指标
	router.Use(svc.Metrics.HTTPMiddleware)
	// 限流中间件
	router.Use(rateLimiter.Middleware)
	// 使用配置中的CORS设置
	corsConfig := &middleware.CORSConfig{
		AllowedOrigins: cfg.GetCORSAllowedOrigins(),
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Requested-With", "X-Captcha-Id", "X-Captcha-Answer"},
		MaxAge:         86400, // 24小时
	}

	// 验证CORS配置（生产环境禁止通配符）
	if err := corsConfig.Validate(cfg.Env); err != nil {
		return nil, nil, fmt.Errorf("CORS配置验证失败: %w", err)
	}

	router.Use(middleware.CORS(corsConfig))

	// 添加语言中间件，支持多语言错误消息
	router.Use(middleware.Language)

	// ==== 注册路由 ====
	// 健康检查端点 (不需要认证)
	// 注：/healthz 与 /readyz 探针端点在函数末尾通过独立路由器注册，
	// 绕过限流和 metrics 中间件，避免 k8s 探针流量干扰业务指标或触发限流
	router.HandleFunc("/health", healthHandler).Methods("GET")

	// 安全设计：初始化面板路由 (/init, /api/v1/init/*) 不再注册到公网主路由
	// 改由独立的 loopback HTTP 服务器承载（见 internal/app/init_server.go），
	// 监听 127.0.0.1:9091，与公网主路由完全隔离，
	// 杜绝反向代理在本机转发时把公网请求识别成本机请求的风险

	// Prometheus指标端点 (使用Basic Auth保护)
	router.Handle("/metrics", middleware.BasicAuth(cfg.MetricsUsername, cfg.MetricsPassword)(http.HandlerFunc(metricsHandler.HandleMetrics))).Methods("GET")

	// OIDC Discovery端点 (公开)
	router.HandleFunc("/.well-known/openid-configuration", wellKnownHandler.HandleDiscovery).Methods("GET")
	router.HandleFunc("/.well-known/jwks.json", wellKnownHandler.HandleJWKS).Methods("GET")

	// 第三方登录端点 (公开)
	// /auth/providers 仅返回静态 provider 列表，使用全局限流即可
	router.HandleFunc("/auth/providers", socialHandler.HandleProviders).Methods("GET")

	// API端点
	api := router.PathPrefix("/api/v1").Subrouter()

	// 创建敏感端点独立限流器（更严格的限额：全局限额的1/10，窗口1分钟）
	// 确保至少为1，防止全局限流配置过低时敏感端点完全无限流
	sensitiveLimit := cfg.RateLimitRequests / 10
	if sensitiveLimit < 1 && cfg.RateLimitRequests > 0 {
		sensitiveLimit = 1
	}
	sensitiveLimiter := newEndpointRateLimiter(svc.Cache, sensitiveLimit, 1*time.Minute, "ratelimit:sensitive")
	// 阶段 4：注入指标回调（同主限流器）
	injectRateLimitMetrics(sensitiveLimiter, svc.Metrics)

	// 阶段 D 修复（L4）：/auth/{provider} 和 /auth/{provider}/callback 涉及 OAuth code 交换，
	// 与登录/注册等敏感端点同等重要，需纳入敏感限流防止暴力枚举 provider/code 等攻击
	// 由于位于 /api/v1 之外，独立应用 sensitiveLimiter 中间件
	if sensitiveLimiter != nil {
		authSensitive := router.PathPrefix("/auth").Subrouter()
		authSensitive.Use(sensitiveLimiter.Middleware)
		authSensitive.HandleFunc("/{provider}", socialHandler.HandleLogin).Methods("GET")
		authSensitive.HandleFunc("/{provider}/callback", socialHandler.HandleCallback).Methods("GET")
	} else {
		router.HandleFunc("/auth/{provider}", socialHandler.HandleLogin).Methods("GET")
		router.HandleFunc("/auth/{provider}/callback", socialHandler.HandleCallback).Methods("GET")
	}

	// 公开端点 (不需要认证)
	if sensitiveLimiter != nil {
		// 敏感端点使用独立的更严格限流器
		sensitive := api.PathPrefix("").Subrouter()
		sensitive.Use(sensitiveLimiter.Middleware)
		sensitive.HandleFunc("/register", registerHandler.Handle).Methods("POST")
		sensitive.HandleFunc("/login", loginHandler.Handle).Methods("POST")
		sensitive.HandleFunc("/login/mfa/verify", loginHandler.HandleVerifyMFALogin).Methods("POST")
		sensitive.HandleFunc("/forgot-password", userHandler.HandleForgotPassword).Methods("POST")
		sensitive.HandleFunc("/reset-password", userHandler.HandleResetPassword).Methods("POST")
	} else {
		// 无限流时直接注册到 api 路由器
		api.HandleFunc("/register", registerHandler.Handle).Methods("POST")
		api.HandleFunc("/login", loginHandler.Handle).Methods("POST")
		api.HandleFunc("/login/mfa/verify", loginHandler.HandleVerifyMFALogin).Methods("POST")
		api.HandleFunc("/forgot-password", userHandler.HandleForgotPassword).Methods("POST")
		api.HandleFunc("/reset-password", userHandler.HandleResetPassword).Methods("POST")
	}

	// 一般公开端点（使用全局限流）
	api.HandleFunc("/captcha", captchaHandler.Handle).Methods("GET")
	api.HandleFunc("/token", tokenHandler.HandleToken).Methods("POST")
	api.HandleFunc("/verify-email", userHandler.HandleVerifyEmail).Methods("GET")

	// 受保护的端点 (需要认证)
	protected := api.PathPrefix("").Subrouter()
	protected.Use(middleware.AuthMiddlewareWithMetrics(svc.JWT, svc.Store, svc.Cache, func() {
		svc.Metrics.Increment("security_invalid_token_total")
	}))
	protected.HandleFunc("/userinfo", userInfoHandler.Handle).Methods("GET")
	protected.HandleFunc("/authorize", authorizeHandler.HandleAuthorize).Methods("GET")
	protected.HandleFunc("/authorize/approve", authorizeHandler.HandleApprove).Methods("POST")
	protected.HandleFunc("/authorize/deny", authorizeHandler.HandleDeny).Methods("POST")
	protected.HandleFunc("/token/revoke", tokenHandler.HandleRevoke).Methods("POST")
	protected.HandleFunc("/verify-email/send", userHandler.HandleSendVerificationEmail).Methods("POST")
	protected.HandleFunc("/change-password", userHandler.HandleChangePassword).Methods("POST")
	protected.HandleFunc("/logout-all", tokenHandler.HandleLogoutAll).Methods("POST")
	protected.HandleFunc("/mfa/setup", mfaHandler.HandleSetupMFA).Methods("POST")
	protected.HandleFunc("/mfa/verify", mfaHandler.HandleVerifyMFA).Methods("POST")
	protected.HandleFunc("/mfa/disable", mfaHandler.HandleDisableMFA).Methods("POST")
	protected.HandleFunc("/mfa/status", mfaHandler.HandleMFAStatus).Methods("GET")
	// MFA 恢复码管理（已认证用户才能生成/验证/查询恢复码）
	protected.HandleFunc("/mfa/recovery-codes/generate", mfaHandler.HandleGenerateRecoveryCodes).Methods("POST")
	protected.HandleFunc("/mfa/recovery-codes/verify", mfaHandler.HandleVerifyRecoveryCode).Methods("POST")
	protected.HandleFunc("/mfa/recovery-codes/status", mfaHandler.HandleGetRecoveryCodeStatus).Methods("GET")

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

	// 质量仪表盘API端点
	admin.HandleFunc("/quality/api/metrics", svc.Dashboard.HandleMetricsAPI).Methods("GET")
	admin.HandleFunc("/quality/api/report/weekly", svc.Dashboard.HandleWeeklyReportAPI).Methods("GET")

	// API 文档调试前端（Scalar + OpenAPI 3.0）
	// 安全：继承 admin 路由的 AuthMiddleware + RequireAdmin，仅管理员可访问
	// 离线模式：Scalar JS 通过 //go:embed 内嵌并由同源路由提供，主应用 CSP 无需放松
	admin.HandleFunc("/api-docs", apiDocsHandler.HandlePage).Methods("GET")
	admin.HandleFunc("/api-docs/openapi.json", apiDocsHandler.HandleSpec).Methods("GET")
	admin.HandleFunc("/api-docs/scalar.js", apiDocsHandler.HandleScalarJS).Methods("GET")

	// ==== 探针端点（独立路由器，绕过限流和metrics中间件） ====
	// k8s liveness/readiness 探针频繁请求，若经过限流/metrics会干扰业务指标
	// 并可能在限流收紧时导致探针失败、Pod被误驱逐
	probeRouter := mux.NewRouter()
	probeRouter.Use(middleware.SecurityHeaders)
	probeRouter.Use(middleware.RequestID)
	probeRouter.Use(middleware.Recover)
	probeRouter.Use(middleware.Logger)
	probeRouter.HandleFunc("/healthz", healthHandler).Methods("GET")
	probeRouter.HandleFunc("/readyz", readyzHandler(svc.Store)).Methods("GET")

	// 顶层分发：探针路径走探针路由器，其余走主路由
	topHandler := probeDispatcher(probeRouter, router)

	return topHandler, []middleware.RateLimitMiddleware{rateLimiter, sensitiveLimiter}, nil
}

// probeDispatcher 按路径将探针请求分流到独立探针路由器，其余交给主路由
// 探针路径（/healthz、/readyz）绕过主路由的限流和 metrics 中间件
func probeDispatcher(probe, main http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			probe.ServeHTTP(w, r)
			return
		}
		main.ServeHTTP(w, r)
	})
}

// newEndpointRateLimiter 创建端点级限流器
// Redis可用时使用分布式限流器，否则使用本地限流器
// limit <= 0 时表示不限流，返回 nil
func newEndpointRateLimiter(cacheSvc cache.Cache, limit int, window time.Duration, keyPrefix string) middleware.RateLimitMiddleware {
	if limit <= 0 {
		return nil
	}
	if rc, ok := cacheSvc.(*cache.RedisCache); ok {
		// T10（方案 B）：敏感端点限流器启用内存降级
		return middleware.NewDistributedRateLimiter(rc.Client(), limit, window, keyPrefix).WithMemoryFallback()
	}
	return middleware.NewRateLimiter(limit, window)
}

// injectRateLimitMetrics 给限流器注入指标回调
// 阶段 4 安全增强：
//   - 本地限流器：注入 WithMetrics（限流触发计数）
//   - 分布式限流器：注入 WithMetrics（限流触发计数）+ WithErrorCallback（fail-open 错误计数）
//   - nil 限流器（禁用）：跳过
//
// 满足 AGENTS.md 第 8.4 节"禁止忽略错误"规则
func injectRateLimitMetrics(rl middleware.RateLimitMiddleware, metricsSvc *metrics.Service) {
	if rl == nil {
		return
	}
	// 本地限流器接口
	if rl, ok := rl.(interface {
		WithMetrics(metricFunc func()) *middleware.RateLimiter
	}); ok {
		rl.WithMetrics(func() {
			metricsSvc.Increment("security_rate_limit_total")
		})
		return
	}
	// 分布式限流器接口
	if rl, ok := rl.(interface {
		WithMetrics(metricFunc func()) *middleware.DistributedRateLimiter
	}); ok {
		rl.WithMetrics(func() {
			metricsSvc.Increment("security_rate_limit_total")
		}).WithErrorCallback(func() {
			metricsSvc.Increment("security_ratelimit_error_total")
		})
		return
	}
}

// healthHandler 健康检查处理器
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"sso","timestamp":"` + time.Now().Format(time.RFC3339) + `"}`))
}

// readyzHandler 就绪探针处理器
func readyzHandler(storeSvc store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := storeSvc.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unready","service":"sso"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready","service":"sso"}`))
	}
}
