//go:build e2e

// Package e2e 端到端测试辅助函数
package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/sso/internal/util/retryutil"
	"github.com/example/sso/internal/util/testutil"
	_ "github.com/lib/pq" // PostgreSQL 驱动
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

	// e2eDB 是 E2E 测试专用的 DB 连接，在 TestMain 中初始化。
	// 用途：registerUser 注册后从 DB 查 user_id（注册接口为防用户枚举不再返回 user_id）。
	// 复用 testutil.ConnectTestDB 获得统一重试+超时。
	e2eDB *sql.DB
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

	// 3. 检查管理员账户环境变量是否设置
	if adminEmail == "" || adminPassword == "" {
		fmt.Println("FATAL: 管理员账户环境变量未设置")
		fmt.Println("  必须设置 E2E_ADMIN_EMAIL 和 E2E_ADMIN_PASSWORD 环境变量")
		fmt.Println("  示例: export E2E_ADMIN_EMAIL=admin@example.com")
		fmt.Println("        export E2E_ADMIN_PASSWORD=YourSecurePassword123!")
		os.Exit(1)
	}

	// 4. 检查管理员密码复杂度
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

	// 5. 确保管理员账户存在
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

// e2eDBOnce 保证 e2eDB 只初始化一次（懒加载）。
// 注意：helpers.go 不带 _test.go 后缀，其中的 TestMain 不会被 go test 识别执行，
// 所以 e2eDB 不能依赖 TestMain 初始化，改用 sync.Once 在首次需要时懒加载。
var e2eDBOnce sync.Once

