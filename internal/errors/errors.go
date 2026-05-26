// Package errors 统一错误定义
// 提供项目级别的错误类型和错误码
package errors

import (
	"errors"
	"fmt"
)

// ============================================================================
// 错误码定义
// ============================================================================

// ErrorCode 错误码
type ErrorCode string

const (
	// 通用错误
	ErrCodeInternal        ErrorCode = "INTERNAL_ERROR"    // 内部错误
	ErrCodeBadRequest      ErrorCode = "BAD_REQUEST"       // 请求参数错误
	ErrCodeNotFound        ErrorCode = "NOT_FOUND"         // 资源不存在
	ErrCodeConflict        ErrorCode = "CONFLICT"          // 资源冲突
	ErrCodeUnauthorized    ErrorCode = "UNAUTHORIZED"      // 未授权
	ErrCodeForbidden       ErrorCode = "FORBIDDEN"         // 禁止访问
	ErrCodeTooManyRequests ErrorCode = "TOO_MANY_REQUESTS" // 请求过多

	// 认证相关错误
	ErrCodeInvalidCredentials ErrorCode = "INVALID_CREDENTIALS" // #nosec G101 -- 这是错误码常量，不是凭证
	ErrCodeAccountLocked      ErrorCode = "ACCOUNT_LOCKED"      // 账户锁定
	ErrCodeAccountDisabled    ErrorCode = "ACCOUNT_DISABLED"    // 账户禁用
	ErrCodeInvalidToken       ErrorCode = "INVALID_TOKEN"       // Token无效
	ErrCodeTokenExpired       ErrorCode = "TOKEN_EXPIRED"       // Token过期

	// 用户相关错误
	ErrCodeEmailExists         ErrorCode = "EMAIL_EXISTS"          // 邮箱已存在
	ErrCodeEmailInvalid        ErrorCode = "EMAIL_INVALID"         // 邮箱格式无效
	ErrCodeEmailRequired       ErrorCode = "EMAIL_REQUIRED"        // 邮箱必填
	ErrCodePasswordTooShort    ErrorCode = "PASSWORD_TOO_SHORT"    // 密码太短
	ErrCodePasswordTooLong     ErrorCode = "PASSWORD_TOO_LONG"     // 密码太长
	ErrCodePasswordRequired    ErrorCode = "PASSWORD_REQUIRED"     // 密码必填
	ErrCodePasswordMismatch    ErrorCode = "PASSWORD_MISMATCH"     // 密码不匹配
	ErrCodePasswordNoUppercase ErrorCode = "PASSWORD_NO_UPPERCASE" // 密码缺少大写
	ErrCodePasswordNoLowercase ErrorCode = "PASSWORD_NO_LOWERCASE" // 密码缺少小写
	ErrCodePasswordNoDigit     ErrorCode = "PASSWORD_NO_DIGIT"     // 密码缺少数字
	ErrCodePasswordNoSpecial   ErrorCode = "PASSWORD_NO_SPECIAL"   // 密码缺少特殊字符

	// 邮箱验证相关错误
	ErrCodeEmailAlreadyVerified    ErrorCode = "EMAIL_ALREADY_VERIFIED"    // 邮箱已验证
	ErrCodeEmailNotVerified        ErrorCode = "EMAIL_NOT_VERIFIED"        // 邮箱未验证
	ErrCodeVerificationCodeInvalid ErrorCode = "VERIFICATION_CODE_INVALID" // 验证码无效
	ErrCodeVerificationCodeExpired ErrorCode = "VERIFICATION_CODE_EXPIRED" // 验证码过期
	ErrCodeResetTokenInvalid       ErrorCode = "RESET_TOKEN_INVALID"       // #nosec G101 -- 这是错误码常量，不是凭证
	ErrCodeResetTokenExpired       ErrorCode = "RESET_TOKEN_EXPIRED"       // #nosec G101 -- 这是错误码常量，不是凭证
	ErrCodeResetTokenUsed          ErrorCode = "RESET_TOKEN_USED"          // #nosec G101 -- 这是错误码常量，不是凭证
	ErrCodeEmailRateLimitExceeded  ErrorCode = "EMAIL_RATE_LIMIT_EXCEEDED" // 邮件发送频率超限

	// OAuth相关错误
	ErrCodeInvalidClient        ErrorCode = "INVALID_CLIENT"         // 客户端无效
	ErrCodeInvalidRedirectURI   ErrorCode = "INVALID_REDIRECT_URI"   // 重定向URI无效
	ErrCodeInvalidGrantType     ErrorCode = "INVALID_GRANT_TYPE"     // 授权类型无效
	ErrCodeInvalidCode          ErrorCode = "INVALID_CODE"           // 授权码无效
	ErrCodeCodeExpired          ErrorCode = "CODE_EXPIRED"           // 授权码过期
	ErrCodeCodeUsed             ErrorCode = "CODE_USED"              // 授权码已使用
	ErrCodeInvalidCodeVerifier  ErrorCode = "INVALID_CODE_VERIFIER"  // PKCE验证器无效
	ErrCodeInvalidPKCEChallenge ErrorCode = "INVALID_PKCE_CHALLENGE" // PKCE挑战码无效

	// MFA相关错误
	ErrCodeMFAAlreadyEnabled      ErrorCode = "MFA_ALREADY_ENABLED"      // MFA已启用
	ErrCodeMFANotEnabled          ErrorCode = "MFA_NOT_ENABLED"          // MFA未启用
	ErrCodeInvalidTOTPCode        ErrorCode = "INVALID_TOTP_CODE"        // TOTP验证码无效
	ErrCodeTOTPCodeExpired        ErrorCode = "TOTP_CODE_EXPIRED"        // TOTP验证码过期
	ErrCodeInvalidMFASecret       ErrorCode = "INVALID_MFA_SECRET"       // #nosec G101 -- 这是错误码常量，不是凭证
	ErrCodeRecoveryCodeInvalid    ErrorCode = "RECOVERY_CODE_INVALID"    // 恢复码无效
	ErrCodeRecoveryCodeUsed       ErrorCode = "RECOVERY_CODE_USED"       // 恢复码已使用
	ErrCodeRecoveryCodeGeneration ErrorCode = "RECOVERY_CODE_GENERATION" // 恢复码生成失败

	// 第三方登录相关错误
	ErrCodeProviderNotSupported    ErrorCode = "PROVIDER_NOT_SUPPORTED"     // 提供商不支持
	ErrCodeOAuthCodeExchangeFailed ErrorCode = "OAUTH_CODE_EXCHANGE_FAILED" // OAuth授权码交换失败
	ErrCodeSocialLoginFailed       ErrorCode = "SOCIAL_LOGIN_FAILED"        // 社交登录失败
	ErrCodeOAuthStateInvalid       ErrorCode = "OAUTH_STATE_INVALID"        // OAuth状态无效
	ErrCodeOAuthStateExpired       ErrorCode = "OAUTH_STATE_EXPIRED"        // OAuth状态已过期

	// 密钥相关错误
	ErrCodeKeyNotFound    ErrorCode = "KEY_NOT_FOUND"    // 密钥未找到
	ErrCodeKeyPathInvalid ErrorCode = "KEY_PATH_INVALID" // 密钥路径无效
	ErrCodeKeyParseFailed ErrorCode = "KEY_PARSE_FAILED" // 密钥解析失败
	ErrCodeKeyIDEmpty     ErrorCode = "KEY_ID_EMPTY"     // 密钥ID为空
	ErrCodePrivateKeyNil  ErrorCode = "PRIVATE_KEY_NIL"  // 私钥为空
	ErrCodePublicKeyNil   ErrorCode = "PUBLIC_KEY_NIL"   // 公钥为空
	ErrCodeNoActiveKey    ErrorCode = "NO_ACTIVE_KEY"    // 无活跃密钥
	ErrCodeKeyExpired     ErrorCode = "KEY_EXPIRED"      // 密钥已过期

	// 缓存相关错误
	ErrCodeCacheMiss ErrorCode = "CACHE_MISS" // 缓存未命中

	// 限流相关错误
	ErrCodeTooManyRecoveryAttempts ErrorCode = "TOO_MANY_RECOVERY_ATTEMPTS" // 恢复码尝试次数过多

	// 请求相关错误
	ErrCodeRequestBodyTooLarge  ErrorCode = "REQUEST_BODY_TOO_LARGE"  // 请求体过大
	ErrCodeRequestBodyExtraData ErrorCode = "REQUEST_BODY_EXTRA_DATA" // 请求体包含多余数据

	// 配置相关错误
	ErrCodeDBPasswordRequired ErrorCode = "DB_PASSWORD_REQUIRED" // #nosec G101 -- 这是错误码常量，不是凭证
	ErrCodeJWTKeyRequired     ErrorCode = "JWT_KEY_REQUIRED"     // JWT密钥未设置
	ErrCodeBcryptCostTooLow   ErrorCode = "BCRYPT_COST_TOO_LOW"  // bcrypt成本过低
)

