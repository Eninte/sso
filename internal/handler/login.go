// Package handler 登录处理器
// 处理用户登录请求
package handler

import (
	"net/http"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
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
//
// 响应：
//   - 用户未启用 MFA：返回完整 Token 对（access_token + refresh_token）
//   - 用户启用 MFA：返回 mfa_required=true + mfa_challenge 令牌，
//     客户端需调用 POST /api/v1/login/mfa/verify 完成第二阶段验证
//
// Panic恢复由中间件 middleware.Recover 统一处理，此处不再重复捕获
func (h *LoginHandler) Handle(w http.ResponseWriter, r *http.Request) {
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
		// 仅对凭据相关错误（401/403）记录验证码失败计数
		// 排除服务器内部错误(500)和限流错误(429)，避免影响合法用户
		if isCredentialError(err) {
			h.captchaSvc.RecordFailure(r.Context(), auditCtx.IPAddress)
		}
		// 统一处理所有错误
		writeOAuthError(w, r, err)
		return
	}

	// 5. 登录成功，清除失败计数
	// 注意：MFA 第一阶段密码验证成功也视为登录成功（账户已通过凭据认证）
	// 第二阶段失败由 mfa/verify 接口单独计数
	h.captchaSvc.ClearFailures(r.Context(), auditCtx.IPAddress)

	// 6. 返回响应（Token 或 MFA Challenge）
	writeJSON(w, http.StatusOK, resp)
}

// HandleVerifyMFALogin 处理 MFA 两阶段登录第二阶段验证
// POST /api/v1/login/mfa/verify
//
// 请求体：{"mfa_challenge":"<token>", "method":"totp|recovery_code", "code":"..."}
// 成功响应：{"access_token":"...", "refresh_token":"...", "token_type":"Bearer", "expires_in":900}
// 失败响应：标准 OAuth 错误格式
//
// 安全设计：
//   - 此端点必须使用敏感限流（注册到 sensitive 子路由）
//   - Challenge 必须绑定客户端 IP/UA，跨网络使用会被拒绝
//   - 验证失败递增尝试次数，5 次后 Challenge 失效
func (h *LoginHandler) HandleVerifyMFALogin(w http.ResponseWriter, r *http.Request) {
	var req model.MFAVerifyRequest
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	if req.MFAChallenge == "" {
		writeOAuthError(w, r, apperrors.ErrBadRequest.WithDetails("mfa_challenge is required"))
		return
	}
	if req.Code == "" {
		writeOAuthError(w, r, apperrors.ErrBadRequest.WithDetails("code is required"))
		return
	}
	if req.Method != model.MFAMethodTOTP && req.Method != model.MFAMethodRecoveryCode {
		writeOAuthError(w, r, apperrors.ErrBadRequest.WithDetails("invalid method"))
		return
	}

	ipAddress := extractClientIP(r)
	userAgent := r.UserAgent()

	resp, err := h.authSvc.VerifyMFALogin(r.Context(), &req, ipAddress, userAgent)
	if err != nil {
		writeOAuthError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// extractClientIP 从请求中提取客户端IP
// 委托给 middleware.GetClientIP，支持受信代理头
func extractClientIP(r *http.Request) string {
	return middleware.GetClientIP(r)
}
