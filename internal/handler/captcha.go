// Package handler 验证码处理器
// 处理验证码生成请求
package handler

import (
	"net/http"

	"github.com/example/sso/internal/captcha"
	apperrors "github.com/example/sso/internal/errors"
)

// ============================================================================
// CaptchaHandler 验证码处理器
// ============================================================================

// CaptchaHandler 验证码处理器
type CaptchaHandler struct {
	captchaSvc *captcha.Service
}

// NewCaptchaHandler 创建验证码处理器
func NewCaptchaHandler(captchaSvc *captcha.Service) *CaptchaHandler {
	return &CaptchaHandler{captchaSvc: captchaSvc}
}

// Handle 生成验证码
// GET /api/v1/captcha
func (h *CaptchaHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if !h.captchaSvc.IsEnabled() {
		writeError(w, http.StatusNotFound, getMessage(r, apperrors.ErrCodeCaptchaDisabled))
		return
	}

	c, err := h.captchaSvc.Generate(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeCaptchaGenerate))
		return
	}

	writeSuccess(w, http.StatusOK, "", c)
}
