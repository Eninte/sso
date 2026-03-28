// Package errors_test 统一错误定义单元测试
package errors_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	apperrors "github.com/your-org/sso/internal/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// New 构造函数测试
// ============================================================================

func TestNew_CreatesAppError(t *testing.T) {
	err := apperrors.New(apperrors.ErrCodeInternal, "internal error", http.StatusInternalServerError)

	require.NotNil(t, err)
	assert.Equal(t, apperrors.ErrCodeInternal, err.Code)
	assert.Equal(t, "internal error", err.Message)
	assert.Equal(t, http.StatusInternalServerError, err.HTTPStatus)
	assert.Empty(t, err.Details)
	assert.Nil(t, err.Err)
}

func TestNew_VariousStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		code       apperrors.ErrorCode
		message    string
		httpStatus int
	}{
		{"BadRequest", apperrors.ErrCodeBadRequest, "bad request", http.StatusBadRequest},
		{"Unauthorized", apperrors.ErrCodeUnauthorized, "unauthorized", http.StatusUnauthorized},
		{"Forbidden", apperrors.ErrCodeForbidden, "forbidden", http.StatusForbidden},
		{"NotFound", apperrors.ErrCodeNotFound, "not found", http.StatusNotFound},
		{"Conflict", apperrors.ErrCodeConflict, "conflict", http.StatusConflict},
		{"TooManyRequests", apperrors.ErrCodeTooManyRequests, "too many", http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := apperrors.New(tt.code, tt.message, tt.httpStatus)
			assert.Equal(t, tt.code, err.Code)
			assert.Equal(t, tt.message, err.Message)
			assert.Equal(t, tt.httpStatus, err.HTTPStatus)
		})
	}
}

// ============================================================================
// Wrap 包装函数测试
// ============================================================================

func TestWrap_WrapsError(t *testing.T) {
	originalErr := errors.New("database connection failed")
	wrappedErr := apperrors.Wrap(apperrors.ErrCodeInternal, "internal error", http.StatusInternalServerError, originalErr)

	require.NotNil(t, wrappedErr)
	assert.Equal(t, apperrors.ErrCodeInternal, wrappedErr.Code)
	assert.Equal(t, "internal error", wrappedErr.Message)
	assert.Equal(t, http.StatusInternalServerError, wrappedErr.HTTPStatus)
	assert.Equal(t, originalErr, wrappedErr.Err)
}

func TestWrap_NilError(t *testing.T) {
	wrappedErr := apperrors.Wrap(apperrors.ErrCodeInternal, "internal error", http.StatusInternalServerError, nil)

	require.NotNil(t, wrappedErr)
	assert.Nil(t, wrappedErr.Err)
}

// ============================================================================
// Error() 方法测试
// ============================================================================

func TestAppError_Error_WithoutWrappedError(t *testing.T) {
	err := apperrors.New(apperrors.ErrCodeNotFound, "resource not found", http.StatusNotFound)

	assert.Equal(t, "NOT_FOUND: resource not found", err.Error())
}

func TestAppError_Error_WithWrappedError(t *testing.T) {
	originalErr := errors.New("row not found")
	err := apperrors.Wrap(apperrors.ErrCodeNotFound, "user not found", http.StatusNotFound, originalErr)

	assert.Equal(t, "NOT_FOUND: user not found (row not found)", err.Error())
}

// ============================================================================
// Unwrap 测试
// ============================================================================

func TestAppError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	err := apperrors.Wrap(apperrors.ErrCodeInternal, "wrapped", http.StatusInternalServerError, originalErr)

	assert.Equal(t, originalErr, err.Unwrap())
}

func TestAppError_Unwrap_Nil(t *testing.T) {
	err := apperrors.New(apperrors.ErrCodeInternal, "no wrap", http.StatusInternalServerError)

	assert.Nil(t, err.Unwrap())
}

// ============================================================================
// WithDetails 测试
// ============================================================================

func TestWithDetails_AddsDetails(t *testing.T) {
	err := apperrors.New(apperrors.ErrCodeBadRequest, "invalid input", http.StatusBadRequest)
	result := err.WithDetails("field 'email' is required")

	assert.Equal(t, "field 'email' is required", result.Details)
	assert.Equal(t, err, result) // 返回同一实例
}