// ============================================================================
// AppError 应用错误
// ============================================================================

// AppError 应用错误
type AppError struct {
	Code       ErrorCode `json:"code"`              // 错误码
	Message    string    `json:"message"`           // 错误消息
	Details    string    `json:"details,omitempty"` // 详细信息
	HTTPStatus int       `json:"-"`                 // HTTP状态码
	Err        error     `json:"-"`                 // 原始错误
}

// Error 实现error接口
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap 返回原始错误
func (e *AppError) Unwrap() error {
	return e.Err
}

// ============================================================================
// 错误构造函数
// ============================================================================

// New 创建新的应用错误
func New(code ErrorCode, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// Wrap 包装原始错误
func Wrap(code ErrorCode, message string, httpStatus int, err error) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Err:        err,
	}
}

// WithDetails 添加详细信息
func (e *AppError) WithDetails(details string) *AppError {
	e.Details = details
	return e
}

// ============================================================================
// 预定义错误
// ============================================================================

var (
	// 内部错误
	ErrInternal = New(ErrCodeInternal, "内部服务器错误", 500)

	// 请求错误
	ErrBadRequest      = New(ErrCodeBadRequest, "请求参数错误", 400)
	ErrNotFound        = New(ErrCodeNotFound, "资源不存在", 404)
	ErrConflict        = New(ErrCodeConflict, "资源冲突", 409)
	ErrUnauthorized    = New(ErrCodeUnauthorized, "未授权", 401)
	ErrForbidden       = New(ErrCodeForbidden, "禁止访问", 403)
	ErrTooManyRequests = New(ErrCodeTooManyRequests, "请求过多，请稍后重试", 429)

	// 认证错误
	ErrInvalidCredentials = New(ErrCodeInvalidCredentials, "邮箱或密码错误", 401)
	ErrAccountLocked      = New(ErrCodeAccountLocked, "账户已锁定", 403)
	ErrAccountDisabled    = New(ErrCodeAccountDisabled, "账户已被禁用", 403)
	ErrInvalidToken       = New(ErrCodeInvalidToken, "无效的Token", 401)
	ErrTokenExpired       = New(ErrCodeTokenExpired, "Token已过期", 401)

	// 用户错误
	ErrEmailExists         = New(ErrCodeEmailExists, "邮箱已注册", 409)
	ErrEmailInvalid        = New(ErrCodeEmailInvalid, "邮箱地址格式无效", 400)
	ErrEmailRequired       = New(ErrCodeEmailRequired, "邮箱地址不能为空", 400)
	ErrPasswordTooShort    = New(ErrCodePasswordTooShort, "密码长度不能少于8个字符", 400)
	ErrPasswordTooLong     = New(ErrCodePasswordTooLong, "密码长度不能超过72个字符", 400)
	ErrPasswordRequired    = New(ErrCodePasswordRequired, "密码不能为空", 400)
	ErrPasswordMismatch    = New(ErrCodePasswordMismatch, "密码不匹配", 400)
	ErrPasswordNoUppercase = New(ErrCodePasswordNoUppercase, "密码必须包含至少一个大写字母", 400)
	ErrPasswordNoLowercase = New(ErrCodePasswordNoLowercase, "密码必须包含至少一个小写字母", 400)
	ErrPasswordNoDigit     = New(ErrCodePasswordNoDigit, "密码必须包含至少一个数字", 400)
	ErrPasswordNoSpecial   = New(ErrCodePasswordNoSpecial, "密码必须包含至少一个特殊字符", 400)

	// 邮箱验证错误
	ErrEmailAlreadyVerified    = New(ErrCodeEmailAlreadyVerified, "邮箱已验证", 409)
	ErrEmailNotVerified        = New(ErrCodeEmailNotVerified, "请先验证邮箱后再登录", 401)
	ErrVerificationCodeInvalid = New(ErrCodeVerificationCodeInvalid, "验证码无效", 400)
	ErrVerificationCodeExpired = New(ErrCodeVerificationCodeExpired, "验证码已过期", 400)
	ErrResetTokenInvalid       = New(ErrCodeResetTokenInvalid, "重置令牌无效", 400)
	ErrResetTokenExpired       = New(ErrCodeResetTokenExpired, "重置令牌已过期", 400)
	ErrResetTokenUsed          = New(ErrCodeResetTokenUsed, "重置令牌已被使用", 400)
	ErrEmailRateLimitExceeded  = New(ErrCodeEmailRateLimitExceeded, "邮件发送过于频繁，请稍后再试", 429)

	// OAuth错误
	ErrInvalidClient        = New(ErrCodeInvalidClient, "无效的客户端", 400)
	ErrInvalidRedirectURI   = New(ErrCodeInvalidRedirectURI, "无效的重定向URI", 400)
	ErrInvalidGrantType     = New(ErrCodeInvalidGrantType, "无效的授权类型", 400)
	ErrInvalidCode          = New(ErrCodeInvalidCode, "无效的授权码", 400)
	ErrCodeExpiredErr       = New(ErrCodeCodeExpired, "授权码已过期", 400)
	ErrCodeUsedErr          = New(ErrCodeCodeUsed, "授权码已被使用", 400)
	ErrInvalidCodeVerifier  = New(ErrCodeInvalidCodeVerifier, "无效的PKCE验证器", 400)
	ErrInvalidPKCEChallenge = New(ErrCodeInvalidPKCEChallenge, "无效的PKCE挑战码", 400)

	// MFA错误
	ErrMFAAlreadyEnabled    = New(ErrCodeMFAAlreadyEnabled, "MFA已启用", 409)
	ErrMFANotEnabled        = New(ErrCodeMFANotEnabled, "MFA未启用", 400)
	ErrInvalidTOTPCode      = New(ErrCodeInvalidTOTPCode, "验证码错误", 400)
	ErrTOTPCodeExpired      = New(ErrCodeTOTPCodeExpired, "验证码已过期", 400)
	ErrInvalidMFASecret     = New(ErrCodeInvalidMFASecret, "MFA密钥无效", 400)
	ErrRecoveryCodeInvalid  = New(ErrCodeRecoveryCodeInvalid, "恢复码无效", 400)
	ErrRecoveryCodeUsed     = New(ErrCodeRecoveryCodeUsed, "恢复码已使用", 400)
	ErrRecoveryCodeGenerate = New(ErrCodeRecoveryCodeGeneration, "恢复码生成失败", 500)

	// 第三方登录错误
	ErrProviderNotSupported    = New(ErrCodeProviderNotSupported, "不支持的登录提供商", 400)
	ErrOAuthCodeExchangeFailed = New(ErrCodeOAuthCodeExchangeFailed, "OAuth授权码交换失败", 400)
	ErrSocialLoginFailed       = New(ErrCodeSocialLoginFailed, "社交登录失败", 400)
	ErrOAuthStateInvalid       = New(ErrCodeOAuthStateInvalid, "OAuth状态无效", 400)
	ErrOAuthStateExpired       = New(ErrCodeOAuthStateExpired, "OAuth状态已过期，请重新发起登录", 400)

	// 密钥错误
	ErrKeyNotFound    = New(ErrCodeKeyNotFound, "密钥未找到", 500)
	ErrKeyPathInvalid = New(ErrCodeKeyPathInvalid, "密钥路径无效", 500)
	ErrKeyParseFailed = New(ErrCodeKeyParseFailed, "密钥解析失败", 500)
	ErrKeyIDEmpty     = New(ErrCodeKeyIDEmpty, "密钥ID不能为空", 400)
	ErrPrivateKeyNil  = New(ErrCodePrivateKeyNil, "私钥不能为空", 400)
	ErrPublicKeyNil   = New(ErrCodePublicKeyNil, "公钥不能为空", 400)
	ErrNoActiveKey    = New(ErrCodeNoActiveKey, "无活跃密钥可用", 500)
	ErrKeyExpired     = New(ErrCodeKeyExpired, "密钥已过期", 401)

	// 缓存错误
	ErrCacheMiss = New(ErrCodeCacheMiss, "缓存未命中", 404)

	// 限流错误
	ErrTooManyRecoveryAttempts = New(ErrCodeTooManyRecoveryAttempts, "恢复码尝试次数过多，请稍后再试", 429)

	// 请求错误
	ErrRequestBodyTooLarge  = New(ErrCodeRequestBodyTooLarge, "请求体过大", 413)
	ErrRequestBodyExtraData = New(ErrCodeRequestBodyExtraData, "请求体包含多余数据", 400)

	// 配置错误
	ErrDBPasswordRequired = New(ErrCodeDBPasswordRequired, "DB_PASSWORD环境变量必须设置", 500)
	ErrJWTKeyRequired     = New(ErrCodeJWTKeyRequired, "JWT密钥路径必须设置", 500)
	ErrBcryptCostTooLow   = New(ErrCodeBcryptCostTooLow, "生产环境bcrypt cost必须 >= 12", 500)
)

