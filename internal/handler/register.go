// Package handler 注册处理器
// 处理用户注册请求
package handler

import (
	"errors"
	"net/http"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/validator"
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
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidRequestFormat))
		return
	}

	// 2. 调用注册服务
	user, err := h.authSvc.Register(r.Context(), &req)
	if err != nil {
		// 处理已知错误 - 使用errors包的错误而不是store包的错误
		if errors.Is(err, apperrors.ErrEmailExists) {
			writeError(w, http.StatusConflict, getMessage(r, apperrors.ErrCodeEmailExists))
			return
		}
		// 处理验证错误
		if errors.Is(err, validator.ErrEmailInvalid) || errors.Is(err, validator.ErrEmailRequired) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeEmailInvalid))
			return
		}
		// 密码验证错误 - 使用本地化消息
		if errors.Is(err, validator.ErrPasswordTooShort) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodePasswordTooShort))
			return
		}
		if errors.Is(err, validator.ErrPasswordTooLong) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodePasswordTooLong))
			return
		}
		if errors.Is(err, validator.ErrPasswordRequired) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodePasswordRequired))
			return
		}
		if errors.Is(err, validator.ErrPasswordNoUppercase) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodePasswordNoUppercase))
			return
		}
		if errors.Is(err, validator.ErrPasswordNoLowercase) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodePasswordNoLowercase))
			return
		}
		if errors.Is(err, validator.ErrPasswordNoDigit) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodePasswordNoDigit))
			return
		}
		if errors.Is(err, validator.ErrPasswordNoSpecial) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodePasswordNoSpecial))
			return
		}
		if errors.Is(err, service.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeInvalidCredentials))
			return
		}
		// 未知错误
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeRegisterFailed))
		return
	}

	// 3. 返回成功响应
	writeSuccess(w, http.StatusCreated, "注册成功", map[string]string{
		"user_id": user.ID,
		"email":   user.Email,
	})
}
