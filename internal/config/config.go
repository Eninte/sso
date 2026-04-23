// Package config 配置管理
// 负责从环境变量加载服务配置，提供默认值
// 遵循12-Factor App原则，配置通过环境变量注入
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

	// 安全配置
	BcryptCost         int           // bcrypt成本因子
	RateLimitRequests  int           // 限流请求数
	RateLimitWindow    time.Duration // 限流时间窗口
	MaxLoginAttempts   int           // 最大登录失败次数
	LockoutDuration    time.Duration // 账户锁定时长
	MFARecoveryHMACKey string        // MFA恢复码HMAC密钥（生产环境必须设置）

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
	LANDeployment   bool          // LAN部署模式（放宽部分生产环境校验）
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
		LANDeployment:   getEnvBool("LAN_DEPLOYMENT", false),
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
// validateDatabaseConfig 验证数据库配置
// 此函数从Config.validate中提取，用于降低主函数的复杂度
//
// 职责:
//   - 检查DB_PASSWORD是否设置（非空）
//   - 检查生产环境是否启用数据库SSL（DB_SSL_MODE=require）
//
// 参数:
//   - c: 配置对象
//
// 返回:
//   - 如果配置有效，返回nil
//   - 如果DB_PASSWORD为空，返回ErrDBPasswordRequired
//   - 如果生产环境未启用SSL，返回错误
//
// 重构原因: 从Config.validate中提取数据库验证逻辑，降低主函数复杂度（21→<10）
func validateDatabaseConfig(c *Config) error {
	// 验证数据库密码
	if c.DBPassword == "" {
		slog.Error("数据库密码未设置", "env_var", "DB_PASSWORD")
		return ErrDBPasswordRequired
	}

	// 生产环境建议启用数据库SSL（内网部署时允许disable）
	if c.Env == "production" && c.DBSSLMode == "disable" {
		slog.Warn("生产环境数据库未启用SSL，建议使用require或更高模式")
	}

	return nil
}

