// Package handler API 文档调试前端（Scalar + OpenAPI 3.0）
//
// 提供交互式 API 调试界面，便于开发和运维人员快速测试 SSO 服务接口。
//
// 路由（注册在 admin 子路由下，需管理员鉴权）：
//   - GET /api/v1/admin/api-docs            渲染 Scalar HTML 页面
//   - GET /api/v1/admin/api-docs/openapi.json 返回 OpenAPI 3.0 规范文件
//
// 安全说明：
//   - 页面仅管理员可访问（router.go 在 admin 子路由下注册 RequireAdmin）
//   - HTML 页面 CSP 由 handler 覆盖为允许 cdn.jsdelivr.net（仅此路由放松，主应用 CSP 不变）
//   - 规范文件本身不含敏感信息（不含密钥、不含实际凭据）
package handler

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"text/template"

	"github.com/example/sso/internal/middleware"
)

//go:embed templates/api_docs.html
var apiDocsHTMLTemplate string

//go:embed templates/openapi.json
var openapiSpec []byte

//go:embed templates/scalar.js
var scalarJS []byte

// apiDocsTmpl API 文档页面模板（初始化时解析一次）
var apiDocsTmpl = template.Must(template.New("api_docs").Parse(apiDocsHTMLTemplate))

// apiDocsPageCSP 仅对 API 文档页面生效的 CSP
// 离线模式：Scalar JS 通过 //go:embed 内嵌并由同源路由提供，CSP 保持严格 'self'
// 仅需为 Scalar 的内联样式放宽 style-src（用 nonce 保护）
const apiDocsPageCSP = "default-src 'self'; " +
	"script-src 'self' 'nonce-%s'; " +
	"style-src 'self' 'nonce-%s' 'unsafe-inline'; " +
	"img-src 'self' data: https:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

// APIDocsHandler API 文档调试前端处理器
type APIDocsHandler struct {
	baseURL string // 服务基础 URL，注入到 OpenAPI servers 与 Scalar 配置
	version string // 服务版本，显示在页面顶部
}

// NewAPIDocsHandler 创建 API 文档处理器
func NewAPIDocsHandler(baseURL, version string) *APIDocsHandler {
	return &APIDocsHandler{
		baseURL: baseURL,
		version: version,
	}
}

// HandlePage 渲染 Scalar API 文档页面
// GET /api/v1/admin/api-docs
//
// 安全策略：
//   - 覆盖 CSP，允许加载 Scalar CDN（cdn.jsdelivr.net）
//   - 注入 nonce 防 XSS
//   - 注入当前服务 baseURL 与 OpenAPI 规范文件 URL
func (h *APIDocsHandler) HandlePage(w http.ResponseWriter, r *http.Request) {
	nonce := middleware.GetCSPNonce(r.Context())

	// 覆盖主中间件设置的严格 CSP，仅此路由放松
	csp := formatAPIDocsCSP(nonce)
	w.Header().Set("Content-Security-Policy", csp)

	// 渲染页面
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = apiDocsTmpl.Execute(w, map[string]string{
		"Nonce":       nonce,
		"BaseURL":     h.baseURL,
		"SpecURL":     "/api/v1/admin/api-docs/openapi.json",
		"ScalarJSURL": "/api/v1/admin/api-docs/scalar.js",
		"Version":     h.version,
	})
}

// HandleSpec 返回 OpenAPI 3.0 规范文件
// GET /api/v1/admin/api-docs/openapi.json
//
// 在返回前注入当前服务 baseURL 到 servers 字段，便于 Scalar 直接发起调试请求
func (h *APIDocsHandler) HandleSpec(w http.ResponseWriter, r *http.Request) {
	// 反序列化规范文件
	var spec map[string]interface{}
	if err := json.Unmarshal(openapiSpec, &spec); err != nil {
		http.Error(w, "OpenAPI 规范文件解析失败", http.StatusInternalServerError)
		return
	}

	// 注入运行时 baseURL 到 servers（覆盖静态占位）
	spec["servers"] = []map[string]string{
		{
			"url":         h.baseURL,
			"description": "当前服务实例",
		},
	}

	// 注入版本信息到 info
	if info, ok := spec["info"].(map[string]interface{}); ok {
		info["version"] = h.version
	}

	writeJSON(w, http.StatusOK, spec)
}

// HandleScalarJS 返回 Scalar 前端 JS（离线内嵌）
// GET /api/v1/admin/api-docs/scalar.js
//
// 通过 //go:embed 在编译时打包进二进制，运行时不依赖任何外部 CDN
// 设置长缓存（不可变资源，路径含版本可通过查询参数强制刷新）
func (h *APIDocsHandler) HandleScalarJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(scalarJS)
}

// formatAPIDocsCSP 用 nonce 格式化 API 文档页面的 CSP
func formatAPIDocsCSP(nonce string) string {
	return formatCSP(apiDocsPageCSP, nonce, nonce)
}

// formatCSP 替换 CSP 模板中的 %s 占位符
// 使用显式参数避免 fmt.Sprintf 在 CSP 字符串中误解释 % 字符
func formatCSP(tmpl, nonce1, nonce2 string) string {
	// 简单实现：CSP 中 %s 仅出现两次，依次替换
	// 不使用 fmt.Sprintf 是因为 CSP 中可能包含 % 字符
	out := make([]byte, 0, len(tmpl)+len(nonce1)+len(nonce2))
	count := 0
	for i := 0; i < len(tmpl); i++ {
		if i+1 < len(tmpl) && tmpl[i] == '%' && tmpl[i+1] == 's' {
			if count == 0 {
				out = append(out, nonce1...)
			} else {
				out = append(out, nonce2...)
			}
			count++
			i++ // 跳过 's'
			continue
		}
		out = append(out, tmpl[i])
	}
	return string(out)
}