func TestWithDetails_Chaining(t *testing.T) {
	err := apperrors.New(apperrors.ErrCodeBadRequest, "invalid", http.StatusBadRequest).
		WithDetails("detail1")

	assert.Equal(t, "detail1", err.Details)
}

// ============================================================================
// Is 错误判断测试
// ============================================================================

func TestIs_MatchingError(t *testing.T) {
	target := apperrors.New(apperrors.ErrCodeNotFound, "not found", http.StatusNotFound)
	wrapped := fmt.Errorf("context: %w", target)

	assert.True(t, apperrors.Is(wrapped, target))
}

func TestIs_NonMatchingError(t *testing.T) {
	err1 := apperrors.New(apperrors.ErrCodeNotFound, "not found", http.StatusNotFound)
	err2 := apperrors.New(apperrors.ErrCodeBadRequest, "bad request", http.StatusBadRequest)

	assert.False(t, apperrors.Is(err1, err2))
}

func TestIs_StandardError(t *testing.T) {
	stdErr := errors.New("standard error")
	assert.True(t, apperrors.Is(stdErr, stdErr))
}

// ============================================================================
// As 类型转换测试
// ============================================================================

func TestAsConvertsAppError(t *testing.T) {
	original := apperrors.New(apperrors.ErrCodeNotFound, "not found", http.StatusNotFound)
	wrapped := fmt.Errorf("wrapped: %w", original)

	var target *apperrors.AppError
	assert.True(t, apperrors.As(wrapped, &target))
	assert.Equal(t, apperrors.ErrCodeNotFound, target.Code)
}

func TestAsReturnsFalseForNonAppError(t *testing.T) {
	stdErr := errors.New("standard error")

	var target *apperrors.AppError
	assert.False(t, apperrors.As(stdErr, &target))
}

// ============================================================================
// GetHTTPStatus 测试
// ============================================================================

func TestGetHTTPStatus_AppError(t *testing.T) {
	tests := []struct {
		name     string
		err      *apperrors.AppError
		expected int
	}{
		{"500 Internal", apperrors.New(apperrors.ErrCodeInternal, "err", 500), 500},
		{"400 BadRequest", apperrors.New(apperrors.ErrCodeBadRequest, "err", 400), 400},
		{"401 Unauthorized", apperrors.New(apperrors.ErrCodeUnauthorized, "err", 401), 401},
		{"403 Forbidden", apperrors.New(apperrors.ErrCodeForbidden, "err", 403), 403},
		{"404 NotFound", apperrors.New(apperrors.ErrCodeNotFound, "err", 404), 404},
		{"409 Conflict", apperrors.New(apperrors.ErrCodeConflict, "err", 409), 409},
		{"413 TooLarge", apperrors.New(apperrors.ErrCodeRequestBodyTooLarge, "err", 413), 413},
		{"429 TooMany", apperrors.New(apperrors.ErrCodeTooManyRequests, "err", 429), 429},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, apperrors.GetHTTPStatus(tt.err))
		})
	}
}

func TestGetHTTPStatus_NonAppError(t *testing.T) {
	stdErr := errors.New("standard error")
	assert.Equal(t, 500, apperrors.GetHTTPStatus(stdErr))
}

func TestGetHTTPStatus_WrappedAppError(t *testing.T) {
	appErr := apperrors.New(apperrors.ErrCodeNotFound, "not found", 404)
	wrapped := fmt.Errorf("context: %w", appErr)

	assert.Equal(t, 404, apperrors.GetHTTPStatus(wrapped))
}

// ============================================================================
// GetErrorCode 测试
// ============================================================================

func TestGetErrorCode_AppError(t *testing.T) {
	tests := []struct {
		name     string
		err      *apperrors.AppError
		expected apperrors.ErrorCode
	}{
		{"Internal", apperrors.New(apperrors.ErrCodeInternal, "err", 500), apperrors.ErrCodeInternal},
		{"BadRequest", apperrors.New(apperrors.ErrCodeBadRequest, "err", 400), apperrors.ErrCodeBadRequest},
		{"NotFound", apperrors.New(apperrors.ErrCodeNotFound, "err", 404), apperrors.ErrCodeNotFound},
		{"Unauthorized", apperrors.New(apperrors.ErrCodeUnauthorized, "err", 401), apperrors.ErrCodeUnauthorized},
		{"EmailExists", apperrors.New(apperrors.ErrCodeEmailExists, "err", 409), apperrors.ErrCodeEmailExists},
		{"InvalidToken", apperrors.New(apperrors.ErrCodeInvalidToken, "err", 401), apperrors.ErrCodeInvalidToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, apperrors.GetErrorCode(tt.err))
		})
	}
}