// ============================================================================
// Handler消息错误码
// ============================================================================

const (
	// 通用消息
	ErrCodeInvalidRequestFormat    ErrorCode = "INVALID_REQUEST_FORMAT"
	ErrCodeMissingRequiredParam    ErrorCode = "MISSING_REQUIRED_PARAM"
	ErrCodeMissingUserID           ErrorCode = "MISSING_USER_ID"
	ErrCodeMissingToken            ErrorCode = "MISSING_TOKEN"
	ErrCodeMissingCode             ErrorCode = "MISSING_CODE"
	ErrCodeMissingClientID         ErrorCode = "MISSING_CLIENT_ID"
	ErrCodeMissingRedirectURI      ErrorCode = "MISSING_REDIRECT_URI"
	ErrCodeMissingRefreshToken     ErrorCode = "MISSING_REFRESH_TOKEN"
	ErrCodeMissingAuthCode         ErrorCode = "MISSING_AUTH_CODE"
	ErrCodeMissingOldPassword      ErrorCode = "MISSING_OLD_PASSWORD"
	ErrCodeMissingNewPassword      ErrorCode = "MISSING_NEW_PASSWORD"
	ErrCodeMissingVerificationCode ErrorCode = "MISSING_VERIFICATION_CODE"

	// OAuth消息
	ErrCodeStateInvalid        ErrorCode = "STATE_INVALID"
	ErrCodeInvalidRefreshToken ErrorCode = "INVALID_REFRESH_TOKEN"

	// 操作失败消息
	ErrCodeLoginFailed                 ErrorCode = "LOGIN_FAILED"
	ErrCodeRegisterFailed              ErrorCode = "REGISTER_FAILED"
	ErrCodeLogoutFailed                ErrorCode = "LOGOUT_FAILED"
	ErrCodeSendVerificationEmailFailed ErrorCode = "SEND_VERIFICATION_EMAIL_FAILED"
	ErrCodeVerifyEmailFailed           ErrorCode = "VERIFY_EMAIL_FAILED"
	ErrCodeForgotPasswordFailed        ErrorCode = "FORGOT_PASSWORD_FAILED"
	ErrCodeResetPasswordFailed         ErrorCode = "RESET_PASSWORD_FAILED"
	ErrCodeChangePasswordFailed        ErrorCode = "CHANGE_PASSWORD_FAILED"
	ErrCodeRefreshTokenFailed          ErrorCode = "REFRESH_TOKEN_FAILED"
	ErrCodeRevokeTokenFailed           ErrorCode = "REVOKE_TOKEN_FAILED"  // #nosec G101 -- 这是错误码常量，不是凭证
	ErrCodeExchangeCodeFailed          ErrorCode = "EXCHANGE_CODE_FAILED" // #nosec G101 -- 这是错误码常量，不是凭证

	// MFA消息
	ErrCodeSetupMFAFailed     ErrorCode = "SETUP_MFA_FAILED"
	ErrCodeVerifyMFAFailed    ErrorCode = "VERIFY_MFA_FAILED"
	ErrCodeDisableMFAFailed   ErrorCode = "DISABLE_MFA_FAILED"
	ErrCodeGetMFAStatusFailed ErrorCode = "GET_MFA_STATUS_FAILED"

	// 管理员消息
	ErrCodeListUsersFailed    ErrorCode = "LIST_USERS_FAILED"
	ErrCodeGetUserFailed      ErrorCode = "GET_USER_FAILED"
	ErrCodeDisableUserFailed  ErrorCode = "DISABLE_USER_FAILED"
	ErrCodeEnableUserFailed   ErrorCode = "ENABLE_USER_FAILED"
	ErrCodeSystemHealthFailed ErrorCode = "SYSTEM_HEALTH_FAILED"
	ErrCodeCleanupFailed      ErrorCode = "CLEANUP_FAILED"

	// 授权消息
	ErrCodeUnsupportedGrantType   ErrorCode = "UNSUPPORTED_GRANT_TYPE"
	ErrCodeUnsupportedLoginMethod ErrorCode = "UNSUPPORTED_LOGIN_METHOD"
)

// ============================================================================
// 错误判断函数
// ============================================================================

// Is 判断错误是否为目标错误
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As 将错误转换为目标类型
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}

// GetHTTPStatus 获取错误的HTTP状态码
func GetHTTPStatus(err error) int {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.HTTPStatus
	}
	return 500
}

// GetErrorCode 获取错误码
func GetErrorCode(err error) ErrorCode {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ErrCodeInternal
}

// ============================================================================
// 额外的密钥错误
// ============================================================================

const (
	ErrCodeKeyPermissionOpen ErrorCode = "KEY_PERMISSION_OPEN" // 密钥权限过于开放
	ErrCodeKeyTooShort       ErrorCode = "KEY_TOO_SHORT"       // RSA密钥长度不足
)

var (
	ErrKeyPermissionOpen = New(ErrCodeKeyPermissionOpen, "密钥文件权限不安全", 500)
	ErrKeyTooShort       = New(ErrCodeKeyTooShort, "RSA密钥长度必须至少为2048位", 500)
)
