// Package handler 内部测试（覆盖 helpers 和 setup 的 0% 函数）
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/util/retryutil"
	"github.com/example/sso/internal/util/testutil"
	"github.com/example/sso/internal/validator"
)

// withLang 辅助：构造带语言上下文的请求
func withLang(r *http.Request, lang string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), middleware.LanguageKey, lang))
}

// ============================================================================
// writeLocalizedError 测试
// 覆盖原本 0% 的本地化错误响应函数
// ============================================================================

func TestWriteLocalizedError(t *testing.T) {
	t.Run("中文环境_返回本地化消息", func(t *testing.T) {
		// 使用 fresh error，避免 WithDetails 污染全局错误变量
		appErr := apperrors.New(apperrors.ErrCodeBadRequest, "请求参数错误", http.StatusBadRequest).WithDetails("参数错误")
		req := withLang(httptest.NewRequest(http.MethodPost, "/test", nil), "zh")

		rec := httptest.NewRecorder()
		writeLocalizedError(rec, req, appErr)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["message"], "应返回本地化错误消息")
		assert.Equal(t, string(apperrors.ErrCodeBadRequest), resp["code"])
		assert.Equal(t, "参数错误", resp["details"])
	})

	t.Run("英文环境_返回英文消息", func(t *testing.T) {
		appErr := apperrors.New(apperrors.ErrCodeBadRequest, "bad request", http.StatusBadRequest)
		req := withLang(httptest.NewRequest(http.MethodPost, "/test", nil), "en")

		rec := httptest.NewRecorder()
		writeLocalizedError(rec, req, appErr)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.NotEmpty(t, rec.Body.String())
	})

	t.Run("无语言上下文_使用默认值zh-CN", func(t *testing.T) {
		appErr := apperrors.New(apperrors.ErrCodeInternal, "内部错误", http.StatusInternalServerError)
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		// 不设置语言上下文

		rec := httptest.NewRecorder()
		assert.NotPanics(t, func() {
			writeLocalizedError(rec, req, appErr)
		})
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("HTTP状态码与AppError一致", func(t *testing.T) {
		tests := []struct {
			name   string
			appErr *apperrors.AppError
			status int
		}{
			{"BadRequest", apperrors.ErrBadRequest, http.StatusBadRequest},
			{"Unauthorized", apperrors.ErrUnauthorized, http.StatusUnauthorized},
			{"Forbidden", apperrors.ErrForbidden, http.StatusForbidden},
			{"NotFound", apperrors.ErrNotFound, http.StatusNotFound},
			{"Conflict", apperrors.ErrEmailExists, http.StatusConflict},
			{"Internal", apperrors.ErrInternal, http.StatusInternalServerError},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := withLang(httptest.NewRequest(http.MethodGet, "/", nil), "zh")
				rec := httptest.NewRecorder()
				writeLocalizedError(rec, req, tt.appErr)
				assert.Equal(t, tt.status, rec.Code)
			})
		}
	})
}

// ============================================================================
// handleServiceError 测试
// 覆盖原本 0% 的服务层错误统一处理函数
// ============================================================================