func TestGetErrorCode_NonAppError(t *testing.T) {
	stdErr := errors.New("standard error")
	assert.Equal(t, apperrors.ErrCodeInternal, apperrors.GetErrorCode(stdErr))
}

// ============================================================================
// 预定义错误验证
// ============================================================================

func TestPredefinedErrors_Internal(t *testing.T) {
	assert.Equal(t, apperrors.ErrCodeInternal, apperrors.ErrInternal.Code)
	assert.Equal(t, 500, apperrors.ErrInternal.HTTPStatus)
	assert.NotEmpty(t, apperrors.ErrInternal.Message)
}

func TestPredefinedErrors_BadRequest(t *testing.T) {
	assert.Equal(t, apperrors.ErrCodeBadRequest, apperrors.ErrBadRequest.Code)
	assert.Equal(t, 400, apperrors.ErrBadRequest.HTTPStatus)
}

func TestPredefinedErrors_NotFound(t *testing.T) {
	assert.Equal(t, apperrors.ErrCodeNotFound, apperrors.ErrNotFound.Code)
	assert.Equal(t, 404, apperrors.ErrNotFound.HTTPStatus)
}

func TestPredefinedErrors_Conflict(t *testing.T) {
	assert.Equal(t, apperrors.ErrCodeConflict, apperrors.ErrConflict.Code)
	assert.Equal(t, 409, apperrors.ErrConflict.HTTPStatus)
}

func TestPredefinedErrors_Unauthorized(t *testing.T) {
	assert.Equal(t, apperrors.ErrCodeUnauthorized, apperrors.ErrUnauthorized.Code)
	assert.Equal(t, 401, apperrors.ErrUnauthorized.HTTPStatus)
}

func TestPredefinedErrors_Forbidden(t *testing.T) {
	assert.Equal(t, apperrors.ErrCodeForbidden, apperrors.ErrForbidden.Code)
	assert.Equal(t, 403, apperrors.ErrForbidden.HTTPStatus)
}

func TestPredefinedErrors_TooManyRequests(t *testing.T) {
	assert.Equal(t, apperrors.ErrCodeTooManyRequests, apperrors.ErrTooManyRequests.Code)
	assert.Equal(t, 429, apperrors.ErrTooManyRequests.HTTPStatus)
}

func TestPredefinedErrors_AuthErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperrors.AppError
		code       apperrors.ErrorCode
		httpStatus int
	}{
		{"InvalidCredentials", apperrors.ErrInvalidCredentials, apperrors.ErrCodeInvalidCredentials, 401},
		{"AccountLocked", apperrors.ErrAccountLocked, apperrors.ErrCodeAccountLocked, 403},
		{"AccountDisabled", apperrors.ErrAccountDisabled, apperrors.ErrCodeAccountDisabled, 403},
		{"InvalidToken", apperrors.ErrInvalidToken, apperrors.ErrCodeInvalidToken, 401},
		{"TokenExpired", apperrors.ErrTokenExpired, apperrors.ErrCodeTokenExpired, 401},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.httpStatus, tt.err.HTTPStatus)
			assert.NotEmpty(t, tt.err.Message)
		})
	}
}

