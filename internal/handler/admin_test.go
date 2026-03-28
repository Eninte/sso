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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/handler"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
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