func TestHandleServiceError(t *testing.T) {
	makeReq := func() (*http.Request, *httptest.ResponseRecorder) {
		req := withLang(httptest.NewRequest(http.MethodPost, "/test", nil), "zh")
		return req, httptest.NewRecorder()
	}

	t.Run("验证错误_匹配validationErrors_返回对应状态码", func(t *testing.T) {
		// service.ErrInvalidCredentials 在 validationErrors 中映射为 401
		req, rec := makeReq()
		handleServiceError(rec, req, service.ErrInvalidCredentials, apperrors.ErrCodeInternal)

		assert.Equal(t, http.StatusUnauthorized, rec.Code, "ErrInvalidCredentials 应映射为 401")
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["error"])
	})

	t.Run("验证错误_邮箱相关", func(t *testing.T) {
		req, rec := makeReq()
		handleServiceError(rec, req, validator.ErrEmailInvalid, apperrors.ErrCodeInternal)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("验证错误_密码相关", func(t *testing.T) {
		req, rec := makeReq()
		handleServiceError(rec, req, validator.ErrPasswordTooShort, apperrors.ErrCodeInternal)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("未知错误_使用默认错误码_返回500", func(t *testing.T) {
		req, rec := makeReq()
		unknownErr := errors.New("some unexpected error")
		handleServiceError(rec, req, unknownErr, apperrors.ErrCodeInternal)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["error"], "应返回默认错误码对应的消息")
	})

	t.Run("包装的验证错误_仍能匹配", func(t *testing.T) {
		// errors.Is 应能匹配被 fmt.Errorf %w 包装的错误
		req, rec := makeReq()
		wrappedErr := wrapErr(service.ErrInvalidCredentials)
		handleServiceError(rec, req, wrappedErr, apperrors.ErrCodeInternal)

		assert.Equal(t, http.StatusUnauthorized, rec.Code, "包装的验证错误应被 errors.Is 匹配")
	})
}

// wrapErr 辅助：包装错误以测试 errors.Is 链
func wrapErr(err error) error {
	return &wrappedError{err: err}
}

type wrappedError struct{ err error }

func (w *wrappedError) Error() string { return "wrapped: " + w.err.Error() }
func (w *wrappedError) Unwrap() error { return w.err }

// ============================================================================
// HandleSetupTestDB 测试
// 覆盖原本 0% 的数据库连接测试端点
// ============================================================================

func TestHandleSetupTestDB(t *testing.T) {
	handler := NewSetupHandler(".env", "1.0.0")

	t.Run("非本地请求_403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/setup/test-db", strings.NewReader(`{}`))
		req.RemoteAddr = "192.168.1.100:12345"
		rec := httptest.NewRecorder()

		handler.HandleSetupTestDB(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("无效JSON_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/setup/test-db", strings.NewReader(`invalid json`))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		handler.HandleSetupTestDB(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("缺少必填字段_400", func(t *testing.T) {
		body := `{"host":"localhost"}` // 缺 port/name/user
		req := httptest.NewRequest(http.MethodPost, "/setup/test-db", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		handler.HandleSetupTestDB(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("端口非法_400", func(t *testing.T) {
		body := `{"host":"localhost","port":"abc","name":"db","user":"u","password":"p"}`
		req := httptest.NewRequest(http.MethodPost, "/setup/test-db", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		handler.HandleSetupTestDB(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("端口越界_400", func(t *testing.T) {
		body := `{"host":"localhost","port":"99999","name":"db","user":"u","password":"p"}`
		req := httptest.NewRequest(http.MethodPost, "/setup/test-db", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		handler.HandleSetupTestDB(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("无效SSLMode_400", func(t *testing.T) {
		body := `{"host":"localhost","port":"5432","name":"db","user":"u","password":"p","ssl_mode":"invalid"}`
		req := httptest.NewRequest(http.MethodPost, "/setup/test-db", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		handler.HandleSetupTestDB(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("连接不存在的主机_500", func(t *testing.T) {
		// 用不存在的端口，触发 testDBConnection 失败
		body := `{"host":"127.0.0.1","port":"1","name":"db","user":"u","password":"p"}`
		req := httptest.NewRequest(http.MethodPost, "/setup/test-db", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		handler.HandleSetupTestDB(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, "连接失败应返回500")
	})

	t.Run("空SSLMode默认disable", func(t *testing.T) {
		// 不传 ssl_mode，应默认 disable 并尝试连接（连接失败返回500，但不报 ssl_mode 错误）
		body := `{"host":"127.0.0.1","port":"1","name":"db","user":"u","password":"p"}`
		req := httptest.NewRequest(http.MethodPost, "/setup/test-db", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		handler.HandleSetupTestDB(rec, req)
		// 应通过 ssl_mode 校验（默认 disable），到达连接阶段
		assert.Equal(t, http.StatusInternalServerError, rec.Code, "默认 disable 后连接失败返回500")
	})
}

// ============================================================================
// HandleSetupTestRedis 测试
// 覆盖原本 0% 的 Redis 连接测试端点
// ============================================================================

func TestHandleSetupTestRedis(t *testing.T) {
	handler := NewSetupHandler(".env", "1.0.0")

	t.Run("非本地请求_403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/setup/test-redis", strings.NewReader(`{}`))
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()

		handler.HandleSetupTestRedis(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("无效JSON_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/setup/test-redis", strings.NewReader(`bad`))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		handler.HandleSetupTestRedis(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("连接不存在的主机_500", func(t *testing.T) {
		body := `{"host":"127.0.0.1","port":"1","password":"","db":0}`
		req := httptest.NewRequest(http.MethodPost, "/setup/test-redis", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		handler.HandleSetupTestRedis(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, "连接失败应返回500")
	})
}

// ============================================================================
// testDBConnection / testRedisConnection 直接测试
// 覆盖原本 0% 的底层连接测试函数
// ============================================================================

func TestTestDBConnection(t *testing.T) {
	t.Run("无效DSN_返回错误", func(t *testing.T) {
		// 空或格式错误的 DSN，sql.Open 可能不报错，但 Ping 会失败
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err := testDBConnection(ctx, "postgres://u:p@127.0.0.1:1/nonexistent?sslmode=disable")
		assert.Error(t, err, "连接不存在的 DB 应返回错误")
		assert.Contains(t, err.Error(), "database", "错误消息应包含提示")
	})

	t.Run("超时上下文_返回错误", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(2 * time.Millisecond) // 确保已超时
		err := testDBConnection(ctx, "postgres://u:p@127.0.0.1:5432/db?sslmode=disable")
		assert.Error(t, err)
	})
}

func TestTestRedisConnection(t *testing.T) {
	t.Run("不存在的主机_返回错误", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err := testRedisConnection(ctx, "127.0.0.1:1", "", 0)
		assert.Error(t, err, "连接不存在的 Redis 应返回错误")
		assert.Contains(t, err.Error(), "redis", "错误消息应包含提示")
	})

	t.Run("超时上下文_返回错误", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(2 * time.Millisecond)
		err := testRedisConnection(ctx, "127.0.0.1:6379", "", 0)
		assert.Error(t, err)
	})
}

// ============================================================================
// 真实 DB/Redis 集成测试（成功路径）
// 覆盖 HandleSetupTestDB / HandleSetupTestRedis 中"连接成功"分支
//
// 复用 retryutil.ExponentialBackoffRetry 提供统一的重试机制，
// 与 internal/util/testutil 中其他真实 DB/Redis 测试保持一致。
//
// 环境变量：
//   - DATABASE_URL: 形如 postgres://user:pass@host:port/db?sslmode=disable
//     若未设置，从 DB_HOST/DB_PORT/DB_NAME/DB_USER/DB_PASSWORD/DB_SSL_MODE 拼装
//   - REDIS_TEST_ADDR: 形如 host:port（与 .env.test 中保持一致）
//     若未设置，从 REDIS_HOST/REDIS_PORT 拼装
//   - TEST_CONN_MAX_RETRIES / TEST_CONN_BASE_DELAY / TEST_CONN_TIMEOUT（重试与超时）
//
// 未配置时自动 t.Skip，不影响默认 `go test` 运行
// ============================================================================

// buildDSNFromEnv 从环境变量构造 PostgreSQL DSN
// 优先使用 DATABASE_URL；否则用 DB_* 系列变量拼装
func buildDSNFromEnv() (string, bool) {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v, true
	}
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	name := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")
	sslMode := os.Getenv("DB_SSL_MODE")
	if sslMode == "" {
		sslMode = "disable"
	}
	if host == "" || port == "" || name == "" || user == "" {
		return "", false
	}
	return "postgres://" + user + ":" + pass + "@" + host + ":" + port + "/" + name + "?sslmode=" + sslMode, true
}

// buildRedisAddrFromEnv 从环境变量构造 Redis 地址
// 优先使用 REDIS_TEST_ADDR；否则用 REDIS_HOST/REDIS_PORT 拼装
func buildRedisAddrFromEnv() (addr, password string, db int, ok bool) {
	if v := os.Getenv("REDIS_TEST_ADDR"); v != "" {
		return v, os.Getenv("REDIS_PASSWORD"), 0, true
	}
	host := os.Getenv("REDIS_HOST")
	port := os.Getenv("REDIS_PORT")
	if host == "" || port == "" {
		return "", "", 0, false
	}
	return host + ":" + port, os.Getenv("REDIS_PASSWORD"), 0, true
}

// TestHandleSetupTestDB_RealDB 真实数据库成功路径
// 覆盖 HandleSetupTestDB 中"连接成功"分支，返回 200 + "数据库连接成功"
func TestHandleSetupTestDB_RealDB(t *testing.T) {
	dsn, ok := buildDSNFromEnv()
	if !ok {
		t.Skip("跳过真实 DB 测试：未设置 DATABASE_URL 或 DB_* 环境变量")
	}
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	name := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")
	sslMode := os.Getenv("DB_SSL_MODE")
	if sslMode == "" {
		sslMode = "disable"
	}

	body := fmt.Sprintf(`{"host":%q,"port":%q,"name":%q,"user":%q,"password":%q,"ssl_mode":%q}`,
		host, port, name, user, pass, sslMode)

	assertSetupHandlerSuccess(t, "DB", dsn, body, "数据库连接成功", func(rec *httptest.ResponseRecorder) {
		handler := NewSetupHandler(".env", "1.0.0")
		req := httptest.NewRequest(http.MethodPost, "/setup/test-db", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		handler.HandleSetupTestDB(rec, req)
	})
}

// TestHandleSetupTestRedis_RealRedis 真实 Redis 成功路径
// 覆盖 HandleSetupTestRedis 中"连接成功"分支，返回 200 + "Redis连接成功"
func TestHandleSetupTestRedis_RealRedis(t *testing.T) {
	addr, password, db, ok := buildRedisAddrFromEnv()
	if !ok {
		t.Skip("跳过真实 Redis 测试：未设置 REDIS_TEST_ADDR 或 REDIS_HOST/REDIS_PORT 环境变量")
	}

	host, port, found := strings.Cut(addr, ":")
	if !found {
		t.Fatalf("REDIS_TEST_ADDR 格式非法: %s", addr)
	}
	body := fmt.Sprintf(`{"host":%q,"port":%q,"password":%q,"db":%d}`,
		host, port, password, db)

	assertSetupHandlerSuccess(t, "Redis", addr, body, "Redis连接成功", func(rec *httptest.ResponseRecorder) {
		handler := NewSetupHandler(".env", "1.0.0")
		req := httptest.NewRequest(http.MethodPost, "/setup/test-redis", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		handler.HandleSetupTestRedis(rec, req)
	})
}

// assertSetupHandlerSuccess 用 retryutil 重试调用 setup handler，断言最终成功
//
// 复用 retryutil.ExponentialBackoffRetry，与 internal/util/testutil 保持一致：
//   - 重试次数由 TEST_CONN_MAX_RETRIES 控制（默认 3）
//   - 基础延迟由 TEST_CONN_BASE_DELAY 控制（默认 500ms）
//   - 整体超时由 TEST_CONN_TIMEOUT 控制（默认 30s）
//
// 注意：handler 内部已有 10s 连接超时，这里在测试层再加一层重试，
// 应对 CI service container 启动后短暂抖动。
func assertSetupHandlerSuccess(t *testing.T, target, endpoint, _ string, successMarker string, doRequest func(rec *httptest.ResponseRecorder)) {
	t.Helper()

	cfg := testutil.LoadConnConfig()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	var lastCode int
	var lastBody string

	err := retryutil.ExponentialBackoffRetry(ctx, func(ctx context.Context) error {
		rec := httptest.NewRecorder()
		doRequest(rec)
		lastCode = rec.Code
		lastBody = rec.Body.String()

		if lastCode == http.StatusOK && strings.Contains(lastBody, successMarker) {
			return nil
		}
		return fmt.Errorf("%s 连接测试失败：status=%d, body=%s, endpoint=%s", target, lastCode, lastBody, endpoint)
	}, cfg.RetryConfig())

	require.NoErrorf(t, err, "%s 连接测试在重试后仍失败，endpoint=%s，最后状态码=%d，响应=%s",
		target, endpoint, lastCode, lastBody)
}
