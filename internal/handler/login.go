// Package handler 登录处理器
// 处理用户登录请求
package handler

import (
	"log/slog"
	"net/http"

	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
)

// ============================================================================
// LoginHandler 登录处理器
// ============================================================================

// LoginHandler 登录处理器
type LoginHandler struct {
	authSvc    service.AuthServiceInterface
	captchaSvc captchaVerifier
}

// NewLoginHandler 创建登录处理器
func NewLoginHandler(authSvc service.AuthServiceInterface, captchaSvc captchaVerifier) *LoginHandler {
	return &LoginHandler{authSvc: authSvc, captchaSvc: captchaSvc}
}

// Handle 处理登录请求
// POST /api/v1/login
func (h *LoginHandler) Handle(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("LoginHandler panic", "error", err)
			writeError(w, http.StatusInternalServerError, getMessage(r, "INTERNAL_ERROR"))
		}
	}()

	// 1. 解析请求体 (带大小限制)
	var req model.LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	// 2. 验证验证码
	if !verifyCaptcha(w, r, h.captchaSvc) {
		return
	}

	// 3. 构建审计上下文（含客户端IP，用于IP维度登录限流）
	auditCtx := &service.AuditContext{
		IPAddress: extractClientIP(r),
		UserAgent: r.UserAgent(),
	}

	// 4. 调用登录服务（带IP限流）
	resp, err := h.authSvc.LoginWithAudit(r.Context(), &req, auditCtx)
	if err != nil {
		// 记录验证码失败计数
		h.captchaSvc.RecordFailure(r.Context(), auditCtx.IPAddress)
		// 统一处理所有错误
		writeOAuthError(w, r, err)
		return
	}

	// 5. 登录成功，清除失败计数
	h.captchaSvc.ClearFailures(r.Context(), auditCtx.IPAddress)

	// 6. 返回Token
	writeJSON(w, http.StatusOK, resp)
}

// extractClientIP 从请求中提取客户端IP
// 委托给 middleware.GetClientIP，支持受信代理头
func extractClientIP(r *http.Request) string {
	return middleware.GetClientIP(r)
}
