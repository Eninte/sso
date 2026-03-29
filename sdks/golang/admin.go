package sdk

import (
	"context"
	"fmt"
)

// ============================================================================
// 管理员相关方法
// ============================================================================

// AdminHealth 管理员健康检查
func (c *Client) AdminHealth(ctx context.Context) (*HealthResponse, error) {
	body, err := c.doGet(ctx, "/admin/health", true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[HealthResponse](body)
}

// AdminCleanup 清理过期数据
func (c *Client) AdminCleanup(ctx context.Context) (*MessageResponse, error) {
	body, err := c.doPost(ctx, "/admin/cleanup", nil, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}

// ListUsers 获取用户列表（分页）
func (c *Client) ListUsers(ctx context.Context, page, pageSize int) (*UserListResponse, error) {
	path := fmt.Sprintf("/admin/users?page=%d&pageSize=%d", page, pageSize)

	body, err := c.doGet(ctx, path, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[UserListResponse](body)
}

// GetUser 获取用户详情
func (c *Client) GetUser(ctx context.Context, userID string) (*UserItem, error) {
	path := fmt.Sprintf("/admin/users?id=%s", userID)

	body, err := c.doGet(ctx, path, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[UserItem](body)
}

// DisableUser 禁用用户
func (c *Client) DisableUser(ctx context.Context, userID string) (*MessageResponse, error) {
	body, err := c.doPost(ctx, "/admin/users/disable", DisableUserRequest{
		UserID: userID,
	}, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}

// EnableUser 启用用户
func (c *Client) EnableUser(ctx context.Context, userID string) (*MessageResponse, error) {
	body, err := c.doPost(ctx, "/admin/users/enable", EnableUserRequest{
		UserID: userID,
	}, true)
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
