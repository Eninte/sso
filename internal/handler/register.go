// Package handler 注册处理器
// 处理用户注册请求
package handler

import (
	"net/http"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
)

// ============================================================================
// RegisterHandler 注册处理器
// ============================================================================

// RegisterHandler 注册处理器
type RegisterHandler struct {
	authSvc service.AuthServiceInterface
}

// NewRegisterHandler 创建注册处理器
func NewRegisterHandler(authSvc service.AuthServiceInterface) *RegisterHandler {
	return &RegisterHandler{authSvc: authSvc}
}

// Handle 处理注册请求
// POST /api/v1/register
func (h *RegisterHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// 1. 解析请求体 (带大小限制)
	var req model.RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	// 2. 调用注册服务
	_, err := h.authSvc.Register(r.Context(), &req)
	if err != nil {
		// 使用统一的错误处理函数
		if !writeValidationError(w, r, err) {
			// 未知错误
			writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeRegisterFailed))
		}
		return
	}

	// 3. 返回成功响应（不暴露user_id，不区分邮箱是否已存在）
	// user为nil表示邮箱已注册，返回相同响应防止用户枚举
	writeSuccess(w, http.StatusCreated, "注册成功，如果邮箱未验证将收到验证邮件", nil)
}
