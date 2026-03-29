package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Client 客户端
// ============================================================================

// Client SSO服务客户端
type Client struct {
	baseURL    string
	httpClient *http.Client

	// Token管理
	mu           sync.RWMutex
	accessToken  string
	refreshToken string
	tokenExpiry  time.Time
}

// Option 客户端配置选项
type Option func(*Client)

// WithHTTPClient 自定义HTTP客户端
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// WithTimeout 请求超时
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// WithAccessToken 预设Access Token
func WithAccessToken(token string) Option {
	return func(c *Client) {
		c.accessToken = token
	}
}

// WithRefreshToken 预设Refresh Token
func WithRefreshToken(token string) Option {
	return func(c *Client) {
		c.refreshToken = token
	}
}

// NewClient 创建SSO客户端
func NewClient(baseURL string, opts ...Option) *Client {
	baseURL = strings.TrimRight(baseURL, "/")

	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// BaseURL 返回服务基础URL
func (c *Client) BaseURL() string {
	return c.baseURL
}

// SetTokens 设置Token
func (c *Client) SetTokens(accessToken, refreshToken string, expiresIn int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.accessToken = accessToken
	c.refreshToken = refreshToken
	c.tokenExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
}

// AccessToken 获取当前Access Token
func (c *Client) AccessToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.accessToken
}

// ============================================================================
// HTTP请求辅助方法
// ============================================================================

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, auth bool) ([]byte, error) {
	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("sdo: marshal request: %w", err)
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("sso: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if auth {
		token, err := c.ensureToken(ctx)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sso: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sso: read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		errResp := newError(resp.StatusCode, respBody)
		return nil, errResp.toError()
	}

	return respBody, nil
}

func (c *Client) doGet(ctx context.Context, path string, auth bool) ([]byte, error) {
	return c.doRequest(ctx, http.MethodGet, path, nil, auth)
}

func (c *Client) doPost(ctx context.Context, path string, body interface{}, auth bool) ([]byte, error) {
	return c.doRequest(ctx, http.MethodPost, path, body, auth)
}

// ensureToken 确保有有效的Token，必要时自动刷新
func (c *Client) ensureToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	token := c.accessToken
	needsRefresh := time.Now().After(c.tokenExpiry.Add(-30 * time.Second))
	refreshToken := c.refreshToken
	c.mu.RUnlock()

	if token == "" {
		return "", &Error{
			HTTPStatus: http.StatusUnauthorized,
			Code:       ErrCodeUnauthorized,
			Message:    "no access token available, please login first",
		}
	}

	if needsRefresh && refreshToken != "" {
		resp, err := c.refreshTokenInternal(ctx, refreshToken)
		if err != nil {
			return "", err
		}
		c.SetTokens(resp.AccessToken, resp.RefreshToken, resp.ExpiresIn)
		return resp.AccessToken, nil
	}

	return token, nil
}

func (c *Client) refreshTokenInternal(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/token", TokenRequest{
		GrantType:    "refresh_token",
		RefreshToken: refreshToken,
	}, false)
	if err != nil {
		return nil, err
	}

	var resp TokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("sso: parse token response: %w", err)
	}
	return &resp, nil
}

// unmarshalJSON 通用JSON反序列化辅助
func unmarshalJSON[T any](body []byte) (*T, error) {
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, fmt.Errorf("sso: parse response: %w", err)
	}
	return &v, nil
}
