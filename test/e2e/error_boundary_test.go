//go:build e2e

// Package e2e 错误边界测试
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 超大请求体测试
// ============================================================================

func TestLargeRequestBody(t *testing.T) {
	t.Run("超大JSON请求体", func(t *testing.T) {
		// 创建一个超过1MB的请求体
		largePassword := strings.Repeat("a", 2*1024*1024) // 2MB
		req := map[string]string{
			"email":    "test@example.com",
			"password": largePassword,
		}

		bodyBytes, _ := json.Marshal(req)
		httpReq, err := http.NewRequest("POST", baseURL+"/api/v1/register", bytes.NewReader(bodyBytes))
		require.NoError(t, err)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 应该返回413 Request Entity Too Large或400
		assert.True(t, resp.StatusCode == http.StatusRequestEntityTooLarge || resp.StatusCode == http.StatusBadRequest)
	})

	t.Run("超大URL参数", func(t *testing.T) {
		// 创建一个超长的URL参数
		longParam := strings.Repeat("a", 10000)
		url := fmt.Sprintf("/api/v1/verify-email?token=%s", longParam)

		resp, _, err := doRequest("GET", url, nil, "")
		require.NoError(t, err)

		// 应该返回错误
		assert.True(t, resp.StatusCode >= 400)
	})
}

// ============================================================================
// 无效Content-Type测试
// ============================================================================

