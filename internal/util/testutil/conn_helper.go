// Package testutil 提供跨测试包复用的测试辅助函数
//
// 本文件提供真实 DB/Redis 连接的统一辅助函数：
//   - 自动从环境变量读取连接配置（DATABASE_URL / REDIS_TEST_ADDR）
//   - 未配置时自动 t.Skip，不影响默认 `go test`
//   - 内置重试机制（复用 retryutil.ExponentialBackoffRetry），应对 CI service container 启动抖动
//   - 内置超时控制（context.WithTimeout）
//
// 重试与超时参数可通过环境变量调节：
//   - TEST_CONN_MAX_RETRIES（默认 3）
//   - TEST_CONN_BASE_DELAY（默认 500ms）
//   - TEST_CONN_TIMEOUT（默认 30s）
//
// 所有真实 DB/Redis 集成测试应使用本辅助函数，避免裸连导致的 CI 抖动。
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // 注册 PostgreSQL 数据库驱动
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/util/retryutil"
)

// ============================================================================
// 配置
// ============================================================================

// ConnConfig 从环境变量读取连接测试的重试与超时配置
//
// 字段均可通过环境变量调节：
//   - TEST_CONN_MAX_RETRIES（默认 3）
//   - TEST_CONN_BASE_DELAY（默认 500ms）
//   - TEST_CONN_TIMEOUT（默认 30s）
type ConnConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	Timeout    time.Duration
}

// LoadConnConfig 从环境变量加载连接测试配置
//
// 供需要自行实现重试逻辑的测试包使用（如测试 HTTP handler 而非直接建连的场景），
// 与 ConnectTestDB / ConnectTestRedis 共享同一套环境变量与默认值。
func LoadConnConfig() ConnConfig {
	return ConnConfig{
		MaxRetries: envInt("TEST_CONN_MAX_RETRIES", 3),
		BaseDelay:  envDuration("TEST_CONN_BASE_DELAY", 500*time.Millisecond),
		Timeout:    envDuration("TEST_CONN_TIMEOUT", 30*time.Second),
	}
}

// RetryConfig 返回复用 retryutil 的配置
func (c ConnConfig) RetryConfig() retryutil.RetryConfig {
	return retryutil.RetryConfig{
		MaxRetries:   c.MaxRetries,
		BaseDelay:    c.BaseDelay,
		MaxDelay:     5 * time.Second,
		JitterFactor: 0.25,
	}
}

// envInt 读取环境变量为 int，缺失或解析失败时返回默认值
func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return defaultVal
}

// envDuration 读取环境变量为 time.Duration，缺失或解析失败时返回默认值
func envDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}

// ============================================================================
// PostgreSQL 连接辅助
// ============================================================================

// ConnectTestDB 返回一个已 ping 通的真实 PostgreSQL 连接
//
// 行为：
//   - 未设置 DATABASE_URL 时 t.Skip
//   - 使用 retryutil.ExponentialBackoffRetry 重试连接（默认 3 次）
//   - 整体超时由 TEST_CONN_TIMEOUT 控制（默认 30s）
//   - 测试结束自动关闭连接（t.Cleanup）
//
// 参数接受 testing.TB 接口，同时兼容 *testing.T（普通测试）和 *testing.B（基准测试）。
//
// 环境变量：
//   - DATABASE_URL: postgres://user:pass@host:port/db?sslmode=disable
//   - TEST_CONN_MAX_RETRIES / TEST_CONN_BASE_DELAY / TEST_CONN_TIMEOUT
func ConnectTestDB(tb testing.TB) *sql.DB {
	tb.Helper()

	if testing.Short() {
		tb.Skip("跳过集成测试：-short 模式")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		tb.Skip("跳过集成测试：未设置 DATABASE_URL 环境变量")
	}

	cfg := LoadConnConfig()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// sql.Open 不实际连接，需 Ping 才会真正建立连接
	db, err := sql.Open("pgx", dbURL)
	require.NoError(tb, err, "sql.Open 失败")

	// 用 retryutil 重试 Ping
	pingErr := retryutil.ExponentialBackoffRetry(ctx, func(ctx context.Context) error {
		return db.PingContext(ctx)
	}, cfg.RetryConfig())

	if pingErr != nil {
		_ = db.Close()
		require.NoErrorf(tb, pingErr, "数据库连接在 %d 次重试后仍失败（超时 %s），DSN host 见 DATABASE_URL",
			cfg.MaxRetries, cfg.Timeout)
	}

	tb.Cleanup(func() { _ = db.Close() })
	return db
}

// ============================================================================
// Redis 连接辅助
// ============================================================================

// ConnectTestRedis 返回一个已 ping 通的真实 Redis 客户端
//
// 行为：
//   - 未设置 REDIS_TEST_ADDR 时 t.Skip
//   - 使用 retryutil.ExponentialBackoffRetry 重试连接（默认 3 次）
//   - 整体超时由 TEST_CONN_TIMEOUT 控制（默认 30s）
//   - 测试结束自动关闭客户端（t.Cleanup）
//
// 参数接受 testing.TB 接口，同时兼容 *testing.T（普通测试）和 *testing.B（基准测试）。
//
// 环境变量：
//   - REDIS_TEST_ADDR: host:port
//   - REDIS_PASSWORD（可选）
//   - TEST_CONN_MAX_RETRIES / TEST_CONN_BASE_DELAY / TEST_CONN_TIMEOUT
func ConnectTestRedis(tb testing.TB) *redis.Client {
	tb.Helper()

	if testing.Short() {
		tb.Skip("跳过集成测试：-short 模式")
	}

	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		tb.Skip("跳过集成测试：未设置 REDIS_TEST_ADDR 环境变量")
	}

	cfg := LoadConnConfig()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("REDIS_PASSWORD"),
	})

	pingErr := retryutil.ExponentialBackoffRetry(ctx, func(ctx context.Context) error {
		return client.Ping(ctx).Err()
	}, cfg.RetryConfig())

	if pingErr != nil {
		_ = client.Close()
		require.NoErrorf(tb, pingErr, "Redis 连接在 %d 次重试后仍失败（超时 %s），addr=%s",
			cfg.MaxRetries, cfg.Timeout, addr)
	}

	tb.Cleanup(func() { _ = client.Close() })
	return client
}

// RedisAddr 返回测试用 Redis 地址（不做连接，仅取配置）
// 供需要直接传地址给被测代码的场景使用
func RedisAddr() string {
	if addr := os.Getenv("REDIS_TEST_ADDR"); addr != "" {
		return addr
	}
	return ""
}
