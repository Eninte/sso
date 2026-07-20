package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ============================================================================
// 错误码常量
// ============================================================================

// ErrorCode 错误码类型
type ErrorCode string

const (
	ErrCodeInternal             ErrorCode = "INTERNAL_ERROR"
	ErrCodeBadRequest           ErrorCode = "BAD_REQUEST"
	ErrCodeNotFound             ErrorCode = "NOT_FOUND"
	ErrCodeConflict             ErrorCode = "CONFLICT"
	ErrCodeUnauthorized         ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden            ErrorCode = "FORBIDDEN"
	ErrCodeTooManyRequests      ErrorCode = "TOO_MANY_REQUESTS"
	ErrCodeInvalidCredentials   ErrorCode = "INVALID_CREDENTIALS" // #nosec G101 -- 这是错误码常量，不是凭证
	ErrCodeAccountLocked        ErrorCode = "ACCOUNT_LOCKED"
	ErrCodeAccountDisabled      ErrorCode = "ACCOUNT_DISABLED"
	ErrCodeInvalidToken         ErrorCode = "INVALID_TOKEN"
	ErrCodeTokenExpired         ErrorCode = "TOKEN_EXPIRED"
	ErrCodeEmailExists          ErrorCode = "EMAIL_EXISTS"
	ErrCodeEmailInvalid         ErrorCode = "EMAIL_INVALID"
	ErrCodeEmailRequired        ErrorCode = "EMAIL_REQUIRED"
	ErrCodePasswordTooShort     ErrorCode = "PASSWORD_TOO_SHORT"
	ErrCodePasswordTooLong      ErrorCode = "PASSWORD_TOO_LONG"
	ErrCodePasswordRequired     ErrorCode = "PASSWORD_REQUIRED"
	ErrCodeInvalidRequestFormat ErrorCode = "INVALID_REQUEST_FORMAT"
	ErrCodeRequestBodyTooLarge  ErrorCode = "REQUEST_BODY_TOO_LARGE"
	// MISSING_AUTH_CODE：社交登录回调未携带 code 参数
	// 阶段 B 审查修复：补齐服务端 internal/errors/errors.go:339 中已定义的错误码，
	// ExchangeSocialCode 文档中已引用，需在 SDK 中暴露常量供调用方判断。
	ErrCodeMissingAuthCode ErrorCode = "MISSING_AUTH_CODE"

	// 阶段 5 SDK 同步：以下错误码由服务端阶段 2/3/4 引入，SDK 必须能识别以便正确处理
	//
	// Token 轮换 / 重放（阶段 2.1）
	// 当 Refresh Token 已被使用过又再次出现时返回，是重放攻击的典型特征
	// SDK 收到此错误应清空本地 Token 并要求用户重新登录
	ErrCodeTokenRotated ErrorCode = "TOKEN_ROTATED"

	// OAuth Scope / PKCE / Consent（阶段 2.2）
	ErrCodeInvalidScope    ErrorCode = "INVALID_SCOPE"    // scope 超出客户端允许或白名单
	ErrCodePKCERequired    ErrorCode = "PKCE_REQUIRED"    // 公共客户端必须使用 PKCE（S256）
	ErrCodeConsentRequired ErrorCode = "CONSENT_REQUIRED" // 需要用户同意授权
	ErrCodeConsentDenied   ErrorCode = "CONSENT_DENIED"   // 用户拒绝授权
	ErrCodeConsentInvalid  ErrorCode = "CONSENT_INVALID"  // consent_token 无效或已过期
	ErrCodeClientMismatch  ErrorCode = "CLIENT_MISMATCH"  // refresh_token 客户端归属不一致

	// MFA 两阶段登录（阶段 2.x）
	ErrCodeMFAChallengeInvalid   ErrorCode = "MFA_CHALLENGE_INVALID"   // Challenge 无效或已被使用
	ErrCodeMFAChallengeExpired   ErrorCode = "MFA_CHALLENGE_EXPIRED"   // Challenge 已过期
	ErrCodeInvalidMFACode        ErrorCode = "INVALID_MFA_CODE"        // TOTP 或恢复码无效
	ErrCodeTooManyMFAAttempts    ErrorCode = "TOO_MANY_MFA_ATTEMPTS"   // 尝试次数过多（默认 5 次）
	ErrCodeMFAServiceUnavailable ErrorCode = "MFA_SERVICE_UNAVAILABLE" // MFA 服务未装配

	// Social Login 基础（阶段 2.2 改造）
	ErrCodeProviderNotSupported    ErrorCode = "PROVIDER_NOT_SUPPORTED"     // 提供商不支持
	ErrCodeOAuthCodeExchangeFailed ErrorCode = "OAUTH_CODE_EXCHANGE_FAILED" // 授权码交换失败
	ErrCodeSocialLoginFailed       ErrorCode = "SOCIAL_LOGIN_FAILED"        // 社交登录失败
	ErrCodeOAuthStateInvalid       ErrorCode = "OAUTH_STATE_INVALID"        // state 无效
	ErrCodeOAuthStateExpired       ErrorCode = "OAUTH_STATE_EXPIRED"        // state 已过期

	// Social Login 安全增强（阶段 2.3 新增）
	ErrCodeProviderEmailNotVerified ErrorCode = "PROVIDER_EMAIL_NOT_VERIFIED" // provider 返回 email 未验证
	ErrCodeSocialAccountConflict    ErrorCode = "SOCIAL_ACCOUNT_CONFLICT"     // 社交账号已绑定到其他用户
	ErrCodeEmailConflictWithLocal   ErrorCode = "EMAIL_CONFLICT_WITH_LOCAL"   // email 与本地账号冲突，需手动绑定
	ErrCodeProviderUserIDMissing    ErrorCode = "PROVIDER_USER_ID_MISSING"    // provider 未返回 user_id

	// 邮件（阶段 4.3）
	// 服务端 SMTP 错误统一返回此通用错误码，不暴露 SMTP 内部信息
	ErrCodeEmailSendFailed ErrorCode = "EMAIL_SEND_FAILED"
)

