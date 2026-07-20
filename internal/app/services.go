// Package app 组合根（composition root）
// 负责装配服务依赖、初始化处理器与路由、启动 HTTP 服务器。
// 将原本集中在 main 包的初始化逻辑抽离，使 main.go 仅保留进程入口。
package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/captcha"
	"github.com/example/sso/internal/config"
	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/metrics"
	"github.com/example/sso/internal/quality/dashboard"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/postgres"
)

// Services 包含所有服务实例
type Services struct {
	Store     *postgres.Store
	Cache     cache.Cache
	Password  *crypto.PasswordService
	JWT       *crypto.JWTService
	Email     *service.EmailService
	Metrics   *metrics.Service
	Token     *service.TokenService
	User      *service.UserService
	Auth      *service.AuthService
	OAuth     *service.OAuthService
	Audit     *service.AuditService
	MFA       *service.MFAService
	Admin     *service.AdminService
	Social    *service.SocialLoginService
	Captcha   *captcha.Service
	Dashboard *dashboard.Server
}

// initServices 初始化所有业务服务
// 职责: 连接数据库、初始化缓存、创建所有服务实例
// version/buildTime 用于注入到管理员服务与初始化面板
func initServices(cfg *config.Config, version, buildTime string) (*Services, *sql.DB, error) {
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
	// NewCacheWithFallback 内部已处理 Redis 连接失败并回退到内存缓存，
	// 因此 initCache 不会返回错误，无需额外的 fallback 分支
	cacheSvc, _ := initCache(cfg, metricsSvc)
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
			_ = db.Close()
			return nil, nil, err
		}
		slog.Info("密钥轮换模式已启用",
			"transition_keys_count", len(transitionPubKeyPaths),
		)
	} else {
		// 标准模式（单密钥）
		privateKey, err := crypto.LoadPrivateKeyFromFile(cfg.JWTPrivateKeyPath)
		if err != nil {
			_ = db.Close()
			return nil, nil, err
		}
		publicKey, err := crypto.LoadPublicKeyFromFile(cfg.JWTPublicKeyPath)
		if err != nil {
			_ = db.Close()
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

	// ==== 初始化审计与 MFA 服务（先于 authSvc 装配，便于 WithMFA 选项注入） ====
	auditSvc := service.NewAuditService(store)
	mfaSvc := service.NewMFAServiceWithAudit(store, auditSvc)
	// 设置MFA恢复码HMAC密钥（与数据库层使用相同密钥）
	if cfg.MFARecoveryHMACKey != "" {
		mfaSvc.SetHMACKey([]byte(cfg.MFARecoveryHMACKey))
	}

	// ==== 初始化业务服务（带缓存） ====
	userSvc := service.NewUserService(store, passwordSvc, emailSvc, cfg.BaseURL())
	// 仅在限流启用时配置邮件限流器（RATE_LIMIT_REQUESTS <= 0 表示禁用所有限流）
	if cfg.RateLimitRequests > 0 {
		userSvc.WithEmailRateLimit(service.NewEmailRateLimiter(cacheSvc))
	}
	// 仅在限流启用时配置登录限流器
	var loginRateLimitOpt service.AuthServiceOption
	if cfg.RateLimitRequests > 0 {
		loginRateLimitOpt = service.WithLoginRateLimit(service.NewLoginRateLimiter(cacheSvc))
	}
	authSvcOpts := []service.AuthServiceOption{
		service.WithCache(cacheSvc),
		service.WithMetrics(metricsSvc),
		service.WithUserService(userSvc),
		// 装配 MFA 服务，启用两阶段登录
		// 未调用 WithMFA 时，启用 MFA 的用户登录会返回 ErrMFAServiceUnavailable
		service.WithMFA(mfaSvc, cfg.MFAChallengeTTL),
	}
	if loginRateLimitOpt != nil {
		authSvcOpts = append(authSvcOpts, loginRateLimitOpt)
	}
	authSvc := service.NewAuthServiceWithOptions(
		store,
		passwordSvc,
		jwtSvc,
		cfg.MaxLoginAttempts,
		cfg.LockoutDuration,
		authSvcOpts...,
	)
	oauthSvc := service.NewOAuthServiceWithOptions(store, tokenSvc, service.WithOAuthCache(cacheSvc), service.WithOAuthPassword(passwordSvc))
	adminSvc := service.NewAdminServiceWithOptions(store, service.WithAdminCache(cacheSvc), service.WithAdminVersion(version, buildTime), service.WithAdminAudit(auditSvc))

	// ==== 初始化第三方登录服务 ====
	socialSvc := service.NewSocialLoginService(store, jwtSvc, cfg.BaseURL(), cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GitHubClientID, cfg.GitHubClientSecret)
	socialSvc.SetAuditService(auditSvc)
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		slog.Info("Google第三方登录已启用")
	}
	if cfg.GitHubClientID != "" && cfg.GitHubClientSecret != "" {
		slog.Info("GitHub第三方登录已启用")
	}

	// ==== 初始化验证码服务 ====
	captchaSvc := captcha.NewServiceWithAdaptive(cacheSvc, cfg.CaptchaEnabled, cfg.CaptchaTTL, cfg.CaptchaFailThreshold, cfg.CaptchaFailWindow)
	if cfg.CaptchaEnabled {
		slog.Info("验证码服务已启用", "ttl", cfg.CaptchaTTL, "fail_threshold", cfg.CaptchaFailThreshold, "fail_window", cfg.CaptchaFailWindow)
	} else {
		slog.Info("验证码服务已禁用")
	}

	// ==== 初始化质量仪表盘 ====
	dashboardSvc := dashboard.NewServer(store, slog.Default())

	// ==== 返回所有服务 ====
	return &Services{
		Store:     store,
		Cache:     cacheSvc,
		Password:  passwordSvc,
		JWT:       jwtSvc,
		Email:     emailSvc,
		Metrics:   metricsSvc,
		Token:     tokenSvc,
		User:      userSvc,
		Auth:      authSvc,
		OAuth:     oauthSvc,
		Audit:     auditSvc,
		MFA:       mfaSvc,
		Admin:     adminSvc,
		Social:    socialSvc,
		Captcha:   captchaSvc,
		Dashboard: dashboardSvc,
	}, db, nil
}

// connectDatabase 连接数据库并验证连接
func connectDatabase(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURL())
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

// initCache 初始化缓存服务
func initCache(cfg *config.Config, metricsSvc *metrics.Service) (cache.Cache, error) {
	opt := &cache.Option{
		RedisEnable:       cfg.RedisEnable,
		RedisHost:         cfg.RedisHost,
		RedisPort:         cfg.RedisPort,
		RedisPassword:     cfg.RedisPassword,
		RedisDB:           cfg.RedisDB,
		RedisPoolSize:     cfg.RedisPoolSize,
		RedisMinIdleConns: cfg.RedisMinIdleConns,
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
