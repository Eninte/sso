// Package handler 用户处理器
// 处理用户相关的HTTP请求
package handler

import (
	"net/http"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/service"
)

// ============================================================================
// UserHandler 用户处理器
// ============================================================================

// UserHandler 用户处理器
type UserHandler struct {
	userSvc service.UserServiceInterface
}

// NewUserHandler 创建用户处理器
func NewUserHandler(userSvc service.UserServiceInterface) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

// HandleSendVerificationEmail 处理发送验证邮件请求
// POST /api/v1/verify-email/send
func (h *UserHandler) HandleSendVerificationEmail(w http.ResponseWriter, r *http.Request) {
	// 1. 获取当前用户ID
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	// 2. 发送验证邮件
	err := h.userSvc.SendVerificationEmail(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeSendVerificationEmailFailed))
		return
	}

	writeSuccess(w, http.StatusOK, "验证邮件已发送", nil)
}

// HandleVerifyEmail 处理邮箱验证请求
// GET /api/v1/verify-email
func (h *UserHandler) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	// 1. 获取参数
	token := r.URL.Query().Get("token")
	userID := r.URL.Query().Get("user_id")

	if token == "" || userID == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingRequiredParam))
		return
	}

	// 2. 验证邮箱
	err := h.userSvc.VerifyEmail(r.Context(), userID, token)
	if err != nil {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeVerifyEmailFailed))
		return
	}

	writeSuccess(w, http.StatusOK, "邮箱验证成功", nil)
}

// HandleForgotPassword 处理忘记密码请求
// POST /api/v1/forgot-password
func (h *UserHandler) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidRequestFormat))
		return
	}

	if req.Email == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeEmailRequired))
		return
	}

	// 发送重置邮件
	err := h.userSvc.ForgotPassword(r.Context(), req.Email)
	if err != nil {
		// 为了安全，不暴露具体错误
		writeSuccess(w, http.StatusOK, "如果该邮箱已注册，重置邮件已发送", nil)
		return
	}

	writeSuccess(w, http.StatusOK, "如果该邮箱已注册，重置邮件已发送", nil)
}

// HandleResetPassword 处理重置密码请求
// POST /api/v1/reset-password
func (h *UserHandler) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		UserID      string `json:"user_id"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidRequestFormat))
		return
	}

	if req.Token == "" || req.UserID == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingRequiredParam))
		return
	}

	// 重置密码
	err := h.userSvc.ResetPassword(r.Context(), req.UserID, req.Token, req.NewPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeResetPasswordFailed))
		return
	}

	writeSuccess(w, http.StatusOK, "密码重置成功", nil)
}

// HandleChangePassword 处理修改密码请求
// POST /api/v1/change-password
func (h *UserHandler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	// 1. 获取当前用户ID
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidRequestFormat))
		return
	}

	if req.OldPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingRequiredParam))
		return
	}

	// 修改密码
	err := h.userSvc.ChangePassword(r.Context(), userID, req.OldPassword, req.NewPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeChangePasswordFailed))
		return
	}

	writeSuccess(w, http.StatusOK, "密码修改成功", nil)
}
