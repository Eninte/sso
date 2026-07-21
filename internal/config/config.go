// Package config 配置管理
// 负责从环境变量加载服务配置，提供默认值
// 遵循12-Factor App原则，配置通过环境变量注入
package config

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/joho/godotenv"
)

// 配置错误定义（使用统一错误定义）
var (
	ErrDBPasswordRequired = apperrors.ErrDBPasswordRequired
	ErrJWTKeyRequired     = apperrors.ErrJWTKeyRequired
	ErrBcryptCostTooLow   = apperrors.ErrBcryptCostTooLow
)

// Config 服务配置结构
// 包含所有服务运行所需的配置项
type Config struct {
	// 服务器配置
	ServerHost    string // 服务器监听地址
	ServerPort    string // 服务器监听端口
	Env           string // 运行环境 (development/production)
	PublicBaseURL string // 公共基础URL（用于邮件链接等对外暴露场景，必须 HTTPS）

	// 阶段 4 配置项：用于对外暴露场景
	// 若为空，回退到 BaseURL()（开发环境友好）
	// 生产环境必须设置为 https:// 开头

	// 数据库配置
	DBHost     string // 数据库主机
	DBPort     string // 数据库端口
	DBName     string // 数据库名称
	DBUser     string // 数据库用户
	DBPassword string // 数据库密码
	DBSSLMode  string // SSL模式

	// 数据库连接池配置
	DBMaxOpenConns    int           // 最大打开连接数
	DBMaxIdleConns    int           // 最大空闲连接数
	DBConnMaxLifetime time.Duration // 连接最大生命周期
	DBConnMaxIdleTime time.Duration // 连接最大空闲时间
	DBQueryTimeout    time.Duration // 查询超时时间

	// Redis配置
	RedisEnable       bool          // 是否启用Redis缓存
	RedisHost         string        // Redis主机
	RedisPort         string        // Redis端口
	RedisPassword     string        // Redis密码
	RedisDB           int           // Redis数据库编号 (0-15)
	RedisConnTimeout  time.Duration // Redis连接超时
	RedisPoolSize     int           // Redis连接池大小
	RedisMinIdleConns int           // Redis最小空闲连接数

	// JWT配置
	JWTPrivateKeyPath string        // JWT私钥路径
	JWTPublicKeyPath  string        // JWT公钥路径
	AccessTokenTTL    time.Duration // Access Token有效期
	RefreshTokenTTL   time.Duration // Refresh Token有效期
	JWTIssuer         string        // Token签发者标识

	// 密钥轮换配置
	KeyRotationEnabled       bool          // 是否启用密钥轮换
	KeyRotationInterval      time.Duration // 密钥轮换周期
	KeyTransitionPeriod      time.Duration // 密钥过渡期时长
	JWTTransitionPubKeyPaths string        // 轮换期间的旧公钥路径（逗号分隔）
	JWTKeyEncryptionKey      string        // T7：DB 私钥信封加密 KEK（64 位 hex = 32 字节）

	// 安全配置
	BcryptCost         int           // bcrypt成本因子
	RateLimitRequests  int           // 限流请求数
	RateLimitWindow    time.Duration // 限流时间窗口
	MaxLoginAttempts   int           // 最大登录失败次数
	LockoutDuration    time.Duration // 账户锁定时长
	MFARecoveryHMACKey string        // MFA恢复码HMAC密钥（生产环境必须设置）
	MFAChallengeTTL    time.Duration // MFA 两阶段登录 Challenge 有效期（默认 5 分钟）

	// 邮件配置
	SMTPHost     string // SMTP服务器地址
	SMTPPort     int    // SMTP端口
	SMTPUser     string // SMTP用户名
	SMTPPassword string // SMTP密码
	SMTPFrom     string // 发件人地址

	// 第三方登录配置
	GoogleClientID     string // Google客户端ID
	GoogleClientSecret string // Google客户端密钥
	GitHubClientID     string // GitHub客户端ID
	GitHubClientSecret string // GitHub客户端密钥
	// SocialStateCookieBinding T11：社交登录 state 会话绑定（login CSRF 防护）
	// 默认开启；纯 API 客户端（无 Cookie）场景可设为 false 恢复旧行为
	SocialStateCookieBinding bool

	// 受信代理配置（X-Real-IP仅在请求来自受信代理时才被信任）
	TrustedProxies string // 受信代理IP列表 (逗号分隔，如 "10.0.0.1,172.16.0.0/12")

	// CORS配置
	CORSAllowedOrigins string // 允许的跨域源 (逗号分隔)

	// Metrics配置
	MetricsUsername string // Metrics Basic Auth用户名
	MetricsPassword string // Metrics Basic Auth密码

	// 验证码配置
	CaptchaEnabled       bool          // 是否启用验证码
	CaptchaTTL           time.Duration // 验证码有效期
	CaptchaFailThreshold int           // 连续失败N次后触发验证码
	CaptchaFailWindow    time.Duration // 失败计数窗口

	// 优雅关闭配置
	ShutdownTimeout time.Duration // 优雅关闭超时时间
	LANDeployment   bool          // LAN部署模式（放宽部分生产环境校验）

	// 初始化面板配置
	// 安全设计：初始化路由仅注册在独立 loopback HTTP 服务器上，
	// 与公网主路由完全隔离，杜绝反向代理绕过 isLocalRequest 的风险
	InitEnabled    bool   // 是否启用初始化面板（初始化完成后应设为 false 永久关闭）
	InitListenAddr string // 初始化面板监听地址（必须为 loopback，强制 127.0.0.1）
}

