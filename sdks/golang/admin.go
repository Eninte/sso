package sdk

import (
	"context"
	"net/url"
	"strconv"
)

// ============================================================================
// 管理员相关方法
// ============================================================================

// AdminHealth 管理员健康检查
func (c *Client) AdminHealth(ctx context.Context) (*HealthResponse, error) {
	body, err := c.doGet(ctx, "/api/v1/admin/health", true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[HealthResponse](body)
}

// AdminCleanup 清理过期数据
func (c *Client) AdminCleanup(ctx context.Context) (*MessageResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/admin/cleanup", nil, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}

// ListUsers 获取用户列表（分页）
func (c *Client) ListUsers(ctx context.Context, page, pageSize int) (*UserListResponse, error) {
	// 使用 url.Values 构造查询串，避免手动拼接
	values := url.Values{}
	values.Set("page", strconv.Itoa(page))
	values.Set("pageSize", strconv.Itoa(pageSize))
	path := "/api/v1/admin/users?" + values.Encode()

	body, err := c.doGet(ctx, path, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[UserListResponse](body)
}

// GetUser 获取用户详情
// 服务端使用路径参数 /api/v1/admin/users/{id}
func (c *Client) GetUser(ctx context.Context, userID string) (*UserItem, error) {
	path := "/api/v1/admin/users/" + url.PathEscape(userID)

	body, err := c.doGet(ctx, path, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[UserItem](body)
}

// DisableUser 禁用用户
// 服务端使用路径参数 /api/v1/admin/users/{id}/disable，无需请求体
func (c *Client) DisableUser(ctx context.Context, userID string) (*MessageResponse, error) {
	path := "/api/v1/admin/users/" + url.PathEscape(userID) + "/disable"

	body, err := c.doPost(ctx, path, nil, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}

// EnableUser 启用用户
// 服务端使用路径参数 /api/v1/admin/users/{id}/enable，无需请求体
func (c *Client) EnableUser(ctx context.Context, userID string) (*MessageResponse, error) {
	path := "/api/v1/admin/users/" + url.PathEscape(userID) + "/enable"

	body, err := c.doPost(ctx, path, nil, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}

// ============================================================================
// OIDC 相关方法
// ============================================================================

// Discovery 获取OIDC Discovery配置
func (c *Client) Discovery(ctx context.Context) (*DiscoveryResponse, error) {
	body, err := c.doGet(ctx, "/.well-known/openid-configuration", false)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[DiscoveryResponse](body)
}

// JWKS 获取JWKS公钥
func (c *Client) JWKS(ctx context.Context) (*JWKSResponse, error) {
	body, err := c.doGet(ctx, "/.well-known/jwks.json", false)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[JWKSResponse](body)
}
