// Package handler 第三方登录处理器
// 处理OAuth2第三方登录（阶段 2.2 改造：mux.Vars + 统一错误处理）
package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/util/handlerutil"
)

// ============================================================================
// SocialLoginHandler 第三方登录处理器
// ============================================================================

// SocialLoginHandler 第三方登录处理器
type SocialLoginHandler struct {
	socialSvc service.SocialLoginServiceInterface

	// T11（M2）：state 会话绑定（login CSRF 防护）
	stateBindingEnabled bool
	stateHMACKey        []byte // state 指纹 HMAC 密钥
	stateCookieSecure   bool   // 生产环境 Cookie 加 Secure
}

// 社交登录 state 指纹 Cookie 配置（T11）
const (
	// socialStateCookieName state 指纹 Cookie 名
	socialStateCookieName = "sso_social_state"
	// socialStateCookieMaxAge Cookie 有效期（与 service.OAuthStateTTL 一致，5 分钟）
	socialStateCookieMaxAge = 5 * 60
)

// NewSocialLoginHandler 创建第三方登录处理器
func NewSocialLoginHandler(socialSvc service.SocialLoginServiceInterface) *SocialLoginHandler {
	return &SocialLoginHandler{socialSvc: socialSvc}
}

// WithStateBinding 启用 state 会话绑定（T11：login CSRF 防护）
//
// 启用后：发起端忽略客户端传入的 state（始终由服务端生成），并下发
// HttpOnly + SameSite=Lax 的 state 指纹 Cookie；回调端校验 Cookie 指纹
// 与 state 匹配（ConstantTimeCompare），无 Cookie 或不匹配一律拒绝。
//
// hmacKey 复用 MFA_RECOVERY_HMAC_KEY（生产必填的共享配置密钥）：
// 多副本部署下发起与回调可能落在不同实例，进程级随机密钥会导致跨实例
// 回调失败，因此必须选用跨实例一致的配置密钥。
// hmacKey 为空（开发环境未配置）时退化为进程级随机密钥并 Warn（仅限单实例）。
// secure 为 true 时 Cookie 带 Secure 属性（生产 HTTPS）。
func (h *SocialLoginHandler) WithStateBinding(enabled bool, hmacKey []byte, secure bool) *SocialLoginHandler {
	h.stateBindingEnabled = enabled
	h.stateCookieSecure = secure
	if !enabled {
		return h
	}
	if len(hmacKey) == 0 {
		// 开发环境兜底：进程级随机密钥（多副本部署下跨实例回调会失败，仅单实例可用）
		hmacKey = make([]byte, 32)
		if _, err := rand.Read(hmacKey); err != nil {
			// crypto/rand 失败属极端异常，拒绝启用绑定而非静默弱化
			slog.Error("生成 state 指纹密钥失败，state 会话绑定未启用", "error", err)
			h.stateBindingEnabled = false
			return h
		}
		slog.Warn("state 指纹使用进程级随机密钥（MFA_RECOVERY_HMAC_KEY 未配置），多实例部署下回调将失败")
	}
	h.stateHMACKey = hmacKey
	return h
}

// stateFingerprint 计算 state 的 HMAC-SHA256 指纹（hex）
func (h *SocialLoginHandler) stateFingerprint(state string) string {
	mac := hmac.New(sha256.New, h.stateHMACKey)
	mac.Write([]byte(state))
	return hex.EncodeToString(mac.Sum(nil))
}

