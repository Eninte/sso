// Package handler_test Admin Handler单元测试
// 注意：管理员权限检查由 AdminMiddleware 处理，不在handler中测试
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestAdminHandler 创建测试用的管理员处理器
func createTestAdminHandler() (*handler.AdminHandler, *mock.Store) {
	store := mock.New()
	adminSvc := service.NewAdminService(store)
	adminHandler := handler.NewAdminHandler(adminSvc)
	return adminHandler, store
}

// addAdminContext 添加管理员上下文
func addAdminContext(r *http.Request, email string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, middleware.UserIDKey, "admin-user-id")
	ctx = context.WithValue(ctx, middleware.UserEmailKey, email)
	return r.WithContext(ctx)
}

// ============================================================================
// HandleListUsers 测试
// ============================================================================

func TestAdminHandler_HandleListUsers(t *testing.T) {
	adminHandler, store := createTestAdminHandler()

	t.Run("成功获取用户列表", func(t *testing.T) {
		store.Reset()

		// 添加测试用户
		for i := 0; i < 5; i++ {
			user := &model.User{
				ID:            fmt.Sprintf("user-%d", i),
				Email:         fmt.Sprintf("user%d@example.com", i),
				Status:        model.UserStatusActive,
				EmailVerified: true,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			store.AddUser(user)
		}

		req := httptest.NewRequest("GET", "/admin/users?page=1&pageSize=10", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleListUsers(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, float64(5), resp["total"])
		assert.Equal(t, float64(1), resp["page"])
	})

	t.Run("分页参数", func(t *testing.T) {
		store.Reset()

		// 添加25个用户
		for i := 0; i < 25; i++ {
			user := &model.User{
				ID:        fmt.Sprintf("user-%d", i),
				Email:     fmt.Sprintf("user%d@example.com", i),
				Status:    model.UserStatusActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			store.AddUser(user)
		}

		// 请求第2页，每页10条
		req := httptest.NewRequest("GET", "/admin/users?page=2&pageSize=10", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleListUsers(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, float64(25), resp["total"])
		assert.Equal(t, float64(2), resp["page"])
		assert.Equal(t, float64(10), resp["page_size"])
		assert.Equal(t, float64(3), resp["total_pages"])
	})
}

// ============================================================================
// HandleGetUser 测试
// ============================================================================

func TestAdminHandler_HandleGetUser(t *testing.T) {
	adminHandler, store := createTestAdminHandler()

	// 添加测试用户
	user := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		Status:        model.UserStatusActive,
		EmailVerified: true,
		MFAEnabled:    false,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(user)

	t.Run("成功获取用户", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users?id=test-user-id", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleGetUser(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "test-user-id", resp["id"])
		assert.Equal(t, "test@example.com", resp["email"])
	})

	t.Run("用户不存在", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users?id=nonexistent", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleGetUser(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("缺少用户ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleGetUser(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ============================================================================
// HandleDisableUser 测试
// ============================================================================

func TestAdminHandler_HandleDisableUser(t *testing.T) {
	adminHandler, store := createTestAdminHandler()

	user := &model.User{
		ID:     "test-user-id",
		Email:  "test@example.com",
		Status: model.UserStatusActive,
	}
	store.AddUser(user)

	t.Run("成功禁用用户", func(t *testing.T) {
		body := map[string]string{"user_id": "test-user-id"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/admin/users/disable", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleDisableUser(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 验证用户已被禁用
		updatedUser, _ := store.GetByID(context.Background(), "test-user-id")
		assert.Equal(t, "disabled", updatedUser.Status)
	})

	t.Run("用户不存在", func(t *testing.T) {
		body := map[string]string{"user_id": "nonexistent"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/admin/users/disable", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleDisableUser(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("无效的JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/users/disable", bytes.NewReader([]byte("invalid")))
		req.Header.Set("Content-Type", "application/json")
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleDisableUser(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ============================================================================
// HandleEnableUser 测试
// ============================================================================

func TestAdminHandler_HandleEnableUser(t *testing.T) {
	adminHandler, store := createTestAdminHandler()

	user := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		Status:        "disabled",
		LoginAttempts: 5,
	}
	store.AddUser(user)

	t.Run("成功启用用户", func(t *testing.T) {
		body := map[string]string{"user_id": "test-user-id"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/admin/users/enable", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleEnableUser(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 验证用户已启用
		updatedUser, _ := store.GetByID(context.Background(), "test-user-id")
		assert.Equal(t, "active", updatedUser.Status)
		assert.Equal(t, 0, updatedUser.LoginAttempts)
	})
}

// ============================================================================
// HandleSystemHealth 测试
// ============================================================================

func TestAdminHandler_HandleSystemHealth(t *testing.T) {
	adminHandler, _ := createTestAdminHandler()

	t.Run("成功获取健康状态", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/health", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleSystemHealth(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "ok", resp["status"])
		assert.Contains(t, resp, "timestamp")
		assert.Contains(t, resp, "database")
		assert.Contains(t, resp, "version")
		assert.Contains(t, resp, "build_time")
	})
}

// ============================================================================
// HandleCleanup 测试
// ============================================================================

func TestAdminHandler_HandleCleanup(t *testing.T) {
	adminHandler, _ := createTestAdminHandler()

	t.Run("成功清理", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/cleanup", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleCleanup(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "清理完成", resp["message"])
	})
}

// ============================================================================
// HandleListUsers 错误处理测试
// ============================================================================

func TestAdminHandler_HandleListUsers_InvalidParams(t *testing.T) {
	adminHandler, _ := createTestAdminHandler()

	t.Run("无效的page参数", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/users?page=abc&pageSize=10", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleListUsers(w, req)

		// 应该使用默认值处理
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ============================================================================
// HandleDisableUser / HandleEnableUser 补充测试
// ============================================================================

func TestAdminHandler_HandleEnableUser_NotFound(t *testing.T) {
	adminHandler, _ := createTestAdminHandler()

	t.Run("启用不存在的用户", func(t *testing.T) {
		body := map[string]string{"user_id": "nonexistent"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/admin/users/enable", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleEnableUser(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("无效的JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/users/enable", bytes.NewReader([]byte("invalid")))
		req.Header.Set("Content-Type", "application/json")
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleEnableUser(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ============================================================================
// HandleDeleteUser 测试
// ============================================================================

func TestAdminHandler_HandleDeleteUser(t *testing.T) {
	adminHandler, store := createTestAdminHandler()

	t.Run("删除用户成功", func(t *testing.T) {
		store.Reset()

		// 添加测试用户
		user := &model.User{
			ID:        "test-user-id",
			Email:     "test@example.com",
			Status:    model.UserStatusActive,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		store.AddUser(user)

		req := httptest.NewRequest("DELETE", "/admin/users/test-user-id", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		// 使用 mux 设置变量
		req = mux.SetURLVars(req, map[string]string{"id": "test-user-id"})

		adminHandler.HandleDeleteUser(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("删除用户 - 缺少ID", func(t *testing.T) {
		store.Reset()

		req := httptest.NewRequest("DELETE", "/admin/users/", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleDeleteUser(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("删除用户 - 用户不存在", func(t *testing.T) {
		store.Reset()

		req := httptest.NewRequest("DELETE", "/admin/users/nonexistent", nil)
		req = addAdminContext(req, "admin@example.com")
		req = mux.SetURLVars(req, map[string]string{"id": "nonexistent"})
		w := httptest.NewRecorder()

		adminHandler.HandleDeleteUser(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ============================================================================
// HandleAuditLogs 测试
// ============================================================================

func TestAdminHandler_HandleAuditLogs(t *testing.T) {
	adminHandler, store := createTestAdminHandler()

	t.Run("获取审计日志 - 默认分页", func(t *testing.T) {
		store.Reset()

		// 添加测试审计日志
		for i := 0; i < 5; i++ {
			log := &model.AuditLog{
				ID:        fmt.Sprintf("log-%d", i),
				UserID:    "test-user",
				EventType: "login",
				IPAddress: "127.0.0.1",
				Details:   "test details",
				Timestamp: time.Now(),
			}
			_ = store.StoreAuditLog(context.Background(), log)
		}

		req := httptest.NewRequest("GET", "/admin/audit-logs", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleAuditLogs(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("获取审计日志 - 自定义分页", func(t *testing.T) {
		store.Reset()

		req := httptest.NewRequest("GET", "/admin/audit-logs?page=2&pageSize=10", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleAuditLogs(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("获取审计日志 - 按事件类型过滤", func(t *testing.T) {
		store.Reset()

		req := httptest.NewRequest("GET", "/admin/audit-logs?event_type=login", nil)
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleAuditLogs(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ============================================================================
// T14：管理员操作防护 Handler 测试（状态码透传：403 本人操作 / 409 末位管理员）
// ============================================================================

func TestAdminHandler_HandleDisableUser_Protection(t *testing.T) {
	t.Run("禁止禁用本人账户返回403", func(t *testing.T) {
		adminHandler, store := createTestAdminHandler()

		// addAdminContext 固定操作者 ID 为 "admin-user-id"
		store.AddUser(&model.User{
			ID:     "admin-user-id",
			Email:  "admin@example.com",
			Role:   model.UserRoleUser,
			Status: model.UserStatusActive,
		})

		body := map[string]string{"user_id": "admin-user-id"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/admin/users/disable", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleDisableUser(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "SELF_OPERATION_FORBIDDEN", resp["code"])

		// 用户状态未被修改
		u, _ := store.GetByID(context.Background(), "admin-user-id")
		assert.Equal(t, model.UserStatusActive, u.Status)
	})

	t.Run("禁止禁用最后一个活跃管理员返回409", func(t *testing.T) {
		adminHandler, store := createTestAdminHandler()

		store.AddUser(&model.User{
			ID:     "only-admin",
			Email:  "only-admin@example.com",
			Role:   model.UserRoleAdmin,
			Status: model.UserStatusActive,
		})

		body := map[string]string{"user_id": "only-admin"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/admin/users/disable", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()

		adminHandler.HandleDisableUser(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "LAST_ACTIVE_ADMIN", resp["code"])
	})
}

func TestAdminHandler_HandleDeleteUser_Protection(t *testing.T) {
	t.Run("禁止删除本人账户返回403", func(t *testing.T) {
		adminHandler, store := createTestAdminHandler()

		store.AddUser(&model.User{
			ID:     "admin-user-id",
			Email:  "admin@example.com",
			Role:   model.UserRoleUser,
			Status: model.UserStatusActive,
		})

		req := httptest.NewRequest("DELETE", "/admin/users/admin-user-id", nil)
		req = addAdminContext(req, "admin@example.com")
		req = mux.SetURLVars(req, map[string]string{"id": "admin-user-id"})
		w := httptest.NewRecorder()

		adminHandler.HandleDeleteUser(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		// 用户未被删除
		_, err := store.GetByID(context.Background(), "admin-user-id")
		assert.NoError(t, err)
	})

	t.Run("禁止删除最后一个活跃管理员返回409", func(t *testing.T) {
		adminHandler, store := createTestAdminHandler()

		store.AddUser(&model.User{
			ID:     "only-admin",
			Email:  "only-admin@example.com",
			Role:   model.UserRoleAdmin,
			Status: model.UserStatusActive,
		})

		req := httptest.NewRequest("DELETE", "/admin/users/only-admin", nil)
		req = addAdminContext(req, "admin@example.com")
		req = mux.SetURLVars(req, map[string]string{"id": "only-admin"})
		w := httptest.NewRecorder()

		adminHandler.HandleDeleteUser(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)

		_, err := store.GetByID(context.Background(), "only-admin")
		assert.NoError(t, err)
	})
}
