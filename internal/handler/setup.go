package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"
	"text/template"
	"time"

	"github.com/example/sso/internal/config"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/util/handlerutil"
)

//go:embed templates/setup.html
var setupHTMLTemplate string

var setupTmpl = template.Must(template.New("setup").Parse(setupHTMLTemplate))

// SetupHandler 配置向导handler
// 在服务配置不完整时启动，接收表单写入.env文件，触发重启
type SetupHandler struct {
	envPath       string
	version       string
	setupToken    atomic.Pointer[string]
	validateConns func(dbDSN, redisAddr, redisPassword string, redisDB int) error
}

func NewSetupHandler(envPath string, version string) *SetupHandler {
	h := &SetupHandler{
		envPath: envPath,
		version: version,
	}
	h.generateSetupToken()
	return h
}

func (h *SetupHandler) SetValidateConns(fn func(dbDSN, redisAddr, redisPassword string, redisDB int) error) {
	h.validateConns = fn
}

// generateSetupToken 生成一次性配置令牌
func (h *SetupHandler) generateSetupToken() {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return
	}
	token := fmt.Sprintf("%x", b)
	h.setupToken.Store(&token)
}

// validateSetupToken 验证配置令牌
func (h *SetupHandler) validateSetupToken(r *http.Request) bool {
	tokenPtr := h.setupToken.Load()
	if tokenPtr == nil {
		return false
	}
	reqToken := r.Header.Get("X-Setup-Token")
	if reqToken == "" {
		reqToken = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(reqToken), []byte(*tokenPtr)) == 1
}

// GetSetupToken 返回当前配置令牌（用于页面渲染）
func (h *SetupHandler) GetSetupToken() string {
	tokenPtr := h.setupToken.Load()
	if tokenPtr == nil {
		return ""
	}
	return *tokenPtr
}

// HandleSetupPage 渲染配置向导页面
func (h *SetupHandler) HandleSetupPage(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("配置向导仅允许本地访问"))
		return
	}

	// 注意：配置向导只有在配置加载失败时才会启动（见cmd/server/main.go的startSetupWizard）
	// 因此不需要检查.env文件是否存在，因为：
	// 1. .env可能存在但内容无效（如DB_PASSWORD为空）
	// 2. 配置向导启动本身就说明配置有问题，应该允许访问和修复

	// 如果token为空（首次访问或保存后失效），重新生成
	if h.setupToken.Load() == nil {
		h.generateSetupToken()
	}

	nonce := middleware.GetCSPNonce(r.Context())

	// 获取默认密钥路径（优先使用当前工作目录）
	defaultKeyPath := "/app/keys"
	if cwd, err := os.Getwd(); err == nil {
		defaultKeyPath = filepath.Join(cwd, "keys")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := setupTmpl.Execute(w, map[string]string{
		"Nonce":             nonce,
		"Version":           h.version,
		"SetupToken":        h.GetSetupToken(),
		"DefaultKeyPath":    defaultKeyPath,
		"DefaultPrivateKey": filepath.Join(defaultKeyPath, "private.pem"),
		"DefaultPublicKey":  filepath.Join(defaultKeyPath, "public.pem"),
	}); err != nil {
		slog.Error("setup template render failed", "error", err)
	}
}