// initE2EDB 懒加载 e2eDB。复用 testutil 的重试+超时配置（TEST_CONN_* 环境变量）。
// 若 DATABASE_URL 未配置或连接失败，e2eDB 保持 nil，调用方需处理此情况。
func initE2EDB() {
	e2eDBOnce.Do(func() {
		dbURL := os.Getenv("DATABASE_URL")
		if dbURL == "" {
			fmt.Println("WARN: DATABASE_URL 未设置，registerUser 将无法返回 user_id")
			return
		}
		cfg := testutil.LoadConnConfig()
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			fmt.Printf("WARN: sql.Open 失败，e2eDB 未初始化: %v\n", err)
			return
		}
		pingErr := retryutil.ExponentialBackoffRetry(ctx, func(ctx context.Context) error {
			return db.PingContext(ctx)
		}, cfg.RetryConfig())
		if pingErr != nil {
			_ = db.Close()
			fmt.Printf("WARN: E2E DB 连接失败，registerUser 将无法返回 user_id: %v\n", pingErr)
			return
		}
		e2eDB = db
		fmt.Println("[OK] E2E DB 连接已建立（用于 user_id 查询）")
	})
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
// 注意：管理员账户必须预先在数据库中配置（角色为 admin、邮箱已验证）
// 此函数仅验证管理员账户是否可登录
func ensureAdminUser() error {
	// 尝试登录验证管理员账户
	req := loginRequest{Email: adminEmail, Password: adminPassword}
	bodyBytes, _ := json.Marshal(req)
	resp, err := client.Post(baseURL+"/api/v1/login", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("登录请求失败: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil // 管理员账户可登录
	}

	// 登录失败，尝试注册并提示手动配置
	regReq := registerRequest{Email: adminEmail, Password: adminPassword}
	regBody, _ := json.Marshal(regReq)
	regResp, err := client.Post(baseURL+"/api/v1/register", "application/json", bytes.NewReader(regBody))
	if err != nil {
		return fmt.Errorf("管理员登录失败（%d），注册尝试也失败: %w", resp.StatusCode, err)
	}
	regResp.Body.Close()

	return fmt.Errorf("管理员账户已注册但无法登录（需要邮箱验证和管理员角色），请手动配置数据库: UPDATE users SET email_verified=true, role='admin' WHERE email='%s'", adminEmail)
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
	UserID      string `json:"user_id"`
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

// registerUser 注册用户并返回包含 user_id 的 map。
//
// 历史背景：注册接口曾返回 {"data":{"user_id":...,"email":...}}，
// 2026-06-17 的安全修复（commit 7054cc7 "修复用户枚举漏洞 H3"）移除了 user_id 返回，
// 防止攻击者通过注册接口枚举已注册邮箱。响应改为 {"message":"..."}（无 data 字段）。
//
// 但 E2E 测试需要 user_id（verifyEmail/setUserRole/getResetTokenFromDB 都依赖它），
// 所以这里在注册成功后，用 e2eDB 按邮箱从 DB 查询 user_id 并填充到返回的 map 中。
// 这不破坏生产代码的安全设计——user_id 仅在测试进程内通过 DB 直查获得，
// 不经过 HTTP 响应暴露。
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

	// 兼容两种响应格式：
	//   旧格式（已废弃）: {"data": {"user_id": "...", "email": "..."}, "message": "..."}
	//   新格式（当前）:   {"message": "..."}（无 data 字段，防用户枚举）
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	// 优先用响应中的 data.user_id（若 handler 将来恢复返回）
	if data, ok := response["data"].(map[string]interface{}); ok {
		if _, ok := data["user_id"].(string); ok {
			return data, nil
		}
		return data, nil // 有 data 但无 user_id，返回已有的
	}

	// 新格式：响应无 data 字段，从 DB 查 user_id 填充
	initE2EDB()
	if e2eDB == nil {
		return nil, fmt.Errorf("响应无 data.user_id 且 e2eDB 未初始化（DATABASE_URL 未配置或连接失败），无法获取 user_id")
	}

	var userID string
	queryErr := e2eDB.QueryRow(
		`SELECT id FROM users WHERE email = $1`, email,
	).Scan(&userID)
	if queryErr != nil {
		return nil, fmt.Errorf("从 DB 查询 user_id 失败 (email=%s): %w", email, queryErr)
	}

	return map[string]interface{}{
		"user_id": userID,
		"email":   email,
	}, nil
}

// verifyEmail 使用测试API验证邮箱
// 注意：如果测试专用API不可用（返回404），则跳过验证
func verifyEmail(userID string) error {
	// 使用测试专用API验证邮箱
	req := map[string]string{"user_id": userID}
	resp, _, err := doRequest("POST", "/api/v1/test/verify-email", req, "")
	if err != nil {
		return fmt.Errorf("验证邮箱请求失败: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		// 测试API不可用，跳过验证
		return nil
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
// 数据库辅助函数（用于黑盒测试的白盒探针）
// 说明：E2E 测试默认是 HTTP 黑盒，但某些链路（如邮件中的 token）
// 需要直连 DB 读取真实值才能验证完整流程。
// 不污染生产代码：不添加测试专用 HTTP 端点，不修改 DB schema。
// ============================================================================

// openTestDB 打开测试数据库连接
// 通过 DATABASE_URL 环境变量获取连接字符串（与 Makefile 的 test-e2e target 一致），
// 并复用 testutil.ConnectTestDB 的重试与超时机制（TEST_CONN_* 环境变量）。
//
// 历史问题：之前读 TEST_DATABASE_URL 环境变量是 bug——Makefile 实际传给测试进程的是
// DATABASE_URL（见 Makefile 的 `DATABASE_URL="$(TEST_DATABASE_URL)" gotestsum ...`），
// 导致 openTestDB 在 make test-e2e 下总是 t.Skip，getResetTokenFromDB 永远不执行。
//
// 连接生命周期由 testutil 通过 t.Cleanup 管理，无需全局缓存。
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testutil.ConnectTestDB(t)
}

// getResetTokenFromDB 从 reset_tokens 表读取真实重置令牌
// 用于在邮件不可用的测试环境中验证完整密码重置链路
func getResetTokenFromDB(t *testing.T, userID string) string {
	t.Helper()
	db := openTestDB(t)
	var token string
	err := db.QueryRow(
		`SELECT token FROM reset_tokens WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`,
		userID,
	).Scan(&token)
	if err != nil {
		t.Fatalf("从 DB 读取重置令牌失败 (user_id=%s): %v", userID, err)
	}
	return token
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
