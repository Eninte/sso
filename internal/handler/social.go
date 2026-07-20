// Package handler 第三方登录处理器
// 处理OAuth2第三方登录（阶段 2.2 改造：mux.Vars + 统一错误处理）
package handler

import (
	"net/http"

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
}

// NewSocialLoginHandler 创建第三方登录处理器
func NewSocialLoginHandler(socialSvc service.SocialLoginServiceInterface) *SocialLoginHandler {
	return &SocialLoginHandler{socialSvc: socialSvc}
}

// HandleLogin 处理第三方登录请求
// GET /auth/{provider}
//
// 阶段 2.3 改造：使用 mux.Vars 解析 provider（替代脆弱的字符串切片）
func (h *SocialLoginHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// 1. 从路径变量获取 provider
	vars := mux.Vars(r)
	provider := vars["provider"]
	if provider == "" {
		handlerutil.WriteValidationError(w, "provider", getMessage(r, apperrors.ErrCodeUnsupportedLoginMethod))
		return
	}

	// 2. 获取状态参数（客户端可选传入，未传则由 service 生成）
	state := r.URL.Query().Get("state")

	// 3. 获取授权URL（redirectURI由服务端固定，不接受客户端传入）
	authURL, err := h.socialSvc.GetAuthorizationURL(provider, state)
	if err != nil {
		handlerutil.WriteJSONError(w, err)
		return
	}

	// 4. 重定向到授权URL
	// authURL 由服务层从预配置的 OAuth 提供商端点构造，
	// provider 已在 GetAuthorizationURL 内白名单验证，非用户可控的开放重定向
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect) // nosec G710
}

// HandleCallback 处理OAuth回调
// GET /auth/{provider}/callback
//
// 阶段 2.3 改造：
//   - 使用 mux.Vars 解析 provider
//   - 使用 handlerutil.WriteJSONError 统一错误响应
//   - 区分错误类型返回合适的 HTTP 状态码
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

	// 4. 处理回调（验证state，redirectURI从state缓存中获取防止开放重定向）
	token, err := h.socialSvc.HandleCallback(r.Context(), provider, code, state)
	if err != nil {
		// 阶段 2.3：使用 handlerutil.WriteJSONError 统一错误响应
		// 错误已带 HTTP 状态码（apperrors.New 指定）
		handlerutil.WriteJSONError(w, err)
		return
	}

	// 5. 返回Token
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
