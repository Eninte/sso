// Package handler 注册处理器
// 处理用户注册请求
package handler

import (
	"net/http"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
)

// ============================================================================
// RegisterHandler 注册处理器
// ============================================================================

// RegisterHandler 注册处理器
type RegisterHandler struct {
	authSvc    service.AuthServiceInterface
	captchaSvc captchaVerifier
}

// NewRegisterHandler 创建注册处理器
func NewRegisterHandler(authSvc service.AuthServiceInterface, captchaSvc captchaVerifier) *RegisterHandler {
	return &RegisterHandler{authSvc: authSvc, captchaSvc: captchaSvc}
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

	// 2. 验证验证码
	if !verifyCaptcha(w, r, h.captchaSvc) {
		return
	}

	// 3. 调用注册服务
	_, err := h.authSvc.Register(r.Context(), &req)
	if err != nil {
		// 注册失败不记录验证码失败计数
		// 原因：注册错误多为输入校验（邮箱格式等），非安全敏感操作
		// 注册端点有独立限流保护，无需通过验证码计数器叠加
		if !writeValidationError(w, r, err) {
			writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeRegisterFailed))
		}
		return
	}

	// 4. 返回成功响应（不暴露user_id，不区分邮箱是否已存在）
	// user为nil表示邮箱已注册，返回相同响应防止用户枚举
	writeSuccess(w, http.StatusCreated, "注册成功，如果邮箱未验证将收到验证邮件", nil)
}
