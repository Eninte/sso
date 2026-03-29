package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

	var resp TokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("sso: parse token response: %w", err)
	}

	return &resp, nil
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
	body, err := c.doGet(ctx, fmt.Sprintf("/api/v1/verify-email?token=%s&user_id=%s", token, userID), false)
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
