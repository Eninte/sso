// Package handler 第三方登录处理器
// 处理OAuth2第三方登录
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/your-org/sso/internal/service"
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
func (h *SocialLoginHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// 1. 获取提供商名称
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		// 从URL路径获取
		// /auth/google -> google
		path := r.URL.Path
		if len(path) > 7 { // /auth/ = 7 chars
			provider = path[7:]
		}
	}

	// 2. 获取回调URL
	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		redirectURI = r.Referer()
	}

	// 3. 获取状态参数
	state := r.URL.Query().Get("state")

	// 4. 获取授权URL
	authURL, err := h.socialSvc.GetAuthorizationURL(provider, redirectURI, state)
	if err != nil {
		writeError(w, http.StatusBadRequest, "不支持的登录方式")
		return
	}

	// 5. 重定向到授权URL
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// HandleCallback 处理OAuth回调
// GET /auth/{provider}/callback
func (h *SocialLoginHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// 1. 获取提供商名称
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		path := r.URL.Path
		// /auth/google/callback -> google
		if len(path) > 17 { // /auth/ + /callback = 17 chars
			provider = path[7 : len(path)-9]
		}
	}

	// 2. 获取授权码
	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "缺少授权码")
		return
	}

	// 3. 获取回调URL
	redirectURI := r.URL.Query().Get("redirect_uri")

	// 4. 处理回调
	token, err := h.socialSvc.HandleCallback(r.Context(), provider, code, redirectURI)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}

	// 5. 返回Token
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(providers)
}
