//go:build e2e

// Package e2e 端到端测试辅助函数
package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
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
		fmt.Printf("FATAL: SSO服务不可达 (%s)\n  错误: %v\n  请先启动服务: set -a; source .env.test; set +a; ./bin/sso &\n", baseURL, err)
		os.Exit(1)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("FATAL: 健康检查返回 %d，期望 200\n", resp.StatusCode)
		os.Exit(1)
	}
	fmt.Println("[OK] 服务可达")

	// 2. 检测限流是否已禁用
	rateLimitDisabled = checkRateLimitDisabled()
	if !rateLimitDisabled {
		fmt.Println("FATAL: 限流未禁用，大量测试将因 429 失败")
		fmt.Println("  请以 RATE_LIMIT_REQUESTS=0 启动服务")
		fmt.Println("  示例: RATE_LIMIT_REQUESTS=0 set -a; source .env.test; set +a; ./bin/sso &")
		os.Exit(1)
	}
	fmt.Println("[OK] 限流已禁用")

	// 3. 检查测试专用 API 可用性
	probeReq := map[string]string{"user_id": "00000000-0000-0000-0000-000000000000"}
	probeBody, _ := json.Marshal(probeReq)
	probeResp, err := client.Post(baseURL+"/api/v1/test/verify-email", "application/json", bytes.NewReader(probeBody))
	if err != nil {
		fmt.Printf("FATAL: 测试专用 API 不可达: %v\n", err)
		os.Exit(1)
	}
	probeResp.Body.Close()
	// 404 表示端点未注册（非 dev 环境），403 表示环境不允许
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

	// 4. 检查管理员账户环境变量是否设置
	if adminEmail == "" || adminPassword == "" {
		fmt.Println("FATAL: 管理员账户环境变量未设置")
		fmt.Println("  必须设置 E2E_ADMIN_EMAIL 和 E2E_ADMIN_PASSWORD 环境变量")
		fmt.Println("  示例: export E2E_ADMIN_EMAIL=admin@example.com")
		fmt.Println("        export E2E_ADMIN_PASSWORD=YourSecurePassword123!")
		os.Exit(1)
	}

	// 5. 检查管理员密码复杂度
	if err := validatePasswordStrength(adminPassword); err != nil {
		fmt.Printf("FATAL: 管理员密码不符合安全要求: %v\n", err)
		fmt.Println("  密码必须满足以下条件:")
		fmt.Println("    - 至少8个字符")
		fmt.Println("    - 包含至少一个大写字母")
		fmt.Println("    - 包含至少一个小写字母")
		fmt.Println("    - 包含至少一个数字")
		fmt.Println("    - 包含至少一个特殊字符 (!@#$%^&*()_+-=[]{}|;:,.<>?)")
		os.Exit(1)
	}

	// 6. 确保管理员账户存在
	if err := ensureAdminUser(); err != nil {
		fmt.Printf("FATAL: 管理员账户准备失败: %v\n", err)
		fmt.Printf("  请确保数据库中存在管理员账户: %s\n", adminEmail)
		os.Exit(1)
	}
	fmt.Println("[OK] 管理员账户可用")

	fmt.Println("=== 环境自检通过，开始运行测试 ===")
	fmt.Println()

	os.Exit(m.Run())
}

// checkRateLimitDisabled 快速检测限流是否已禁用
func checkRateLimitDisabled() bool {
	// 连续发 5 个请求到 /health，任一返回 429 则限流未禁用
	for i := 0; i < 5; i++ {
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

// ensureAdminUser 确保管理员账户存在且可登录
// 注意：此函数仅在测试环境中使用，生产环境禁止自动创建管理员账户
func ensureAdminUser() error {
	req := loginRequest{Email: adminEmail, Password: adminPassword}
	bodyBytes, _ := json.Marshal(req)
	resp, err := client.Post(baseURL+"/api/v1/login", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("登录请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil // 管理员已存在且可登录
	}

	// 管理员不存在，检查是否允许自动创建
	// 仅当 E2E_AUTO_CREATE_ADMIN=true 时才允许自动创建
	if os.Getenv("E2E_AUTO_CREATE_ADMIN") != "true" {
		return fmt.Errorf("管理员账户不存在且自动创建被禁用（设置 E2E_AUTO_CREATE_ADMIN=true 启用）")
	}

	// 生成随机密码用于自动创建的管理员账户（增强安全性）
	autoPassword := adminPassword
	if os.Getenv("E2E_AUTO_CREATE_ADMIN") == "true" {
		// 如果环境变量设置了强制使用随机密码，则生成随机密码
		if os.Getenv("E2E_USE_RANDOM_ADMIN_PASSWORD") == "true" {
			randomPass, err := generateRandomPassword(16)
			if err != nil {
				return fmt.Errorf("生成随机密码失败: %w", err)
			}
			autoPassword = randomPass
			fmt.Printf("[WARN] 使用随机生成的管理员密码（请记录）: %s\n", autoPassword)
		}
	}

	fmt.Printf("[INFO] 管理员账户不存在，尝试自动创建...\n")
	regReq := registerRequest{Email: adminEmail, Password: autoPassword}
	regBody, _ := json.Marshal(regReq)
	regResp, err := client.Post(baseURL+"/api/v1/register", "application/json", bytes.NewReader(regBody))
	if err != nil {
		return fmt.Errorf("注册管理员失败: %w", err)
	}
	defer regResp.Body.Close()

	if regResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("注册管理员返回 %d（期望 201）", regResp.StatusCode)
	}

	// 解析 user_id
	var regResult map[string]interface{}
	regRespBody, _ := io.ReadAll(regResp.Body)
	if err := json.Unmarshal(regRespBody, &regResult); err != nil {
		return fmt.Errorf("解析注册响应失败: %w", err)
	}
	data, _ := regResult["data"].(map[string]interface{})
	userID, _ := data["user_id"].(string)
	if userID == "" {
		return fmt.Errorf("注册响应中无 user_id")
	}

	// 验证邮箱（使用测试 API）
	verifyReq := map[string]string{"user_id": userID}
	verifyBody, _ := json.Marshal(verifyReq)
	verifyResp, err := client.Post(baseURL+"/api/v1/test/verify-email", "application/json", bytes.NewReader(verifyBody))
	if err != nil {
		return fmt.Errorf("验证邮箱失败: %w", err)
	}
	verifyResp.Body.Close()

	if verifyResp.StatusCode != http.StatusOK {
		return fmt.Errorf("验证邮箱返回 %d（期望 200）", verifyResp.StatusCode)
	}

	fmt.Printf("[OK] 管理员账户已自动创建: %s\n", adminEmail)
	return nil
}

// generateRandomPassword 生成随机密码
func generateRandomPassword(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

// validatePasswordStrength 验证密码复杂度
// 要求：至少8字符，包含大小写字母、数字和特殊字符
func validatePasswordStrength(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("密码长度不足8个字符")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, ch := range password {
		switch {
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>?", ch):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("密码必须包含至少一个大写字母")
	}
	if !hasLower {
		return fmt.Errorf("密码必须包含至少一个小写字母")
	}
	if !hasDigit {
		return fmt.Errorf("密码必须包含至少一个数字")
	}
	if !hasSpecial {
		return fmt.Errorf("密码必须包含至少一个特殊字符")
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

// registerAndLogin 注册并登录用户
func registerAndLogin() (string, *loginResponse, error) {
	email := generateUniqueEmail("e2e")
	password := generateTestPassword()

	_, err := registerUser(email, password)
	if err != nil {
		return "", nil, err
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
