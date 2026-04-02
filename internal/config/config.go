// Package config 配置管理
// 负责从环境变量加载服务配置，提供默认值
// 遵循12-Factor App原则，配置通过环境变量注入
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/your-org/sso/internal/errors"
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
	ServerHost string // 服务器监听地址
	ServerPort string // 服务器监听端口
	Env        string // 运行环境 (development/production)

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

	// 安全配置
	BcryptCost        int           // bcrypt成本因子
	RateLimitRequests int           // 限流请求数
	RateLimitWindow   time.Duration // 限流时间窗口
	MaxLoginAttempts  int           // 最大登录失败次数
	LockoutDuration   time.Duration // 账户锁定时长

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

	// CORS配置
	CORSAllowedOrigins string // 允许的跨域源 (逗号分隔)

	// Metrics配置
	MetricsUsername string // Metrics Basic Auth用户名
	MetricsPassword string // Metrics Basic Auth密码

	// 优雅关闭配置
	ShutdownTimeout time.Duration // 优雅关闭超时时间
}

// Load 从环境变量加载配置
// 如果环境变量不存在，使用预设的默认值
// 注意：敏感配置（如密码）必须通过环境变量设置
func Load() (*Config, error) {
	cfg := &Config{
		// 服务器配置
		ServerHost: getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort: getEnv("SERVER_PORT", "9090"),
		Env:        getEnv("SERVER_ENV", "development"),

		// 数据库配置
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBName:     getEnv("DB_NAME", "sso"),
		DBUser:     getEnv("DB_USER", "sso"),
		DBPassword: os.Getenv("DB_PASSWORD"), // 必须通过环境变量设置
		DBSSLMode:  getEnv("DB_SSL_MODE", "prefer"),

		// 数据库连接池配置
		DBMaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 50),
		DBMaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 25),
		DBConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
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

		// 安全配置
		// BcryptCost: bcrypt成本因子，影响密码哈希性能
		// 推荐值: 12-14，值越高越安全但性能越低
		// cost=12: ~200ms, cost=13: ~400ms, cost=14: ~800ms
		// 生产环境必须 >= 12
		BcryptCost:        getEnvInt("BCRYPT_COST", 12),
		RateLimitRequests: getEnvInt("RATE_LIMIT_REQUESTS", 100),
		RateLimitWindow:   getEnvDuration("RATE_LIMIT_WINDOW", 1*time.Minute),
		MaxLoginAttempts:  getEnvInt("MAX_LOGIN_ATTEMPTS", 5),
		LockoutDuration:   getEnvDuration("LOCKOUT_DURATION", 30*time.Minute),

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

		// CORS配置
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"),

		// Metrics配置
		MetricsUsername: os.Getenv("METRICS_USERNAME"),
		MetricsPassword: os.Getenv("METRICS_PASSWORD"),

		// 优雅关闭配置
		ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
	}

	// 验证必需的配置
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate 验证配置的有效性
// validateDatabaseConfig 验证数据库配置
// 检查数据库密码是否设置，以及生产环境是否启用SSL
func validateDatabaseConfig(c *Config) error {
	// 验证数据库密码
	if c.DBPassword == "" {
		slog.Error("数据库密码未设置", "env_var", "DB_PASSWORD")
		return ErrDBPasswordRequired
	}

	// 生产环境必须启用数据库SSL
	if c.Env == "production" && c.DBSSLMode == "disable" {
		slog.Error("生产环境数据库必须启用SSL")
		return fmt.Errorf("生产环境必须设置 DB_SSL_MODE=require 或更高")
	}

	return nil
}

func (c *Config) validate() error {
	// 验证数据库配置
	if err := validateDatabaseConfig(c); err != nil {
		return err
	}

	// 验证JWT密钥路径
	if c.JWTPrivateKeyPath == "" {
		c.JWTPrivateKeyPath = "./keys/private.pem"
		slog.Warn("JWT私钥路径未设置，使用默认值", "path", c.JWTPrivateKeyPath)
	}
	if c.JWTPublicKeyPath == "" {
		c.JWTPublicKeyPath = "./keys/public.pem"
		slog.Warn("JWT公钥路径未设置，使用默认值", "path", c.JWTPublicKeyPath)
	}

	// 验证环境设置
	if c.Env != "development" && c.Env != "production" {
		slog.Warn("未知的运行环境，应为 development 或 production", "env", c.Env)
	}

	// 验证端口范围
	if port, err := strconv.Atoi(c.ServerPort); err != nil || port < 1 || port > 65535 {
		slog.Warn("服务器端口无效", "port", c.ServerPort)
	}

	// 验证bcrypt cost
	if c.BcryptCost < 4 || c.BcryptCost > 31 {
		slog.Warn("bcrypt cost 超出推荐范围 (4-31)", "cost", c.BcryptCost)
	}

	// 验证Token TTL
	if c.AccessTokenTTL < 1*time.Minute {
		slog.Warn("Access Token TTL 过短，建议至少1分钟", "ttl", c.AccessTokenTTL)
	}
	if c.RefreshTokenTTL < c.AccessTokenTTL {
		slog.Warn("Refresh Token TTL 应大于 Access Token TTL",
			"access_ttl", c.AccessTokenTTL,
			"refresh_ttl", c.RefreshTokenTTL)
	}

	// 生产环境额外验证
	if c.Env == "production" {
		// 检查默认值
		if c.CORSAllowedOrigins == "http://localhost:3000" {
			slog.Error("生产环境不能使用默认CORS配置")
			return fmt.Errorf("生产环境必须设置 CORS_ALLOWED_ORIGINS")
		}
		if c.BcryptCost < 12 {
			slog.Error("生产环境bcrypt cost应至少为12", "current", c.BcryptCost)
			return ErrBcryptCostTooLow
		}
		if c.JWTIssuer == "sso" {
			slog.Warn("生产环境使用默认JWT Issuer，建议自定义")
		}
		// 检查SMTP配置
		if c.SMTPHost == "localhost" {
			slog.Warn("生产环境使用localhost作为SMTP服务器")
		}
		// 检查Metrics认证配置
		if c.MetricsUsername != "" && c.MetricsPassword == "" {
			slog.Error("生产环境配置了METRICS_USERNAME但未设置METRICS_PASSWORD")
			return fmt.Errorf("生产环境配置了METRICS_USERNAME时必须设置METRICS_PASSWORD")
		}
	}

	return nil
}

// DatabaseURL 构建PostgreSQL数据库连接URL
func (c *Config) DatabaseURL() string {
	return "postgres://" + c.DBUser + ":" + c.DBPassword +
		"@" + c.DBHost + ":" + c.DBPort + "/" + c.DBName +
		"?sslmode=" + c.DBSSLMode
}

// RedisURL 构建Redis连接URL
func (c *Config) RedisURL() string {
	if c.RedisPassword != "" {
		return "redis://:" + c.RedisPassword + "@" + c.RedisHost + ":" + c.RedisPort
	}
	return "redis://" + c.RedisHost + ":" + c.RedisPort
}

// BaseURL 构建服务基础URL
func (c *Config) BaseURL() string {
	return "http://" + c.ServerHost + ":" + c.ServerPort
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

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return strings.ToLower(value) == "true" || value == "1"
	}
	return defaultValue
}

// GetCORSAllowedOrigins 获取CORS允许的源列表
func (c *Config) GetCORSAllowedOrigins() []string {
	return splitAndTrim(c.CORSAllowedOrigins)
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
