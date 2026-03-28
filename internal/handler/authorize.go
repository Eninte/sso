// Package handler 授权处理器
// 处理OAuth2授权请求
package handler

import (
	"net/http"
	"strings"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/service"
)

// ============================================================================
// AuthorizeHandler 授权处理器
// ============================================================================

// AuthorizeHandler 授权处理器
type AuthorizeHandler struct {
	oauthSvc service.OAuthServiceInterface
}

// NewAuthorizeHandler 创建授权处理器
func NewAuthorizeHandler(oauthSvc service.OAuthServiceInterface) *AuthorizeHandler {
	return &AuthorizeHandler{oauthSvc: oauthSvc}
}

// HandleAuthorize 处理授权请求
// GET /authorize
// 显示授权页面或直接返回授权码
func (h *AuthorizeHandler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	// 1. 获取请求参数
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")

	// 2. 验证必需参数
	if clientID == "" || redirectURI == "" || responseType != "code" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeBadRequest))
		return
	}

	// 2.1 验证 state 参数（CSRF 防护）
	if state == "" || len(state) < 16 {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeStateInvalid))
		return
	}

	// 3. 解析scope
	scopes := strings.Split(scope, " ")

	// 4. 获取当前登录用户 (如果已登录)
	userID := middleware.GetUserIDFromContext(r.Context())

	// 5. 如果用户未登录，返回登录提示
	if userID == "" {
		// 返回需要登录的响应
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error":             "login_required",
			"error_description": "请先登录",
		})
		return
	}

	// 6. 创建授权码
	code, err := h.oauthSvc.CreateAuthorizationCode(
		r.Context(),
		clientID,
		userID,
		redirectURI,
		scopes,
		codeChallenge,
		codeChallengeMethod,
	)
	if err != nil {
		writeOAuthError(w, r, err)
		return
	}

	// 7. 返回授权码
	writeJSON(w, http.StatusOK, map[string]string{
		"code":  code,
		"state": state,
	})
}

// HandleApprove 处理用户授权确认
// POST /authorize/approve
func (h *AuthorizeHandler) HandleApprove(w http.ResponseWriter, r *http.Request) {
	// 1. 获取当前登录用户
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	// 2. 解析请求
	var req struct {
		ClientID            string `json:"client_id"`
		RedirectURI         string `json:"redirect_uri"`
		Scope               string `json:"scope"`
		State               string `json:"state"`
		CodeChallenge       string `json:"code_challenge"`
		CodeChallengeMethod string `json:"code_challenge_method"`
	}
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	// 2.1 验证 state 参数（CSRF 防护）
	if req.State == "" || len(req.State) < 16 {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeStateInvalid))
		return
	}

	// 3. 解析scope
	scopes := strings.Split(req.Scope, " ")

	// 4. 创建授权码
	code, err := h.oauthSvc.CreateAuthorizationCode(
		r.Context(),
		req.ClientID,
		userID,
		req.RedirectURI,
		scopes,
		req.CodeChallenge,
		req.CodeChallengeMethod,
	)
	if err != nil {
		writeOAuthError(w, r, err)
		return
	}

	// 5. 返回授权码
	writeJSON(w, http.StatusOK, map[string]string{
		"code":  code,
		"state": req.State,
	})
}
