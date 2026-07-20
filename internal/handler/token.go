// Package handler Token处理器
// 处理Token签发、刷新和撤销
package handler

import (
	"log/slog"
	"net/http"
	"strings"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/logging"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/util/handlerutil"
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
		h.handleRefreshToken(w, r, &req)
	case model.GrantTypeAuthorizationCode:
		h.handleAuthorizationCode(w, r, &req)
	default:
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeUnsupportedGrantType))
	}
}

// handleRefreshToken 处理刷新Token请求（阶段 2.2：携带 client_id 校验归属）
func (h *TokenHandler) handleRefreshToken(w http.ResponseWriter, r *http.Request, req *model.TokenRequest) {
	slog.Debug("handleRefreshToken: 开始处理", "refresh_token_length", len(req.RefreshToken))
	if req.RefreshToken == "" {
		slog.Warn("handleRefreshToken: refresh_token为空")
		handlerutil.WriteValidationError(w, "refresh_token", getMessage(r, apperrors.ErrCodeMissingRefreshToken))
		return
	}

	// 阶段 2.2：调用 RefreshTokenWithClientID 校验 token 客户端归属
	// 若 token 由 OAuth 流程签发（含 ClientID），则 req.ClientID 必须与之一致
	// 登录流程签发的 token（ClientID 为 nil）不要求 req.ClientID
	slog.Debug("handleRefreshToken: 调用authSvc.RefreshTokenWithClientID")
	resp, err := h.authSvc.RefreshTokenWithClientID(r.Context(), req.RefreshToken, req.ClientID)
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
//
// 阶段 B 审查修复（H3 + L6）：
//   - H3: 增加 token 所有权校验，防止已认证用户撤销他人 token（RFC 7009 §2.1）
//   - L6: 返回 204 No Content 空响应体（RFC 7009 §2.2 规定响应体应为空）
//
// 即使 token 不存在或不属于当前用户，也返回 204 不暴露存在性（RFC 7009 §2.2）。
// 仅在 JSON 解析失败或 token 字段为空时返回 400。
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

	// 阶段 B 审查修复（H3）：token 所有权校验
	// 当前认证用户 ID（AuthMiddleware 从 JWT claims 注入）
	authenticatedUserID := middleware.GetUserIDFromContext(r.Context())
	if authenticatedUserID == "" {
		// 极端情况：AuthMiddleware 未注入 user_id（应不可能，已 protected 路由）
		slog.Error("HandleRevoke: 认证用户ID为空")
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	tokenUserID, err := h.authSvc.GetTokenOwnerID(r.Context(), req.Token)
	if err != nil {
		// DB 错误：fail-closed，记录日志但返回 204 不暴露内部错误
		slog.Error("HandleRevoke: 查询token所有者失败",
			"error", logging.SanitizeDBURL(err.Error()),
			"authenticated_user_id", authenticatedUserID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if tokenUserID == "" {
		// token 不存在：按 RFC 7009 §2.2 返回 204 不暴露存在性
		slog.Info("HandleRevoke: token不存在", "authenticated_user_id", authenticatedUserID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if tokenUserID != authenticatedUserID {
		// 所有权不匹配：记录可疑活动，但返回 204 不暴露存在性
		// 防止已认证恶意用户枚举或撤销他人 token
		slog.Warn("HandleRevoke: token所有权不匹配，拒绝撤销",
			"authenticated_user_id", authenticatedUserID,
			"token_user_id", tokenUserID)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// 所有权校验通过，执行撤销
	if err := h.authSvc.Logout(r.Context(), req.Token); err != nil {
		// 撤销失败：仍返回 204 不暴露内部错误（RFC 7009 §2.2）
		slog.Error("HandleRevoke: 撤销失败",
			"error", logging.SanitizeDBURL(err.Error()),
			"user_id", authenticatedUserID)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// 撤销成功：返回 204 No Content 空响应体
	w.WriteHeader(http.StatusNoContent)
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
