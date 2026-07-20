package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ============================================================================
// 认证相关方法
// ============================================================================

// Register 注册新用户
func (c *Client) Register(ctx context.Context, email, password string) (*RegisterResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/register", RegisterRequest{
		Email:    email,
		Password: password,
	}, false)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[RegisterResponse](body)
}

// Login 登录
//
// 阶段 5.4 契约扩展：若用户启用了 MFA，响应中的 MFARequired 字段为 true，
// 此时 AccessToken/RefreshToken 为空，调用方需展示 MFA 输入页面，
// 收到用户输入的 TOTP/恢复码后调用 VerifyMFALogin 完成第二阶段验证。
// 若 MFARequired 为 false，则直接返回 Token 并自动缓存到客户端。
func (c *Client) Login(ctx context.Context, email, password string) (*TokenResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/login", LoginRequest{
		Email:    email,
		Password: password,
	}, false)
	if err != nil {
		return nil, err
	}

	var resp TokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("sso: parse login response: %w", err)
	}

	// MFA 两阶段登录：不缓存 Token，等待第二阶段验证
	if !resp.MFARequired {
		c.SetTokens(resp.AccessToken, resp.RefreshToken, resp.ExpiresIn)
	}

	return &resp, nil
}

// VerifyMFALogin 完成 MFA 两阶段登录的第二阶段
//
// 阶段 5.4 新增：调用 /api/v1/login/mfa/verify 端点。
// 成功后返回标准 TokenResponse 并自动缓存到客户端。
//
// 错误处理建议：
//   - ErrCodeMFAChallengeInvalid / ErrCodeMFAChallengeExpired → 重新发起登录获取新 challenge
//   - ErrCodeInvalidMFACode → 提示用户重新输入（challenge 仍有效，尝试次数会递增）
//   - ErrCodeTooManyMFAAttempts → challenge 已失效，需重新登录
func (c *Client) VerifyMFALogin(ctx context.Context, req LoginMFAVerifyRequest) (*TokenResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/login/mfa/verify", req, false)
	if err != nil {
		return nil, err
	}

	var resp TokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("sso: parse MFA login response: %w", err)
	}

	c.SetTokens(resp.AccessToken, resp.RefreshToken, resp.ExpiresIn)

	return &resp, nil
}

// RefreshToken 刷新Token
func (c *Client) RefreshToken(ctx context.Context) (*TokenResponse, error) {
	c.mu.RLock()
	refreshToken := c.refreshToken
	c.mu.RUnlock()

	if refreshToken == "" {
		return nil, &Error{
			HTTPStatus: http.StatusUnauthorized,
			Code:       ErrCodeUnauthorized,
			Message:    "no refresh token available",
		}
	}

	resp, err := c.refreshTokenInternal(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	c.SetTokens(resp.AccessToken, resp.RefreshToken, resp.ExpiresIn)

	return resp, nil
}

// ExchangeCode 用授权码交换Token（OAuth2）
// 服务端响应使用 handlerutil.WriteJSONSuccess 包裹，格式为 {"data": {...}}，
// 需先解析外层结构，再取 data 字段反序列化为 TokenResponse。
func (c *Client) ExchangeCode(ctx context.Context, code, clientID, clientSecret, redirectURI, codeVerifier string) (*TokenResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/token", TokenRequest{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
		CodeVerifier: codeVerifier,
	}, false)
	if err != nil {
		return nil, err
	}

	// 剥离 data 包装层
	var wrapper struct {
		Data TokenResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("sso: parse token response: %w", err)
	}

	return &wrapper.Data, nil
}

// RevokeToken 撤销Token（登出）
func (c *Client) RevokeToken(ctx context.Context) error {
	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	if token == "" {
		return nil
	}

	_, err := c.doPost(ctx, "/api/v1/token/revoke", RevokeRequest{
		Token: token,
	}, false)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.accessToken = ""
	c.refreshToken = ""
	c.tokenExpiry = time.Time{}
	c.mu.Unlock()

	return nil
}

// ForgotPassword 忘记密码（发送重置邮件）
func (c *Client) ForgotPassword(ctx context.Context, email string) (*MessageResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/forgot-password", ForgotPasswordRequest{
		Email: email,
	}, false)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}

// ResetPassword 重置密码
func (c *Client) ResetPassword(ctx context.Context, token, userID, newPassword string) (*MessageResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/reset-password", ResetPasswordRequest{
		Token:       token,
		UserID:      userID,
		NewPassword: newPassword,
	}, false)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}

// VerifyEmail 验证邮箱
func (c *Client) VerifyEmail(ctx context.Context, token, userID string) (*MessageResponse, error) {
	// 使用 url.Values 构造查询串，避免手动拼接导致编码问题
	values := url.Values{}
	values.Set("token", token)
	values.Set("user_id", userID)
	path := "/api/v1/verify-email?" + values.Encode()

	body, err := c.doGet(ctx, path, false)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}

// SendVerificationEmail 发送验证邮件
func (c *Client) SendVerificationEmail(ctx context.Context) (*MessageResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/verify-email/send", nil, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}
