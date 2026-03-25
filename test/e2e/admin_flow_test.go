//go:build e2e

// Package e2e 管理员操作端到端测试
package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 管理员配置
// ============================================================================

var (
	adminEmail    = getEnvOrDefault("E2E_ADMIN_EMAIL", "admin@example.com")
	adminPassword = getEnvOrDefault("E2E_ADMIN_PASSWORD", "AdminPassword123!")
)

// ============================================================================
// 管理员登录辅助函数
// ============================================================================

func loginAdmin() (*loginResponse, error) {
	// 尝试使用管理员账户登录
	tokens, err := loginUser(adminEmail, adminPassword)
	if err != nil {
		// 如果管理员账户不存在，跳过管理员测试
		return nil, fmt.Errorf("管理员登录失败: %w", err)
	}
	return tokens, nil
}

// ============================================================================
// 用户列表测试
// ============================================================================

func TestAdminListUsers(t *testing.T) {
	// 尝试管理员登录
	adminTokens, err := loginAdmin()
	if err != nil {
		t.Skip("管理员账户未配置，跳过管理员测试")
		return
	}

	t.Run("获取用户列表", func(t *testing.T) {
		resp, body, err := doRequest("GET", "/api/v1/admin/users", nil, adminTokens.AccessToken)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			t.Skip("管理员用户列表端点未实现")
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var userList userListResponse
		err = json.Unmarshal(body, &userList)
		if err == nil {
			t.Logf("用户总数: %d", userList.Total)
		}
	})

	t.Run("分页查询", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/users?page=1&limit=10", nil, adminTokens.AccessToken)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			t.Skip("管理员用户列表端点未实现")
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// ============================================================================
// 获取用户详情测试
// ============================================================================

func TestAdminGetUser(t *testing.T) {
	adminTokens, err := loginAdmin()
	if err != nil {
		t.Skip("管理员账户未配置，跳过管理员测试")
		return
	}

	// 先创建一个测试用户
	testEmail := generateUniqueEmail("admintest")
	testPassword := generateTestPassword()
	testUser, err := registerUser(testEmail, testPassword)
	require.NoError(t, err)

	t.Run("获取用户详情", func(t *testing.T) {
		userID := testUser["id"].(string)
		resp, body, err := doRequest("GET", "/api/v1/admin/users/"+userID, nil, adminTokens.AccessToken)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			t.Skip("管理员用户详情端点未实现")
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var user map[string]interface{}
		err = json.Unmarshal(body, &user)
		if err == nil {
			assert.Equal(t, testEmail, user["email"])
			t.Logf("获取用户详情成功: %s", user["email"])
		}
	})

	t.Run("获取不存在的用户", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/users/nonexistent-id", nil, adminTokens.AccessToken)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			// 可能是端点不存在或用户不存在
			t.Logf("返回404，可能是端点或用户不存在")
			return
		}

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// ============================================================================
// 禁用/启用用户测试
// ============================================================================

func TestAdminDisableEnableUser(t *testing.T) {
	adminTokens, err := loginAdmin()
	if err != nil {
		t.Skip("管理员账户未配置，跳过管理员测试")
		return
	}

	// 创建测试用户
	testEmail := generateUniqueEmail("disabletest")
	testPassword := generateTestPassword()
	testUser, err := registerUser(testEmail, testPassword)
	require.NoError(t, err)
	userID := testUser["id"].(string)

	t.Run("禁用用户", func(t *testing.T) {
		req := adminUserActionRequest{UserID: userID}
		resp, _, err := doRequest("POST", "/api/v1/admin/users/"+userID+"/disable", req, adminTokens.AccessToken)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			t.Skip("禁用用户端点未实现")
			return
		}

		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent)
	})

	t.Run("禁用后用户无法登录", func(t *testing.T) {
		// 尝试用被禁用的账户登录
		loginReq := loginRequest{Email: testEmail, Password: testPassword}
		resp, _, err := doRequest("POST", "/api/v1/login", loginReq, "")
		require.NoError(t, err)

		// 应该返回禁止或未授权
		assert.True(t, resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized)
	})

	t.Run("启用用户", func(t *testing.T) {
		req := adminUserActionRequest{UserID: userID}
		resp, _, err := doRequest("POST", "/api/v1/admin/users/"+userID+"/enable", req, adminTokens.AccessToken)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			t.Skip("启用用户端点未实现")
			return
		}

		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent)
	})

	t.Run("启用后用户可以登录", func(t *testing.T) {
		tokens, err := loginUser(testEmail, testPassword)
		require.NoError(t, err)
		assert.NotEmpty(t, tokens.AccessToken)
	})
}

// ============================================================================
// 非管理员访问测试
// ============================================================================

func TestAdminUnauthorized(t *testing.T) {
	// 创建普通用户
	testEmail := generateUniqueEmail("nonadmin")
	testPassword := generateTestPassword()
	_, err := registerUser(testEmail, testPassword)
	require.NoError(t, err)

	// 普通用户登录
	userTokens, err := loginUser(testEmail, testPassword)
	require.NoError(t, err)

	t.Run("普通用户无法访问管理员接口", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/users", nil, userTokens.AccessToken)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			t.Skip("管理员端点未实现")
			return
		}

		// 应该返回禁止
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("未认证用户无法访问管理员接口", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/users", nil, "")
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			t.Skip("管理员端点未实现")
			return
		}

		// 应该返回未授权
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// ============================================================================
// 管理员删除用户测试
// ============================================================================

func TestAdminDeleteUser(t *testing.T) {
	adminTokens, err := loginAdmin()
	if err != nil {
		t.Skip("管理员账户未配置，跳过管理员测试")
		return
	}

	// 创建测试用户
	testEmail := generateUniqueEmail("deletetest")
	testPassword := generateTestPassword()
	testUser, err := registerUser(testEmail, testPassword)
	require.NoError(t, err)
	userID := testUser["id"].(string)

	t.Run("删除用户", func(t *testing.T) {
		resp, _, err := doRequest("DELETE", "/api/v1/admin/users/"+userID, nil, adminTokens.AccessToken)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			t.Skip("删除用户端点未实现")
			return
		}

		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent)
	})

	t.Run("删除后用户无法登录", func(t *testing.T) {
		loginReq := loginRequest{Email: testEmail, Password: testPassword}
		resp, _, err := doRequest("POST", "/api/v1/login", loginReq, "")
		require.NoError(t, err)

		// 应该返回未授权
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// ============================================================================
// 管理员审计日志测试
// ============================================================================

func TestAdminAuditLogs(t *testing.T) {
	adminTokens, err := loginAdmin()
	if err != nil {
		t.Skip("管理员账户未配置，跳过管理员测试")
		return
	}

	t.Run("获取审计日志", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/audit-logs", nil, adminTokens.AccessToken)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			t.Skip("审计日志端点未实现")
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("按事件类型过滤", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/audit-logs?event_type=login", nil, adminTokens.AccessToken)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusNotFound {
			t.Skip("审计日志端点未实现")
			return
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