// ============================================================================
// Error 错误类型
// ============================================================================

// Error SSO API错误
type Error struct {
	HTTPStatus int       `json:"-"`
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message,omitempty"`
	RawBody    string    `json:"-"`
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("sso: %s (HTTP %d): %s", e.Code, e.HTTPStatus, e.Message)
	}
	return fmt.Sprintf("sso: %s (HTTP %d)", e.Code, e.HTTPStatus)
}

// IsNotFound 是否为404错误
func (e *Error) IsNotFound() bool {
	return e.HTTPStatus == http.StatusNotFound
}

// IsUnauthorized 是否为401错误
func (e *Error) IsUnauthorized() bool {
	return e.HTTPStatus == http.StatusUnauthorized
}

// IsForbidden 是否为403错误
func (e *Error) IsForbidden() bool {
	return e.HTTPStatus == http.StatusForbidden
}

// IsConflict 是否为409错误
func (e *Error) IsConflict() bool {
	return e.HTTPStatus == http.StatusConflict
}

// IsRateLimited 是否被限流
func (e *Error) IsRateLimited() bool {
	return e.HTTPStatus == http.StatusTooManyRequests
}

// ============================================================================
// 阶段 5.2 辅助判断方法
//
// 服务端阶段 2/3/4 引入了大量新的错误码，下游集成方若每次都通过字符串比较
// 判断错误码可读性差且易错。这里提供一组语义化方法，覆盖最常见的安全处理分支：
//   - IsTokenRotated       → 立即清空本地 Token，要求用户重新登录
//   - IsConsentRequired    → 触发用户同意授权 UI
//   - IsPKCERequired       → 重新发起授权请求并携带 PKCE
//   - IsClientMismatch     → 检查本地配置的 client_id
//   - IsMFAChallenge*      → 引导用户重新输入 MFA
//   - IsSocialLoginError   → 统一社交登录错误处理
//   - IsEmailSendFailed    → 提示稍后重试，不暴露 SMTP 内部信息
// ============================================================================

// IsTokenRotated Refresh Token 已被使用过（重放攻击特征）
// 收到此错误的 SDK 应立即清空本地存储的 access_token 和 refresh_token，
// 并要求用户重新登录。服务端会同步撤销该用户的所有 Token。
func (e *Error) IsTokenRotated() bool {
	return e.Code == ErrCodeTokenRotated
}

// IsConsentRequired 需要用户同意授权
// 包含 CONSENT_REQUIRED（首次授权）和 CONSENT_INVALID（consent_token 失效需重新获取）两种情况。
// SDK 收到此错误应重新调用 Authorize 获取 consent_token，并展示授权同意页面。
func (e *Error) IsConsentRequired() bool {
	return e.Code == ErrCodeConsentRequired || e.Code == ErrCodeConsentInvalid
}

// IsConsentDenied 用户主动拒绝授权
// SDK 收到此错误应终止授权流程并返回用户取消登录。
func (e *Error) IsConsentDenied() bool {
	return e.Code == ErrCodeConsentDenied
}

// IsPKCERequired 公共客户端必须使用 PKCE（S256）
// SDK 收到此错误应生成新的 code_verifier 并重新发起授权请求。
func (e *Error) IsPKCERequired() bool {
	return e.Code == ErrCodePKCERequired
}

