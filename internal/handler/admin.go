// Package handler 管理员处理器
// 提供管理员API
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/service"
)

// ============================================================================
// 常量定义
// ============================================================================

const (
	// DefaultPageSize 默认分页大小
	DefaultPageSize = 20

	// MaxPageSize 最大分页大小
	MaxPageSize = 100
)

// ============================================================================
// AdminHandler 管理员处理器
// ============================================================================

// AdminHandler 管理员处理器
// 注意：管理员权限检查由 AdminMiddleware 处理
type AdminHandler struct {
	adminSvc service.AdminServiceInterface
}

// NewAdminHandler 创建管理员处理器
func NewAdminHandler(adminSvc service.AdminServiceInterface) *AdminHandler {
	return &AdminHandler{adminSvc: adminSvc}
}

// ============================================================================
// 用户管理
// ============================================================================

// HandleListUsers 处理用户列表请求
// GET /admin/users?page=1&pageSize=20
// 注意：管理员权限检查由 AdminMiddleware 处理
func (h *AdminHandler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	// 解析分页参数
	page := 1
	pageSize := DefaultPageSize

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if pageSizeStr := r.URL.Query().Get("pageSize"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= MaxPageSize {
			pageSize = ps
		}
	}

	// 计算偏移量
	offset := (page - 1) * pageSize

	// 通过Service获取用户列表
	users, total, err := h.adminSvc.ListUsers(r.Context(), offset, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeListUsersFailed))
		return
	}

	// 构建响应
	userList := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		userList = append(userList, map[string]interface{}{
			"id":             user.ID,
			"email":          user.Email,
			"email_verified": user.EmailVerified,
			"mfa_enabled":    user.MFAEnabled,
			"status":         user.Status,
			"created_at":     user.CreatedAt,
			"updated_at":     user.UpdatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"users":       userList,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + pageSize - 1) / pageSize,
	})
}

// HandleGetUser 处理获取用户请求
// GET /admin/users/{id}
// 注意：管理员权限检查由 AdminMiddleware 处理
func (h *AdminHandler) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	// 使用mux.Vars安全获取路由参数，避免路径遍历
	vars := mux.Vars(r)
	userID := vars["id"]

	// 也支持查询参数（向后兼容）
	if userID == "" {
		userID = r.URL.Query().Get("id")
	}

	if userID == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingUserID))
		return
	}

	// 通过Service获取用户
	user, err := h.adminSvc.GetUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, getMessage(r, apperrors.ErrCodeNotFound))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":             user.ID,
		"email":          user.Email,
		"email_verified": user.EmailVerified,
		"mfa_enabled":    user.MFAEnabled,
		"status":         user.Status,
		"created_at":     user.CreatedAt,
		"updated_at":     user.UpdatedAt,
	})
}

// HandleDisableUser 处理禁用用户请求
// POST /admin/users/disable
// 注意：管理员权限检查由 AdminMiddleware 处理
func (h *AdminHandler) HandleDisableUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidRequestFormat))
		return
	}

	// 通过Service禁用用户
	if err := h.adminSvc.DisableUser(r.Context(), req.UserID); err != nil {
		writeError(w, http.StatusNotFound, getMessage(r, apperrors.ErrCodeNotFound))
		return
	}

	writeSuccess(w, http.StatusOK, "用户已禁用", nil)
}

// HandleEnableUser 处理启用用户请求
// POST /admin/users/enable
// 注意：管理员权限检查由 AdminMiddleware 处理
func (h *AdminHandler) HandleEnableUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeInvalidRequestFormat))
		return
	}

	// 通过Service启用用户
	if err := h.adminSvc.EnableUser(r.Context(), req.UserID); err != nil {
		writeError(w, http.StatusNotFound, getMessage(r, apperrors.ErrCodeNotFound))
		return
	}

	writeSuccess(w, http.StatusOK, "用户已启用", nil)
}

// ============================================================================
// 系统管理
// ============================================================================

// HandleSystemHealth 处理系统健康检查
// GET /admin/health
// 注意：管理员权限检查由 AdminMiddleware 处理
func (h *AdminHandler) HandleSystemHealth(w http.ResponseWriter, r *http.Request) {
	// 通过Service获取系统健康信息
	health, err := h.adminSvc.SystemHealth(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeSystemHealthFailed))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    health.Status,
		"timestamp": health.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		"database":  health.Database,
		"version":   health.Version,
	})
}

// HandleCleanup 处理清理过期数据请求
// POST /admin/cleanup
// 注意：管理员权限检查由 AdminMiddleware 处理
func (h *AdminHandler) HandleCleanup(w http.ResponseWriter, r *http.Request) {
	// 通过Service清理过期数据
	if err := h.adminSvc.CleanupExpired(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeCleanupFailed))
		return
	}

	writeSuccess(w, http.StatusOK, "清理完成", nil)
}
