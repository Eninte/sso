// Package handler HTTP处理器
// 提供API端点的请求处理
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/util/handlerutil"
	"github.com/example/sso/internal/validator"
)

// 请求体大小限制常量
const (
	MaxRequestBodySize = 1 << 20 // 1MB
)

// ============================================================================
// 验证码接口和辅助函数
// ============================================================================

// captchaVerifier 验证码验证接口
// 由 captcha.Service 实现，便于测试时使用 mock
type captchaVerifier interface {
	IsEnabled() bool
	ShouldRequireCaptcha(ctx context.Context, key string) bool
	// ShouldRequireCaptchaForAccount T15：账号（邮箱）维度触发判定，与 IP 维度并行
	ShouldRequireCaptchaForAccount(ctx context.Context, account string) bool
	Verify(ctx context.Context, id, answer string) (bool, error)
	RecordFailure(ctx context.Context, key string)
	// RecordAccountFailure T15：账号维度失败计数（键为归一化邮箱的 SHA-256）
	RecordAccountFailure(ctx context.Context, account string)
	ClearFailures(ctx context.Context, key string)
	// ClearAccountFailures T15：登录成功后清除账号维度失败计数
	ClearAccountFailures(ctx context.Context, account string)
}

// isCredentialError 判断是否为凭据相关错误（401/403）
// 仅此类错误应触发验证码失败计数，排除服务器内部错误和限流错误
func isCredentialError(err error) bool {
	return errors.Is(err, service.ErrInvalidCredentials) ||
		errors.Is(err, service.ErrAccountLocked) ||
		errors.Is(err, service.ErrAccountDisabled) ||
		errors.Is(err, service.ErrEmailNotVerified)
}

// verifyCaptcha 自适应验证码校验逻辑
// 仅当失败次数达到阈值时才要求验证码。
// T15：IP 维度与账号（邮箱）维度并行判定，任一维度达到阈值即要求验证码，
// 防止攻击者更换 IP 对同一账号无限尝试；account 为空时仅按 IP 维度判定
// 返回 true 表示验证通过（或不需要验证码），false 表示验证失败（已写入响应）
func verifyCaptcha(w http.ResponseWriter, r *http.Request, svc captchaVerifier, account string) bool {
	if !svc.IsEnabled() {
		return true
	}

	// 自适应触发：IP 维度或账号维度任一达到阈值即要求验证码
	clientIP := extractClientIP(r)
	if !svc.ShouldRequireCaptcha(r.Context(), clientIP) &&
		!svc.ShouldRequireCaptchaForAccount(r.Context(), account) {
		return true // 两个维度均未达阈值，跳过验证码
	}

	// 从请求头获取验证码信息
	captchaID := r.Header.Get("X-Captcha-Id")
	captchaAnswer := r.Header.Get("X-Captcha-Answer")

	if captchaID == "" || captchaAnswer == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeCaptchaRequired))
		return false
	}

	ok, err := svc.Verify(r.Context(), captchaID, captchaAnswer)
	if err != nil || !ok {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeCaptchaInvalid))
		return false
	}

	return true
}

// 请求处理错误定义（使用统一错误定义）
var (
	ErrRequestBodyTooLarge  = apperrors.ErrRequestBodyTooLarge
	ErrRequestBodyExtraData = apperrors.ErrRequestBodyExtraData
)

// ============================================================================
// 辅助函数
// ============================================================================

// getMessage 获取本地化的错误消息
func getMessage(r *http.Request, code apperrors.ErrorCode) string {
	lang := middleware.GetLanguageFromContext(r.Context())
	return apperrors.GetMessage(code, lang)
}

// writeJSON 写入JSON响应
// 委托给 handlerutil.WriteJSON，统一处理编码错误
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	handlerutil.WriteJSON(w, status, data)
}

// writeError 写入错误响应
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}

// writeLocalizedError 写入本地化错误响应
func writeLocalizedError(w http.ResponseWriter, r *http.Request, appErr *apperrors.AppError) {
	lang := middleware.GetLanguageFromContext(r.Context())
	writeJSON(w, appErr.HTTPStatus, appErr.ToLocalizedResponse(lang))
}

// writeServiceError 写入 service 层错误响应（T14）
// 保留错误链中 AppError 的 HTTP 状态码与本地化消息
// （如 403 本人操作防护、409 末位管理员保护、404 资源不存在）；
// 非 AppError 一律返回 500，不暴露内部实现细节
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *apperrors.AppError
	if apperrors.As(err, &appErr) {
		writeLocalizedError(w, r, appErr)
		return
	}
	writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeInternal))
}

// writeSuccess 写入成功响应
func writeSuccess(w http.ResponseWriter, status int, message string, data interface{}) {
	response := map[string]interface{}{
		"message": message,
	}
	if data != nil {
		response["data"] = data
	}
	writeJSON(w, status, response)
}

// decodeJSON 安全的JSON解码
func decodeJSON(r *http.Request, v interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, MaxRequestBodySize)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(v); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return ErrRequestBodyTooLarge
		}
		return err
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ErrRequestBodyExtraData
	}

	return nil
}

