// Package handler Token处理器
// 处理Token签发、刷新和撤销
package handler

import (
	"log/slog"
	"net/http"
	"strings"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/util/handlerutil"
)

// ============================================================================
// TokenHandler Token处理器
// ============================================================================

// TokenHandler Token处理器
type TokenHandler struct {
	authSvc  service.AuthServiceInterface
	oauthSvc service.OAuthServiceInterface
}

// NewTokenHandler 创建Token处理器
func NewTokenHandler(authSvc service.AuthServiceInterface, oauthSvc service.OAuthServiceInterface) *TokenHandler {
	return &TokenHandler{
		authSvc:  authSvc,
		oauthSvc: oauthSvc,
	}
}

// HandleToken 处理Token请求
// POST /api/v1/token
func (h *TokenHandler) HandleToken(w http.ResponseWriter, r *http.Request) {
	var req model.TokenRequest
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	// 根据授权类型分发处理
	switch req.GrantType {
	case model.GrantTypeRefreshToken:
		slog.Debug("HandleToken: 收到刷新Token请求", "refresh_token_length", len(req.RefreshToken))
		h.handleRefreshToken(w, r, req.RefreshToken)
	case model.GrantTypeAuthorizationCode:
		h.handleAuthorizationCode(w, r, &req)
	default:
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeUnsupportedGrantType))
	}
}

// handleRefreshToken 处理刷新Token请求
func (h *TokenHandler) handleRefreshToken(w http.ResponseWriter, r *http.Request, refreshToken string) {
	slog.Debug("handleRefreshToken: 开始处理", "refresh_token_length", len(refreshToken))
	if refreshToken == "" {
		slog.Warn("handleRefreshToken: refresh_token为空")
		handlerutil.WriteValidationError(w, "refresh_token", getMessage(r, apperrors.ErrCodeMissingRefreshToken))
		return
	}

	slog.Debug("handleRefreshToken: 调用authSvc.RefreshToken")
	resp, err := h.authSvc.RefreshToken(r.Context(), refreshToken)
	if err != nil {
		slog.Error("handleRefreshToken: 刷新失败", "error", err)
		handlerutil.WriteJSONError(w, err)
		return
	}

	slog.Debug("handleRefreshToken: 刷新成功")
	writeJSON(w, http.StatusOK, resp)
}

// handleAuthorizationCode 处理授权码交换
// 支持PKCE验证
func (h *TokenHandler) handleAuthorizationCode(w http.ResponseWriter, r *http.Request, req *model.TokenRequest) {
	// 验证必需参数
	if req.Code == "" {
		handlerutil.WriteValidationError(w, "code", getMessage(r, apperrors.ErrCodeMissingCode))
		return
	}
	if req.ClientID == "" {
		handlerutil.WriteValidationError(w, "client_id", getMessage(r, apperrors.ErrCodeMissingClientID))
		return
	}
	if req.RedirectURI == "" {
		handlerutil.WriteValidationError(w, "redirect_uri", getMessage(r, apperrors.ErrCodeMissingRedirectURI))
		return
	}

	// 交换授权码获取Token
	token, err := h.oauthSvc.ExchangeAuthorizationCode(
		r.Context(),
		req.Code,
		req.ClientID,
		req.ClientSecret,
		req.RedirectURI,
		req.CodeVerifier,
	)
	if err != nil {
		handlerutil.WriteJSONError(w, err)
		return
	}

	// 返回Token响应
	handlerutil.WriteJSONSuccess(w, map[string]interface{}{
		"access_token":  token.AccessToken,
		"refresh_token": token.RefreshToken,
		"token_type":    "Bearer",
		"expires_in":    int(h.oauthSvc.GetAccessTokenTTL().Seconds()),
		"scope":         strings.Join(token.Scopes, " "),
	})
}

// HandleRevoke 处理Token撤销请求
// POST /api/v1/token/revoke
func (h *TokenHandler) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	if req.Token == "" {
		handlerutil.WriteValidationError(w, "token", getMessage(r, apperrors.ErrCodeMissingToken))
		return
	}

	if err := h.authSvc.Logout(r.Context(), req.Token); err != nil {
		handlerutil.WriteJSONError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, "Token已撤销", nil)
}

// HandleLogoutAll 处理登出所有设备请求
// POST /api/v1/logout-all
func (h *TokenHandler) HandleLogoutAll(w http.ResponseWriter, r *http.Request) {
	// 从上下文获取用户ID（由认证中间件设置）
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	// 撤销用户所有Token
	if err := h.authSvc.LogoutAll(r.Context(), userID); err != nil {
		handlerutil.WriteJSONError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, "已登出所有设备", nil)
}
