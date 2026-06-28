// Package handler MFA处理器
// 处理多因素认证相关的HTTP请求
package handler

import (
	"errors"
	"net/http"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/util/handlerutil"
)

// ============================================================================
// MFAHandler MFA处理器
// ============================================================================

// MFAHandler MFA处理器
type MFAHandler struct {
	mfaSvc service.MFAServiceInterface
}

// NewMFAHandler 创建MFA处理器
func NewMFAHandler(mfaSvc service.MFAServiceInterface) *MFAHandler {
	return &MFAHandler{mfaSvc: mfaSvc}
}

// MaxRecoveryCodeCount 单次生成恢复码的最大数量（与服务层限制保持一致）
const MaxRecoveryCodeCount = 20

// HandleSetupMFA 处理MFA设置请求
// POST /api/v1/mfa/setup
func (h *MFAHandler) HandleSetupMFA(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	result, err := h.mfaSvc.SetupMFA(r.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrMFAAlreadyEnabled) {
			writeError(w, http.StatusConflict, getMessage(r, apperrors.ErrCodeMFAAlreadyEnabled))
			return
		}
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeSetupMFAFailed))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"secret":       result.Secret,
		"qr_code_url":  result.QRCodeURL,
		"manual_entry": result.ManualEntry,
	})
}

// HandleVerifyMFA 处理MFA验证请求
// POST /api/v1/mfa/verify
func (h *MFAHandler) HandleVerifyMFA(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	if req.Code == "" {
		handlerutil.WriteValidationError(w, "code", getMessage(r, apperrors.ErrCodeMissingVerificationCode))
		return
	}

	err := h.mfaSvc.VerifyAndEnableMFA(r.Context(), userID, req.Code)
	if err != nil {
		if errors.Is(err, service.ErrInvalidTOTPCode) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidTOTPCode))
			return
		}
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeVerifyMFAFailed))
		return
	}

	writeSuccess(w, http.StatusOK, "MFA已启用", nil)
}

// HandleDisableMFA 处理禁用MFA请求
// POST /api/v1/mfa/disable
func (h *MFAHandler) HandleDisableMFA(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	if req.Code == "" {
		handlerutil.WriteValidationError(w, "code", getMessage(r, apperrors.ErrCodeMissingVerificationCode))
		return
	}

	err := h.mfaSvc.DisableMFA(r.Context(), userID, req.Code)
	if err != nil {
		if errors.Is(err, service.ErrMFANotEnabled) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMFANotEnabled))
			return
		}
		if errors.Is(err, service.ErrInvalidTOTPCode) {
			writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidTOTPCode))
			return
		}
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeDisableMFAFailed))
		return
	}

	writeSuccess(w, http.StatusOK, "MFA已禁用", nil)
}

// HandleMFAStatus 处理MFA状态查询
// GET /api/v1/mfa/status
func (h *MFAHandler) HandleMFAStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	status, err := h.mfaSvc.GetMFAStatus(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeGetMFAStatusFailed))
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// ============================================================================
// MFA恢复码处理器
// ============================================================================

// HandleGenerateRecoveryCodes 处理生成恢复码请求
// POST /api/v1/mfa/recovery-codes/generate
func (h *MFAHandler) HandleGenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		handlerutil.WriteJSONError(w, apperrors.ErrUnauthorized)
		return
	}

	var req struct {
		Count int `json:"count"`
	}
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	count := req.Count
	if count <= 0 {
		count = 8
	}
	if count > MaxRecoveryCodeCount {
		handlerutil.WriteValidationError(w, "count", getMessage(r, apperrors.ErrCodeBadRequest))
		return
	}

	codes, err := h.mfaSvc.GenerateRecoveryCodes(r.Context(), userID, count)
	if err != nil {
		handlerutil.WriteJSONError(w, err)
		return
	}

	handlerutil.WriteJSONSuccess(w, map[string]interface{}{
		"codes": codes,
	})
}

// HandleVerifyRecoveryCode 处理验证恢复码请求
// POST /api/v1/mfa/recovery-codes/verify
func (h *MFAHandler) HandleVerifyRecoveryCode(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		handlerutil.WriteJSONError(w, apperrors.ErrUnauthorized)
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	if req.Code == "" {
		handlerutil.WriteValidationError(w, "code", getMessage(r, apperrors.ErrCodeMissingVerificationCode))
		return
	}

	valid, err := h.mfaSvc.VerifyRecoveryCode(r.Context(), userID, req.Code, extractClientIP(r))
	if err != nil {
		handlerutil.WriteJSONError(w, err)
		return
	}

	if !valid {
		handlerutil.WriteJSONError(w, apperrors.ErrRecoveryCodeInvalid)
		return
	}

	handlerutil.WriteJSONSuccess(w, map[string]bool{
		"valid": true,
	})
}

// HandleGetRecoveryCodeStatus 处理获取恢复码状态请求
// GET /api/v1/mfa/recovery-codes/status
func (h *MFAHandler) HandleGetRecoveryCodeStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		handlerutil.WriteJSONError(w, apperrors.ErrUnauthorized)
		return
	}

	count, err := h.mfaSvc.GetRecoveryCodeStatus(r.Context(), userID)
	if err != nil {
		handlerutil.WriteJSONError(w, err)
		return
	}

	handlerutil.WriteJSONSuccess(w, map[string]int{
		"remaining": count,
	})
}
