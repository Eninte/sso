// Package postgres PostgreSQL MFA恢复码存储实现
package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"os"
)

// recoveryCodeHMACKey 恢复码HMAC密钥（从环境变量读取，默认为固定值）
func recoveryCodeHMACKey() []byte {
	key := os.Getenv("MFA_RECOVERY_HMAC_KEY")
	if key == "" {
		key = "sso-mfa-recovery-default-key-change-in-production"
	}
	return []byte(key)
}

// hashRecoveryCode 使用HMAC-SHA256哈希恢复码，实现O(1)查找
func hashRecoveryCode(code string) string {
	mac := hmac.New(sha256.New, recoveryCodeHMACKey())
	mac.Write([]byte(code))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

// ============================================================================
// MFA恢复码存储实现
// ============================================================================

// StoreMFARecoveryCodes 存储MFA恢复码（HMAC-SHA256哈希后）
func (s *Store) StoreMFARecoveryCodes(ctx context.Context, userID string, codeHashes []string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 先删除旧恢复码
	_, err := s.db.ExecContext(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete old recovery codes failed: %w", err)
	}

	// 插入新恢复码
	for _, codeHash := range codeHashes {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO mfa_recovery_codes (user_id, code_hash) VALUES ($1, $2)`,
			userID, codeHash,
		)
		if err != nil {
			return fmt.Errorf("insert recovery code failed: %w", err)
		}
	}

	return nil
}

// GetUnusedMFARecoveryCodes 获取用户未使用的恢复码哈希列表
func (s *Store) GetUnusedMFARecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `SELECT code_hash FROM mfa_recovery_codes WHERE user_id = $1 AND used_at IS NULL`
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("查询恢复码失败: %w", err)
	}
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("扫描恢复码失败: %w", err)
		}
		codes = append(codes, code)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return codes, nil
}

// VerifyAndUseMFARecoveryCode 验证并使用恢复码（O(1) HMAC查找）
// 返回是否验证成功
func (s *Store) VerifyAndUseMFARecoveryCode(ctx context.Context, userID, code string) (bool, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 使用HMAC-SHA256哈希输入的恢复码，直接数据库匹配
	codeHash := hashRecoveryCode(code)

	// 直接匹配哈希，O(1)操作
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM mfa_recovery_codes WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL)`
	err := s.db.QueryRowContext(ctx, query, userID, codeHash).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("query recovery code failed: %w", err)
	}

	if !exists {
		return false, nil
	}

	// 标记为已使用
	_, err = s.db.ExecContext(ctx,
		`UPDATE mfa_recovery_codes SET used_at = NOW() WHERE user_id = $1 AND code_hash = $2`,
		userID, codeHash,
	)
	if err != nil {
		return false, fmt.Errorf("mark recovery code used failed: %w", err)
	}

	return true, nil
}

// DeleteUsedMFARecoveryCodes 删除已使用的恢复码
func (s *Store) DeleteUsedMFARecoveryCodes(ctx context.Context, userID string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	_, err := s.db.ExecContext(ctx,
		`DELETE FROM mfa_recovery_codes WHERE user_id = $1 AND used_at IS NOT NULL`,
		userID,
	)
	return err
}