// Load 从环境变量加载配置
// 如果环境变量不存在，使用预设的默认值
// 注意：敏感配置（如密码）必须通过环境变量设置
func Load() (*Config, error) {
	// 首先尝试加载.env文件到环境变量
	// 这样配置向导生成的.env文件才能被读取
	loadEnvFile()

	cfg := &Config{
		// 服务器配置
		ServerHost:    getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort:    getEnv("SERVER_PORT", "9090"),
		Env:           getEnv("SERVER_ENV", "development"),
		PublicBaseURL: os.Getenv("PUBLIC_BASE_URL"), // 阶段 4：对外暴露场景的公共 URL（生产必须 HTTPS）

		// 数据库配置
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBName:     getEnv("DB_NAME", "sso"),
		DBUser:     getEnv("DB_USER", "sso"),
		DBPassword: os.Getenv("DB_PASSWORD"), // 必须通过环境变量设置
		DBSSLMode:  getEnv("DB_SSL_MODE", "prefer"),

		// 数据库连接池配置
		DBMaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 100),
		DBMaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 50),
		DBConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		DBConnMaxIdleTime: getEnvDuration("DB_CONN_MAX_IDLE_TIME", 1*time.Minute),
		DBQueryTimeout:    getEnvDuration("DB_QUERY_TIMEOUT", 10*time.Second),

		// Redis配置
		RedisEnable:       getEnvBool("REDIS_ENABLE", true),
		RedisHost:         getEnv("REDIS_HOST", "localhost"),
		RedisPort:         getEnv("REDIS_PORT", "6379"),
		RedisPassword:     os.Getenv("REDIS_PASSWORD"),
		RedisDB:           getEnvInt("REDIS_DB", 0),
		RedisConnTimeout:  getEnvDuration("REDIS_CONN_TIMEOUT", 5*time.Second),
		RedisPoolSize:     getEnvInt("REDIS_POOL_SIZE", 10),
		RedisMinIdleConns: getEnvInt("REDIS_MIN_IDLE_CONNS", 5),

		// JWT配置
		JWTPrivateKeyPath: os.Getenv("JWT_PRIVATE_KEY_PATH"), // 必须通过环境变量设置
		JWTPublicKeyPath:  os.Getenv("JWT_PUBLIC_KEY_PATH"),  // 必须通过环境变量设置
		AccessTokenTTL:    getEnvDuration("JWT_ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:   getEnvDuration("JWT_REFRESH_TOKEN_TTL", 168*time.Hour),
		JWTIssuer:         getEnv("JWT_ISSUER", "sso"),

		// 密钥轮换配置
		KeyRotationEnabled:       getEnvBool("KEY_ROTATION_ENABLED", false),
		KeyRotationInterval:      getEnvDuration("KEY_ROTATION_INTERVAL", 2160*time.Hour), // 90天
		KeyTransitionPeriod:      getEnvDuration("KEY_TRANSITION_PERIOD", 24*time.Hour),   // 24小时
		JWTTransitionPubKeyPaths: os.Getenv("JWT_TRANSITION_PUBKEY_PATHS"),                // 轮换期间的旧公钥路径（逗号分隔）
		JWTKeyEncryptionKey:      os.Getenv("JWT_KEY_ENCRYPTION_KEY"),                     // T7：DB 私钥信封加密 KEK（64 位 hex）

		// 安全配置
		// BcryptCost: bcrypt成本因子，影响密码哈希性能
		// 推荐值: 12-14，值越高越安全但性能越低
		// cost=12: ~200ms, cost=13: ~400ms, cost=14: ~800ms
		// 生产环境必须 >= 12
		BcryptCost:         getEnvInt("BCRYPT_COST", 12),
		RateLimitRequests:  getEnvInt("RATE_LIMIT_REQUESTS", 100),
		RateLimitWindow:    getEnvDuration("RATE_LIMIT_WINDOW", 1*time.Minute),
		MaxLoginAttempts:   getEnvInt("MAX_LOGIN_ATTEMPTS", 5),
		LockoutDuration:    getEnvDuration("LOCKOUT_DURATION", 30*time.Minute),
		MFARecoveryHMACKey: getEnv("MFA_RECOVERY_HMAC_KEY", ""),
		MFAChallengeTTL:    getEnvDuration("MFA_CHALLENGE_TTL", 5*time.Minute),

		// 邮件配置
		SMTPHost:     getEnv("SMTP_HOST", "localhost"),
		SMTPPort:     getEnvInt("SMTP_PORT", 587),
		SMTPUser:     os.Getenv("SMTP_USER"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:     getEnv("SMTP_FROM", "noreply@example.com"),

		// 第三方登录配置
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		// T11：社交登录 state 会话绑定，默认开启
		SocialStateCookieBinding: getEnvBool("SOCIAL_STATE_COOKIE_BINDING", true),

		// 受信代理配置
		TrustedProxies: getEnv("TRUSTED_PROXIES", ""),

		// CORS配置
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"),

		// Metrics配置
		MetricsUsername: os.Getenv("METRICS_USERNAME"),
		MetricsPassword: os.Getenv("METRICS_PASSWORD"),

		// 验证码配置
		CaptchaEnabled:       getEnvBool("CAPTCHA_ENABLED", true),
		CaptchaTTL:           getEnvDuration("CAPTCHA_TTL", 5*time.Minute),
		CaptchaFailThreshold: getEnvInt("CAPTCHA_FAIL_THRESHOLD", 3),
		CaptchaFailWindow:    getEnvDuration("CAPTCHA_FAIL_WINDOW", 15*time.Minute),

		// 优雅关闭配置
		ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		LANDeployment:   getEnvBool("LAN_DEPLOYMENT", false),

		// 初始化面板配置
		// 默认启用，监听 127.0.0.1:9091，与公网主路由完全隔离
		// 初始化完成后应设置 INIT_ENABLED=false 永久关闭
		InitEnabled:    getEnvBool("INIT_ENABLED", true),
		InitListenAddr: getInitListenAddr(),
	}

	// 验证必需的配置
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validateDatabaseConfig 验证数据库配置：DB_PASSWORD 必填，生产环境需启用 SSL（LAN 部署除外）
func validateDatabaseConfig(c *Config) error {
	// 验证数据库密码
	if c.DBPassword == "" {
		slog.Error("数据库密码未设置", "env_var", "DB_PASSWORD")
		return ErrDBPasswordRequired
	}

	// 生产环境建议启用数据库SSL（LAN部署时允许disable）
	// 阶段 4 安全增强：拒绝 prefer（若服务端不支持 SSL 会降级为明文，存在 MITM 风险）
	if c.Env == "production" && (c.DBSSLMode == "disable" || c.DBSSLMode == "prefer" || c.DBSSLMode == "allow") {
		if c.LANDeployment {
			slog.Warn("生产环境数据库未启用强制SSL（LAN部署模式）", "ssl_mode", c.DBSSLMode)
		} else {
			slog.Error("生产环境数据库必须启用强制SSL", "ssl_mode", c.DBSSLMode)
			return fmt.Errorf("DB_SSL_MODE must be 'require' or higher in production (current: %s)", c.DBSSLMode)
		}
	}

	return nil
}

// validateJWTConfig 验证JWT配置：补全密钥路径默认值，校验 Token TTL 为正数
func validateJWTConfig(c *Config) error {
	// 验证JWT密钥路径，如果为空则设置默认值
	if c.JWTPrivateKeyPath == "" {
		c.JWTPrivateKeyPath = "./keys/private.pem"
		slog.Warn("JWT私钥路径未设置，使用默认值", "path", c.JWTPrivateKeyPath)
	}
	if c.JWTPublicKeyPath == "" {
		c.JWTPublicKeyPath = "./keys/public.pem"
		slog.Warn("JWT公钥路径未设置，使用默认值", "path", c.JWTPublicKeyPath)
	}

	// 验证Token TTL值为正数
	if c.AccessTokenTTL <= 0 {
		slog.Error("Access Token TTL 必须为正数", "ttl", c.AccessTokenTTL)
		return fmt.Errorf("access token TTL must be positive")
	}
	if c.RefreshTokenTTL <= 0 {
		slog.Error("Refresh Token TTL 必须为正数", "ttl", c.RefreshTokenTTL)
		return fmt.Errorf("refresh token TTL must be positive")
	}

	// 验证Token TTL合理性
	if c.AccessTokenTTL < 1*time.Minute {
		slog.Warn("Access Token TTL 过短，建议至少1分钟", "ttl", c.AccessTokenTTL)
	}
	if c.RefreshTokenTTL < c.AccessTokenTTL {
		slog.Warn("Refresh Token TTL 应大于 Access Token TTL",
			"access_ttl", c.AccessTokenTTL,
			"refresh_ttl", c.RefreshTokenTTL)
	}

	return nil
}

// validateSecurityConfig 验证安全配置：bcrypt cost、限流、登录保护
func validateSecurityConfig(c *Config) error {
	// 验证bcrypt cost范围
	// 阶段 4 安全增强：bcrypt 算法上限为 31，超出会导致 bcrypt 调用 panic，拒绝启动
	if c.BcryptCost > 31 {
		slog.Error("bcrypt cost 超出算法上限 (4-31)，拒绝启动", "cost", c.BcryptCost)
		return fmt.Errorf("BCRYPT_COST must be <= 31 (bcrypt algorithm limit), current value: %d", c.BcryptCost)
	}
	if c.BcryptCost < 4 {
		slog.Warn("bcrypt cost 低于推荐下限 (4)", "cost", c.BcryptCost)
	}

	// 生产环境必须使用足够强的bcrypt cost
	if c.Env == "production" && c.BcryptCost < 12 {
		slog.Error("生产环境bcrypt cost应至少为12", "current", c.BcryptCost)
		return ErrBcryptCostTooLow
	}

	// 验证限流配置
	if c.RateLimitRequests <= 0 {
		if c.Env == "production" && !c.LANDeployment {
			slog.Error("生产环境限流请求数必须为正数", "requests", c.RateLimitRequests)
			return fmt.Errorf("RATE_LIMIT_REQUESTS must be positive in production, current value: %d", c.RateLimitRequests)
		}
		slog.Warn("限流请求数应为正数", "requests", c.RateLimitRequests)
	}

	// 验证登录保护配置
	if c.MaxLoginAttempts <= 0 {
		if c.Env == "production" && !c.LANDeployment {
			slog.Error("生产环境最大登录尝试次数必须为正数", "attempts", c.MaxLoginAttempts)
			return fmt.Errorf("MAX_LOGIN_ATTEMPTS must be positive in production, current value: %d", c.MaxLoginAttempts)
		}
		slog.Warn("最大登录尝试次数应为正数", "attempts", c.MaxLoginAttempts)
	}
	if c.LockoutDuration <= 0 {
		slog.Warn("账户锁定时长应为正数", "duration", c.LockoutDuration)
	}

	// T7：KEK 格式与生产必填校验
	if err := validateKeyEncryptionConfig(c); err != nil {
		return err
	}

	// T11：生产环境关闭 state 会话绑定会暴露 login CSRF 风险，告警提示
	if c.Env == "production" && !c.SocialStateCookieBinding {
		slog.Warn("生产环境已关闭社交登录 state 会话绑定（SOCIAL_STATE_COOKIE_BINDING=false），存在 login CSRF 风险，仅纯 API 客户端场景适用")
	}

	return nil
}

// validateKeyEncryptionConfig 验证 JWT_KEY_ENCRYPTION_KEY（T7）
// 所有环境配置即须为 64 位 hex（fail-fast）；
// 生产环境启用密钥轮换（DB 密钥存储）时必填，否则私钥只能明文落库
func validateKeyEncryptionConfig(c *Config) error {
	// KEK 格式校验（所有环境，配置即须合法，fail-fast）
	if c.JWTKeyEncryptionKey != "" {
		if _, err := hex.DecodeString(c.JWTKeyEncryptionKey); err != nil || len(c.JWTKeyEncryptionKey) != 64 {
			slog.Error("JWT_KEY_ENCRYPTION_KEY 必须为64位hex（32字节）")
			return fmt.Errorf("JWT_KEY_ENCRYPTION_KEY must be 64 hex chars (32 bytes for AES-256)")
		}
	}

	// 生产环境启用密钥轮换（DB 密钥存储）时 KEK 必填
	if c.Env == "production" && c.KeyRotationEnabled && c.JWTKeyEncryptionKey == "" && !c.LANDeployment {
		slog.Error("生产环境启用密钥轮换时必须设置 JWT_KEY_ENCRYPTION_KEY")
		return fmt.Errorf("JWT_KEY_ENCRYPTION_KEY must be set in production when KEY_ROTATION_ENABLED=true")
	}
	return nil
}

// validateProductionConfig 验证生产环境配置：CORS、JWT Issuer、SMTP、Metrics 认证等安全要求
// 非生产环境直接跳过
func validateProductionConfig(c *Config) error {
	// 仅在生产环境执行验证
	if c.Env != "production" {
		return nil
	}

	lanMode := c.LANDeployment

	// 依次执行各项生产环境检查
	checks := []func(*Config, bool) error{
		validateProdBypassCORS,
		validateProdDefaultCORS,
		validateProdJWTIssuer,
		validateProdSMTPHost,
		validateProdMetricsAuth,
		validateProdRedisPassword,
		validateProdPublicBaseURL,
		validateProdJWTKeyPermissions,
		validateProdMFARecoveryKey,
		validateProdInitEnabled,
	}
	for _, check := range checks {
		if err := check(c, lanMode); err != nil {
			return err
		}
	}

	return nil
}

// validateProdBypassCORS 检查 CORS 配置不包含 localhost
func validateProdBypassCORS(c *Config, lanMode bool) error {
	if !strings.Contains(strings.ToLower(c.CORSAllowedOrigins), "localhost") {
		return nil
	}
	if lanMode {
		slog.Warn("生产环境CORS配置包含localhost（LAN部署模式）", "cors_origins", c.CORSAllowedOrigins)
		return nil
	}
	slog.Error("生产环境CORS配置不能包含localhost", "cors_origins", c.CORSAllowedOrigins)
	return fmt.Errorf("CORS_ALLOWED_ORIGINS cannot contain 'localhost' in production")
}

// validateProdDefaultCORS 检查默认 CORS 配置
func validateProdDefaultCORS(c *Config, lanMode bool) error {
	if c.CORSAllowedOrigins != "http://localhost:3000" {
		return nil
	}
	if lanMode {
		slog.Warn("生产环境使用默认CORS配置（LAN部署模式）")
		return nil
	}
	slog.Error("生产环境不能使用默认CORS配置")
	return fmt.Errorf("CORS_ALLOWED_ORIGINS must be set in production")
}

// validateProdJWTIssuer 检查 JWT Issuer 配置
func validateProdJWTIssuer(c *Config, lanMode bool) error {
	if c.JWTIssuer != "sso" {
		return nil
	}
	if lanMode {
		slog.Warn("生产环境使用默认JWT Issuer（LAN部署模式）")
		return nil
	}
	slog.Error("生产环境不能使用默认JWT Issuer")
	return fmt.Errorf("JWT_ISSUER must be set in production, cannot use default value 'sso'")
}

// validateProdSMTPHost 检查 SMTP 配置
func validateProdSMTPHost(c *Config, lanMode bool) error {
	if c.SMTPHost != "localhost" {
		return nil
	}
	if lanMode {
		slog.Warn("生产环境使用localhost作为SMTP服务器（LAN部署模式）")
		return nil
	}
	slog.Error("生产环境不能使用localhost作为SMTP服务器")
	return fmt.Errorf("SMTP_HOST must be set in production, cannot be 'localhost'")
}

// validateProdMetricsAuth 检查 Metrics 认证配置
// 阶段 4 安全增强：生产环境必须同时设置 METRICS_USERNAME 和 METRICS_PASSWORD
// 否则 /metrics 端点完全无认证，暴露 Prometheus 指标
func validateProdMetricsAuth(c *Config, lanMode bool) error {
	if c.MetricsUsername != "" && c.MetricsPassword != "" {
		return nil
	}
	if lanMode {
		slog.Warn("生产环境未配置 METRICS 认证（LAN部署模式）")
		return nil
	}
	slog.Error("生产环境必须配置 METRICS_USERNAME 和 METRICS_PASSWORD")
	return fmt.Errorf("METRICS_USERNAME and METRICS_PASSWORD must both be set in production")
}

// validateProdRedisPassword 阶段 4 安全增强：Redis 启用时强制密码非空
func validateProdRedisPassword(c *Config, lanMode bool) error {
	if !c.RedisEnable || c.RedisPassword != "" {
		return nil
	}
	if lanMode {
		slog.Warn("生产环境启用 Redis 但未设置 REDIS_PASSWORD（LAN部署模式）")
		return nil
	}
	slog.Error("生产环境启用 Redis 时必须设置 REDIS_PASSWORD")
	return fmt.Errorf("REDIS_PASSWORD must be set when REDIS_ENABLE=true in production")
}

// validateProdPublicBaseURL 阶段 4 安全增强：PUBLIC_BASE_URL 必须设置且为 HTTPS
// 防止邮件链接中 token 在明文 HTTP 传输中被窃取
func validateProdPublicBaseURL(c *Config, lanMode bool) error {
	if c.PublicBaseURL == "" {
		if lanMode {
			slog.Warn("生产环境未设置 PUBLIC_BASE_URL（LAN部署模式），邮件链接将使用 BaseURL()")
			return nil
		}
		slog.Error("生产环境必须设置 PUBLIC_BASE_URL")
		return fmt.Errorf("PUBLIC_BASE_URL must be set in production")
	}
	if !strings.HasPrefix(c.PublicBaseURL, "https://") {
		if lanMode {
			slog.Warn("生产环境 PUBLIC_BASE_URL 非 HTTPS scheme（LAN部署模式）", "public_base_url", c.PublicBaseURL)
			return nil
		}
		slog.Error("生产环境 PUBLIC_BASE_URL 必须为 HTTPS scheme", "public_base_url", c.PublicBaseURL)
		return fmt.Errorf("PUBLIC_BASE_URL must start with 'https://' in production (current: %s)", c.PublicBaseURL)
	}
	return nil
}

// validateProdJWTKeyPermissions 阶段 4 安全增强：JWT 私钥文件权限校验
// 私钥必须仅所有者可读写（chmod 600），否则拒绝启动
func validateProdJWTKeyPermissions(c *Config, lanMode bool) error {
	if err := validateJWTPrivateKeyPermissions(c); err != nil {
		if lanMode {
			slog.Warn("生产环境 JWT 私钥权限校验失败（LAN部署模式）", "error", err)
			return nil
		}
		slog.Error("生产环境 JWT 私钥权限校验失败", "error", err)
		return err
	}
	return nil
}

// validateProdMFARecoveryKey 检查 MFA 恢复码 HMAC 密钥（生产环境强制要求）
// 防止攻击者通过数据库泄露推导恢复码，AGENTS.md 硬约束
//
// T4 强度校验：生产环境密钥长度必须 >= 32 字节（建议 openssl rand -hex 32 生成），
// 过短密钥熵不足，恢复码 HMAC 可被暴力推导，拒绝启动
func validateProdMFARecoveryKey(c *Config, lanMode bool) error {
	if len(c.MFARecoveryHMACKey) >= 32 {
		return nil
	}
	if lanMode {
		slog.Warn("生产环境 MFA_RECOVERY_HMAC_KEY 未设置或少于32字节（LAN部署模式）",
			"key_length", len(c.MFARecoveryHMACKey))
		return nil
	}
	slog.Error("生产环境 MFA_RECOVERY_HMAC_KEY 必须设置且不少于32字节",
		"key_length", len(c.MFARecoveryHMACKey))
	return fmt.Errorf("MFA_RECOVERY_HMAC_KEY must be set and at least 32 bytes in production")
}

// validateProdInitEnabled 检查初始化面板配置：生产环境推荐初始化完成后关闭 INIT_ENABLED
// 不强制拒绝（允许重复初始化），但发出告警
func validateProdInitEnabled(c *Config, lanMode bool) error {
	if c.InitEnabled && !lanMode {
		slog.Warn("生产环境启用 INIT_ENABLED，建议初始化完成后设置为 false 永久关闭",
			"init_listen_addr", c.InitListenAddr)
	}
	return nil
}

// validateJWTPrivateKeyPermissions 校验 JWT 私钥文件权限
// 阶段 4 安全增强：私钥文件必须仅所有者可读写（mode & 0077 == 0）
// 容器环境或 STRICT_KEY_PERMISSIONS=false 时跳过校验（与 keyloader.go 行为一致）
func validateJWTPrivateKeyPermissions(c *Config) error {
	// STRICT_KEY_PERMISSIONS=false 时跳过（与 keyloader.go 行为一致）
	if os.Getenv("STRICT_KEY_PERMISSIONS") == "false" {
		return nil
	}

	// 容器环境跳过（Kubernetes Secret 挂载默认权限 0644）
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return nil
	}

	// 解析私钥路径（支持相对路径）
	keyPath := c.JWTPrivateKeyPath
	if !filepath.IsAbs(keyPath) {
		// 相对路径基于当前工作目录解析
		absPath, err := filepath.Abs(keyPath)
		if err != nil {
			return fmt.Errorf("resolve JWT private key path failed: %w", err)
		}
		keyPath = absPath
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		// 文件不存在不在此处报错（keyloader 会在加载时校验）
		//nolint:nilerr // 故意吞掉 stat 错误，由 keyloader 统一处理
		return nil
	}
	mode := info.Mode().Perm()
	if mode&0077 != 0 {
		return fmt.Errorf("JWT private key file permissions too open (mode=%o), require chmod 600: %s", mode, keyPath)
	}
	return nil
}

