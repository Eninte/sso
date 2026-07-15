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
