//go:build e2e

// Package e2e 端到端测试辅助函数
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ============================================================================
// 测试配置
// ============================================================================

var (
	baseURL = getEnvOrDefault("E2E_BASE_URL", "http://localhost:9090")
	client  = &http.Client{Timeout: 10 * time.Second}
)

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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
	AccessToken string `json:"access_token"`
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

	// 响应格式: {"data": {"access_token": "...", ...}, ...}
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("响应格式错误: 缺少data字段")
	}

	dataBytes, _ := json.Marshal(data)
	var tokens loginResponse
	if err := json.Unmarshal(dataBytes, &tokens); err != nil {
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
