//go:build e2e

// Package e2e 端到端测试辅助函数
package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// ============================================================================
// 测试配置
// ============================================================================

var (
	baseURL = getEnvOrDefault("E2E_BASE_URL", "http://localhost:9090")
	client  = &http.Client{Timeout: 30 * time.Second}

	// 管理员账户配置 - 必须从环境变量读取，禁止硬编码默认值
	adminEmail    = os.Getenv("E2E_ADMIN_EMAIL")
	adminPassword = os.Getenv("E2E_ADMIN_PASSWORD")

	// rateLimitDisabled 在 TestMain 中检测，为 true 表示限流已禁用
	rateLimitDisabled bool
)

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ============================================================================
// TestMain 环境自检
// ============================================================================

func TestMain(m *testing.M) {
	fmt.Println("=== E2E 测试环境自检 ===")

	// 1. 检查服务可达性
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		fmt.Printf("FATAL: SSO服务不可达 (%s)\n  错误: %v\n  请先启动服务\n", baseURL, err)
		os.Exit(1)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("FATAL: 健康检查返回 %d，期望 200\n", resp.StatusCode)
		os.Exit(1)
	}
	fmt.Println("[OK] 服务可达")

	// 2. 检测限流是否已禁用（连续 20 个请求，全部应返回非 429）
	rateLimitDisabled = checkRateLimitDisabled()
	if !rateLimitDisabled {
		fmt.Println("FATAL: 限流未禁用，大量测试将因 429 失败")
		fmt.Println("  请以 RATE_LIMIT_REQUESTS=0 启动服务")
		os.Exit(1)
	}
	fmt.Println("[OK] 限流已禁用（RATE_LIMIT_REQUESTS=0 生效）")

	// 3. 检查测试专用 API 可用性
	probeReq := map[string]string{"user_id": "00000000-0000-0000-0000-000000000000"}
	probeBody, _ := json.Marshal(probeReq)
	probeResp, err := client.Post(baseURL+"/api/v1/test/verify-email", "application/json", bytes.NewReader(probeBody))
	if err != nil {
		fmt.Printf("FATAL: 测试专用 API 不可达: %v\n", err)
		os.Exit(1)
	}
	probeResp.Body.Close()
	if probeResp.StatusCode == http.StatusNotFound {
		fmt.Println("FATAL: /api/v1/test/verify-email 返回 404")
		fmt.Println("  服务需要以 SERVER_ENV=development 启用测试 API")
		os.Exit(1)
	}
	if probeResp.StatusCode == http.StatusForbidden {
		fmt.Println("FATAL: /api/v1/test/verify-email 返回 403")
		fmt.Println("  服务需要 SERVER_ENV=development 或 E2E_ENABLED=true")
		os.Exit(1)
	}
	fmt.Println("[OK] 测试专用 API 可用")

	fmt.Println("=== 环境自检通过，开始运行测试 ===")
	fmt.Println()

	os.Exit(m.Run())
}

// checkRateLimitDisabled 检测限流是否已禁用。
// 连续发送 20 个请求到 /health，若有任何一个返回 429 则判定限流未禁用。
func checkRateLimitDisabled() bool {
	for i := 0; i < 20; i++ {
		resp, err := client.Get(baseURL + "/health")
		if err != nil {
			return false
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			return false
		}
	}
	return true
}

// ============================================================================
// 数据库直接操作辅助（测试数据准备）
// ============================================================================

// setUserRoleDB 直接通过数据库修改用户角色。
// 测试数据准备不走 API，直接操作数据库是 E2E 测试的标准做法。
func setUserRoleDB(userID, role string) error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL 未设置")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}
	defer db.Close()

	_, err = db.Exec("UPDATE users SET role = $1 WHERE id = $2", role, userID)
	if err != nil {
		return fmt.Errorf("更新用户角色失败: %w", err)
	}
	return nil
}

// ============================================================================
// 测试断言辅助
// ============================================================================

// assertNotRateLimited 检查 HTTP 响应是否被限流。
// 如果返回 429，标记测试失败并提示检查环境配置。
func assertNotRateLimited(t *testing.T, resp *http.Response) bool {
	t.Helper()
	if resp.StatusCode == http.StatusTooManyRequests {
		t.Fatalf("请求被限流 (429)，请确认服务以 RATE_LIMIT_REQUESTS=0 启动")
		return false
	}
	return true
}

