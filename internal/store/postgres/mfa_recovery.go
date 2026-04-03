// Package postgres PostgreSQL MFA恢复码存储实现
package postgres

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ============================================================================
// MFA恢复码存储实现
// ============================================================================

// StoreMFARecoveryCodes 存储MFA恢复码（哈希后）
func (s *Store) StoreMFARecoveryCodes(ctx context.Context, userID string, codeHashes []string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 先删除该用户的旧恢复码
	_, err := s.db.ExecContext(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete old recovery codes failed: %w", err)
	}

	// 插入新的恢复码
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

// VerifyAndUseMFARecoveryCode 验证并使用恢复码
// 返回是否验证成功
// 注意: code 参数是用户输入的明文恢复码，数据库中存储的是 bcrypt 哈希值
func (s *Store) VerifyAndUseMFARecoveryCode(ctx context.Context, userID, code string) (bool, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 查询未使用的恢复码
	query := `SELECT code_hash FROM mfa_recovery_codes WHERE user_id = $1 AND used_at IS NULL`
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return false, fmt.Errorf("query recovery codes failed: %w", err)
	}
	defer rows.Close()

	var foundHash string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return false, fmt.Errorf("scan recovery code failed: %w", err)
		}
		// 使用 bcrypt 比较：CompareHashAndPassword(hash, password)
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(code)); err == nil {
			foundHash = hash
			break
		}
	}

	if err := rows.Err(); err != nil {
		return false, err
	}

	if foundHash == "" {
		return false, nil // 没有找到匹配的恢复码
	}

	// 标记为已使用
	_, err = s.db.ExecContext(ctx,
		`UPDATE mfa_recovery_codes SET used_at = NOW() WHERE code_hash = $1`,
		foundHash,
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
