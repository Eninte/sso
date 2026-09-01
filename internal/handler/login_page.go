// Package handler 登录处理器
// 浏览器 OIDC 兼容的 HTML 登录页与表单提交
// （标准授权码流程：authorize 未认证 → 302 到 /ui/login → 表单登录 → 会话 cookie → 302 回 authorize）
package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/example/sso/internal/captcha"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
)

// ============================================================================
// LoginPageHandler 浏览器登录页处理器
// ============================================================================

// LoginPageHandler 浏览器登录页处理器
type LoginPageHandler struct {
	authSvc    service.AuthServiceInterface
	captchaSvc captchaVerifier
	secure     bool // 生产环境 Cookie 加 Secure（与社交登录 state cookie 一致）
}

// captchaGenerator 验证码生成能力（*captcha.Service 实现），
// 用于登录页在服务端渲染验证码题目（页面无 JS，不能由前端调 /api/v1/captcha）
type captchaGenerator interface {
	Generate(ctx context.Context) (*captcha.Captcha, error)
}

// NewLoginPageHandler 创建浏览器登录页处理器
// secure 为 true 时 Cookie 带 Secure 属性（生产 HTTPS）
func NewLoginPageHandler(authSvc service.AuthServiceInterface, captchaSvc captchaVerifier, secure bool) *LoginPageHandler {
	return &LoginPageHandler{authSvc: authSvc, captchaSvc: captchaSvc, secure: secure}
}

