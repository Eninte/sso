// Package handler HTTP处理器
// 提供API端点的请求处理
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/service"
)

// 请求体大小限制常量
const (
	MaxRequestBodySize = 1 << 20 // 1MB
)

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
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
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
	default:
		writeJSON(w, http.StatusInternalServerError, apperrors.ErrInternal.ToLocalizedResponse(lang))
	}
}
