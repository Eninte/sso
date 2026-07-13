//go:build e2e

// Package e2e 并发测试
package e2e

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 并发注册测试
// ============================================================================

func TestConcurrentRegister(t *testing.T) {
	const concurrency = 10
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)
	success := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			email := fmt.Sprintf("test-concurrent-%d-%d@example.com", index, time.Now().UnixNano())
			password := "TestPassword123!"

			req := registerRequest{Email: email, Password: password}
			resp, _, err := doRequest("POST", "/api/v1/register", req, "")
			if err != nil {
				errors <- err
				return
			}

			if resp.StatusCode == http.StatusCreated {
				success <- true
			} else {
				t.Logf("注册失败，状态码: %d", resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	close(success)

	// 检查错误
	for err := range errors {
		t.Errorf("并发注册错误: %v", err)
	}

	// 统计成功数
	successCount := len(success)
	t.Logf("并发注册成功数: %d/%d", successCount, concurrency)
	assert.Greater(t, successCount, 0)
}

// ============================================================================
// 并发登录测试
// ============================================================================

func TestConcurrentLogin(t *testing.T) {
	email := testAwareEmail(t, "concurrentlogin")
	password := generateTestPassword()

	// 先注册用户并验证邮箱
	user, err := registerUser(email, password)
	require.NoError(t, err)

	userID := user["user_id"].(string)
	err = verifyEmail(userID)
	require.NoError(t, err)

	const concurrency = 10
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)
	success := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := loginRequest{Email: email, Password: password}
			resp, _, err := doRequest("POST", "/api/v1/login", req, "")
			if err != nil {
				errors <- err
				return
			}

			if resp.StatusCode == http.StatusOK {
				success <- true
			}
		}()
	}

	wg.Wait()
	close(errors)
	close(success)

	// 检查错误
	for err := range errors {
		t.Errorf("并发登录错误: %v", err)
	}

	// 统计成功数
	successCount := len(success)
	t.Logf("并发登录成功数: %d/%d", successCount, concurrency)
	assert.Greater(t, successCount, 0)
}

// ============================================================================
// 并发Token刷新测试
// ============================================================================

func TestConcurrentTokenRefreshFull(t *testing.T) {
	email := testAwareEmail(t, "concrefreshfull")
	password := generateTestPassword()

	// 注册并验证邮箱
	user, err := registerUser(email, password)
	require.NoError(t, err)

	userID := user["user_id"].(string)
	err = verifyEmail(userID)
	require.NoError(t, err)

	// 登录多次获取多个RefreshToken
	tokens := make([]*loginResponse, 5)
	for i := 0; i < 5; i++ {
		tokens[i], err = loginUser(email, password)
		require.NoError(t, err)
	}

	const concurrency = 5
	var wg sync.WaitGroup
	success := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			refreshReq := tokenRequest{
				GrantType:    "refresh_token",
				RefreshToken: tokens[index].RefreshToken,
			}
			resp, _, _ := doRequest("POST", "/api/v1/token", refreshReq, "")
			if resp.StatusCode == http.StatusOK {
				success <- true
			}
		}(i)
	}

	wg.Wait()
	close(success)

	successCount := len(success)
	t.Logf("并发Token刷新成功数: %d/%d", successCount, concurrency)
	assert.Greater(t, successCount, 0)
}

// ============================================================================
// 并发资源访问测试
// ============================================================================

func TestConcurrentResourceAccess(t *testing.T) {
	email := testAwareEmail(t, "concurrentaccess")
	password := generateTestPassword()

	// 注册并验证邮箱
	user, err := registerUser(email, password)
	require.NoError(t, err)

	userID := user["user_id"].(string)
	err = verifyEmail(userID)
	require.NoError(t, err)

	tokens, err := loginUser(email, password)
	require.NoError(t, err)

	const concurrency = 20
	var wg sync.WaitGroup
	success := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			resp, _, _ := doRequest("GET", "/api/v1/userinfo", nil, tokens.AccessToken)
			if resp.StatusCode == http.StatusOK {
				success <- true
			}
		}()
	}

	wg.Wait()
	close(success)

	successCount := len(success)
	t.Logf("并发资源访问成功数: %d/%d", successCount, concurrency)
	assert.GreaterOrEqual(t, successCount, concurrency*9/10,
		"至少 90%% 的并发请求应成功")
}

// ============================================================================
// 并发注册邮箱冲突测试
// ============================================================================

func TestConcurrentRegisterSameEmail(t *testing.T) {
	email := testAwareEmail(t, "sameemail")
	password := generateTestPassword()

	const concurrency = 5
	var wg sync.WaitGroup
	conflictCount := 0
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := registerRequest{Email: email, Password: password}
			resp, _, _ := doRequest("POST", "/api/v1/register", req, "")

			if resp.StatusCode == http.StatusConflict {
				mu.Lock()
				conflictCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	t.Logf("邮箱冲突次数: %d/%d", conflictCount, concurrency)
	// 应该有concurrency-1次冲突
	assert.Equal(t, concurrency-1, conflictCount)
}

// ============================================================================
// 并发忘记密码测试
// ============================================================================

func TestConcurrentForgotPasswordFull(t *testing.T) {
	email := testAwareEmail(t, "forgotconcurrent")
	password := generateTestPassword()

	// 注册用户
	_, err := registerUser(email, password)
	require.NoError(t, err)

	const concurrency = 5
	var wg sync.WaitGroup
	success := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := forgotPasswordRequest{Email: email}
			resp, _, _ := doRequest("POST", "/api/v1/forgot-password", req, "")
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
				success <- true
			}
		}()
	}

	wg.Wait()
	close(success)

	successCount := len(success)
	t.Logf("并发忘记密码成功数: %d/%d", successCount, concurrency)
	assert.Greater(t, successCount, 0)
}

// ============================================================================
// 并发健康检查测试
// ============================================================================

func TestConcurrentHealthCheck(t *testing.T) {
	const concurrency = 50
	var wg sync.WaitGroup
	success := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			resp, _, _ := doRequest("GET", "/health", nil, "")
			if resp.StatusCode == http.StatusOK {
				success <- true
			}
		}()
	}

	wg.Wait()
	close(success)

	successCount := len(success)
	t.Logf("并发健康检查成功数: %d/%d", successCount, concurrency)
	// 并发场景下允许少量请求因调度延迟失败
	assert.GreaterOrEqual(t, successCount, concurrency*9/10,
		"至少 90%% 的并发健康检查应成功")
}