// validate 验证配置有效性：依次执行数据库、JWT、安全、生产环境校验
func (c *Config) validate() error {
	// 验证数据库配置
	if err := validateDatabaseConfig(c); err != nil {
		return err
	}

	// 验证JWT配置
	if err := validateJWTConfig(c); err != nil {
		return err
	}

	// 验证安全配置
	if err := validateSecurityConfig(c); err != nil {
		return err
	}

	// 验证生产环境配置
	if err := validateProductionConfig(c); err != nil {
		return err
	}

	// 验证环境设置（T5：SERVER_ENV 白名单，未知值拒绝启动而非仅警告）
	// test 为合法值：CI E2E 与 .env.test 使用，行为等同非生产（不启用生产校验）
	if c.Env != "development" && c.Env != "production" && c.Env != "test" {
		slog.Error("未知的运行环境", "env", c.Env)
		return fmt.Errorf("SERVER_ENV must be one of: development, production, test (got %q)", c.Env)
	}

	// T4：非生产环境 MFA_RECOVERY_HMAC_KEY 未设置或强度不足时告警（不拒绝启动）
	if c.Env != "production" && len(c.MFARecoveryHMACKey) < 32 {
		slog.Warn("MFA_RECOVERY_HMAC_KEY 未设置或少于32字节，生产环境将拒绝启动",
			"key_length", len(c.MFARecoveryHMACKey))
	}

	// 验证端口范围
	if port, err := strconv.Atoi(c.ServerPort); err != nil || port < 1 || port > 65535 {
		slog.Warn("服务器端口无效", "port", c.ServerPort)
	}

	return nil
}

