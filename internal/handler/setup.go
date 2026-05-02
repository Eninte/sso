package handler

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"text/template"
	"time"

	"github.com/your-org/sso/internal/config"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/util/handlerutil"
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
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("无效的请求格式"))
		return
	}

	if req.Host == "" || req.Port == "" || req.Name == "" || req.User == "" {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("缺少必填字段"))
		return
	}

	port, err := strconv.Atoi(req.Port)
	if err != nil || port < 1 || port > 65535 {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("端口号必须为1-65535之间的数字"))
		return
	}

	if req.SSLMode == "" {
		req.SSLMode = "disable"
	}

	validSSLModes := map[string]bool{
		"disable":     true,
		"require":     true,
		"prefer":      true,
		"verify-ca":   true,
		"verify-full": true,
	}
	if !validSSLModes[req.SSLMode] {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("无效的 ssl_mode，仅支持: disable, require, prefer, verify-ca, verify-full"))
		return
	}

	hostPort := net.JoinHostPort(req.Host, req.Port)
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s",
		url.PathEscape(req.User), url.PathEscape(req.Password), hostPort, url.PathEscape(req.Name), req.SSLMode)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := testDBConnection(ctx, dsn); err != nil {
		slog.Error("数据库连接失败", "error", err, "host", req.Host, "port", req.Port, "database", req.Name)
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails(err.Error()))
		return
	}

	handlerutil.WriteJSONSuccess(w, map[string]string{
		"message": "数据库连接成功",
	})
}

// HandleSetupTestRedis 测试Redis连接
func (h *SetupHandler) HandleSetupTestRedis(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
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
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("无效的请求格式"))
		return
	}

	addr := req.Host + ":" + req.Port

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := testRedisConnection(ctx, addr, req.Password, req.DB); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails(err.Error()))
		return
	}

	handlerutil.WriteJSONSuccess(w, map[string]string{
		"message": "Redis连接成功",
	})
}

// HandleSetupGenerateKeys 生成RSA密钥对
func (h *SetupHandler) HandleSetupGenerateKeys(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("配置向导仅允许本地访问"))
		return
	}
	var req struct {
		PrivatePath string `json:"private_path"`
		PublicPath  string `json:"public_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("无效的请求格式"))
		return
	}

	if req.PrivatePath == "" || req.PublicPath == "" {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("密钥路径不能为空"))
		return
	}

	// 验证路径安全性（防止路径遍历攻击）
	if err := ValidateKeyPath(req.PrivatePath); err != nil {
		slog.Error("私钥路径验证失败", "path", req.PrivatePath, "error", err)
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("私钥路径无效"))
		return
	}
	if err := ValidateKeyPath(req.PublicPath); err != nil {
		slog.Error("公钥路径验证失败", "path", req.PublicPath, "error", err)
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("公钥路径无效"))
		return
	}

	// 确保目录存在（自动创建）
	privDir := filepath.Dir(req.PrivatePath)
	if err := os.MkdirAll(privDir, 0755); err != nil {
		slog.Error("创建私钥目录失败", "dir", privDir, "error", err)
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("创建密钥目录失败"))
		return
	}

	pubDir := filepath.Dir(req.PublicPath)
	if pubDir != privDir {
		if err := os.MkdirAll(pubDir, 0755); err != nil {
			slog.Error("创建公钥目录失败", "dir", pubDir, "error", err)
			handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("创建公钥目录失败"))
			return
		}
	}

	// 生成RSA密钥对（3072位，符合当前安全最佳实践）
	privateKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("生成密钥失败"))
		return
	}

	// 写入私钥
	privFile, err := os.Create(req.PrivatePath) // #nosec G304 -- 路径已验证
	if err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("创建私钥文件失败"))
		return
	}
	defer privFile.Close()
	if err := privFile.Chmod(0600); err != nil { // #nosec G302 -- 私钥文件必须限制权限
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("设置私钥文件权限失败"))
		return
	}

	privPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	if err := pem.Encode(privFile, privPEM); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("写入私钥失败"))
		return
	}

	// 写入公钥
	pubFile, err := os.Create(req.PublicPath) // #nosec G304 -- 路径已验证
	if err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("创建公钥文件失败"))
		return
	}
	defer pubFile.Close()
	if err := pubFile.Chmod(0644); err != nil { // #nosec G302
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("设置公钥文件权限失败"))
		return
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("编码公钥失败"))
		return
	}

	pubPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}
	if err := pem.Encode(pubFile, pubPEM); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("写入公钥失败"))
		return
	}

	handlerutil.WriteJSONSuccess(w, map[string]string{
		"private_path": req.PrivatePath,
		"public_path":  req.PublicPath,
	})
}

// ValidateKeyPath 验证密钥路径安全性
// 防止路径遍历攻击，确保路径在允许的目录内
func ValidateKeyPath(path string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}

	cleanPath := filepath.Clean(path)

	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("必须使用绝对路径")
	}

	// 检查路径是否包含危险字符或模式
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("路径不能包含 '..'")
	}

	// 获取目录路径
	// 如果路径本身是目录,使用路径本身;否则使用父目录
	dir := cleanPath
	if info, err := os.Stat(cleanPath); err == nil && !info.IsDir() {
		dir = filepath.Dir(cleanPath)
	} else if err != nil && !os.IsNotExist(err) {
		// 如果是其他错误(非文件不存在),返回错误
		return fmt.Errorf("无法访问路径: %w", err)
	} else if os.IsNotExist(err) {
		// 如果文件不存在,使用父目录进行检查
		dir = filepath.Dir(cleanPath)
	}

	// 检查是否为符号链接，防止绕过白名单
	fileInfo, err := os.Lstat(dir)
	if err == nil && fileInfo.Mode()&os.ModeSymlink != 0 {
		// 如果是符号链接，解析真实路径
		realPath, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return fmt.Errorf("无法解析符号链接: %w", err)
		}
		dir = realPath
	}

	allowedDirs := getKeyPathWhitelist()
	for _, allowedDir := range allowedDirs {
		// 检查路径是否完全匹配或在允许目录的子目录中
		// 添加路径分隔符检查防止前缀匹配绕过
		// 例如: /app/keys 不应匹配 /app/keys_malicious
		if dir == allowedDir || strings.HasPrefix(dir, allowedDir+string(filepath.Separator)) {
			return nil
		}
	}

	return fmt.Errorf("路径必须在允许的目录内: %v", allowedDirs)
}

// getKeyPathWhitelist 获取密钥路径白名单
// 支持通过环境变量 KEY_PATH_WHITELIST 自定义（逗号分隔）
// 默认值：/app/keys, /keys, /etc/sso/keys, 当前工作目录/keys
func getKeyPathWhitelist() []string {
	// 默认允许的目录
	defaultDirs := []string{"/app/keys", "/keys", "/etc/sso/keys"}

	// 添加当前工作目录的keys子目录（配置向导友好）
	if cwd, err := os.Getwd(); err == nil {
		cwdKeys := filepath.Join(cwd, "keys")
		defaultDirs = append(defaultDirs, cwdKeys)
	}

	// 从环境变量读取自定义白名单
	customDirs := os.Getenv("KEY_PATH_WHITELIST")
	if customDirs == "" {
		return defaultDirs
	}

	// 解析逗号分隔的路径列表
	dirs := strings.Split(customDirs, ",")
	result := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir != "" && filepath.IsAbs(dir) {
			result = append(result, filepath.Clean(dir))
		}
	}

	// 如果自定义白名单为空或无效，返回默认值
	if len(result) == 0 {
		return defaultDirs
	}

	return result
}
