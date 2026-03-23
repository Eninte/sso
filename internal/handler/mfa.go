// Package handler MFA处理器
// 处理多因素认证相关的HTTP请求
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/service"
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

// HandleSetupMFA 处理MFA设置请求
// POST /api/v1/mfa/setup
func (h *MFAHandler) HandleSetupMFA(w http.ResponseWriter, r *http.Request) {
	// 1. 获取当前用户ID
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "未认证")
		return
	}

	// 2. 设置MFA
	result, err := h.mfaSvc.SetupMFA(r.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrMFAAlreadyEnabled) {
			writeError(w, http.StatusConflict, "MFA已启用")
			return
		}
		writeError(w, http.StatusInternalServerError, "设置MFA失败")
		return
	}

	// 3. 返回设置结果
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"secret":       result.Secret,
		"qr_code_url":  result.QRCodeURL,
		"manual_entry": result.ManualEntry,
	})
}

// HandleVerifyMFA 处理MFA验证请求
// POST /api/v1/mfa/verify
func (h *MFAHandler) HandleVerifyMFA(w http.ResponseWriter, r *http.Request) {
	// 1. 获取当前用户ID
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "未认证")
		return
	}

	// 2. 解析请求
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求格式")
		return
	}

	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "验证码不能为空")
		return
	}

	// 3. 验证并启用MFA
	err := h.mfaSvc.VerifyAndEnableMFA(r.Context(), userID, req.Code)
	if err != nil {
		if errors.Is(err, service.ErrInvalidTOTPCode) {
			writeError(w, http.StatusBadRequest, "验证码错误")
			return
		}
		writeError(w, http.StatusInternalServerError, "验证失败")
		return
	}

	writeSuccess(w, http.StatusOK, "MFA已启用", nil)
}

// HandleDisableMFA 处理禁用MFA请求
// POST /api/v1/mfa/disable
func (h *MFAHandler) HandleDisableMFA(w http.ResponseWriter, r *http.Request) {
	// 1. 获取当前用户ID
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "未认证")
		return
	}

	// 2. 解析请求
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求格式")
		return
	}

	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "验证码不能为空")
		return
	}

	// 3. 禁用MFA
	err := h.mfaSvc.DisableMFA(r.Context(), userID, req.Code)
	if err != nil {
		if errors.Is(err, service.ErrMFANotEnabled) {
			writeError(w, http.StatusBadRequest, "MFA未启用")
			return
		}
		if errors.Is(err, service.ErrInvalidTOTPCode) {
			writeError(w, http.StatusBadRequest, "验证码错误")
			return
		}
		writeError(w, http.StatusInternalServerError, "禁用MFA失败")
		return
	}

	writeSuccess(w, http.StatusOK, "MFA已禁用", nil)
}

// HandleMFAStatus 处理MFA状态查询
// GET /api/v1/mfa/status
func (h *MFAHandler) HandleMFAStatus(w http.ResponseWriter, r *http.Request) {
	// 1. 获取当前用户ID
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "未认证")
		return
	}

	// 2. 获取MFA状态
	status, err := h.mfaSvc.GetMFAStatus(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取MFA状态失败")
		return
	}

	// 3. 返回MFA状态
	writeJSON(w, http.StatusOK, status)
}