// DatabaseURL 构建PostgreSQL数据库连接URL
// 使用 net/url 对用户名、密码、数据库名进行转义，避免特殊字符破坏连接串
func (c *Config) DatabaseURL() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.DBUser, c.DBPassword),
		Host:   c.DBHost + ":" + c.DBPort,
		Path:   "/" + c.DBName,
	}
	q := u.Query()
	q.Set("sslmode", c.DBSSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

// RedisURL 构建Redis连接URL
// 对密码进行URL转义，避免特殊字符破坏连接串
func (c *Config) RedisURL() string {
	u := &url.URL{
		Scheme: "redis",
		Host:   c.RedisHost + ":" + c.RedisPort,
	}
	if c.RedisPassword != "" {
		u.User = url.UserPassword("", c.RedisPassword)
	}
	return u.String()
}

// BaseURL 构建服务基础URL
func (c *Config) BaseURL() string {
	return "http://" + c.ServerHost + ":" + c.ServerPort
}

// PublicBaseURLOrFallback 返回对外暴露场景的公共 URL
// 阶段 4：优先使用 PUBLIC_BASE_URL，未设置时回退到 BaseURL()
// 生产环境校验在 validateProductionConfig 中强制要求 HTTPS scheme
func (c *Config) PublicBaseURLOrFallback() string {
	if c.PublicBaseURL != "" {
		return c.PublicBaseURL
	}
	return c.BaseURL()
}

// IsDevelopment 判断是否为开发环境
func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

// getEnv 获取字符串类型环境变量
// 如果环境变量不存在，返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt 获取整数类型环境变量
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvDuration 获取时间间隔类型环境变量
// 支持格式: "15m", "1h", "30s" 等
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// getEnvBool 获取布尔类型环境变量
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return strings.ToLower(value) == "true" || value == "1"
	}
	return defaultValue
}

