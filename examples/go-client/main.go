// Go客户端集成示例
// 演示如何使用SSO服务进行用户认证
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ============================================================================
// 配置
// ============================================================================

// SSO服务配置
const (
	SSOBaseURL   = "http://localhost:9090"
	ClientID     = "my-app-client"
	ClientSecret = "my-app-secret"
	RedirectURI  = "http://localhost:3000/callback"
	Scopes       = "openid profile email"
)

// ============================================================================
// SSO客户端结构
// ============================================================================

// SSOToken SSO Token响应
type SSOToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// UserInfo 用户信息
type UserInfo struct {
	Sub   string   `json:"sub"`
	Email string   `json:"email"`
	Scope []string `json:"scope"`
}

// SSOClient SSO客户端
type SSOClient struct {
	baseURL      string
	clientID     string
	clientSecret string
	redirectURI  string
	httpClient   *http.Client
}

// NewSSOClient 创建SSO客户端
func NewSSOClient(baseURL, clientID, clientSecret, redirectURI string) *SSOClient {
	return &SSOClient{
		baseURL:      baseURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ============================================================================
// PKCE工具函数
// ============================================================================

// generateCodeVerifier 生成PKCE Code Verifier
func generateCodeVerifier() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// generateCodeChallenge 生成PKCE Code Challenge (S256)
func generateCodeChallenge(verifier string) string {
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// ============================================================================
// OAuth2授权码流程
// ============================================================================

// GetAuthorizationURL 获取授权URL
// 用户需要访问此URL进行登录授权
func (c *SSOClient) GetAuthorizationURL(state string, usePKCE bool) (string, string, error) {
	params := url.Values{
		"client_id":     {c.clientID},
		"redirect_uri":  {c.redirectURI},
		"response_type": {"code"},
		"scope":         {Scopes},
		"state":         {state},
	}

	var codeVerifier string
	if usePKCE {
		// 生成PKCE参数
		verifier, err := generateCodeVerifier()
		if err != nil {
			return "", "", err
		}
		codeVerifier = verifier
		params.Set("code_challenge", generateCodeChallenge(verifier))
		params.Set("code_challenge_method", "S256")
	}

	authURL := c.baseURL + "/api/v1/authorize?" + params.Encode()
	return authURL, codeVerifier, nil
}

// ExchangeCode 交换授权码获取Token
func (c *SSOClient) ExchangeCode(ctx context.Context, code, codeVerifier string) (*SSOToken, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.redirectURI},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}

	if codeVerifier != "" {
		data.Set("code_verifier", codeVerifier)
	}

	resp, err := c.httpClient.PostForm(c.baseURL+"/api/v1/token", data)
	if err != nil {
		return nil, fmt.Errorf("请求Token失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Token请求失败: %s", string(body))
	}

	var token SSOToken
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("解析Token失败: %w", err)
	}

	return &token, nil
}

// RefreshToken 刷新Token
func (c *SSOClient) RefreshToken(ctx context.Context, refreshToken string) (*SSOToken, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}

	resp, err := c.httpClient.PostForm(c.baseURL+"/api/v1/token", data)
	if err != nil {
		return nil, fmt.Errorf("刷新Token失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("刷新Token失败: %s", string(body))
	}

	var token SSOToken
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("解析Token失败: %w", err)
	}

	return &token, nil
}

// GetUserInfo 获取用户信息
func (c *SSOClient) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求用户信息失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取用户信息失败: %s", string(body))
	}

	var userInfo UserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("解析用户信息失败: %w", err)
	}

	return &userInfo, nil
}

// RevokeToken 撤销Token
func (c *SSOClient) RevokeToken(ctx context.Context, token string) error {
	data := url.Values{
		"token": {token},
	}

	resp, err := c.httpClient.PostForm(c.baseURL+"/api/v1/token/revoke", data)
	if err != nil {
		return fmt.Errorf("撤销Token失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("撤销Token失败: %s", string(body))
	}

	return nil
}

// ============================================================================
// 示例用法
// ============================================================================

func main() {
	fmt.Println("=== SSO客户端集成示例 ===")
	fmt.Println()

	// 创建SSO客户端
	client := NewSSOClient(SSOBaseURL, ClientID, ClientSecret, RedirectURI)

	// 示例1: 获取授权URL (使用PKCE)
	fmt.Println("1. 获取授权URL (使用PKCE)")
	state := "random-state-123"
	authURL, codeVerifier, err := client.GetAuthorizationURL(state, true)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}
	fmt.Printf("   授权URL: %s\n", authURL)
	fmt.Printf("   Code Verifier: %s\n", codeVerifier)
	fmt.Println()

	// 示例2: 用户登录后，使用授权码交换Token
	fmt.Println("2. 交换授权码获取Token")
	fmt.Println("   (用户需要访问授权URL并完成登录)")
	fmt.Println()

	// 模拟授权码 (实际使用时从回调URL获取)
	code := "example-auth-code"
	token, err := client.ExchangeCode(context.Background(), code, codeVerifier)
	if err != nil {
		fmt.Printf("   错误: %v\n", err)
		fmt.Println("   (这是预期的，因为授权码是模拟的)")
	} else {
		fmt.Printf("   Access Token: %s\n", token.AccessToken[:20]+"...")
		fmt.Printf("   Token Type: %s\n", token.TokenType)
		fmt.Printf("   过期时间: %d秒\n", token.ExpiresIn)
	}
	fmt.Println()

	// 示例3: 使用Token获取用户信息
	fmt.Println("3. 获取用户信息")
	fmt.Println("   (需要有效的Access Token)")
	fmt.Println()

	// 示例4: 刷新Token
	fmt.Println("4. 刷新Token")
	fmt.Println("   (需要有效的Refresh Token)")
	fmt.Println()

	// 示例5: 撤销Token
	fmt.Println("5. 撤销Token")
	fmt.Println("   (登出)")
	fmt.Println()

	fmt.Println("=== 示例完成 ===")
}
