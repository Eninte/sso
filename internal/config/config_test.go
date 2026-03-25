// Package config_test 配置测试
// 测试配置加载和验证逻辑
package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/your-org/sso/internal/config"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// setupTestEnv 设置测试环境变量
func setupTestEnv(t *testing.T) func() {
	// 保存原始环境变量
	origEnv := map[string]string{
		"DB_PASSWORD":          os.Getenv("DB_PASSWORD"),
		"JWT_PRIVATE_KEY_PATH": os.Getenv("JWT_PRIVATE_KEY_PATH"),
		"JWT_PUBLIC_KEY_PATH":  os.Getenv("JWT_PUBLIC_KEY_PATH"),
		"SERVER_ENV":           os.Getenv("SERVER_ENV"),
		"CORS_ALLOWED_ORIGINS": os.Getenv("CORS_ALLOWED_ORIGINS"),
		"ADMIN_EMAILS":         os.Getenv("ADMIN_EMAILS"),
		"BCRYPT_COST":          os.Getenv("BCRYPT_COST"),
	}

	// 设置测试环境变量
	os.Setenv("DB_PASSWORD", "test_password")
	os.Setenv("JWT_PRIVATE_KEY_PATH", "/keys/private.pem")
	os.Setenv("JWT_PUBLIC_KEY_PATH", "/keys/public.pem")

	// 返回清理函数
	return func() {
		for key, value := range origEnv {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}
}

// ============================================================================
// 配置加载测试
// ============================================================================

func TestLoad_ValidConfig(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.Load()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "test_password", cfg.DBPassword)
}

func TestLoad_MissingDBPassword(t *testing.T) {
	// 保存并清除DB_PASSWORD
	origPassword := os.Getenv("DB_PASSWORD")
	defer os.Setenv("DB_PASSWORD", origPassword)
	os.Unsetenv("DB_PASSWORD")

	// 设置其他必需的环境变量
	os.Setenv("JWT_PRIVATE_KEY_PATH", "/keys/private.pem")
	os.Setenv("JWT_PUBLIC_KEY_PATH", "/keys/public.pem")

	cfg, err := config.Load()
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoad_DefaultJWTKeyPath(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	// 清除JWT密钥路径，测试默认值
	os.Unsetenv("JWT_PRIVATE_KEY_PATH")
	os.Unsetenv("JWT_PUBLIC_KEY_PATH")

	cfg, err := config.Load()
	assert.NoError(t, err)
	assert.Equal(t, "./keys/private.pem", cfg.JWTPrivateKeyPath)
	assert.Equal(t, "./keys/public.pem", cfg.JWTPublicKeyPath)
}

// ============================================================================
// 配置验证测试
// ============================================================================

func TestValidate_BcryptCostRange(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

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
			cost:    "10",
			wantErr: true,
			env:     "production",
		},
		{
			name:    "cost超出范围",
			cost:    "50",
			wantErr: false, // 只有警告，不报错
			env:     "development",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("BCRYPT_COST", tt.cost)
			os.Setenv("SERVER_ENV", tt.env)
			os.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")
			os.Setenv("ADMIN_EMAILS", "admin@company.com")

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
	cleanup := setupTestEnv(t)
	defer cleanup()

	os.Setenv("SERVER_ENV", "production")
	os.Setenv("BCRYPT_COST", "12")

	tests := []struct {
		name        string
		corsOrigins string
		adminEmails string
		wantErr     bool
	}{
		{
			name:        "默认CORS配置",
			corsOrigins: "http://localhost:3000",
			adminEmails: "admin@example.com",
			wantErr:     true,
		},
		{
			name:        "默认管理员邮箱",
			corsOrigins: "https://example.com",
			adminEmails: "admin@example.com",
			wantErr:     true,
		},
		{
			name:        "有效生产配置",
			corsOrigins: "https://example.com",
			adminEmails: "real-admin@company.com",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("CORS_ALLOWED_ORIGINS", tt.corsOrigins)
			os.Setenv("ADMIN_EMAILS", tt.adminEmails)

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
	cleanup := setupTestEnv(t)
	defer cleanup()

	os.Setenv("SERVER_ENV", "development")
	os.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	os.Setenv("ADMIN_EMAILS", "admin@example.com")

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
			os.Setenv("JWT_ACCESS_TOKEN_TTL", tt.accessTTL)
			os.Setenv("JWT_REFRESH_TOKEN_TTL", tt.refreshTTL)

			cfg, err := config.Load()
			assert.NoError(t, err)
			assert.NotNil(t, cfg)
		})
	}
}