// getInitListenAddr 获取初始化面板监听地址
// 安全设计：强制 loopback（127.0.0.1），拒绝绑定到公网或局域网地址
// 支持自定义端口：环境变量 INIT_LISTEN=9091 或 INIT_LISTEN=127.0.0.1:9091
// 任何非 loopback 主机部分都会被忽略并回退到 127.0.0.1
func getInitListenAddr() string {
	raw := getEnv("INIT_LISTEN", "127.0.0.1:9091")

	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		// 仅端口格式（如 "9091"），回退到默认地址
		slog.Warn("INIT_LISTEN 格式无效，应为 host:port 或 port，使用默认值", "raw", raw, "default", "127.0.0.1:9091")
		return "127.0.0.1:9091"
	}

	// 强制 loopback：仅允许 127.0.0.1 / ::1 / localhost
	if !isLoopbackHost(host) {
		slog.Error("INIT_LISTEN 拒绝非 loopback 地址，初始化面板仅允许本地访问", "raw", raw)
		return "127.0.0.1:9091"
	}

	// 校验端口范围
	if portNum, err := strconv.Atoi(port); err != nil || portNum < 1 || portNum > 65535 {
		slog.Warn("INIT_LISTEN 端口无效，使用默认 9091", "port", port)
		return "127.0.0.1:9091"
	}

	return host + ":" + port
}

