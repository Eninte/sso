// Package config 配置管理
// 负责从环境变量加载服务配置，提供默认值
// 遵循12-Factor App原则，配置通过环境变量注入
package config

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// 配置错误定义
var (
	ErrDBPasswordRequired = errors.New("DB_PASSWORD环境变量必须设置")
	ErrJWTKeyRequired     = errors.New("JWT密钥路径必须设置")
	ErrBcryptCostTooLow   = errors.New("生产环境bcrypt cost必须 >= 12")
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
	RedisHost     string // Redis主机
	RedisPort     string // Redis端口
	RedisPassword string // Redis密码

	// JWT配置
	JWTPrivateKeyPath string        // JWT私钥路径
	JWTPublicKeyPath  string        // JWT公钥路径
	AccessTokenTTL    time.Duration // Access Token有效期
	RefreshTokenTTL   time.Duration // Refresh Token有效期
	JWTIssuer         string        // Token签发者标识

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

	// 管理员配置
	AdminEmails  string // 管理员邮箱白名单 (逗号分隔)
	AdminDomains string // 管理员域名白名单 (逗号分隔)
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
		DBSSLMode:  getEnv("DB_SSL_MODE", "disable"),

		// 数据库连接池配置
		DBMaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 50),
		DBMaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 25),
		DBConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		DBQueryTimeout:    getEnvDuration("DB_QUERY_TIMEOUT", 10*time.Second),

		// Redis配置
		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"), // 可选，无默认值

		// JWT配置
		JWTPrivateKeyPath: os.Getenv("JWT_PRIVATE_KEY_PATH"), // 必须通过环境变量设置
		JWTPublicKeyPath:  os.Getenv("JWT_PUBLIC_KEY_PATH"),  // 必须通过环境变量设置
		AccessTokenTTL:    getEnvDuration("JWT_ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:   getEnvDuration("JWT_REFRESH_TOKEN_TTL", 168*time.Hour),
		JWTIssuer:         getEnv("JWT_ISSUER", "sso"),

		// 安全配置
		// BcryptCost: bcrypt成本因子，影响密码哈希性能
		// 推荐值: 12-14，值越高越安全但性能越低
		// cost=10: ~50ms, cost=11: ~100ms, cost=12: ~200ms
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

		// 管理员配置
		AdminEmails:  getEnv("ADMIN_EMAILS", "admin@example.com"),
		AdminDomains: getEnv("ADMIN_DOMAINS", "admin.com"),
	}

	// 验证必需的配置
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate 验证配置的有效性
func (c *Config) validate() error {
	// 验证数据库密码
	if c.DBPassword == "" {
		slog.Error("数据库密码未设置", "env_var", "DB_PASSWORD")
		return ErrDBPasswordRequired
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

	// 生产环境额外验证
	if c.Env == "production" {
		if c.CORSAllowedOrigins == "http://localhost:3000" {
			slog.Warn("生产环境使用默认CORS配置，建议修改")
		}
		if c.AdminEmails == "admin@example.com" {
			slog.Warn("生产环境使用默认管理员邮箱，建议修改")
		}
		if c.BcryptCost < 12 {
			slog.Warn("生产环境bcrypt cost应至少为12", "current", c.BcryptCost)
			return ErrBcryptCostTooLow
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

// GetAdminEmails 获取管理员邮箱列表
func (c *Config) GetAdminEmails() []string {
	return splitAndTrim(c.AdminEmails)
}

// GetAdminDomains 获取管理员域名列表
func (c *Config) GetAdminDomains() []string {
	return splitAndTrim(c.AdminDomains)
}

// GetCORSAllowedOrigins 获取CORS允许的源列表
func (c *Config) GetCORSAllowedOrigins() []string {
	return splitAndTrim(c.CORSAllowedOrigins)
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
