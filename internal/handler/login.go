// Package handler 登录处理器
// 处理用户登录请求
package handler

import (
	"log/slog"
	"net"
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

	// 2. 构建审计上下文（含客户端IP，用于IP维度登录限流）
	auditCtx := &service.AuditContext{
		IPAddress: extractClientIP(r),
		UserAgent: r.UserAgent(),
	}

	// 3. 调用登录服务（带IP限流）
	resp, err := h.authSvc.LoginWithAudit(r.Context(), &req, auditCtx)
	if err != nil {
		// 统一处理所有错误
		writeOAuthError(w, r, err)
		return
	}

	// 4. 返回Token
	writeJSON(w, http.StatusOK, resp)
}

// extractClientIP 从请求中提取客户端IP
func extractClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