func TestPredefinedErrors_UserErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperrors.AppError
		code       apperrors.ErrorCode
		httpStatus int
	}{
		{"EmailExists", apperrors.ErrEmailExists, apperrors.ErrCodeEmailExists, 409},
		{"EmailInvalid", apperrors.ErrEmailInvalid, apperrors.ErrCodeEmailInvalid, 400},
		{"EmailRequired", apperrors.ErrEmailRequired, apperrors.ErrCodeEmailRequired, 400},
		{"PasswordTooShort", apperrors.ErrPasswordTooShort, apperrors.ErrCodePasswordTooShort, 400},
		{"PasswordTooLong", apperrors.ErrPasswordTooLong, apperrors.ErrCodePasswordTooLong, 400},
		{"PasswordRequired", apperrors.ErrPasswordRequired, apperrors.ErrCodePasswordRequired, 400},
		{"PasswordMismatch", apperrors.ErrPasswordMismatch, apperrors.ErrCodePasswordMismatch, 400},
		{"PasswordNoUppercase", apperrors.ErrPasswordNoUppercase, apperrors.ErrCodePasswordNoUppercase, 400},
		{"PasswordNoLowercase", apperrors.ErrPasswordNoLowercase, apperrors.ErrCodePasswordNoLowercase, 400},
		{"PasswordNoDigit", apperrors.ErrPasswordNoDigit, apperrors.ErrCodePasswordNoDigit, 400},
		{"PasswordNoSpecial", apperrors.ErrPasswordNoSpecial, apperrors.ErrCodePasswordNoSpecial, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.httpStatus, tt.err.HTTPStatus)
			assert.NotEmpty(t, tt.err.Message)
		})
	}
}

func TestPredefinedErrors_EmailVerificationErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperrors.AppError
		code       apperrors.ErrorCode
		httpStatus int
	}{
		{"EmailAlreadyVerified", apperrors.ErrEmailAlreadyVerified, apperrors.ErrCodeEmailAlreadyVerified, 409},
		{"VerificationCodeInvalid", apperrors.ErrVerificationCodeInvalid, apperrors.ErrCodeVerificationCodeInvalid, 400},
		{"VerificationCodeExpired", apperrors.ErrVerificationCodeExpired, apperrors.ErrCodeVerificationCodeExpired, 400},
		{"ResetTokenInvalid", apperrors.ErrResetTokenInvalid, apperrors.ErrCodeResetTokenInvalid, 400},
		{"ResetTokenExpired", apperrors.ErrResetTokenExpired, apperrors.ErrCodeResetTokenExpired, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.httpStatus, tt.err.HTTPStatus)
			assert.NotEmpty(t, tt.err.Message)
		})
	}
}

func TestPredefinedErrors_OAuthErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperrors.AppError
		code       apperrors.ErrorCode
		httpStatus int
	}{
		{"InvalidClient", apperrors.ErrInvalidClient, apperrors.ErrCodeInvalidClient, 400},
		{"InvalidRedirectURI", apperrors.ErrInvalidRedirectURI, apperrors.ErrCodeInvalidRedirectURI, 400},
		{"InvalidGrantType", apperrors.ErrInvalidGrantType, apperrors.ErrCodeInvalidGrantType, 400},
		{"InvalidCode", apperrors.ErrInvalidCode, apperrors.ErrCodeInvalidCode, 400},
		{"CodeExpired", apperrors.ErrCodeExpiredErr, apperrors.ErrCodeCodeExpired, 400},
		{"CodeUsed", apperrors.ErrCodeUsedErr, apperrors.ErrCodeCodeUsed, 400},
		{"InvalidCodeVerifier", apperrors.ErrInvalidCodeVerifier, apperrors.ErrCodeInvalidCodeVerifier, 400},
		{"InvalidPKCEChallenge", apperrors.ErrInvalidPKCEChallenge, apperrors.ErrCodeInvalidPKCEChallenge, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.httpStatus, tt.err.HTTPStatus)
			assert.NotEmpty(t, tt.err.Message)
		})
	}
}

func TestPredefinedErrors_MFAErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperrors.AppError
		code       apperrors.ErrorCode
		httpStatus int
	}{
		{"MFAAlreadyEnabled", apperrors.ErrMFAAlreadyEnabled, apperrors.ErrCodeMFAAlreadyEnabled, 409},
		{"MFANotEnabled", apperrors.ErrMFANotEnabled, apperrors.ErrCodeMFANotEnabled, 400},
		{"InvalidTOTPCode", apperrors.ErrInvalidTOTPCode, apperrors.ErrCodeInvalidTOTPCode, 400},
		{"TOTPCodeExpired", apperrors.ErrTOTPCodeExpired, apperrors.ErrCodeTOTPCodeExpired, 400},
		{"InvalidMFASecret", apperrors.ErrInvalidMFASecret, apperrors.ErrCodeInvalidMFASecret, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.httpStatus, tt.err.HTTPStatus)
			assert.NotEmpty(t, tt.err.Message)
		})
	}
}

