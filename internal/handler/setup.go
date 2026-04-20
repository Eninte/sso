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
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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
	envPath    string
	version    string
	setupToken atomic.Pointer[string]
}

// NewSetupHandler 创建配置向导handler
func NewSetupHandler(envPath string, version string) *SetupHandler {
	h := &SetupHandler{
		envPath: envPath,
		version: version,
	}
	h.generateSetupToken()
	return h
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
	nonce := middleware.GetCSPNonce(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = setupTmpl.Execute(w, map[string]string{
		"Nonce":      nonce,
		"Version":    h.version,
		"SetupToken": h.GetSetupToken(),
	})
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

	if err := config.WriteEnvFile(h.envPath, filtered); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("写入配置文件失败"))
		return
	}

	// 令牌失效，防止重复写入
	h.setupToken.Store(nil)

	handlerutil.WriteJSONSuccess(w, map[string]string{
		"message": "配置已保存，请手动重启服务以加载新配置",
		"note":    "如果使用 systemd/supervisor 等进程管理器，请执行相应的重启命令",
	})
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

	// 验证 ssl_mode 白名单，防止 DSN 注入
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

	db, err := openDB(dsn)
	if err != nil {
		// 不暴露具体错误信息，避免泄露DSN中的密码
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("数据库连接失败，请检查配置"))
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		// 不暴露具体错误信息，避免泄露连接详情
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("数据库连接测试失败，请检查网络和凭据"))
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
	client := newRedisClient(addr, req.Password, req.DB)
	defer client.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("Redis连接失败"))
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
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("私钥路径无效"))
		return
	}
	if err := ValidateKeyPath(req.PublicPath); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("公钥路径无效"))
		return
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

	// 检查是否为符号链接，防止绕过白名单
	fileInfo, err := os.Lstat(cleanPath)
	if err == nil && fileInfo.Mode()&os.ModeSymlink != 0 {
		// 如果是符号链接，解析真实路径
		realPath, err := filepath.EvalSymlinks(cleanPath)
		if err != nil {
			return fmt.Errorf("无法解析符号链接: %w", err)
		}
		cleanPath = realPath
	}

	allowedDirs := getKeyPathWhitelist()
	for _, dir := range allowedDirs {
		if strings.HasPrefix(cleanPath, dir+"/") || cleanPath == dir {
			return nil
		}
	}

	return fmt.Errorf("路径必须在允许的目录内: %v", allowedDirs)
}

// getKeyPathWhitelist 获取密钥路径白名单
// 支持通过环境变量 KEY_PATH_WHITELIST 自定义（逗号分隔）
// 默认值：/app/keys, /keys, /etc/sso/keys
func getKeyPathWhitelist() []string {
	// 默认允许的目录
	defaultDirs := []string{"/app/keys", "/keys", "/etc/sso/keys"}

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