// handleDecodeJSONError 统一处理decodeJSON错误
// 根据错误类型返回精确的HTTP状态码和错误消息
func handleDecodeJSONError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrRequestBodyTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, getMessage(r, apperrors.ErrCodeRequestBodyTooLarge))
	case errors.Is(err, ErrRequestBodyExtraData):
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeRequestBodyExtraData))
	default:
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidRequestFormat))
	}
}

// writeOAuthError 统一处理OAuth相关错误，支持本地化
// 这是一个通用的错误处理函数，可以处理所有类型的服务错误
func writeOAuthError(w http.ResponseWriter, r *http.Request, err error) {
	lang := middleware.GetLanguageFromContext(r.Context())

	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		writeJSON(w, appErr.HTTPStatus, appErr.ToLocalizedResponse(lang))
		return
	}

	// 处理服务层错误
	switch {
	case errors.Is(err, service.ErrInvalidClient):
		writeJSON(w, http.StatusBadRequest, apperrors.ErrInvalidClient.ToLocalizedResponse(lang))
	case errors.Is(err, service.ErrInvalidRedirectURI):
		writeJSON(w, http.StatusBadRequest, apperrors.ErrInvalidRedirectURI.ToLocalizedResponse(lang))
	case errors.Is(err, service.ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, apperrors.ErrInvalidCredentials.ToLocalizedResponse(lang))
	case errors.Is(err, service.ErrAccountLocked):
		writeJSON(w, http.StatusForbidden, apperrors.ErrAccountLocked.ToLocalizedResponse(lang))
	case errors.Is(err, service.ErrAccountDisabled):
		writeJSON(w, http.StatusForbidden, apperrors.ErrAccountDisabled.ToLocalizedResponse(lang))
	case errors.Is(err, service.ErrInvalidToken):
		writeJSON(w, http.StatusUnauthorized, apperrors.ErrInvalidToken.ToLocalizedResponse(lang))
	case errors.Is(err, service.ErrEmailNotVerified):
		writeJSON(w, http.StatusUnauthorized, apperrors.ErrEmailNotVerified.ToLocalizedResponse(lang))
	default:
		writeJSON(w, http.StatusInternalServerError, apperrors.ErrInternal.ToLocalizedResponse(lang))
	}
}

// ============================================================================
// 验证错误处理
// ============================================================================

// validationError 定义验证错误映射
type validationError struct {
	err        error
	code       apperrors.ErrorCode
	httpStatus int
}

// validationErrors 验证错误映射表
var validationErrors = []validationError{
	// 邮箱相关错误
	{validator.ErrEmailRequired, apperrors.ErrCodeEmailRequired, http.StatusBadRequest},
	{validator.ErrEmailInvalid, apperrors.ErrCodeEmailInvalid, http.StatusBadRequest},

	// 密码相关错误
	{validator.ErrPasswordRequired, apperrors.ErrCodePasswordRequired, http.StatusBadRequest},
	{validator.ErrPasswordTooShort, apperrors.ErrCodePasswordTooShort, http.StatusBadRequest},
	{validator.ErrPasswordTooLong, apperrors.ErrCodePasswordTooLong, http.StatusBadRequest},
	{validator.ErrPasswordNoUppercase, apperrors.ErrCodePasswordNoUppercase, http.StatusBadRequest},
	{validator.ErrPasswordNoLowercase, apperrors.ErrCodePasswordNoLowercase, http.StatusBadRequest},
	{validator.ErrPasswordNoDigit, apperrors.ErrCodePasswordNoDigit, http.StatusBadRequest},
	{validator.ErrPasswordNoSpecial, apperrors.ErrCodePasswordNoSpecial, http.StatusBadRequest},

	// 认证相关错误
	{service.ErrInvalidCredentials, apperrors.ErrCodeInvalidCredentials, http.StatusUnauthorized},
	{apperrors.ErrEmailExists, apperrors.ErrCodeEmailExists, http.StatusConflict},

	// 邮箱验证相关错误
	{service.ErrEmailAlreadyVerified, apperrors.ErrCodeEmailAlreadyVerified, http.StatusConflict},
}

// writeValidationError 统一处理验证错误
// 返回true表示错误已处理，false表示未知错误
func writeValidationError(w http.ResponseWriter, r *http.Request, err error) bool {
	lang := middleware.GetLanguageFromContext(r.Context())

	for _, ve := range validationErrors {
		if errors.Is(err, ve.err) {
			writeJSON(w, ve.httpStatus, map[string]string{
				"error": apperrors.GetMessage(ve.code, lang),
			})
			return true
		}
	}

	return false
}

// handleServiceError 统一处理服务层错误
// 首先尝试使用writeValidationError，失败则使用默认错误码
func handleServiceError(w http.ResponseWriter, r *http.Request, err error, defaultCode apperrors.ErrorCode) {
	if writeValidationError(w, r, err) {
		return
	}

	// 未知错误，使用默认错误码
	lang := middleware.GetLanguageFromContext(r.Context())
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"error": apperrors.GetMessage(defaultCode, lang),
	})
}
