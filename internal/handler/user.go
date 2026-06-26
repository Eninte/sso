// Package handler 用户处理器
// 处理用户相关的HTTP请求
package handler

import (
	"net/http"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/validator"
)

// ============================================================================
// UserHandler 用户处理器
// ============================================================================

// UserHandler 用户处理器
type UserHandler struct {
	userSvc    service.UserServiceInterface
	captchaSvc captchaVerifier
}

// NewUserHandler 创建用户处理器
func NewUserHandler(userSvc service.UserServiceInterface, captchaSvc captchaVerifier) *UserHandler {
	return &UserHandler{userSvc: userSvc, captchaSvc: captchaSvc}
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
		// 区分业务错误与系统错误，避免将业务异常误映射为 500
		if !writeValidationError(w, r, err) {
			writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeSendVerificationEmailFailed))
		}
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
		handleDecodeJSONError(w, r, err)
		return
	}

	// 校验邮箱格式：空值与格式错误都属于客户端输入错误，应在进入业务逻辑前拦截
	if err := validator.ValidateEmail(req.Email); err != nil {
		if !writeValidationError(w, r, err) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeEmailInvalid))
		}
		return
	}

	// 验证验证码
	if !verifyCaptcha(w, r, h.captchaSvc) {
		return
	}

	// 发送重置邮件
	err := h.userSvc.ForgotPassword(r.Context(), req.Email)
	if err != nil {
		// 记录验证码失败计数
		// 虽然前端始终收到成功响应（防枚举），但后端仍需记录失败
		// 以便在攻击者大量尝试时触发验证码
		h.captchaSvc.RecordFailure(r.Context(), extractClientIP(r))
		// 为了安全，不暴露具体错误
		writeSuccess(w, http.StatusOK, "如果该邮箱已注册，重置邮件已发送", nil)
		return
	}

	// 成功，清除失败计数
	h.captchaSvc.ClearFailures(r.Context(), extractClientIP(r))
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
		handleDecodeJSONError(w, r, err)
		return
	}

	if req.Token == "" || req.UserID == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingRequiredParam))
		return
	}

	// 验证验证码
	if !verifyCaptcha(w, r, h.captchaSvc) {
		return
	}

	// 重置密码
	err := h.userSvc.ResetPassword(r.Context(), req.UserID, req.Token, req.NewPassword)
	if err != nil {
		// 记录验证码失败计数
		h.captchaSvc.RecordFailure(r.Context(), extractClientIP(r))
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeResetPasswordFailed))
		return
	}

	// 成功，清除失败计数
	h.captchaSvc.ClearFailures(r.Context(), extractClientIP(r))
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
		handleDecodeJSONError(w, r, err)
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
