package sdk

import "context"

// ============================================================================
// MFA 相关方法
// ============================================================================

// MFASetup 初始化MFA设置
func (c *Client) MFASetup(ctx context.Context) (*MFASetupResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/mfa/setup", nil, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MFASetupResponse](body)
}

// MFAVerify 验证TOTP码并启用MFA
func (c *Client) MFAVerify(ctx context.Context, code string) (*MessageResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/mfa/verify", MFAVerifyRequest{
		Code: code,
	}, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}

// MFADisable 禁用MFA
func (c *Client) MFADisable(ctx context.Context, code string) (*MessageResponse, error) {
	body, err := c.doPost(ctx, "/api/v1/mfa/disable", MFAVerifyRequest{
		Code: code,
	}, true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MessageResponse](body)
}

// MFAStatus 获取MFA状态
func (c *Client) MFAStatus(ctx context.Context) (*MFAStatusResponse, error) {
	body, err := c.doGet(ctx, "/api/v1/mfa/status", true)
	if err != nil {
		return nil, err
	}

	return unmarshalJSON[MFAStatusResponse](body)
}
