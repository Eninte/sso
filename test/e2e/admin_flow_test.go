//go:build e2e

// Package e2e 管理员操作端到端测试
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 管理员登录辅助函数
// ============================================================================

// loginAdmin 登录管理员账户。如果管理员不存在，自动创建并提升角色。
func loginAdmin() (*loginResponse, error) {
	// 尝试登录
	tokens, err := loginUser(adminEmail, adminPassword)
	if err == nil {
		return tokens, nil
	}

	// 登录失败，尝试自动创建管理员
	fmt.Printf("[INFO] 管理员账户不存在，自动创建中...\n")

	// 注册
	regReq := registerRequest{Email: adminEmail, Password: adminPassword}
	regBody, _ := json.Marshal(regReq)
	regResp, postErr := client.Post(baseURL+"/api/v1/register", "application/json", bytes.NewReader(regBody))
	if postErr != nil {
		return nil, fmt.Errorf("注册管理员失败: %w", postErr)
	}
	defer regResp.Body.Close()

	if regResp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("注册管理员返回 %d（期望 201）", regResp.StatusCode)
	}

	// 解析 user_id
	var regResult map[string]interface{}
	regRespBody, _ := io.ReadAll(regResp.Body)
	if err := json.Unmarshal(regRespBody, &regResult); err != nil {
		return nil, fmt.Errorf("解析注册响应失败: %w", err)
	}
	data, _ := regResult["data"].(map[string]interface{})
	userID, _ := data["user_id"].(string)
	if userID == "" {
		return nil, fmt.Errorf("注册响应中无 user_id")
	}

	// 验证邮箱
	if err := verifyEmail(userID); err != nil {
		return nil, fmt.Errorf("验证管理员邮箱失败: %w", err)
	}

	// 直接通过数据库提升角色为 admin
	if err := setUserRoleDB(userID, "admin"); err != nil {
		return nil, fmt.Errorf("设置管理员角色失败: %w", err)
	}

	fmt.Printf("[OK] 管理员账户已自动创建: %s\n", adminEmail)

	// 重新登录
	tokens, err = loginUser(adminEmail, adminPassword)
	if err != nil {
		return nil, fmt.Errorf("管理员创建后登录失败: %w", err)
	}
	return tokens, nil
}

// ============================================================================
// 用户列表测试
// ============================================================================

func TestAdminListUsers(t *testing.T) {
	adminTokens, err := loginAdmin()
	require.NoError(t, err, "管理员登录失败")

	t.Run("获取用户列表", func(t *testing.T) {
		resp, body, err := doRequest("GET", "/api/v1/admin/users", nil, adminTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result userListResponse
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, result.Total, 0)
	})

	t.Run("分页参数", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/users?page=1&limit=5", nil, adminTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// ============================================================================
// 用户详情测试
// ============================================================================

func TestAdminGetUser(t *testing.T) {
	adminTokens, err := loginAdmin()
	require.NoError(t, err, "管理员登录失败")

	// 先创建一个测试用户
	testEmail := generateUniqueEmail("admintest")
	testPassword := generateTestPassword()
	user, err := registerUser(testEmail, testPassword)
	require.NoError(t, err)
	userID := user["user_id"].(string)

	t.Run("获取用户详情", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/users/"+userID, nil, adminTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("获取不存在的用户", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/users/nonexistent-id", nil, adminTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// ============================================================================
// 用户禁用/启用测试
// ============================================================================

func TestAdminDisableEnableUser(t *testing.T) {
	adminTokens, err := loginAdmin()
	require.NoError(t, err, "管理员登录失败")

	// 创建测试用户
	testEmail := generateUniqueEmail("disabletest")
	testPassword := generateTestPassword()
	user, err := registerUser(testEmail, testPassword)
	require.NoError(t, err)
	userID := user["user_id"].(string)

	t.Run("禁用用户", func(t *testing.T) {
		resp, _, err := doRequest("POST", "/api/v1/admin/users/"+userID+"/disable", nil, adminTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("启用用户", func(t *testing.T) {
		resp, _, err := doRequest("POST", "/api/v1/admin/users/"+userID+"/enable", nil, adminTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// ============================================================================
// 非管理员访问测试
// ============================================================================

func TestAdminUnauthorized(t *testing.T) {
	// 创建普通用户并验证邮箱
	testEmail := generateUniqueEmail("nonadmin")
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

		// 应该返回禁止（403），不是未认证（401）
		assert.Equal(t, http.StatusForbidden, resp.StatusCode,
			"普通用户访问管理员接口应返回 403 Forbidden，实际 %d", resp.StatusCode)
	})

	t.Run("未认证用户无法访问管理员接口", func(t *testing.T) {
		resp, _, err := doRequest("GET", "/api/v1/admin/users", nil, "")
		require.NoError(t, err)

		// 未提供Token应返回未认证
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// ============================================================================
// 删除用户测试
// ============================================================================

func TestAdminDeleteUser(t *testing.T) {
	adminTokens, err := loginAdmin()
	require.NoError(t, err, "管理员登录失败")

	// 创建测试用户
	testEmail := generateUniqueEmail("deletetest")
	testPassword := generateTestPassword()
	user, err := registerUser(testEmail, testPassword)
	require.NoError(t, err)
	userID := user["user_id"].(string)

	t.Run("删除用户", func(t *testing.T) {
		resp, _, err := doRequest("DELETE", "/api/v1/admin/users/"+userID, nil, adminTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("删除不存在的用户", func(t *testing.T) {
		resp, _, err := doRequest("DELETE", "/api/v1/admin/users/nonexistent-id", nil, adminTokens.AccessToken)
		require.NoError(t, err)
		// 可能返回 404 或 200（幂等删除）
		assert.True(t, resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusOK,
			"期望 404 或 200，实际 %d", resp.StatusCode)
	})
}

// ============================================================================
// 审计日志测试
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
		resp, _, err := doRequest("GET", "/api/v1/admin/audit-logs?event_type=auth.login", nil, adminTokens.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
