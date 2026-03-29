package sdk

import (
	"context"
	"fmt"
)

// ============================================================================
// OAuth2 授权相关方法
// ============================================================================

// Authorize 获取OAuth2授权码
func (c *Client) Authorize(ctx context.Context, clientID, redirectURI, scope, state string) (*AuthorizeResponse, error) {
	path := fmt.Sprintf("/api/v1/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
		clientID, redirectURI, scope, state)

	body, err := c.doGet(ctx, path, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[AuthorizeResponse](body)
}

// AuthorizeWithPKCE 获取OAuth2授权码（带PKCE）
func (c *Client) AuthorizeWithPKCE(ctx context.Context, clientID, redirectURI, scope, state, codeChallenge string) (*AuthorizeResponse, error) {
	path := fmt.Sprintf("/api/v1/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s&code_challenge=%s&code_challenge_method=S256",
		clientID, redirectURI, scope, state, codeChallenge)

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