// HandleSetupSave 接收配置并写入.env文件
// 注意：配置保存后需要手动重启服务以加载新配置
var allowedEnvKeys = map[string]bool{
	"SERVER_HOST": true, "SERVER_PORT": true, "SERVER_ENV": true,
	"DB_HOST": true, "DB_PORT": true, "DB_NAME": true, "DB_USER": true, "DB_PASSWORD": true, "DB_SSL_MODE": true,
	"DB_MAX_OPEN_CONNS": true, "DB_MAX_IDLE_CONNS": true, "DB_CONN_MAX_LIFETIME": true, "DB_CONN_MAX_IDLE_TIME": true, "DB_QUERY_TIMEOUT": true,
	"REDIS_ENABLE": true, "REDIS_HOST": true, "REDIS_PORT": true, "REDIS_PASSWORD": true, "REDIS_DB": true,
	"REDIS_CONN_TIMEOUT": true, "REDIS_POOL_SIZE": true, "REDIS_MIN_IDLE_CONNS": true,
	"JWT_PRIVATE_KEY_PATH": true, "JWT_PUBLIC_KEY_PATH": true, "JWT_ACCESS_TOKEN_TTL": true, "JWT_REFRESH_TOKEN_TTL": true, "JWT_ISSUER": true,
	"KEY_ROTATION_ENABLED": true, "KEY_ROTATION_INTERVAL": true, "KEY_TRANSITION_PERIOD": true,
	"BCRYPT_COST": true, "RATE_LIMIT_REQUESTS": true, "RATE_LIMIT_WINDOW": true,
	"MAX_LOGIN_ATTEMPTS": true, "LOCKOUT_DURATION": true, "MFA_RECOVERY_HMAC_KEY": true,
	"SMTP_HOST": true, "SMTP_PORT": true, "SMTP_USER": true, "SMTP_PASSWORD": true, "SMTP_FROM": true,
	"CORS_ALLOWED_ORIGINS": true,
	"METRICS_USERNAME":     true, "METRICS_PASSWORD": true,
	"SHUTDOWN_TIMEOUT": true, "LAN_DEPLOYMENT": true,
}

func (h *SetupHandler) HandleSetupSave(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("配置向导仅允许本地访问"))
		return
	}
	if !h.validateSetupToken(r) {
		handlerutil.WriteJSONError(w, apperrors.ErrUnauthorized.WithDetails("无效的配置令牌"))
		return
	}

	var values map[string]string
	if err := json.NewDecoder(r.Body).Decode(&values); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("无效的请求格式"))
		return
	}

	filtered := make(map[string]string, len(values))
	for k, v := range values {
		if !allowedEnvKeys[k] {
			handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("不允许的环境变量: "+k))
			return
		}
		filtered[k] = v
	}

	if err := validateSetupConfig(filtered); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails(err.Error()))
		return
	}

	dbDSN, redisAddr, redisPassword, redisDB, err := buildDSNs(filtered)
	if err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails(err.Error()))
		return
	}

	if err := h.testConnections(r, dbDSN, redisAddr, redisPassword, redisDB, filtered); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails(err.Error()))
		return
	}

	if err := config.WriteEnvFile(h.envPath, filtered); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("写入配置文件失败"))
		return
	}

	h.setupToken.Store(nil)

	handlerutil.WriteJSONSuccess(w, map[string]string{
		"message": "配置已保存，服务将优雅关闭并重启",
		"note":    "请确保进程管理器（systemd、Docker等）配置了自动重启",
	})

	go func() {
		time.Sleep(3 * time.Second)
		slog.Info("配置已保存，触发优雅关闭...")

		if os.Getenv("SETUP_SKIP_RESTART") != "" {
			slog.Info("跳过重启（SETUP_SKIP_RESTART 已设置）")
			return
		}

		if p, err := os.FindProcess(os.Getpid()); err == nil {
			if err := p.Signal(syscall.SIGTERM); err != nil {
				slog.Error("发送SIGTERM信号失败", "error", err)
			}
		} else {
			slog.Error("无法获取当前进程", "error", err)
		}
	}()
}