// isLoopbackHost 判断主机名是否为 loopback 地址
func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "127.0.0.1", "::1", "localhost":
		return true
	default:
		return false
	}
}

// GetCORSAllowedOrigins 获取CORS允许的源列表
func (c *Config) GetCORSAllowedOrigins() []string {
	return splitAndTrim(c.CORSAllowedOrigins)
}

// GetTrustedProxies 获取受信代理IP列表
func (c *Config) GetTrustedProxies() []string {
	return splitAndTrim(c.TrustedProxies)
}

// GetJWTTransitionPubKeyPaths 获取轮换期间的旧公钥路径列表
func (c *Config) GetJWTTransitionPubKeyPaths() []string {
	return splitAndTrim(c.JWTTransitionPubKeyPaths)
}

// splitAndTrim 分割字符串并去除空格
func splitAndTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// GetEnvPath 返回.env文件路径
func GetEnvPath() string {
	// 优先使用环境变量指定的路径
	if envPath := os.Getenv("ENV_FILE_PATH"); envPath != "" {
		return envPath
	}

	// 尝试当前工作目录的.env
	if cwd, err := os.Getwd(); err == nil {
		cwdEnv := filepath.Join(cwd, ".env")
		// 只有当前目录的.env存在时才使用它
		if _, err := os.Stat(cwdEnv); err == nil {
			return cwdEnv
		}
	}

	// 默认使用/app/.env（Docker环境）
	return "/app/.env"
}

