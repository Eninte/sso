// Package handler 管理员处理器
// 提供管理员API
package handler

import (
	"context"
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

// ============================================================================
// 用户状态管理（管理员权限）
// ============================================================================

// handleUserStatusChange 处理用户状态变更请求（通用方法）
// 用于禁用/启用用户等操作
func (h *AdminHandler) handleUserStatusChange(
	w http.ResponseWriter,
	r *http.Request,
	action func(context.Context, string) error,
	successMessage string,
) {
	var userID string

	// 优先从URL路径参数获取user_id
	vars := mux.Vars(r)
	if id, ok := vars["id"]; ok && id != "" {
		userID = id
	} else {
		// 兼容旧的请求体方式
		var req struct {
			UserID string `json:"user_id"`
		}
		if err := decodeJSON(r, &req); err != nil {
			handleDecodeJSONError(w, r, err)
			return
		}
		userID = req.UserID
	}

	if userID == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingUserID))
		return
	}

	if err := action(r.Context(), userID); err != nil {
		writeError(w, http.StatusNotFound, getMessage(r, apperrors.ErrCodeNotFound))
		return
	}

	writeSuccess(w, http.StatusOK, successMessage, nil)
}

// HandleDisableUser 处理禁用用户请求
// POST /admin/users/{id}/disable 或 POST /admin/users/disable
// 注意：管理员权限检查由 AdminMiddleware 处理
func (h *AdminHandler) HandleDisableUser(w http.ResponseWriter, r *http.Request) {
	h.handleUserStatusChange(w, r, h.adminSvc.DisableUser, "用户已禁用")
}

// HandleEnableUser 处理启用用户请求
// POST /admin/users/{id}/enable 或 POST /admin/users/enable
// 注意：管理员权限检查由 AdminMiddleware 处理
func (h *AdminHandler) HandleEnableUser(w http.ResponseWriter, r *http.Request) {
	h.handleUserStatusChange(w, r, h.adminSvc.EnableUser, "用户已启用")
}

// HandleDeleteUser 处理删除用户请求
// DELETE /admin/users/{id}
// 注意：管理员权限检查由 AdminMiddleware 处理
func (h *AdminHandler) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["id"]

	if userID == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingUserID))
		return
	}

	if err := h.adminSvc.DeleteUser(r.Context(), userID); err != nil {
		writeError(w, http.StatusNotFound, getMessage(r, apperrors.ErrCodeNotFound))
		return
	}

	writeSuccess(w, http.StatusOK, "用户已删除", nil)
}

// HandleAuditLogs 处理审计日志查询请求
// GET /admin/audit-logs?page=1&pageSize=20
// 注意：管理员权限检查由 AdminMiddleware 处理
func (h *AdminHandler) HandleAuditLogs(w http.ResponseWriter, r *http.Request) {
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

	// 获取事件类型过滤
	eventType := r.URL.Query().Get("event_type")

	// 通过Service获取审计日志
	logs, total, err := h.adminSvc.GetAuditLogs(r.Context(), offset, pageSize, eventType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, getMessage(r, apperrors.ErrCodeInternal))
		return
	}

	// 构建响应
	logList := make([]map[string]interface{}, 0, len(logs))
	for _, log := range logs {
		logList = append(logList, map[string]interface{}{
			"id":         log.ID,
			"event_type": log.EventType,
			"user_id":    log.UserID,
			"ip_address": log.IPAddress,
			"details":    log.Details,
			"success":    log.Success,
			"timestamp":  log.Timestamp,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs":        logList,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + pageSize - 1) / pageSize,
	})
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
		"status":     health.Status,
		"timestamp":  health.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		"database":   health.Database,
		"version":    health.Version,
		"build_time": health.BuildTime,
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