// ============================================================================
// URL生成测试
// ============================================================================

func TestDatabaseURL(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("DB_NAME", "sso")
	os.Setenv("DB_USER", "sso")
	os.Setenv("DB_SSL_MODE", "disable")

	cfg, err := config.Load()
	assert.NoError(t, err)

	url := cfg.DatabaseURL()
	assert.Contains(t, url, "postgres://sso:test_password@localhost:5432/sso")
	assert.Contains(t, url, "sslmode=disable")
}

func TestRedisURL_WithPassword(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	os.Setenv("REDIS_HOST", "localhost")
	os.Setenv("REDIS_PORT", "6379")
	os.Setenv("REDIS_PASSWORD", "redis_pass")

	cfg, err := config.Load()
	assert.NoError(t, err)

	url := cfg.RedisURL()
	assert.Contains(t, url, ":redis_pass@")
	assert.Contains(t, url, "localhost:6379")
}

func TestRedisURL_WithoutPassword(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	os.Setenv("REDIS_HOST", "localhost")
	os.Setenv("REDIS_PORT", "6379")
	os.Unsetenv("REDIS_PASSWORD")

	cfg, err := config.Load()
	assert.NoError(t, err)

	url := cfg.RedisURL()
	assert.Equal(t, "redis://localhost:6379", url)
}

func TestBaseURL(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	os.Setenv("SERVER_HOST", "0.0.0.0")
	os.Setenv("SERVER_PORT", "9090")

	cfg, err := config.Load()
	assert.NoError(t, err)

	url := cfg.BaseURL()
	assert.Equal(t, "http://0.0.0.0:9090", url)
}

func TestIsDevelopment(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

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
			os.Setenv("SERVER_ENV", tt.env)
			// 为生产环境设置非默认值
			os.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")
			os.Setenv("ADMIN_EMAILS", "admin@company.com")

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

func TestGetAdminEmails(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "单个邮箱",
			input:    "admin@example.com",
			expected: []string{"admin@example.com"},
		},
		{
			name:     "多个邮箱",
			input:    "admin@example.com, support@example.com",
			expected: []string{"admin@example.com", "support@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("ADMIN_EMAILS", tt.input)
			// 为每个测试设置其他必需的环境变量
			os.Setenv("SERVER_ENV", "development")

			cfg, err := config.Load()
			assert.NoError(t, err)

			emails := cfg.GetAdminEmails()
			assert.Equal(t, tt.expected, emails)
		})
	}
}

func TestGetCORSAllowedOrigins(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

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
			os.Setenv("CORS_ALLOWED_ORIGINS", tt.input)

			cfg, err := config.Load()
			assert.NoError(t, err)

			origins := cfg.GetCORSAllowedOrigins()
			assert.Equal(t, tt.expected, origins)
		})
	}
}

func TestGetAdminDomains(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	os.Setenv("ADMIN_DOMAINS", "admin.com, example.com")

	cfg, err := config.Load()
	assert.NoError(t, err)

	domains := cfg.GetAdminDomains()
	assert.Equal(t, []string{"admin.com", "example.com"}, domains)
}

// ============================================================================
// 连接池配置测试
// ============================================================================

func TestConnectionPoolConfig(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	os.Setenv("DB_MAX_OPEN_CONNS", "100")
	os.Setenv("DB_MAX_IDLE_CONNS", "50")
	os.Setenv("DB_CONN_MAX_LIFETIME", "10m")
	os.Setenv("DB_QUERY_TIMEOUT", "30s")

	cfg, err := config.Load()
	assert.NoError(t, err)
	assert.Equal(t, 100, cfg.DBMaxOpenConns)
	assert.Equal(t, 50, cfg.DBMaxIdleConns)
	assert.Equal(t, 10*time.Minute, cfg.DBConnMaxLifetime)
	assert.Equal(t, 30*time.Second, cfg.DBQueryTimeout)
}
