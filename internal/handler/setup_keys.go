// 配置向导：密钥生成与路径白名单校验（从 setup.go 拆分）
package handler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/util/handlerutil"
)

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
	if err := os.MkdirAll(privDir, 0750); err != nil {
		slog.Error("创建私钥目录失败", "dir", privDir, "error", err)
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("创建密钥目录失败"))
		return
	}

	pubDir := filepath.Dir(req.PublicPath)
	if pubDir != privDir {
		if err := os.MkdirAll(pubDir, 0750); err != nil {
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
		return fmt.Errorf("path cannot be empty")
	}

	cleanPath := filepath.Clean(path)

	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("absolute path is required")
	}

	// 检查路径是否包含危险字符或模式
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path must not contain '..'")
	}

	// 获取目录路径
	// 如果路径本身是目录,使用路径本身;否则使用父目录
	dir := cleanPath
	if info, err := os.Stat(cleanPath); err == nil && !info.IsDir() {
		dir = filepath.Dir(cleanPath)
	} else if err != nil && !os.IsNotExist(err) {
		// 如果是其他错误(非文件不存在),返回错误
		return fmt.Errorf("unable to access path: %w", err)
	} else if os.IsNotExist(err) {
		// 如果文件不存在,使用父目录进行检查
		dir = filepath.Dir(cleanPath)
	}

	// 解析整条路径中的所有符号链接，防止中间目录符号链接绕过白名单
	// 例如 /app/keys 是指向 /tmp 的符号链接时，仅检查最终组件会漏过
	// 仅当目录存在时才解析，不存在时使用原路径（后续白名单匹配会决定是否接受）
	if _, statErr := os.Stat(dir); statErr == nil {
		realPath, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return fmt.Errorf("unable to resolve symlinks: %w", err)
		}
		dir = realPath
	}

	allowedDirs := getKeyPathWhitelist()
	for _, allowedDir := range allowedDirs {
		// 检查路径本身、解析后的目录是否完全匹配或在允许目录的子目录中
		// cleanPath 检查覆盖「白名单目录本身」的场景（目录可能尚未创建），
		// dir 检查覆盖「白名单目录内的文件」场景（含符号链接解析后的真实路径）
		// 添加路径分隔符检查防止前缀匹配绕过
		// 例如: /app/keys 不应匹配 /app/keys_malicious
		if cleanPath == allowedDir || dir == allowedDir ||
			strings.HasPrefix(dir, allowedDir+string(filepath.Separator)) {
			return nil
		}
	}

	return fmt.Errorf("path must be within allowed directories: %v", allowedDirs)
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
