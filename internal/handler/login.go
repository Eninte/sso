// Package handler 登录处理器
// 处理用户登录请求
package handler

import (
	"errors"
	"net/http"

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
		writeError(w, http.StatusBadRequest, "无效的请求格式")
		return
	}

	// 2. 调用登录服务
	resp, err := h.authSvc.Login(r.Context(), &req)
	if err != nil {
		// 处理已知错误
		if errors.Is(err, service.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "邮箱或密码错误")
			return
		}
		if errors.Is(err, service.ErrAccountLocked) {
			writeError(w, http.StatusForbidden, "账户已锁定，请30分钟后再试")
			return
		}
		if errors.Is(err, service.ErrAccountDisabled) {
			writeError(w, http.StatusForbidden, "账户已被禁用")
			return
		}
		// 未知错误
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}

	// 3. 返回Token
	writeJSON(w, http.StatusOK, resp)
}
