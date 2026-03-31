// Package handler 测试专用API
// 仅用于E2E测试环境
package handler

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/your-org/sso/internal/store"
)

// ============================================================================
// TestHandler 测试专用处理器
// ============================================================================

// TestHandler 测试专用处理器
// 仅在测试环境中启用
type TestHandler struct {
	store store.Store
	env   string
}

// NewTestHandler 创建测试专用处理器
func NewTestHandler(store store.Store, env string) *TestHandler {
	return &TestHandler{
		store: store,
		env:   env,
	}
}

// checkEnvironment 检查是否允许使用测试API
// 仅在开发环境或E2E测试模式下允许
func (h *TestHandler) checkEnvironment(w http.ResponseWriter) bool {
	if h.env != "development" && os.Getenv("E2E_ENABLED") != "true" {
		writeError(w, http.StatusForbidden, "测试API仅限开发环境使用")
		return false
	}
	return true
}

// HandleVerifyEmail 测试专用验证邮箱API
// POST /api/v1/test/verify-email
func (h *TestHandler) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnvironment(w) {
		return
	}
	var req struct {
		UserID string `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求格式")
		return
	}

	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id不能为空")
		return
	}

	// 直接更新用户的邮箱验证状态
	user, err := h.store.GetByID(r.Context(), req.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}

	user.EmailVerified = true
	if err := h.store.Update(r.Context(), user); err != nil {
		writeError(w, http.StatusInternalServerError, "更新失败")
		return
	}

	writeSuccess(w, http.StatusOK, "邮箱验证成功", nil)
}

// HandleSetRole 测试专用设置用户角色API
// POST /api/v1/test/set-role
func (h *TestHandler) HandleSetRole(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnvironment(w) {
		return
	}
	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求格式")
		return
	}

	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id不能为空")
		return
	}

	if req.Role != "admin" && req.Role != "user" {
		writeError(w, http.StatusBadRequest, "role必须是admin或user")
		return
	}

	// 直接更新用户的角色
	user, err := h.store.GetByID(r.Context(), req.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}

	user.Role = req.Role
	if err := h.store.Update(r.Context(), user); err != nil {
		writeError(w, http.StatusInternalServerError, "更新失败")
		return
	}

	writeSuccess(w, http.StatusOK, "角色设置成功", nil)
}
