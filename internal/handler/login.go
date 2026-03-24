// Package handler 登录处理器
// 处理用户登录请求
package handler

import (
	"net/http"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
)

// ============================================================================
// LoginHandler 登录处理器
// ============================================================================

// LoginHandler 登录处理器
type LoginHandler struct {
	authSvc service.AuthServiceInterface
}

// NewLoginHandler 创建登录处理器
func NewLoginHandler(authSvc service.AuthServiceInterface) *LoginHandler {
	return &LoginHandler{authSvc: authSvc}
}

// Handle 处理登录请求
// POST /api/v1/login
func (h *LoginHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// 1. 解析请求体 (带大小限制)
	var req model.LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidRequestFormat))
		return
	}

	// 2. 调用登录服务
	resp, err := h.authSvc.Login(r.Context(), &req)
	if err != nil {
		// 统一处理所有错误
		writeOAuthError(w, r, err)
		return
	}

	// 3. 返回Token
	writeJSON(w, http.StatusOK, resp)
}
