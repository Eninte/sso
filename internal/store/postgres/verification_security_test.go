// Package postgres PostgreSQL 验证令牌存储安全测试
package postgres

import (
	"testing"
)

// TestValidateTableName 测试表名白名单验证
func TestValidateTableName(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		wantErr   bool
	}{
		{
			name:      "允许verification_tokens表",
			tableName: "verification_tokens",
			wantErr:   false,
		},
		{
			name:      "允许reset_tokens表",
			tableName: "reset_tokens",
			wantErr:   false,
		},
		{
			name:      "拒绝users表（SQL注入尝试）",
			tableName: "users",
			wantErr:   true,
		},
		{
			name:      "拒绝恶意SQL注入",
			tableName: "users; DROP TABLE users; --",
			wantErr:   true,
		},
		{
			name:      "拒绝空表名",
			tableName: "",
			wantErr:   true,
		},
		{
			name:      "拒绝未知表名",
			tableName: "unknown_table",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTableName(tt.tableName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTableName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestAllowedTokenTables 测试白名单包含所有必需的表
func TestAllowedTokenTables(t *testing.T) {
	requiredTables := []string{
		"verification_tokens",
		"reset_tokens",
	}

	for _, table := range requiredTables {
		if !allowedTokenTables[table] {
			t.Errorf("白名单缺少必需的表: %s", table)
		}
	}

	// 确保白名单只包含这些表
	if len(allowedTokenTables) != len(requiredTables) {
		t.Errorf("白名单包含额外的表，期望 %d 个，实际 %d 个", len(requiredTables), len(allowedTokenTables))
	}
}
