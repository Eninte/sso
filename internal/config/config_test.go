// Package config_test 配置测试
// 测试配置加载和验证逻辑
package config_test

import (
	"testing"
	"time"

	"github.com/example/sso/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// setupTestEnv 设置测试环境变量
//
// 测试隔离的两条核心原则：
//  1. 禁止读取磁盘 .env 文件——否则测试结果依赖运行环境，换台机器或改了 .env 就会随机失败。
//     通过设置 SKIP_ENV_FILE=1 让 loadEnvFile() 直接返回。
//  2. 隔离影响生产环境校验的关键开关——特别是 LAN_DEPLOYMENT，若残留为 true 会旁路
//     生产环境的 CORS/SSL 等安全校验，使"期望报错"的测试用例静默通过。
//
// CI/CD 修复：所有环境变量设置改用 t.Setenv，自动在测试结束时恢复原值（包括 unset 状态），
// 避免环境变量泄漏到后续测试引发 race condition 或非确定性失败。
// 对于需要测试"变量未设置"的场景，使用 t.Setenv(key, "") —— 因为 config 包的所有
// getEnvXXX 函数都检查 `value != ""`，空字符串与 unset 行为一致。
func setupTestEnv(t *testing.T) {
	t.Helper()

	// 设置测试环境变量（t.Setenv 会自动恢复原值，包括 unset 状态）
	t.Setenv("DB_PASSWORD", "test_password")
	t.Setenv("JWT_PRIVATE_KEY_PATH", "/keys/private.pem")
	t.Setenv("JWT_PUBLIC_KEY_PATH", "/keys/public.pem")
	t.Setenv("JWT_ISSUER", "test-issuer")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	// 设置 MFA 恢复码 HMAC 密钥（生产环境必需，AGENTS.md 硬约束）
	t.Setenv("MFA_RECOVERY_HMAC_KEY", "test-hmac-key-for-mfa-recovery-codes")
	// 阶段 4 安全增强：生产环境校验要求以下变量必填，统一设置默认值
	// 测试用例可通过 t.Setenv(key, "") 单独清除来测试对应校验分支
	t.Setenv("PUBLIC_BASE_URL", "https://example.com")
	t.Setenv("METRICS_USERNAME", "metrics-admin")
	t.Setenv("METRICS_PASSWORD", "metrics-secret")
	// REDIS_ENABLE 默认为 true，生产环境要求 REDIS_PASSWORD 必须非空
	t.Setenv("REDIS_PASSWORD", "test_redis_password")
	// 测试环境跳过 JWT 私钥文件权限校验（CI 中文件权限不一定是 600）
	t.Setenv("STRICT_KEY_PERMISSIONS", "false")
	// 禁止读取磁盘 .env 文件，保证测试环境完全自包含
	t.Setenv("SKIP_ENV_FILE", "1")
	// 默认非 LAN 部署模式，确保生产环境安全校验不被旁路；
	// 需要测试 LAN 模式的用例可单独设置 LAN_DEPLOYMENT=true
	// 空字符串等同于 unset（getEnvBool 检查 value != ""）
	t.Setenv("LAN_DEPLOYMENT", "")
}

// ============================================================================
// 配置加载测试
// ============================================================================

func TestLoad_ValidConfig(t *testing.T) {
	setupTestEnv(t)

	cfg, err := config.Load()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "test_password", cfg.DBPassword)
}