func TestPredefinedErrors_SocialLoginErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperrors.AppError
		code       apperrors.ErrorCode
		httpStatus int
	}{
		{"ProviderNotSupported", apperrors.ErrProviderNotSupported, apperrors.ErrCodeProviderNotSupported, 400},
		{"OAuthCodeExchangeFailed", apperrors.ErrOAuthCodeExchangeFailed, apperrors.ErrCodeOAuthCodeExchangeFailed, 400},
		{"SocialLoginFailed", apperrors.ErrSocialLoginFailed, apperrors.ErrCodeSocialLoginFailed, 400},
		{"OAuthStateInvalid", apperrors.ErrOAuthStateInvalid, apperrors.ErrCodeOAuthStateInvalid, 400},
		{"OAuthStateExpired", apperrors.ErrOAuthStateExpired, apperrors.ErrCodeOAuthStateExpired, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.httpStatus, tt.err.HTTPStatus)
			assert.NotEmpty(t, tt.err.Message)
		})
	}
}

func TestPredefinedErrors_KeyErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperrors.AppError
		code       apperrors.ErrorCode
		httpStatus int
	}{
		{"KeyNotFound", apperrors.ErrKeyNotFound, apperrors.ErrCodeKeyNotFound, 500},
		{"KeyPathInvalid", apperrors.ErrKeyPathInvalid, apperrors.ErrCodeKeyPathInvalid, 500},
		{"KeyParseFailed", apperrors.ErrKeyParseFailed, apperrors.ErrCodeKeyParseFailed, 500},
		{"KeyIDEmpty", apperrors.ErrKeyIDEmpty, apperrors.ErrCodeKeyIDEmpty, 400},
		{"PrivateKeyNil", apperrors.ErrPrivateKeyNil, apperrors.ErrCodePrivateKeyNil, 400},
		{"PublicKeyNil", apperrors.ErrPublicKeyNil, apperrors.ErrCodePublicKeyNil, 400},
		{"NoActiveKey", apperrors.ErrNoActiveKey, apperrors.ErrCodeNoActiveKey, 500},
		{"KeyPermissionOpen", apperrors.ErrKeyPermissionOpen, apperrors.ErrCodeKeyPermissionOpen, 500},
		{"KeyTooShort", apperrors.ErrKeyTooShort, apperrors.ErrCodeKeyTooShort, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.httpStatus, tt.err.HTTPStatus)
			assert.NotEmpty(t, tt.err.Message)
		})
	}
}

func TestPredefinedErrors_CacheAndRequestErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperrors.AppError
		code       apperrors.ErrorCode
		httpStatus int
	}{
		{"CacheMiss", apperrors.ErrCacheMiss, apperrors.ErrCodeCacheMiss, 404},
		{"RequestBodyTooLarge", apperrors.ErrRequestBodyTooLarge, apperrors.ErrCodeRequestBodyTooLarge, 413},
		{"RequestBodyExtraData", apperrors.ErrRequestBodyExtraData, apperrors.ErrCodeRequestBodyExtraData, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.httpStatus, tt.err.HTTPStatus)
			assert.NotEmpty(t, tt.err.Message)
		})
	}
}

func TestPredefinedErrors_ConfigErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperrors.AppError
		code       apperrors.ErrorCode
		httpStatus int
	}{
		{"DBPasswordRequired", apperrors.ErrDBPasswordRequired, apperrors.ErrCodeDBPasswordRequired, 500},
		{"JWTKeyRequired", apperrors.ErrJWTKeyRequired, apperrors.ErrCodeJWTKeyRequired, 500},
		{"BcryptCostTooLow", apperrors.ErrBcryptCostTooLow, apperrors.ErrCodeBcryptCostTooLow, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.httpStatus, tt.err.HTTPStatus)
			assert.NotEmpty(t, tt.err.Message)
		})
	}
}

// ============================================================================
// ErrorCode 常量验证
// ============================================================================