// setStateFingerprintCookie 下发 state 指纹 Cookie
// HttpOnly + SameSite=Lax（OAuth 重定向回来的顶层 GET 导航会携带）+ 生产 Secure
func (h *SocialLoginHandler) setStateFingerprintCookie(w http.ResponseWriter, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     socialStateCookieName,
		Value:    h.stateFingerprint(state),
		Path:     "/auth",
		MaxAge:   socialStateCookieMaxAge,
		HttpOnly: true,
		Secure:   h.stateCookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// verifyStateFingerprint 校验回调 state 与 Cookie 指纹匹配（ConstantTimeCompare）
func (h *SocialLoginHandler) verifyStateFingerprint(r *http.Request, state string) bool {
	cookie, err := r.Cookie(socialStateCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	expected := h.stateFingerprint(state)
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(expected)) == 1
}

// HandleLogin 处理第三方登录请求
// GET /auth/{provider}
//
// 阶段 2.3 改造：使用 mux.Vars 解析 provider（替代脆弱的字符串切片）
// T11（M2）：state 绑定启用时忽略客户端传入的 state，始终由服务端生成
func (h *SocialLoginHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// 1. 从路径变量获取 provider
	vars := mux.Vars(r)
	provider := vars["provider"]
	if provider == "" {
		handlerutil.WriteValidationError(w, "provider", getMessage(r, apperrors.ErrCodeUnsupportedLoginMethod))
		return
	}

	// 2. 获取状态参数
	// T11：绑定启用时不再信任客户端传入的 state（login CSRF 防护），
	// 传空由 service 生成 32 字节随机 state；绑定关闭时保持旧行为（兼容纯 API 客户端）
	state := r.URL.Query().Get("state")
	if h.stateBindingEnabled {
		state = ""
	}

	// 3. 获取授权URL（redirectURI由服务端固定，不接受客户端传入）
	authURL, err := h.socialSvc.GetAuthorizationURL(provider, state)
	if err != nil {
		handlerutil.WriteJSONError(w, err)
		return
	}

	// 4. T11：下发 state 指纹 Cookie（值为服务端生成 state 的 HMAC）
	if h.stateBindingEnabled {
		if generatedState := extractStateFromAuthURL(authURL); generatedState != "" {
			h.setStateFingerprintCookie(w, generatedState)
		}
	}

	// 5. 重定向到授权URL
	// authURL 由服务层从预配置的 OAuth 提供商端点构造，
	// provider 已在 GetAuthorizationURL 内白名单验证，非用户可控的开放重定向
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect) // nosec G710
}

// extractStateFromAuthURL 从授权 URL 中提取服务端生成的 state 参数
func extractStateFromAuthURL(authURL string) string {
	u, err := url.Parse(authURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("state")
}

// HandleCallback 处理OAuth回调
// GET /auth/{provider}/callback
//
// 阶段 2.3 改造：
//   - 使用 mux.Vars 解析 provider
//   - 使用 handlerutil.WriteJSONError 统一错误响应
//   - 区分错误类型返回合适的 HTTP 状态码
//
// T11（M2）：state 绑定启用时校验 Cookie 指纹与 state 匹配，
// 无 Cookie 或指纹不匹配一律拒绝（login CSRF 防护）
func (h *SocialLoginHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// 1. 从路径变量获取 provider
	vars := mux.Vars(r)
	provider := vars["provider"]
	if provider == "" {
		handlerutil.WriteValidationError(w, "provider", getMessage(r, apperrors.ErrCodeUnsupportedLoginMethod))
		return
	}

	// 2. 获取授权码
	code := r.URL.Query().Get("code")
	if code == "" {
		handlerutil.WriteValidationError(w, "code", getMessage(r, apperrors.ErrCodeMissingAuthCode))
		return
	}

	// 3. 获取state参数（用于CSRF防护）
	state := r.URL.Query().Get("state")
	if state == "" {
		handlerutil.WriteValidationError(w, "state", getMessage(r, apperrors.ErrCodeOAuthStateInvalid))
		return
	}

	// 4. T11：校验 state 指纹 Cookie（会话绑定，防 login CSRF）
	if h.stateBindingEnabled && !h.verifyStateFingerprint(r, state) {
		slog.Warn("社交登录回调 state 指纹校验失败，拒绝请求", "provider", provider)
		handlerutil.WriteJSONError(w, apperrors.ErrOAuthStateInvalid)
		return
	}

	// 5. 处理回调（验证state，redirectURI从state缓存中获取防止开放重定向）
	token, err := h.socialSvc.HandleCallback(r.Context(), provider, code, state)
	if err != nil {
		// 阶段 2.3：使用 handlerutil.WriteJSONError 统一错误响应
		// 错误已带 HTTP 状态码（apperrors.New 指定）
		handlerutil.WriteJSONError(w, err)
		return
	}

	// 6. 返回Token
	// 注：当前 token 通过 JSON 返回，后续阶段会改造为 HttpOnly Cookie
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  token.AccessToken,
		"refresh_token": token.RefreshToken,
		"token_type":    "Bearer",
		"expires_in":    900,
	})
}

// HandleProviders 处理获取支持的提供商列表
// GET /auth/providers
func (h *SocialLoginHandler) HandleProviders(w http.ResponseWriter, r *http.Request) {
	// 返回支持的提供商列表
	providers := []map[string]string{
		{
			"name":  "google",
			"label": "Google",
			"icon":  "https://www.google.com/favicon.ico",
		},
		{
			"name":  "github",
			"label": "GitHub",
			"icon":  "https://github.com/favicon.ico",
		},
	}

	writeJSON(w, http.StatusOK, providers)
}
