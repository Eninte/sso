// Package handler Token处理器
// 处理Token签发、刷新和撤销
package handler

import (
	"errors"
	"net/http"
	"strings"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
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
		h.handleRefreshToken(w, r, req.RefreshToken)
	case model.GrantTypeAuthorizationCode:
		h.handleAuthorizationCode(w, r, &req)
	default:
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeUnsupportedGrantType))
	}
}

// handleRefreshToken 处理刷新Token请求
func (h *TokenHandler) handleRefreshToken(w http.ResponseWriter, r *http.Request, refreshToken string) {
	if refreshToken == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingRefreshToken))
		return
	}

	resp, err := h.authSvc.RefreshToken(r.Context(), refreshToken)
	if err != nil {
		if errors.Is(err, service.ErrInvalidToken) {
			writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeInvalidRefreshToken))
			return
		}
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeRefreshTokenFailed))
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleAuthorizationCode 处理授权码交换
// 支持PKCE验证
func (h *TokenHandler) handleAuthorizationCode(w http.ResponseWriter, r *http.Request, req *model.TokenRequest) {
	// 验证必需参数
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingCode))
		return
	}
	if req.ClientID == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingClientID))
		return
	}
	if req.RedirectURI == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingRedirectURI))
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
		if errors.Is(err, service.ErrInvalidCode) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidCode))
			return
		}
		if errors.Is(err, service.ErrCodeExpired) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeCodeExpired))
			return
		}
		if errors.Is(err, service.ErrCodeUsed) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeCodeUsed))
			return
		}
		if errors.Is(err, service.ErrInvalidClient) {
			writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeInvalidClient))
			return
		}
		if errors.Is(err, service.ErrInvalidCodeVerifier) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidCodeVerifier))
			return
		}
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeExchangeCodeFailed))
		return
	}

	// 返回Token响应
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  token.AccessToken,
		"refresh_token": token.RefreshToken,
		"token_type":    "Bearer",
		"expires_in":    900,
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
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingToken))
		return
	}

	if err := h.authSvc.Logout(r.Context(), req.Token); err != nil {
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeRevokeTokenFailed))
		return
	}

	writeSuccess(w, http.StatusOK, "Token已撤销", nil)
}