func TestErrorCodeConstants_NotEmpty(t *testing.T) {
	codes := []apperrors.ErrorCode{
		apperrors.ErrCodeInternal,
		apperrors.ErrCodeBadRequest,
		apperrors.ErrCodeNotFound,
		apperrors.ErrCodeConflict,
		apperrors.ErrCodeUnauthorized,
		apperrors.ErrCodeForbidden,
		apperrors.ErrCodeTooManyRequests,
		apperrors.ErrCodeInvalidCredentials,
		apperrors.ErrCodeAccountLocked,
		apperrors.ErrCodeAccountDisabled,
		apperrors.ErrCodeInvalidToken,
		apperrors.ErrCodeTokenExpired,
		apperrors.ErrCodeEmailExists,
		apperrors.ErrCodeEmailInvalid,
		apperrors.ErrCodeEmailRequired,
		apperrors.ErrCodePasswordTooShort,
		apperrors.ErrCodePasswordTooLong,
		apperrors.ErrCodePasswordRequired,
		apperrors.ErrCodePasswordMismatch,
		apperrors.ErrCodePasswordNoUppercase,
		apperrors.ErrCodePasswordNoLowercase,
		apperrors.ErrCodePasswordNoDigit,
		apperrors.ErrCodePasswordNoSpecial,
		apperrors.ErrCodeEmailAlreadyVerified,
		apperrors.ErrCodeVerificationCodeInvalid,
		apperrors.ErrCodeVerificationCodeExpired,
		apperrors.ErrCodeResetTokenInvalid,
		apperrors.ErrCodeResetTokenExpired,
		apperrors.ErrCodeInvalidClient,
		apperrors.ErrCodeInvalidRedirectURI,
		apperrors.ErrCodeInvalidGrantType,
		apperrors.ErrCodeInvalidCode,
		apperrors.ErrCodeCodeExpired,
		apperrors.ErrCodeCodeUsed,
		apperrors.ErrCodeInvalidCodeVerifier,
		apperrors.ErrCodeInvalidPKCEChallenge,
		apperrors.ErrCodeMFAAlreadyEnabled,
		apperrors.ErrCodeMFANotEnabled,
		apperrors.ErrCodeInvalidTOTPCode,
		apperrors.ErrCodeTOTPCodeExpired,
		apperrors.ErrCodeInvalidMFASecret,
		apperrors.ErrCodeProviderNotSupported,
		apperrors.ErrCodeOAuthCodeExchangeFailed,
		apperrors.ErrCodeSocialLoginFailed,
		apperrors.ErrCodeOAuthStateInvalid,
		apperrors.ErrCodeOAuthStateExpired,
		apperrors.ErrCodeKeyNotFound,
		apperrors.ErrCodeKeyPathInvalid,
		apperrors.ErrCodeKeyParseFailed,
		apperrors.ErrCodeKeyIDEmpty,
		apperrors.ErrCodePrivateKeyNil,
		apperrors.ErrCodePublicKeyNil,
		apperrors.ErrCodeNoActiveKey,
		apperrors.ErrCodeCacheMiss,
		apperrors.ErrCodeRequestBodyTooLarge,
		apperrors.ErrCodeRequestBodyExtraData,
		apperrors.ErrCodeDBPasswordRequired,
		apperrors.ErrCodeJWTKeyRequired,
		apperrors.ErrCodeBcryptCostTooLow,
		apperrors.ErrCodeKeyPermissionOpen,
		apperrors.ErrCodeKeyTooShort,
	}

	for _, code := range codes {
		assert.NotEmpty(t, string(code), "error code should not be empty")
	}
}

// ============================================================================
// 错误码唯一性验证
// ============================================================================