// loadEnvFile 加载.env文件到环境变量
// 使用 godotenv 库解析，支持标准 .env 格式（多行值、引号、export 前缀等）
// 不会覆盖已设置的环境变量（12-Factor App 原则：环境变量优先级高于文件）
// 如果文件不存在或读取失败，静默忽略
// 加载后检查生产环境关键配置项是否缺失，提前发出警告
func loadEnvFile() {
	if os.Getenv("SKIP_ENV_FILE") != "" {
		return
	}

	envPath := GetEnvPath()

	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return
	}

	if err := godotenv.Load(envPath); err != nil {
		slog.Warn("加载 .env 文件失败", "path", envPath, "error", err)
		return
	}

	if os.Getenv("SERVER_ENV") == "production" {
		criticalKeys := []string{
			"DB_PASSWORD",
			"MFA_RECOVERY_HMAC_KEY",
			"JWT_PRIVATE_KEY_PATH",
			"SMTP_PASSWORD",
		}
		for _, key := range criticalKeys {
			if os.Getenv(key) == "" {
				slog.Warn("生产环境关键配置项为空", "key", key)
			}
		}
		if os.Getenv("DB_SSL_MODE") == "disable" {
			slog.Warn("生产环境数据库SSL未启用", "db_ssl_mode", "disable")
		}
	}
}