func TestInvalidContentType(t *testing.T) {
	t.Run("XML Content-Type", func(t *testing.T) {
		req, err := http.NewRequest("POST", baseURL+"/api/v1/login", strings.NewReader("<xml></xml>"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/xml")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 应该返回415或400
		assert.True(t, resp.StatusCode == http.StatusUnsupportedMediaType || resp.StatusCode == http.StatusBadRequest)
	})

	t.Run("Text Content-Type", func(t *testing.T) {
		req, err := http.NewRequest("POST", baseURL+"/api/v1/login", strings.NewReader("plain text"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "text/plain")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 应该返回415或400
		assert.True(t, resp.StatusCode == http.StatusUnsupportedMediaType || resp.StatusCode == http.StatusBadRequest)
	})

	t.Run("缺少Content-Type", func(t *testing.T) {
		req, err := http.NewRequest("POST", baseURL+"/api/v1/login", strings.NewReader("{}"))
		require.NoError(t, err)
		// 不设置Content-Type

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 应该返回400或415
		assert.True(t, resp.StatusCode >= 400)
	})
}

// ============================================================================
// 缺少必需字段测试
// ============================================================================

func TestMissingRequiredFields(t *testing.T) {
	t.Run("注册缺少邮箱", func(t *testing.T) {
		req := map[string]string{
			"password": "TestPassword123!",
		}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("注册缺少密码", func(t *testing.T) {
		req := map[string]string{
			"email": "test@example.com",
		}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("登录缺少邮箱", func(t *testing.T) {
		req := map[string]string{
			"password": "TestPassword123!",
		}
		resp, _, err := doRequest("POST", "/api/v1/login", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("登录缺少密码", func(t *testing.T) {
		req := map[string]string{
			"email": "test@example.com",
		}
		resp, _, err := doRequest("POST", "/api/v1/login", req, "")
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("空请求体", func(t *testing.T) {
		resp, _, err := doRequest("POST", "/api/v1/login", nil, "")
		require.NoError(t, err)
		assert.True(t, resp.StatusCode >= 400)
	})
}

// ============================================================================
// SQL注入尝试测试
// ============================================================================

func TestSQLInjectionAttempt(t *testing.T) {
	sqlPayloads := []string{
		"' OR '1'='1",
		"'; DROP TABLE users; --",
		"admin'--",
		"1' UNION SELECT * FROM users --",
		"' OR 1=1 --",
	}

	for _, payload := range sqlPayloads {
		// 生成安全的测试名称，避免切片越界
		testName := payload
		if len(testName) > 10 {
			testName = testName[:10]
		}
		t.Run(fmt.Sprintf("SQL注入_%s", testName), func(t *testing.T) {
			req := registerRequest{
				Email:    fmt.Sprintf("test%s@example.com", payload),
				Password: "TestPassword123!",
			}
			resp, _, err := doRequest("POST", "/api/v1/register", req, "")
			require.NoError(t, err)

			// 应该返回400（验证失败）而不是500（服务器错误）
			assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode)
			assert.True(t, resp.StatusCode >= 400)
		})
	}

	t.Run("SQL注入_邮箱字段", func(t *testing.T) {
		req := loginRequest{
			Email:    "' OR '1'='1' --",
			Password: "password",
		}
		resp, _, err := doRequest("POST", "/api/v1/login", req, "")
		require.NoError(t, err)

		// 应该返回400或401，而不是500
		assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// ============================================================================
// XSS尝试测试
// ============================================================================

func TestXSSAttempt(t *testing.T) {
	xssPayloads := []string{
		"<script>alert('xss')</script>",
		"javascript:alert('xss')",
		"<img src=x onerror=alert('xss')>",
		"'><script>alert('xss')</script>",
	}

	for _, payload := range xssPayloads {
		t.Run(fmt.Sprintf("XSS_%s", payload[:10]), func(t *testing.T) {
			req := registerRequest{
				Email:    fmt.Sprintf("test@example.com"),
				Password: payload,
			}
			resp, body, err := doRequest("POST", "/api/v1/register", req, "")
			require.NoError(t, err)

			// 验证响应中不包含原始payload
			bodyStr := string(body)
			assert.NotContains(t, bodyStr, "<script>")
			assert.NotContains(t, bodyStr, "javascript:")

			// 应该返回400（验证失败）
			assert.True(t, resp.StatusCode >= 400)
		})
	}
}

// ============================================================================
// 路径遍历尝试测试
// ============================================================================

func TestPathTraversalAttempt(t *testing.T) {
	paths := []string{
		"/api/v1/../etc/passwd",
		"/api/v1/../../etc/passwd",
		"/api/v1/users/../../../etc/passwd",
		"/api/v1/verify-email?token=../../../etc/passwd",
	}

	for _, path := range paths {
		t.Run(fmt.Sprintf("路径遍历_%s", path[:20]), func(t *testing.T) {
			resp, _, err := doRequest("GET", path, nil, "")
			require.NoError(t, err)

			// 应该返回400或404，而不是200
			assert.NotEqual(t, http.StatusOK, resp.StatusCode)
		})
	}
}

// ============================================================================
// HTTP方法测试
// ============================================================================

func TestInvalidHTTPMethod(t *testing.T) {
	methods := []string{"PATCH", "PUT", "DELETE", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(fmt.Sprintf("无效方法_%s", method), func(t *testing.T) {
			resp, _, err := doRequest(method, "/api/v1/login", nil, "")
			require.NoError(t, err)

			// 应该返回405 Method Not Allowed
			// 注意：某些方法可能返回其他错误码
			assert.True(t, resp.StatusCode >= 400)
		})
	}
}

// ============================================================================
// 特殊字符测试
// ============================================================================

func TestSpecialCharacters(t *testing.T) {
	t.Run("Unicode字符", func(t *testing.T) {
		req := registerRequest{
			Email:    "test@例え.com",
			Password: "TestPassword123!",
		}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)

		// 可能返回400（无效邮箱）或成功
		t.Logf("Unicode邮箱响应状态: %d", resp.StatusCode)
	})

	t.Run("空字节", func(t *testing.T) {
		req := registerRequest{
			Email:    "test\x00@example.com",
			Password: "TestPassword123!",
		}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)

		// 应该返回400
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("换行符", func(t *testing.T) {
		req := registerRequest{
			Email:    "test\n@example.com",
			Password: "TestPassword123!",
		}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)

		// 应该返回400
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// ============================================================================
// 边界值测试
// ============================================================================

func TestBoundaryValues(t *testing.T) {
	t.Run("邮箱最大长度", func(t *testing.T) {
		// RFC 5321规定邮箱地址最大254字符
		longEmail := strings.Repeat("a", 250) + "@b.com"
		req := registerRequest{
			Email:    longEmail,
			Password: "TestPassword123!",
		}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)

		// 可能返回400（过长）或成功
		t.Logf("长邮箱响应状态: %d", resp.StatusCode)
	})

	t.Run("密码最小长度", func(t *testing.T) {
		req := registerRequest{
			Email:    generateUniqueEmail("minpwd"),
			Password: "12345678", // 假设最小8位
		}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)

		// 取决于密码策略
		t.Logf("最小密码响应状态: %d", resp.StatusCode)
	})

	t.Run("密码边界长度", func(t *testing.T) {
		req := registerRequest{
			Email:    generateUniqueEmail("boundary"),
			Password: "1234567", // 7位，应该不够
		}
		resp, _, err := doRequest("POST", "/api/v1/register", req, "")
		require.NoError(t, err)

		// 应该返回400
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