// safeRedirect 校验重定向目标：仅接受同站相对路径（以 / 开头且不是 // 协议相对），
// 防止开放重定向。
func safeRedirect(raw string) string {
	if raw == "" {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	return raw
}

// loginPageCSP 登录页专属 Content-Security-Policy：
//   - style-src 放行内联样式（本页所有样式都是内联 <style>，全局 CSP 的 nonce 不适用）
//   - 不含 form-action 限制——部分浏览器对相对 action 的 form-action 'self'
//     判定会误拦同源表单提交（点击登录无反应的真凶）
const loginPageCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'"

// HandlePage 渲染 HTML 登录表单
// GET /ui/login?redirect=<authorize URL>
func (h *LoginPageHandler) HandlePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Security-Policy", loginPageCSP)
	redirect := safeRedirect(r.URL.Query().Get("redirect"))
	errorMsg := r.URL.Query().Get("error")
	captchaBlock := h.renderCaptchaBlock(r)
	page := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>登录 - SSO</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Microsoft YaHei",sans-serif;background:#f4f6f8;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
.card{background:#fff;border-radius:12px;box-shadow:0 2px 16px rgba(0,0,0,.08);padding:40px 36px;width:340px}
h1{font-size:20px;margin:0 0 6px;color:#1a202c}
p.sub{color:#718096;font-size:13px;margin:0 0 24px}
label{display:block;font-size:13px;color:#4a5568;margin:14px 0 6px}
input{width:100%;box-sizing:border-box;padding:10px 12px;border:1px solid #cbd5e0;border-radius:8px;font-size:14px}
input:focus{outline:none;border-color:#4299e1;box-shadow:0 0 0 3px rgba(66,153,225,.15)}
button{width:100%;margin-top:22px;padding:11px;background:#3182ce;color:#fff;border:none;border-radius:8px;font-size:15px;font-weight:600;cursor:pointer}
button:hover{background:#2b6cb0}
.err{background:#fff5f5;color:#c53030;border:1px solid #feb2b2;border-radius:8px;padding:10px 12px;font-size:13px;margin-top:16px}
</style>
</head>
<body>
<div class="card">
<h1>登录</h1>
<p class="sub">使用 SSO 账号继续访问</p>` + errorHTML(errorMsg) + `
<form method="post" action="/ui/login">
<input type="hidden" name="redirect" value="` + htmlEscape(redirect) + `">
<label for="email">邮箱</label>
<input type="email" id="email" name="email" required autofocus autocomplete="username">
<label for="password">密码</label>
<input type="password" id="password" name="password" required autocomplete="current-password">
` + captchaBlock + `
<button type="submit">登录</button>
</form>
</div>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// #nosec G705 -- 页面中所有请求来源的插值（redirect/error/验证码题目与ID）均经 htmlEscape 转义
	fmt.Fprint(w, page)
}

// renderCaptchaBlock 判定是否需要验证码，需要时生成题目并返回表单片段
//
// 触发条件（与 API 登录一致的自适应判定）：
//   - IP 维度失败计数已达阈值（GET 渲染时即可判定）
//   - 上一次 POST 因账号维度触发而回跳（URL 带 captcha=1）
//
// 生成失败或实现不支持生成时降级为不渲染——POST 会再次判定并给出错误提示
func (h *LoginPageHandler) renderCaptchaBlock(r *http.Request) string {
	if !h.captchaSvc.IsEnabled() {
		return ""
	}
	required := h.captchaSvc.ShouldRequireCaptcha(r.Context(), extractClientIP(r)) ||
		r.URL.Query().Get("captcha") == "1"
	if !required {
		return ""
	}
	gen, ok := h.captchaSvc.(captchaGenerator)
	if !ok {
		return ""
	}
	c, err := gen.Generate(r.Context())
	if err != nil {
		slog.Warn("登录页验证码生成失败", "error", err)
		return ""
	}
	return `<label for="captcha_answer">验证码：` + htmlEscape(c.Question) + `</label>
<input type="text" id="captcha_answer" name="captcha_answer" required autocomplete="off">
<input type="hidden" name="captcha_id" value="` + htmlEscape(c.ID) + `">`
}

func errorHTML(msg string) string {
	if msg == "" {
		return ""
	}
	return `<div class="err">` + htmlEscape(msg) + `</div>`
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;").Replace(s)
}

// HandleSubmit 处理表单登录
// POST /ui/login  (application/x-www-form-urlencoded)
// 成功：下发 sso_session cookie（HttpOnly）并 302 回 redirect
func (h *LoginPageHandler) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Security-Policy", loginPageCSP)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	redirect := safeRedirect(r.Form.Get("redirect"))
	email := strings.TrimSpace(r.Form.Get("email"))
	password := r.Form.Get("password")
	back := "/ui/login?redirect=" + url.QueryEscape(redirect)

	if email == "" || password == "" {
		http.Redirect(w, r, back+"&error="+url.QueryEscape("请输入邮箱和密码"), http.StatusFound)
		return
	}

	clientIP := extractClientIP(r)

	// 图形验证码（IP 与账号双维度触发判定）——与 API 登录一致，
	// 但从表单字段读取（浏览器表单无法携带 X-Captcha-* 头），
	// 失败一律 302 回登录页并强制渲染验证码输入区（captcha=1）
	if h.captchaSvc.IsEnabled() &&
		(h.captchaSvc.ShouldRequireCaptcha(r.Context(), clientIP) ||
			h.captchaSvc.ShouldRequireCaptchaForAccount(r.Context(), email)) {
		captchaID := r.Form.Get("captcha_id")
		captchaAnswer := strings.TrimSpace(r.Form.Get("captcha_answer"))
		if captchaID == "" || captchaAnswer == "" {
			http.Redirect(w, r, back+"&captcha=1&error="+url.QueryEscape("请输入验证码"), http.StatusFound)
			return
		}
		ok, verr := h.captchaSvc.Verify(r.Context(), captchaID, captchaAnswer)
		if verr != nil || !ok {
			http.Redirect(w, r, back+"&captcha=1&error="+url.QueryEscape("验证码错误或已过期"), http.StatusFound)
			return
		}
	}

	auditCtx := &service.AuditContext{
		IPAddress: clientIP,
		UserAgent: r.UserAgent(),
	}
	resp, err := h.authSvc.LoginWithAudit(r.Context(), &model.LoginRequest{Email: email, Password: password}, auditCtx)
	if err != nil {
		if isCredentialError(err) {
			h.captchaSvc.RecordFailure(r.Context(), auditCtx.IPAddress)
			h.captchaSvc.RecordAccountFailure(r.Context(), email)
		}
		http.Redirect(w, r, back+"&error="+url.QueryEscape("邮箱或密码错误"), http.StatusFound)
		return
	}
	h.captchaSvc.ClearFailures(r.Context(), auditCtx.IPAddress)
	h.captchaSvc.ClearAccountFailures(r.Context(), email)

	if resp.MFARequired {
		http.Redirect(w, r, back+"&error="+url.QueryEscape("该账号启用了 MFA，请通过 API 登录"), http.StatusFound)
		return
	}
	if resp.AccessToken == "" {
		http.Redirect(w, r, back+"&error="+url.QueryEscape("登录失败：未获取到访问令牌"), http.StatusFound)
		return
	}

	// #nosec G124 -- Secure 按环境设置：生产强制 true；开发环境 http 下 Secure=true 浏览器会拒存 Cookie
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.BrowserSessionCookie,
		Value:    resp.AccessToken,
		Path:     "/",
		MaxAge:   resp.ExpiresIn,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})
	// redirect 从表单解析时其内部 query 的 %20 等已被解码（如 scope=openid email profile），
	// 直接作为 Location 会导致 302 携带未编码空格、浏览器跟随失败。
	// 重新编码 query 后再跳转（语义不变：空格 → + 或 %20）。
	if u, err := url.Parse(redirect); err == nil {
		u.RawQuery = u.Query().Encode()
		redirect = u.String()
	}
	slog.Info("浏览器表单登录成功", "email", email, "ip", auditCtx.IPAddress)
	http.Redirect(w, r, redirect, http.StatusFound)
}