func validateSetupConfig(filtered map[string]string) error {
	requiredKeys := []struct {
		key string
		msg string
	}{
		{"DB_PASSWORD", "数据库密码不能为空"},
		{"DB_HOST", "数据库主机不能为空"},
		{"DB_PORT", "数据库端口不能为空"},
		{"DB_NAME", "数据库名称不能为空"},
		{"DB_USER", "数据库用户不能为空"},
	}
	for _, r := range requiredKeys {
		if filtered[r.key] == "" {
			return fmt.Errorf("%s", r.msg)
		}
	}

	if sslMode := filtered["DB_SSL_MODE"]; sslMode != "" {
		validSSLModes := map[string]bool{
			"disable": true, "prefer": true, "require": true, "verify-ca": true, "verify-full": true,
		}
		if !validSSLModes[sslMode] {
			return fmt.Errorf("无效的 DB_SSL_MODE")
		}
	}

	if costStr := filtered["BCRYPT_COST"]; costStr != "" {
		if cost, err := strconv.Atoi(costStr); err != nil || cost < 4 || cost > 31 {
			return fmt.Errorf("BCRYPT_COST 必须为 4-31 之间的整数")
		}
	}

	if portStr := filtered["SERVER_PORT"]; portStr != "" {
		if port, err := strconv.Atoi(portStr); err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("SERVER_PORT 必须为 1-65535 之间的整数")
		}
	}

	if env := filtered["SERVER_ENV"]; env != "" && env != "development" && env != "production" {
		return fmt.Errorf("SERVER_ENV 必须为 development 或 production")
	}

	return nil
}

func buildDSNs(filtered map[string]string) (dbDSN, redisAddr, redisPassword string, redisDB int, err error) {
	dbHost := filtered["DB_HOST"]
	dbPort := filtered["DB_PORT"]
	dbName := filtered["DB_NAME"]
	dbUser := filtered["DB_USER"]
	dbPassword := filtered["DB_PASSWORD"]
	dbSSLMode := filtered["DB_SSL_MODE"]
	if dbSSLMode == "" {
		dbSSLMode = "disable"
	}

	hostPort := net.JoinHostPort(dbHost, dbPort)
	dbDSN = fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s",
		url.PathEscape(dbUser), url.PathEscape(dbPassword), hostPort, url.PathEscape(dbName), dbSSLMode)

	if redisHost := filtered["REDIS_HOST"]; redisHost != "" && filtered["REDIS_ENABLE"] != "false" {
		redisPort := filtered["REDIS_PORT"]
		if redisPort == "" {
			redisPort = "6379"
		} else {
			if p, err := strconv.Atoi(redisPort); err != nil || p < 1 || p > 65535 {
				return "", "", "", 0, fmt.Errorf("REDIS_PORT 必须为 1-65535 之间的整数")
			}
		}
		redisPassword = filtered["REDIS_PASSWORD"]
		if v := filtered["REDIS_DB"]; v != "" {
			if n, e := strconv.Atoi(v); e == nil {
				redisDB = n
			}
		}
		redisAddr = redisHost + ":" + redisPort
	}
	return dbDSN, redisAddr, redisPassword, redisDB, nil
}