// ============================================================================
// 请求类型定义
// ============================================================================

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

type tokenRequest struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type verifyEmailRequest struct {
	Token string `json:"token"`
}

type revokeRequest struct {
	Token string `json:"token"`
}

type adminUserActionRequest struct {
	UserID string `json:"user_id"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type userInfoResponse struct {
	Sub           string   `json:"sub"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Scopes        []string `json:"scopes"`
}

type userListResponse struct {
	Users []map[string]interface{} `json:"users"`
	Total int                      `json:"total"`
}

// ============================================================================
// HTTP请求辅助函数
// ============================================================================

// doRequest 发送HTTP请求
func doRequest(method, path string, body interface{}, token string) (*http.Response, []byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, baseURL+path, bodyReader)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}

	return resp, respBody, nil
}

// doFormRequest 发送表单请求
func doFormRequest(method, path string, formData map[string]string) (*http.Response, []byte, error) {
	form := url.Values{}
	for key, value := range formData {
		form.Set(key, value)
	}

	req, err := http.NewRequest(method, baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}

	return resp, respBody, nil
}

// ============================================================================
// 测试数据生成辅助函数
// ============================================================================

// generateUniqueEmail 生成唯一邮箱
func generateUniqueEmail(prefix string) string {
	return fmt.Sprintf("test-%s-%d@example.com", prefix, time.Now().UnixNano())
}

// generateTestPassword 生成测试密码
func generateTestPassword() string {
	return "TestPassword123!"
}

// ============================================================================
// 用户操作辅助函数
// ============================================================================

// registerUser 注册用户并返回用户信息
func registerUser(email, password string) (map[string]interface{}, error) {
	req := registerRequest{
		Email:    email,
		Password: password,
	}
	resp, body, err := doRequest("POST", "/api/v1/register", req, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("注册失败: %d", resp.StatusCode)
	}

	// 响应格式: {"data": {"email": "...", "user_id": "..."}, "message": "..."}
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	// 提取 data 字段
	data, ok := response["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("响应格式错误: 缺少data字段")
	}

	return data, nil
}

// verifyEmail 使用测试API验证邮箱
// 注意：这是一个测试专用API，生产环境不存在
func verifyEmail(userID string) error {
	// 使用测试专用API验证邮箱
	req := map[string]string{"user_id": userID}
	resp, _, err := doRequest("POST", "/api/v1/test/verify-email", req, "")
	if err != nil {
		return fmt.Errorf("验证邮箱请求失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("验证邮箱失败: %d", resp.StatusCode)
	}
	return nil
}

// loginUser 登录用户并返回Token
func loginUser(email, password string) (*loginResponse, error) {
	req := loginRequest{
		Email:    email,
		Password: password,
	}
	resp, body, err := doRequest("POST", "/api/v1/login", req, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("登录失败: %d", resp.StatusCode)
	}

	// 响应格式: {"access_token": "...", "refresh_token": "...", ...}
	var tokens loginResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, err
	}

	return &tokens, nil
}

// registerAndLogin 注册、验证邮箱并登录用户
func registerAndLogin() (string, *loginResponse, error) {
	email := generateUniqueEmail("e2e")
	password := generateTestPassword()

	user, err := registerUser(email, password)
	if err != nil {
		return "", nil, err
	}

	// 验证邮箱（使用测试专用API）
	if userID, ok := user["user_id"].(string); ok && userID != "" {
		if err := verifyEmail(userID); err != nil {
			return "", nil, fmt.Errorf("验证邮箱失败: %w", err)
		}
	}

	tokens, err := loginUser(email, password)
	if err != nil {
		return "", nil, err
	}

	return email, tokens, nil
}

// ============================================================================
// 断言辅助函数
// ============================================================================

// parseErrorResponse 解析错误响应
func parseErrorResponse(body []byte) (*errorResponse, error) {
	var errResp errorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return nil, err
	}
	return &errResp, nil
}

// parseUserInfo 解析用户信息响应
func parseUserInfo(body []byte) (*userInfoResponse, error) {
	var userInfo userInfoResponse
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, err
	}
	return &userInfo, nil
}
