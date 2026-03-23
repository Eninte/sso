package model

// MFASetupResponse MFA设置响应
type MFASetupResponse struct {
	Secret      string `json:"secret"`       // MFA密钥
	QRCodeURL   string `json:"qr_code_url"`  // QR码URL
	ManualEntry string `json:"manual_entry"` // 手动输入密钥
}

// MFAStatusResponse MFA状态响应
type MFAStatusResponse struct {
	Enabled bool `json:"enabled"` // MFA是否启用
}
