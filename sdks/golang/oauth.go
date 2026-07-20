package sdk

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// ============================================================================
// OAuth2 授权相关方法
// ============================================================================

// Authorize 获取OAuth2授权码
func (c *Client) Authorize(ctx context.Context, clientID, redirectURI, scope, state string) (*AuthorizeResponse, error) {
	// 使用 url.Values 构造查询串，避免手动拼接导致编码问题
	values := url.Values{}
	values.Set("client_id", clientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("response_type", "code")
	values.Set("scope", scope)
	values.Set("state", state)
	path := "/api/v1/authorize?" + values.Encode()

	body, err := c.doGet(ctx, path, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[AuthorizeResponse](body)
}

// AuthorizeWithPKCE 获取OAuth2授权码（带PKCE）
func (c *Client) AuthorizeWithPKCE(ctx context.Context, clientID, redirectURI, scope, state, codeChallenge string) (*AuthorizeResponse, error) {
	// 使用 url.Values 构造查询串，避免手动拼接导致编码问题
	values := url.Values{}
	values.Set("client_id", clientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("response_type", "code")
	values.Set("scope", scope)
	values.Set("state", state)
	values.Set("code_challenge", codeChallenge)
	values.Set("code_challenge_method", "S256")
	path := "/api/v1/authorize?" + values.Encode()

	body, err := c.doGet(ctx, path, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[AuthorizeResponse](body)
}

// ApproveAuthorization 批准OAuth2授权
//
// 阶段 5.3 契约修复：服务端期望请求体 {consent_token, state}，
// 不再接受 client_id/redirect_uri/scope 等字段（consent_token JWT 内部已携带）。
// 调用方需先调用 Authorize/AuthorizeWithPKCE 获取 ConsentToken，再传给本方法。
//
// 成功后返回 {code, state}，使用 code 调用 ExchangeCode 换取 Access Token。
func (c *Client) ApproveAuthorization(ctx context.Context, req AuthorizeApproveRequest) (*AuthorizeResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/authorize/approve", req, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[AuthorizeResponse](body)
}

// DenyAuthorization 拒绝OAuth2授权
//
// 阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
// 服务端固定返回 HTTP 403 + {error:"access_denied", error_description, state}，
// 本方法将此响应当作正常的 DenyResponse 返回（不视为错误），
// 调用方拿到后应向客户端应用回传 ?error=access_denied&state=xxx。
//
// 注意：仅在用户主动拒绝时调用；其他场景的 403 仍按错误处理。
func (c *Client) DenyAuthorization(ctx context.Context, req AuthorizeDenyRequest) (*AuthorizeDenyResponse, error) {
	// 服务端 deny 端点固定返回 403，doPost 会把非 2xx 视为错误。
	// 这里通过发送请求并解析错误响应体来获取 DenyResponse。
	body, err := c.doPost(ctx, "/api/v1/authorize/deny", req, true)
	if err == nil {
		// 理论上不会进入此分支（服务端固定返回 403），但保留兼容性
		return unmarshalJSON[AuthorizeDenyResponse](body)
	}

	// 尝试从错误响应体解析 DenyResponse
	var ssoErr *Error
	if errors.As(err, &ssoErr) && ssoErr.HTTPStatus == http.StatusForbidden {
		denyResp, parseErr := unmarshalJSON[AuthorizeDenyResponse]([]byte(ssoErr.RawBody))
		if parseErr == nil {
			return denyResp, nil
		}
	}

	return nil, err
}

// ============================================================================
// Social Login 社交登录相关方法
//
// 阶段 5.5 新增：为 6 个 SDK 添加 Social Login 完整封装。
// 服务端契约：
//   - GET /auth/providers         公开端点，返回 OAuthProvider 数组（不包裹 data）
//   - GET /auth/{provider}?state= 公开端点，返回 HTTP 307 重定向到 provider 授权页面
//   - GET /auth/{provider}/callback?code=&state= 公开端点，平铺返回 TokenResponse
// ============================================================================

// GetProviders 获取支持的社交登录提供商列表
//
// 阶段 5.5 新增：调用 GET /auth/providers 公开端点。
// 服务端直接返回数组（不包裹在 data 中），无需认证。
func (c *Client) GetProviders(ctx context.Context) ([]OAuthProvider, error) {
	body, err := c.doGet(ctx, "/auth/providers", false)
	if err != nil {
		return nil, err
	}

	providers, err := unmarshalJSON[[]OAuthProvider](body)
	if err != nil {
		return nil, err
	}
	return *providers, nil
}

// GetSocialLoginURL 构造发起社交登录的 URL
//
// 阶段 5.5 新增：直接构造 URL 字符串，不发起 HTTP 请求。
// 调用方应使用浏览器重定向到此 URL（服务端会返回 307 到 provider 授权页面），
// 而不是 SDK 直接 GET。
//
// 参数：
//   - provider：社交登录提供商名称（如 "google" / "github"）
//   - state：可选，调用方传入的 CSRF 防护 state；为空时由服务端自动生成 UUID
func (c *Client) GetSocialLoginURL(provider, state string) string {
	u := c.baseURL + "/auth/" + url.PathEscape(provider)
	if state != "" {
		values := url.Values{}
		values.Set("state", state)
		u += "?" + values.Encode()
	}
	return u
}

// ExchangeSocialCode 用回调返回的 code+state 完成社交登录
//
// 阶段 5.5 新增：调用 GET /auth/{provider}/callback?code={code}&state={state} 公开端点。
// 服务端直接平铺返回 TokenResponse（不包裹 data），无需认证。
// 成功后调用 SetTokens 缓存到客户端。
//
// 失败错误码：MISSING_AUTH_CODE / OAUTH_STATE_INVALID / OAUTH_STATE_EXPIRED /
// PROVIDER_NOT_SUPPORTED / OAUTH_CODE_EXCHANGE_FAILED / SOCIAL_LOGIN_FAILED /
// PROVIDER_USER_ID_MISSING / PROVIDER_EMAIL_NOT_VERIFIED /
// SOCIAL_ACCOUNT_CONFLICT / EMAIL_CONFLICT_WITH_LOCAL / ACCOUNT_DISABLED / ACCOUNT_LOCKED
func (c *Client) ExchangeSocialCode(ctx context.Context, provider, code, state string) (*TokenResponse, error) {
	values := url.Values{}
	values.Set("code", code)
	values.Set("state", state)
	path := "/auth/" + url.PathEscape(provider) + "/callback?" + values.Encode()

	body, err := c.doGet(ctx, path, false)
	if err != nil {
		return nil, err
	}

	tokenResp, err := unmarshalJSON[TokenResponse](body)
	if err != nil {
		return nil, err
	}

	c.SetTokens(tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.ExpiresIn)
	return tokenResp, nil
}
