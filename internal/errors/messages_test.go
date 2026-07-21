// Package errors_test 错误消息国际化测试
package errors_test

import (
	"testing"

	apperrors "github.com/example/sso/internal/errors"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// GetMessage 测试
// ============================================================================

func TestGetMessage_Chinese(t *testing.T) {
	msg := apperrors.GetMessage(apperrors.ErrCodeInternal, "zh-CN")
	assert.NotEmpty(t, msg)
	assert.NotEqual(t, string(apperrors.ErrCodeInternal), msg)
}

func TestGetMessage_English(t *testing.T) {
	msg := apperrors.GetMessage(apperrors.ErrCodeInternal, "en-US")
	assert.NotEmpty(t, msg)
}

func TestGetMessage_UnknownLanguage(t *testing.T) {
	// 未知语言应该回退到中文
	msg := apperrors.GetMessage(apperrors.ErrCodeInternal, "fr-FR")
	assert.NotEmpty(t, msg)
}

func TestGetMessage_UnknownCode(t *testing.T) {
	// 未知错误码应返回错误码本身
	msg := apperrors.GetMessage("UNKNOWN_CODE_12345", "zh-CN")
	assert.Equal(t, "UNKNOWN_CODE_12345", msg)
}

func TestGetMessage_AcceptLanguage(t *testing.T) {
	// Accept-Language 格式
	msg := apperrors.GetMessage(apperrors.ErrCodeBadRequest, "en-US,en;q=0.9")
	assert.NotEmpty(t, msg)
}

func TestGetMessage_CommonErrorCodes(t *testing.T) {
	codes := []apperrors.ErrorCode{
		apperrors.ErrCodeInternal,
		apperrors.ErrCodeBadRequest,
		apperrors.ErrCodeNotFound,
		apperrors.ErrCodeUnauthorized,
		apperrors.ErrCodeForbidden,
		apperrors.ErrCodeConflict,
		apperrors.ErrCodeTooManyRequests,
		apperrors.ErrCodeInvalidCredentials,
		apperrors.ErrCodeAccountLocked,
		apperrors.ErrCodeEmailExists,
		apperrors.ErrCodeInvalidToken,
		apperrors.ErrCodeTokenExpired,
	}

	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			zhMsg := apperrors.GetMessage(code, "zh-CN")
			enMsg := apperrors.GetMessage(code, "en-US")

			assert.NotEmpty(t, zhMsg, "zh-CN message should not be empty for %s", code)
			assert.NotEmpty(t, enMsg, "en-US message should not be empty for %s", code)
		})
	}
}

// ============================================================================
// AppError.GetMessage 测试
// ============================================================================

func TestAppError_GetMessage(t *testing.T) {
	err := apperrors.ErrInternal

	zhMsg := err.GetMessage("zh-CN")
	enMsg := err.GetMessage("en-US")

	assert.NotEmpty(t, zhMsg)
	assert.NotEmpty(t, enMsg)
}

func TestAppError_GetMessage_UnknownLang(t *testing.T) {
	err := apperrors.ErrBadRequest

	msg := err.GetMessage("ja-JP")
	assert.NotEmpty(t, msg)
}

// ============================================================================
// ToLocalizedResponse 测试
// ============================================================================

func TestToLocalizedResponse_Chinese(t *testing.T) {
	err := apperrors.New(apperrors.ErrCodeBadRequest, "请求参数错误", 400).
		WithDetails("email字段必填")

	resp := err.ToLocalizedResponse("zh-CN")

	assert.Equal(t, apperrors.ErrCodeBadRequest, resp.Code)
	assert.NotEmpty(t, resp.Message)
	assert.Equal(t, "email字段必填", resp.Details)
}

func TestToLocalizedResponse_English(t *testing.T) {
	err := apperrors.New(apperrors.ErrCodeNotFound, "not found", 404)

	resp := err.ToLocalizedResponse("en-US")

	assert.Equal(t, apperrors.ErrCodeNotFound, resp.Code)
	assert.NotEmpty(t, resp.Message)
}

func TestToLocalizedResponse_EmptyDetails(t *testing.T) {
	err := apperrors.New(apperrors.ErrCodeInternal, "internal", 500)

	resp := err.ToLocalizedResponse("zh-CN")

	assert.Equal(t, apperrors.ErrCodeInternal, resp.Code)
	assert.Empty(t, resp.Details)
}

func TestToLocalizedResponse_AllPredefinedErrors(t *testing.T) {
	errors := []*apperrors.AppError{
		apperrors.ErrInternal,
		apperrors.ErrBadRequest,
		apperrors.ErrNotFound,
		apperrors.ErrConflict,
		apperrors.ErrUnauthorized,
		apperrors.ErrForbidden,
		apperrors.ErrTooManyRequests,
		apperrors.ErrInvalidCredentials,
		apperrors.ErrAccountLocked,
		apperrors.ErrEmailExists,
		apperrors.ErrInvalidToken,
		apperrors.ErrTokenExpired,
		apperrors.ErrCacheMiss,
		apperrors.ErrKeyNotFound,
		apperrors.ErrSelfOperationForbidden,
		apperrors.ErrLastActiveAdmin,
	}

	for _, err := range errors {
		t.Run(string(err.Code), func(t *testing.T) {
			resp := err.ToLocalizedResponse("zh-CN")
			assert.Equal(t, err.Code, resp.Code)
			assert.NotEmpty(t, resp.Message)
		})
	}
}
