// Package handler 授权处理器
// 处理OAuth2授权请求（阶段 2.2 引入 consent 流程）
package handler

import (
	"net/http"
	"strings"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/util/handlerutil"
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

// HandleAuthorize 处理授权请求（阶段 2.2 改造为 consent 流程）
//
// GET /api/v1/authorize
//
// 流程：
//  1. 校验请求参数（client_id、redirect_uri、response_type、state、scope、PKCE）
//  2. 校验用户已登录
//  3. 调用 oauthSvc.IssueConsentToken 签发短期 consent_token
//  4. 返回 consent_token 与授权上下文给前端展示授权页面
//
// 前端拿到 consent_token 后渲染授权页面，用户批准后通过
// POST /api/v1/authorize/approve 携带 consent_token 创建授权码
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

	// 4. 获取当前登录用户（如果已登录）
	userID := middleware.GetUserIDFromContext(r.Context())

	// 5. 如果用户未登录，返回登录提示
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error":             "login_required",
			"error_description": "请先登录",
		})
		return
	}

	// 6. 签发 consent_token（阶段 2.2）
	// service 层会校验 client_id 存在性、redirect_uri 合法性、scope 在白名单与客户端允许范围内
	// PKCE 在公共客户端被强制要求 S256
	consentToken, err := h.oauthSvc.IssueConsentToken(
		r.Context(),
		userID,
		clientID,
		redirectURI,
		scopes,
		state,
		codeChallenge,
		codeChallengeMethod,
	)
	if err != nil {
		writeOAuthError(w, r, err)
		return
	}

	// 7. 返回 consent_token 与授权上下文
	// 前端拿到后渲染授权页面，用户决定是否批准
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"consent_token":    consentToken,
		"client_id":        clientID,
		"redirect_uri":     redirectURI,
		"scope":            scope,
		"state":            state,
		"require_approval": true,
	})
}

// HandleApprove 处理用户授权确认（阶段 2.2 改造为 consent_token 流程）
//
// POST /api/v1/authorize/approve
//
// 流程：
//  1. 校验用户已登录
//  2. 接收 consent_token
//  3. 调用 oauthSvc.CreateAuthorizationCodeWithConsent 创建授权码
//  4. 返回授权码与 state
func (h *AuthorizeHandler) HandleApprove(w http.ResponseWriter, r *http.Request) {
	// 1. 获取当前登录用户
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	// 2. 解析请求（仅需 consent_token）
	var req struct {
		ConsentToken string `json:"consent_token"`
		State        string `json:"state"`
	}
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	// 3. 校验 consent_token 必填
	if req.ConsentToken == "" {
		handlerutil.WriteValidationError(w, "consent_token", getMessage(r, apperrors.ErrCodeConsentRequired))
		return
	}

	// 4. 校验 state 必填（consent_token 内部也有 state，但客户端必须显式回传以便对比）
	if req.State == "" || len(req.State) < 16 {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeStateInvalid))
		return
	}

	// 5. 通过 consent_token 创建授权码
	// 阶段 D 审查修复（H1）：传入请求 state 用于与 consent_token 内 state 对比，
	// 防止 GET /authorize 与 POST /authorize/approve 之间 state 被替换
	code, err := h.oauthSvc.CreateAuthorizationCodeWithConsent(r.Context(), userID, req.ConsentToken, req.State)
	if err != nil {
		writeOAuthError(w, r, err)
		return
	}

	// 6. 返回授权码与 state
	writeJSON(w, http.StatusOK, map[string]string{
		"code":  code,
		"state": req.State,
	})
}

// HandleDeny 处理用户拒绝授权（阶段 2.2 新增）
//
// POST /api/v1/authorize/deny
//
// 用户在 consent 页面选择拒绝时调用，返回 access_denied 错误
// 前端应据此向客户端应用回传 error=access_denied
func (h *AuthorizeHandler) HandleDeny(w http.ResponseWriter, r *http.Request) {
	// 1. 获取当前登录用户
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	// 2. 解析请求（仅需 consent_token 与 state）
	var req struct {
		ConsentToken string `json:"consent_token"`
		State        string `json:"state"`
	}
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	// 3. 返回 access_denied 错误响应
	// 前端拿到此响应后应回传客户端 ?error=access_denied&state=xxx
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error":             "access_denied",
		"error_description": "用户拒绝授权",
		"state":             req.State,
	})
}