func TestLoad_MissingDBPassword(t *testing.T) {
	// CI/CD 修复：使用 t.Setenv，空字符串等同于 unset（getEnv 检查 value != ""）
	t.Setenv("DB_PASSWORD", "")
	// 设置其他必需的环境变量
	t.Setenv("JWT_PRIVATE_KEY_PATH", "/keys/private.pem")
	t.Setenv("JWT_PUBLIC_KEY_PATH", "/keys/public.pem")
	t.Setenv("SKIP_ENV_FILE", "1")

	cfg, err := config.Load()
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoad_DefaultJWTKeyPath(t *testing.T) {
	setupTestEnv(t)

	// 清除JWT密钥路径，测试默认值（空字符串等同于 unset）
	t.Setenv("JWT_PRIVATE_KEY_PATH", "")
	t.Setenv("JWT_PUBLIC_KEY_PATH", "")

	cfg, err := config.Load()
	assert.NoError(t, err)
	assert.Equal(t, "./keys/private.pem", cfg.JWTPrivateKeyPath)
	assert.Equal(t, "./keys/public.pem", cfg.JWTPublicKeyPath)
}

// ============================================================================
// 配置验证测试
// ============================================================================

func TestValidate_BcryptCostRange(t *testing.T) {
	setupTestEnv(t)

	tests := []struct {
		name    string
		cost    string
		wantErr bool
		env     string
	}{
		{
			name:    "有效cost-开发环境",
			cost:    "10",
			wantErr: false,
			env:     "development",
		},
		{
			name:    "有效cost-生产环境",
			cost:    "12",
			wantErr: false,
			env:     "production",
		},
		{
			name:    "cost过低-生产环境",
			cost:    "9",
			wantErr: true,
			env:     "production",
		},
		{
			name:    "cost=11不满足生产要求",
			cost:    "11",
			wantErr: true,
			env:     "production",
		},
		{
			name:    "cost=10不满足生产要求",
			cost:    "10",
			wantErr: true,
			env:     "production",
		},
		{
			name:    "cost超出范围",
			cost:    "50",
			wantErr: true, // 阶段 4 安全增强：bcrypt 算法上限为 31，超出拒绝启动（避免运行时 panic）
			env:     "development",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BCRYPT_COST", tt.cost)
			t.Setenv("SERVER_ENV", tt.env)
			t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")
			t.Setenv("ADMIN_EMAILS", "admin@company.com")
			// 生产环境需要设置DB_SSL_MODE
			if tt.env == "production" {
				t.Setenv("DB_SSL_MODE", "require")
				t.Setenv("JWT_ISSUER", "myapp")
				t.Setenv("SMTP_HOST", "smtp.example.com")
			}

			cfg, err := config.Load()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestValidate_ProductionDefaults(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("SERVER_ENV", "production")
	t.Setenv("BCRYPT_COST", "12")
	t.Setenv("DB_SSL_MODE", "require")

	tests := []struct {
		name        string
		corsOrigins string
		wantErr     bool
	}{
		{
			name:        "默认CORS配置",
			corsOrigins: "http://localhost:3000",
			wantErr:     true,
		},
		{
			name:        "有效生产配置",
			corsOrigins: "https://example.com",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CORS_ALLOWED_ORIGINS", tt.corsOrigins)
			t.Setenv("JWT_ISSUER", "myapp")
			t.Setenv("SMTP_HOST", "smtp.example.com")

			cfg, err := config.Load()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestValidateProductionConfig_CORSLocalhostCheck(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("SERVER_ENV", "production")
	t.Setenv("BCRYPT_COST", "12")
	t.Setenv("DB_SSL_MODE", "require")

	tests := []struct {
		name        string
		corsOrigins string
		wantErr     bool
		errContains string
	}{
		{
			name:        "CORS包含localhost-小写",
			corsOrigins: "http://localhost:3000",
			wantErr:     true,
			errContains: "localhost",
		},
		{
			name:        "CORS包含localhost-大写",
			corsOrigins: "http://LOCALHOST:3000",
			wantErr:     true,
			errContains: "localhost",
		},
		{
			name:        "CORS包含localhost-混合大小写",
			corsOrigins: "http://LocalHost:3000",
			wantErr:     true,
			errContains: "localhost",
		},
		{
			name:        "CORS包含localhost在多个源中",
			corsOrigins: "https://example.com,http://localhost:3000,https://app.example.com",
			wantErr:     true,
			errContains: "localhost",
		},
		{
			name:        "CORS不包含localhost",
			corsOrigins: "https://example.com",
			wantErr:     false,
		},
		{
			name:        "CORS多个有效源",
			corsOrigins: "https://example.com,https://app.example.com,https://api.example.com",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CORS_ALLOWED_ORIGINS", tt.corsOrigins)

			cfg, err := config.Load()
			if tt.wantErr {
				// 先断言 err 非 nil，避免后续 err.Error() 触发 nil 指针 panic
				if err == nil {
					t.Errorf("期望返回错误但得到 nil")
					return
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestValidateProductionConfig_MetricsAuth(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("SERVER_ENV", "production")
	t.Setenv("BCRYPT_COST", "12")
	t.Setenv("DB_SSL_MODE", "require")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")

	tests := []struct {
		name            string
		metricsUsername string
		metricsPassword string
		wantErr         bool
		errContains     string
	}{
		{
			name:            "Metrics用户名设置但密码未设置",
			metricsUsername: "admin",
			metricsPassword: "",
			wantErr:         true,
			errContains:     "METRICS_PASSWORD",
		},
		{
			name:            "Metrics用户名和密码都设置",
			metricsUsername: "admin",
			metricsPassword: "secret",
			wantErr:         false,
		},
		{
			name:            "Metrics都未设置",
			metricsUsername: "",
			metricsPassword: "",
			wantErr:         true, // 阶段 4 安全增强：生产环境 METRICS_USERNAME + METRICS_PASSWORD 必须同时设置，避免 /metrics 无认证暴露指标
			errContains:     "METRICS_USERNAME",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("METRICS_USERNAME", tt.metricsUsername)
			t.Setenv("METRICS_PASSWORD", tt.metricsPassword)

			cfg, err := config.Load()
			if tt.wantErr {
				// 先断言 err 非 nil，避免后续 err.Error() 触发 nil 指针 panic
				if err == nil {
					t.Errorf("期望返回错误但得到 nil")
					return
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestValidateProductionConfig_DevelopmentEnvironment(t *testing.T) {
	setupTestEnv(t)

	// 在开发环境下，即使CORS包含localhost也应该通过
	t.Setenv("SERVER_ENV", "development")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")

	cfg, err := config.Load()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "http://localhost:3000", cfg.CORSAllowedOrigins)
}

func TestValidate_ProductionDBSSL(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("SERVER_ENV", "production")
	t.Setenv("BCRYPT_COST", "12")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")

	tests := []struct {
		name    string
		sslMode string
		wantErr bool
	}{
		{
			name:    "生产环境禁用SSL（应报错）",
			sslMode: "disable",
			wantErr: true,
		},
		{
			name:    "生产环境SSL require",
			sslMode: "require",
			wantErr: false,
		},
		{
			name:    "生产环境SSL verify-full",
			sslMode: "verify-full",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DB_SSL_MODE", tt.sslMode)

			cfg, err := config.Load()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestValidate_TokenTTL(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("SERVER_ENV", "development")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")

	tests := []struct {
		name       string
		accessTTL  string
		refreshTTL string
		wantErr    bool
	}{
		{
			name:       "有效TTL",
			accessTTL:  "15m",
			refreshTTL: "168h",
			wantErr:    false,
		},
		{
			name:       "访问令牌TTL过短",
			accessTTL:  "30s",
			refreshTTL: "168h",
			wantErr:    false, // 只有警告
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_ACCESS_TOKEN_TTL", tt.accessTTL)
			t.Setenv("JWT_REFRESH_TOKEN_TTL", tt.refreshTTL)

			cfg, err := config.Load()
			assert.NoError(t, err)
			assert.NotNil(t, cfg)
		})
	}
}

// ============================================================================
// JWT配置验证测试
// ============================================================================

func TestValidateJWTConfig_PositiveTTL(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("SERVER_ENV", "development")
	t.Setenv("JWT_ACCESS_TOKEN_TTL", "15m")
	t.Setenv("JWT_REFRESH_TOKEN_TTL", "168h")

	cfg, err := config.Load()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, 15*time.Minute, cfg.AccessTokenTTL)
	assert.Equal(t, 168*time.Hour, cfg.RefreshTokenTTL)
}

func TestValidateJWTConfig_NegativeTTL(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("SERVER_ENV", "development")
	t.Setenv("JWT_ACCESS_TOKEN_TTL", "-15m")
	t.Setenv("JWT_REFRESH_TOKEN_TTL", "168h")

	cfg, err := config.Load()
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "access token TTL must be positive")
}

func TestValidateJWTConfig_ZeroAccessTTL(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("SERVER_ENV", "development")
	t.Setenv("JWT_ACCESS_TOKEN_TTL", "0s")
	t.Setenv("JWT_REFRESH_TOKEN_TTL", "168h")

	cfg, err := config.Load()
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "access token TTL must be positive")
}

func TestValidateJWTConfig_ZeroRefreshTTL(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("SERVER_ENV", "development")
	t.Setenv("JWT_ACCESS_TOKEN_TTL", "15m")
	t.Setenv("JWT_REFRESH_TOKEN_TTL", "0s")

	cfg, err := config.Load()
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "refresh token TTL must be positive")
}

func TestValidateJWTConfig_DefaultPaths(t *testing.T) {
	setupTestEnv(t)

	// 清除JWT密钥路径环境变量，测试默认值设置（空字符串等同于 unset）
	t.Setenv("JWT_PRIVATE_KEY_PATH", "")
	t.Setenv("JWT_PUBLIC_KEY_PATH", "")
	t.Setenv("SERVER_ENV", "development")
	// 设置有效的TTL值
	t.Setenv("JWT_ACCESS_TOKEN_TTL", "15m")
	t.Setenv("JWT_REFRESH_TOKEN_TTL", "168h")

	cfg, err := config.Load()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "./keys/private.pem", cfg.JWTPrivateKeyPath)
	assert.Equal(t, "./keys/public.pem", cfg.JWTPublicKeyPath)
}

func TestValidateJWTConfig_RefreshTTLLessThanAccess(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("SERVER_ENV", "development")
	t.Setenv("JWT_ACCESS_TOKEN_TTL", "168h")
	t.Setenv("JWT_REFRESH_TOKEN_TTL", "15m")

	// 这应该只产生警告，不应该报错
	cfg, err := config.Load()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, 168*time.Hour, cfg.AccessTokenTTL)
	assert.Equal(t, 15*time.Minute, cfg.RefreshTokenTTL)
}

// ============================================================================
// URL生成测试
// ============================================================================

func TestDatabaseURL(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "sso")
	t.Setenv("DB_USER", "sso")
	t.Setenv("DB_SSL_MODE", "disable")

	cfg, err := config.Load()
	assert.NoError(t, err)

	url := cfg.DatabaseURL()
	assert.Contains(t, url, "postgres://sso:test_password@localhost:5432/sso")
	assert.Contains(t, url, "sslmode=disable")
}

func TestRedisURL_WithPassword(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("REDIS_HOST", "localhost")
	t.Setenv("REDIS_PORT", "6379")
	t.Setenv("REDIS_PASSWORD", "redis_pass")

	cfg, err := config.Load()
	assert.NoError(t, err)

	url := cfg.RedisURL()
	assert.Contains(t, url, ":redis_pass@")
	assert.Contains(t, url, "localhost:6379")
}

func TestRedisURL_WithoutPassword(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("REDIS_HOST", "localhost")
	t.Setenv("REDIS_PORT", "6379")
	// 空字符串等同于 unset（RedisURL 内部检查 password 是否为空）
	t.Setenv("REDIS_PASSWORD", "")

	cfg, err := config.Load()
	assert.NoError(t, err)

	url := cfg.RedisURL()
	assert.Equal(t, "redis://localhost:6379", url)
}

func TestBaseURL(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("SERVER_HOST", "0.0.0.0")
	t.Setenv("SERVER_PORT", "9090")

	cfg, err := config.Load()
	assert.NoError(t, err)

	url := cfg.BaseURL()
	assert.Equal(t, "http://0.0.0.0:9090", url)
}

func TestIsDevelopment(t *testing.T) {
	setupTestEnv(t)

	tests := []struct {
		name     string
		env      string
		expected bool
	}{
		{
			name:     "开发环境",
			env:      "development",
			expected: true,
		},
		{
			name:     "生产环境",
			env:      "production",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SERVER_ENV", tt.env)
			// 为生产环境设置非默认值
			t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")
			t.Setenv("ADMIN_EMAILS", "admin@company.com")
			if tt.env == "production" {
				t.Setenv("DB_SSL_MODE", "require")
				t.Setenv("BCRYPT_COST", "12")
			}

			cfg, err := config.Load()
			require.NoError(t, err)
			require.NotNil(t, cfg)
			assert.Equal(t, tt.expected, cfg.IsDevelopment())
		})
	}
}

// ============================================================================
// 辅助方法测试
// ============================================================================

func TestGetCORSAllowedOrigins(t *testing.T) {
	setupTestEnv(t)

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "单个源",
			input:    "https://example.com",
			expected: []string{"https://example.com"},
		},
		{
			name:     "多个源",
			input:    "https://example.com, https://app.example.com",
			expected: []string{"https://example.com", "https://app.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CORS_ALLOWED_ORIGINS", tt.input)

			cfg, err := config.Load()
			assert.NoError(t, err)

			origins := cfg.GetCORSAllowedOrigins()
			assert.Equal(t, tt.expected, origins)
		})
	}
}

// ============================================================================
// 连接池配置测试
// ============================================================================

func TestConnectionPoolConfig(t *testing.T) {
	setupTestEnv(t)

	t.Setenv("DB_MAX_OPEN_CONNS", "100")
	t.Setenv("DB_MAX_IDLE_CONNS", "50")
	t.Setenv("DB_CONN_MAX_LIFETIME", "10m")
	t.Setenv("DB_QUERY_TIMEOUT", "30s")

	cfg, err := config.Load()
	assert.NoError(t, err)
	assert.Equal(t, 100, cfg.DBMaxOpenConns)
	assert.Equal(t, 50, cfg.DBMaxIdleConns)
	assert.Equal(t, 10*time.Minute, cfg.DBConnMaxLifetime)
	assert.Equal(t, 30*time.Second, cfg.DBQueryTimeout)
}