// validateJWTConfig 验证JWT配置
// 检查JWT密钥路径和Token TTL值
// validateJWTConfig 验证JWT配置
// 此函数从Config.validate中提取，用于降低主函数的复杂度
//
// 职责:
//   - 检查JWT密钥路径，如果为空则设置默认值
//   - 检查Access Token TTL是否为正数
//   - 检查Refresh Token TTL是否为正数
//   - 验证Token TTL的合理性（警告过短或不合理的值）
//
// 参数:
//   - c: 配置对象（会被修改以设置默认值）
//
// 返回:
//   - 如果配置有效，返回nil
//   - 如果Token TTL无效（非正数），返回错误
//
// 重构原因: 从Config.validate中提取JWT验证逻辑，降低主函数复杂度（21→<10）
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
		return fmt.Errorf("access token TTL 必须为正数")
	}
	if c.RefreshTokenTTL <= 0 {
		slog.Error("Refresh Token TTL 必须为正数", "ttl", c.RefreshTokenTTL)
		return fmt.Errorf("refresh token TTL 必须为正数")
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

// validateSecurityConfig 验证安全配置
// 检查bcrypt cost和其他安全参数
// validateSecurityConfig 验证安全配置
// 此函数从Config.validate中提取，用于降低主函数的复杂度
//
// 职责:
//   - 检查bcrypt cost是否在推荐范围内（4-31）
//   - 检查生产环境bcrypt cost是否至少为12
//   - 检查限流配置是否有效
//   - 检查登录保护配置是否有效
//
// 参数:
//   - c: 配置对象
//
// 返回:
//   - 如果配置有效，返回nil
//   - 如果生产环境bcrypt cost过低，返回ErrBcryptCostTooLow
//
// 重构原因: 从Config.validate中提取安全验证逻辑，降低主函数复杂度（21→<10）
func validateSecurityConfig(c *Config) error {
	// 验证bcrypt cost范围
	if c.BcryptCost < 4 || c.BcryptCost > 31 {
		slog.Warn("bcrypt cost 超出推荐范围 (4-31)", "cost", c.BcryptCost)
	}

	// 生产环境必须使用足够强的bcrypt cost
	if c.Env == "production" && c.BcryptCost < 12 {
		slog.Error("生产环境bcrypt cost应至少为12", "current", c.BcryptCost)
		return ErrBcryptCostTooLow
	}

	// 验证限流配置
	if c.RateLimitRequests <= 0 {
		slog.Warn("限流请求数应为正数", "requests", c.RateLimitRequests)
	}

	// 验证登录保护配置
	if c.MaxLoginAttempts <= 0 {
		slog.Warn("最大登录尝试次数应为正数", "attempts", c.MaxLoginAttempts)
	}
	if c.LockoutDuration <= 0 {
		slog.Warn("账户锁定时长应为正数", "duration", c.LockoutDuration)
	}

	return nil
}

// validateProductionConfig 验证生产环境配置
// 检查生产环境特定的安全要求
// validateProductionConfig 验证生产环境配置
// 此函数从Config.validate中提取，用于降低主函数的复杂度
//
// 职责:
//   - 检查CORS配置不包含localhost
//   - 检查是否使用默认CORS配置
//   - 检查JWT Issuer是否自定义
//   - 检查SMTP配置
//   - 检查Metrics认证配置
//
// 参数:
//   - c: 配置对象
//
// 返回:
//   - 如果不是生产环境，返回nil（跳过验证）
//   - 如果配置有效，返回nil
//   - 如果CORS包含localhost或使用默认值，返回错误
//   - 如果Metrics配置不完整，返回错误
//
// 重构原因: 从Config.validate中提取生产环境验证逻辑，降低主函数复杂度（21→<10）
func validateProductionConfig(c *Config) error {
	// 仅在生产环境执行验证
	if c.Env != "production" {
		return nil
	}

	lanMode := c.LANDeployment

	// 检查CORS配置不包含localhost
	if strings.Contains(strings.ToLower(c.CORSAllowedOrigins), "localhost") {
		if lanMode {
			slog.Warn("生产环境CORS配置包含localhost（LAN部署模式）", "cors_origins", c.CORSAllowedOrigins)
		} else {
			slog.Error("生产环境CORS配置不能包含localhost", "cors_origins", c.CORSAllowedOrigins)
			return fmt.Errorf("生产环境CORS_ALLOWED_ORIGINS不能包含localhost")
		}
	}

	// 检查默认CORS配置
	if c.CORSAllowedOrigins == "http://localhost:3000" {
		if lanMode {
			slog.Warn("生产环境使用默认CORS配置（LAN部署模式）")
		} else {
			slog.Error("生产环境不能使用默认CORS配置")
			return fmt.Errorf("生产环境必须设置 CORS_ALLOWED_ORIGINS")
		}
	}

	// 检查JWT Issuer配置
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

	return nil
}

// validate 验证配置的有效性
// 此函数已重构以降低复杂度，通过提取数据库、JWT、安全、生产环境验证逻辑
//
// 职责:
//   - 调用validateDatabaseConfig验证数据库配置
//   - 调用validateJWTConfig验证JWT配置
//   - 调用validateSecurityConfig验证安全配置
//   - 调用validateProductionConfig验证生产环境配置
//   - 验证环境设置和端口范围
//
// 返回:
//   - 如果所有配置都有效，返回nil
//   - 如果任何配置无效，返回第一个错误
//
// 重构原因: 原始复杂度为21，通过提取数据库、JWT、安全、生产环境验证逻辑，降低到<10
// 提取的函数:
//   - validateDatabaseConfig: 验证数据库配置
//   - validateJWTConfig: 验证JWT配置
//   - validateSecurityConfig: 验证安全配置
//   - validateProductionConfig: 验证生产环境配置
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

	// 验证环境设置
	if c.Env != "development" && c.Env != "production" {
		slog.Warn("未知的运行环境，应为 development 或 production", "env", c.Env)
	}

	// 验证端口范围
	if port, err := strconv.Atoi(c.ServerPort); err != nil || port < 1 || port > 65535 {
		slog.Warn("服务器端口无效", "port", c.ServerPort)
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
