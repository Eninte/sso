package sdk

import (
	"context"
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
func (c *Client) ApproveAuthorization(ctx context.Context, req AuthorizeApproveRequest) (*AuthorizeResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/authorize/approve", req, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[AuthorizeResponse](body)
}
