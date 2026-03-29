package sdk

import "context"

// ============================================================================
// 用户相关方法
// ============================================================================

// UserInfo 获取当前用户信息
func (c *Client) UserInfo(ctx context.Context) (*UserInfo, error) {
	body, err := c.doGet(ctx, "/api/v1/userinfo", true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[UserInfo](body)
}

// ChangePassword 修改密码
func (c *Client) ChangePassword(ctx context.Context, oldPassword, newPassword string) (*MessageResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/change-password", ChangePasswordRequest{
		OldPassword: oldPassword,
		NewPassword: newPassword,
	}, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}