func (h *SetupHandler) testConnections(r *http.Request, dbDSN, redisAddr, redisPassword string, redisDB int, filtered map[string]string) error {
	if h.validateConns != nil {
		return h.validateConns(dbDSN, redisAddr, redisPassword, redisDB)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := testDBConnection(ctx, dbDSN); err != nil {
		slog.Error("数据库连接失败", "error", err, "host", filtered["DB_HOST"], "port", filtered["DB_PORT"], "database", filtered["DB_NAME"])
		return err
	}

	if redisAddr != "" {
		redisCtx, redisCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer redisCancel()
		if err := testRedisConnection(redisCtx, redisAddr, redisPassword, redisDB); err != nil {
			slog.Error("Redis连接失败", "error", err, "host", filtered["REDIS_HOST"], "port", filtered["REDIS_PORT"])
			return err
		}
	}
	return nil
}

// HandleSetupTestDB 测试数据库连接
func (h *SetupHandler) HandleSetupTestDB(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		slog.Warn("setup test-db 拒绝非本地请求", "remote_addr", r.RemoteAddr)
		handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("配置向导仅允许本地访问"))
		return
	}
	var req struct {
		Host     string `json:"host"`
		Port     string `json:"port"`
		Name     string `json:"name"`
		User     string `json:"user"`
		Password string `json:"password"`
		SSLMode  string `json:"ssl_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("setup test-db 请求体解析失败", "error", err, "remote_addr", r.RemoteAddr)
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("无效的请求格式"))
		return
	}
	// 注意：不在日志中记录密码
	slog.Info("setup test-db 收到请求", "host", req.Host, "port", req.Port, "database", req.Name, "user", req.User, "ssl_mode", req.SSLMode)

	if req.Host == "" || req.Port == "" || req.Name == "" || req.User == "" {
		slog.Warn("setup test-db 缺少必填字段", "host", req.Host, "port", req.Port, "database", req.Name, "user_empty", req.User == "")
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("缺少必填字段"))
		return
	}

	port, err := strconv.Atoi(req.Port)
	if err != nil || port < 1 || port > 65535 {
		slog.Warn("setup test-db 端口非法", "port", req.Port, "parse_err", err)
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("端口号必须为1-65535之间的数字"))
		return
	}

	if req.SSLMode == "" {
		req.SSLMode = "disable"
		slog.Info("setup test-db ssl_mode 为空，使用默认值 disable")
	}

	validSSLModes := map[string]bool{
		"disable":     true,
		"require":     true,
		"prefer":      true,
		"verify-ca":   true,
		"verify-full": true,
	}
	if !validSSLModes[req.SSLMode] {
		slog.Warn("setup test-db ssl_mode 非法", "ssl_mode", req.SSLMode)
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("无效的 ssl_mode，仅支持: disable, require, prefer, verify-ca, verify-full"))
		return
	}

	hostPort := net.JoinHostPort(req.Host, req.Port)
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s",
		url.PathEscape(req.User), url.PathEscape(req.Password), hostPort, url.PathEscape(req.Name), req.SSLMode)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	slog.Info("setup test-db 开始连接测试", "host_port", hostPort, "database", req.Name, "ssl_mode", req.SSLMode, "timeout", "10s")
	if err := testDBConnection(ctx, dsn); err != nil {
		slog.Error("setup test-db 数据库连接失败", "error", err, "host", req.Host, "port", req.Port, "database", req.Name, "ssl_mode", req.SSLMode, "user", req.User)
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails(err.Error()))
		return
	}

	slog.Info("setup test-db 数据库连接成功", "host", req.Host, "port", req.Port, "database", req.Name, "ssl_mode", req.SSLMode)
	handlerutil.WriteJSONSuccess(w, map[string]string{
		"message": "数据库连接成功",
	})
}

// HandleSetupTestRedis 测试Redis连接
func (h *SetupHandler) HandleSetupTestRedis(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		slog.Warn("setup test-redis 拒绝非本地请求", "remote_addr", r.RemoteAddr)
		handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("配置向导仅允许本地访问"))
		return
	}
	var req struct {
		Host     string `json:"host"`
		Port     string `json:"port"`
		Password string `json:"password"`
		DB       int    `json:"db"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("setup test-redis 请求体解析失败", "error", err, "remote_addr", r.RemoteAddr)
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("无效的请求格式"))
		return
	}
	// 注意：不在日志中记录密码
	slog.Info("setup test-redis 收到请求", "host", req.Host, "port", req.Port, "db", req.DB, "has_password", req.Password != "")

	addr := req.Host + ":" + req.Port

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	slog.Info("setup test-redis 开始连接测试", "addr", addr, "db", req.DB, "timeout", "10s")
	if err := testRedisConnection(ctx, addr, req.Password, req.DB); err != nil {
		slog.Error("setup test-redis Redis连接失败", "error", err, "addr", addr, "db", req.DB)
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails(err.Error()))
		return
	}

	slog.Info("setup test-redis Redis连接成功", "addr", addr, "db", req.DB)
	handlerutil.WriteJSONSuccess(w, map[string]string{
		"message": "Redis连接成功",
	})
}

// HandleSetupGenerateKeys 生成RSA密钥对
