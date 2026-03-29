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
	ErrCodeInvalidCredentials   ErrorCode = "INVALID_CREDENTIALS"
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
func parseErrorResponse(body []byte) struct{ Code, Message string } {
	var result struct {
		Error   string `json:"error"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	_ = json.Unmarshal(body, &result)

	code := result.Code
	if code == "" {
		code = result.Error
	}

	return struct{ Code, Message string }{
		Code:    code,
		Message: result.Message,
	}
}