// escapeEnvValue 转义.env文件值中的特殊字符
// 防止值中包含换行符、引号等导致注入
func escapeEnvValue(value string) string {
	// 如果值包含特殊字符，使用双引号包裹并转义内部字符
	needsQuoting := false
	for _, c := range value {
		if c == '\n' || c == '\r' || c == '"' || c == '\\' || c == ' ' || c == '#' || c == '$' {
			needsQuoting = true
			break
		}
	}
	if !needsQuoting {
		return value
	}
	// 转义反斜杠、双引号和美元符号
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "$", "\\$")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	return "\"" + escaped + "\""
}

// WriteEnvFile 将配置键值对写入.env文件
func WriteEnvFile(path string, values map[string]string) error {
	var lines []string
	// 按固定顺序写入，方便阅读
	order := []string{
		"SERVER_HOST", "SERVER_PORT", "SERVER_ENV",
		"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD", "DB_SSL_MODE",
		"DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME", "DB_CONN_MAX_IDLE_TIME", "DB_QUERY_TIMEOUT",
		"REDIS_ENABLE", "REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD", "REDIS_DB",
		"REDIS_CONN_TIMEOUT", "REDIS_POOL_SIZE", "REDIS_MIN_IDLE_CONNS",
		"JWT_PRIVATE_KEY_PATH", "JWT_PUBLIC_KEY_PATH", "JWT_ACCESS_TOKEN_TTL", "JWT_REFRESH_TOKEN_TTL", "JWT_ISSUER",
		"KEY_ROTATION_ENABLED", "KEY_ROTATION_INTERVAL", "KEY_TRANSITION_PERIOD",
		"BCRYPT_COST", "RATE_LIMIT_REQUESTS", "RATE_LIMIT_WINDOW",
		"MAX_LOGIN_ATTEMPTS", "LOCKOUT_DURATION", "MFA_RECOVERY_HMAC_KEY",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASSWORD", "SMTP_FROM",
		"CORS_ALLOWED_ORIGINS",
		"METRICS_USERNAME", "METRICS_PASSWORD",
		"SHUTDOWN_TIMEOUT", "LAN_DEPLOYMENT",
	}
	written := make(map[string]bool)
	for _, key := range order {
		if val, ok := values[key]; ok {
			lines = append(lines, key+"="+escapeEnvValue(val))
			written[key] = true
		}
	}
	// 写入剩余的键（不在顺序列表中的）
	for key, val := range values {
		if !written[key] {
			lines = append(lines, key+"="+escapeEnvValue(val))
		}
	}
	content := strings.Join(lines, "\n") + "\n"
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