// IsInvalidScope 请求的 scope 超出客户端允许范围或不在白名单
func (e *Error) IsInvalidScope() bool {
	return e.Code == ErrCodeInvalidScope
}

// IsClientMismatch Refresh Token 与客户端归属不一致
// 通常发生在不同 client_id 复用了同一 refresh_token。
func (e *Error) IsClientMismatch() bool {
	return e.Code == ErrCodeClientMismatch
}

// IsMFAChallengeInvalid MFA Challenge 无效或已被使用
// SDK 应重新触发登录获取新的 challenge。
func (e *Error) IsMFAChallengeInvalid() bool {
	return e.Code == ErrCodeMFAChallengeInvalid
}

// IsMFAChallengeExpired MFA Challenge 已过期
// SDK 应重新触发登录获取新的 challenge。
func (e *Error) IsMFAChallengeExpired() bool {
	return e.Code == ErrCodeMFAChallengeExpired
}

// IsTooManyMFAAttempts MFA 验证尝试次数过多（默认 5 次）
// SDK 收到此错误应告知用户 challenge 已失效并重新发起登录。
func (e *Error) IsTooManyMFAAttempts() bool {
	return e.Code == ErrCodeTooManyMFAAttempts
}

// IsSocialLoginError 社交登录相关错误
// 涵盖阶段 2.2/2.3 引入的所有社交登录错误码。
// SDK 收到此错误应统一展示 "社交登录失败" 提示，并根据具体错误码决定是否提供
// "切换本地账号" 或 "联系管理员" 引导。
//
// 阶段 B 审查修复：补入 ErrCodeMissingAuthCode。该错误码虽属于基础校验类，
// 但发生在社交登录回调路径上，调用方通常会按社交登录错误统一处理。
func (e *Error) IsSocialLoginError() bool {
	switch e.Code { //nolint:exhaustive // 仅判断社交登录相关错误码，其他错误码走 default 分支
	case ErrCodeProviderNotSupported,
		ErrCodeOAuthCodeExchangeFailed,
		ErrCodeSocialLoginFailed,
		ErrCodeOAuthStateInvalid,
		ErrCodeOAuthStateExpired,
		ErrCodeProviderEmailNotVerified,
		ErrCodeSocialAccountConflict,
		ErrCodeEmailConflictWithLocal,
		ErrCodeProviderUserIDMissing,
		ErrCodeMissingAuthCode:
		return true
	default:
		return false
	}
}

// IsEmailSendFailed 邮件发送失败
// 服务端 SMTP 错误统一返回此错误码，不暴露 SMTP 内部信息。
// SDK 应提示用户稍后重试。
func (e *Error) IsEmailSendFailed() bool {
	return e.Code == ErrCodeEmailSendFailed
}

// newError 创建SSO错误
func newError(status int, body []byte) *errorResponse {
	// 尝试解析错误响应
	errResp := parseErrorResponse(body)
	return &errorResponse{
		HTTPStatus: status,
		Code:       errResp.Code,
		Message:    errResp.Message,
		RawBody:    string(body),
	}
}

// errorResponse 内部错误响应
type errorResponse struct {
	HTTPStatus int
	Code       string
	Message    string
	RawBody    string
}

func (e *errorResponse) toError() *Error {
	return &Error{
		HTTPStatus: e.HTTPStatus,
		Code:       ErrorCode(e.Code),
		Message:    e.Message,
		RawBody:    e.RawBody,
	}
}

// parseErrorResponse 解析错误响应体
// 服务端存在三种错误响应格式：
//  1. {"error": "<code>", "message": "<msg>"}  —— handlerutil.WriteJSONError，error 为错误码
//  2. {"error": "<message>"}                   —— handler 本地 writeError，error 为消息文本
//  3. {"code": "<code>", "message": "<msg>"}   —— OAuth 错误响应
//
// 解析策略：
//   - 优先取 code 字段（格式3）
//   - 无 code 但 error 非空时，根据 message 字段是否同时存在区分格式1/2：
//     message 非空 → error 为错误码（格式1），message 为消息文本
//     message 为空 → error 为消息文本（格式2），code 置空
func parseErrorResponse(body []byte) struct{ Code, Message string } {
	var result struct {
		Error   string `json:"error"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	_ = json.Unmarshal(body, &result)

	code := result.Code
	message := result.Message

	// 无 code 字段但存在 error 字段：区分格式1（error 为 code）与格式2（error 为消息文本）
	if code == "" && result.Error != "" {
		if message == "" {
			// 格式2：error 字段是消息文本
			message = result.Error
		} else {
			// 格式1：error 字段是错误码，message 字段是消息文本
			code = result.Error
		}
	}

	return struct{ Code, Message string }{
		Code:    code,
		Message: message,
	}
}