func TestErrorCodeConstants_Unique(t *testing.T) {
	codes := []apperrors.ErrorCode{
		apperrors.ErrCodeInternal,
		apperrors.ErrCodeBadRequest,
		apperrors.ErrCodeNotFound,
		apperrors.ErrCodeConflict,
		apperrors.ErrCodeUnauthorized,
		apperrors.ErrCodeForbidden,
		apperrors.ErrCodeTooManyRequests,
		apperrors.ErrCodeInvalidCredentials,
		apperrors.ErrCodeAccountLocked,
		apperrors.ErrCodeAccountDisabled,
		apperrors.ErrCodeInvalidToken,
		apperrors.ErrCodeTokenExpired,
		apperrors.ErrCodeEmailExists,
		apperrors.ErrCodeEmailInvalid,
		apperrors.ErrCodeEmailRequired,
		apperrors.ErrCodePasswordTooShort,
		apperrors.ErrCodePasswordTooLong,
		apperrors.ErrCodePasswordRequired,
		apperrors.ErrCodePasswordMismatch,
		apperrors.ErrCodePasswordNoUppercase,
		apperrors.ErrCodePasswordNoLowercase,
		apperrors.ErrCodePasswordNoDigit,
		apperrors.ErrCodePasswordNoSpecial,
		apperrors.ErrCodeEmailAlreadyVerified,
		apperrors.ErrCodeVerificationCodeInvalid,
		apperrors.ErrCodeVerificationCodeExpired,
		apperrors.ErrCodeResetTokenInvalid,
		apperrors.ErrCodeResetTokenExpired,
		apperrors.ErrCodeInvalidClient,
		apperrors.ErrCodeInvalidRedirectURI,
		apperrors.ErrCodeInvalidGrantType,
		apperrors.ErrCodeInvalidCode,
		apperrors.ErrCodeCodeExpired,
		apperrors.ErrCodeCodeUsed,
		apperrors.ErrCodeInvalidCodeVerifier,
		apperrors.ErrCodeInvalidPKCEChallenge,
		apperrors.ErrCodeMFAAlreadyEnabled,
		apperrors.ErrCodeMFANotEnabled,
		apperrors.ErrCodeInvalidTOTPCode,
		apperrors.ErrCodeTOTPCodeExpired,
		apperrors.ErrCodeInvalidMFASecret,
		apperrors.ErrCodeProviderNotSupported,
		apperrors.ErrCodeOAuthCodeExchangeFailed,
		apperrors.ErrCodeSocialLoginFailed,
		apperrors.ErrCodeOAuthStateInvalid,
		apperrors.ErrCodeOAuthStateExpired,
		apperrors.ErrCodeKeyNotFound,
		apperrors.ErrCodeKeyPathInvalid,
		apperrors.ErrCodeKeyParseFailed,
		apperrors.ErrCodeKeyIDEmpty,
		apperrors.ErrCodePrivateKeyNil,
		apperrors.ErrCodePublicKeyNil,
		apperrors.ErrCodeNoActiveKey,
		apperrors.ErrCodeCacheMiss,
		apperrors.ErrCodeRequestBodyTooLarge,
		apperrors.ErrCodeRequestBodyExtraData,
		apperrors.ErrCodeDBPasswordRequired,
		apperrors.ErrCodeJWTKeyRequired,
		apperrors.ErrCodeBcryptCostTooLow,
		apperrors.ErrCodeKeyPermissionOpen,
		apperrors.ErrCodeKeyTooShort,
	}

	seen := make(map[apperrors.ErrorCode]bool)
	for _, code := range codes {
		assert.False(t, seen[code], "duplicate error code: %s", code)
		seen[code] = true
	}
}

// ============================================================================
// 集成测试
// ============================================================================

func TestErrorWrappingChain(t *testing.T) {
	dbErr := errors.New("connection refused")
	serviceErr := fmt.Errorf("failed to query user: %w", dbErr)
	appErr := apperrors.Wrap(apperrors.ErrCodeInternal, "internal error", 500, serviceErr)

	// 验证错误链
	assert.True(t, errors.Is(appErr, serviceErr))
	assert.True(t, errors.Is(appErr, dbErr))

	// 验证 HTTP 状态码
	assert.Equal(t, 500, apperrors.GetHTTPStatus(appErr))

	// 验证错误码
	assert.Equal(t, apperrors.ErrCodeInternal, apperrors.GetErrorCode(appErr))
}

func TestGetHTTPStatus_WithWrappedAppError(t *testing.T) {
	innerErr := apperrors.New(apperrors.ErrCodeNotFound, "not found", 404)
	outerErr := fmt.Errorf("service: %w", innerErr)

	assert.Equal(t, 404, apperrors.GetHTTPStatus(outerErr))
	assert.Equal(t, apperrors.ErrCodeNotFound, apperrors.GetErrorCode(outerErr))
}
