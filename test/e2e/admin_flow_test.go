//go:build e2e

// Package e2e 管理员操作端到端测试
package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 管理员登录辅助函数
// ============================================================================

func loginAdmin() (*loginResponse, error) {
	// 先尝试直接登录
	tokens, err := loginUser(adminEmail, adminPassword)
	if err == nil {
		return tokens, nil
	}

	// 登录失败，管理员可能不存在，自动创建
	fmt.Printf("[INFO] 管理员登录失败，尝试自动创建管理员账户...\n")

	// 注册管理员账户
	user, regErr := registerUser(adminEmail, adminPassword)
	if regErr != nil {
		return nil, fmt.Errorf("管理员登录失败且自动创建也失败: 登录错误=%v, 注册错误=%v", err, regErr)
	}

	userID, _ := user["user_id"].(string)
	if userID == "" {
		return nil, fmt.Errorf("注册响应中无 user_id")
	}

	// 验证邮箱
	if verifyErr := verifyEmail(userID); verifyErr != nil {
		return nil, fmt.Errorf("验证管理员邮箱失败: %w", verifyErr)
	}

	// 设置角色为 admin
	if roleErr := setUserRole(userID, "admin"); roleErr != nil {
		return nil, fmt.Errorf("设置管理员角色失败: %w", roleErr)
	}

	fmt.Printf("[OK] 管理员账户已自动创建: %s\n", adminEmail)

	// 重新登录
	tokens, err = loginUser(adminEmail, adminPassword)
	if err != nil {
		return nil, fmt.Errorf("管理员创建后登录失败: %w", err)
	}
	return tokens, nil
}

// setUserRole 通过测试API设置用户角色
func setUserRole(userID, role string) error {
	req := map[string]string{"user_id": userID, "role": role}
	resp, _, err := doRequest("POST", "/api/v1/test/set-role", req, "")
	if err != nil {
		return fmt.Errorf("设置角色请求失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("设置角色失败: %d", resp.StatusCode)
	}
	return nil
}

// ============================================================================
// 用户列表测试
// ============================================================================

func TestAdminListUsers(t *testing.T) {
	adminTokens, err := loginAdmin()
	require.NoError(t, err, "管理员登录失败，请检查 E2E_ADMIN_EMAIL 和 E2E_ADMIN_PASSWORD")

	t.Run("获取用户列表", func(t *testing.T) {
		resp, body, err := doRequest("GET", "/api/v1/admin/users", nil, adminTokens.AccessToken)
		require.NoError(t, err)
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
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// ============================================================================
// 获取用户详情测试
// ============================================================================

func TestAdminGetUser(t *testing.T) {
	adminTokens, err := loginAdmin()
	require.NoError(t, err, "管理员登录失败")

	// 先创建一个测试用户
	testEmail := testAwareEmail(t, "admintest")
	testPassword := generateTestPassword()
	testUser, err := registerUser(testEmail, testPassword)
	require.NoError(t, err)

	// 验证邮箱以便后续登录测试
	userID := testUser["user_id"].(string)
	err = verifyEmail(userID)
	require.NoError(t, err)

	t.Run("获取用户详情", func(t *testing.T) {
		resp, body, err := doRequest("GET", "/api/v1/admin/users/"+userID, nil, adminTokens.AccessToken)
		require.NoError(t, err)
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
	require.NoError(t, err, "管理员登录失败")

	// 创建测试用户并验证邮箱
	testEmail := testAwareEmail(t, "disabletest")
	testPassword := generateTestPassword()
	testUser, err := registerUser(testEmail, testPassword)
	require.NoError(t, err)
	userID := testUser["user_id"].(string)

	// 验证邮箱以便后续登录测试
	err = verifyEmail(userID)
	require.NoError(t, err)

	t.Run("禁用用户", func(t *testing.T) {
		resp, _, err := doRequest("POST", "/api/v1/admin/users/"+userID+"/disable", nil, adminTokens.AccessToken)
		require.NoError(t, err)
		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent)
	})

	t.Run("禁用后用户无法登录", func(t *testing.T) {
		loginReq := loginRequest{Email: testEmail, Password: testPassword}
		resp, _, err := doRequest("POST", "/api/v1/login", loginReq, "")
		require.NoError(t, err)

		// 应该返回禁止或未授权
		assert.True(t, resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized)
	})

	t.Run("启用用户", func(t *testing.T) {
		resp, _, err := doRequest("POST", "/api/v1/admin/users/"+userID+"/enable", nil, adminTokens.AccessToken)
		require.NoError(t, err)
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
	// 创建普通用户并验证邮箱
	testEmail := testAwareEmail(t, "nonadmin")
	testPassword := generateTestPassword()
	user, err := registerUser(testEmail, testPassword)
	require.NoError(t, err)

	// 验证邮箱
	userID := user["user_id"].(string)
	err = verifyEmail(userID)
	require.NoError(t, err)

	// 普通用户登录
	userTokens, err := loginUser(testEmail, testPassword)
	require.NoError(t, err)

	t.Run("普通用户无法访问管理员接口", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/users", nil, userTokens.AccessToken)
		require.NoError(t, err)

		// 应该返回禁止
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("未认证用户无法访问管理员接口", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/users", nil, "")
		require.NoError(t, err)

		// 应该返回未授权
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// ============================================================================
// 管理员删除用户测试
// ============================================================================

func TestAdminDeleteUser(t *testing.T) {
	adminTokens, err := loginAdmin()
	require.NoError(t, err, "管理员登录失败")

	// 创建测试用户
	testEmail := testAwareEmail(t, "deletetest")
	testPassword := generateTestPassword()
	testUser, err := registerUser(testEmail, testPassword)
	require.NoError(t, err)
	userID := testUser["user_id"].(string)

	// 注册 UUID 以便清理：删除用户后 UUID 无法从 users 表恢复，
	// 且 EventUserDeleted 的 details 不含 email/testID。
	registerExtraCleanupIDs(userID)

	t.Run("删除用户", func(t *testing.T) {
		resp, _, err := doRequest("DELETE", "/api/v1/admin/users/"+userID, nil, adminTokens.AccessToken)
		require.NoError(t, err)
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
	require.NoError(t, err, "管理员登录失败")

	t.Run("获取审计日志", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/audit-logs", nil, adminTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("按事件类型过滤", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/audit-logs?event_type=login", nil, adminTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
