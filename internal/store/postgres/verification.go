// Package postgres PostgreSQL 验证令牌存储实现
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/example/sso/internal/common"
	"github.com/example/sso/internal/store"
)

// ============================================================================
// 验证令牌存储实现
// ============================================================================

// allowedTokenTables 允许的令牌表名白名单
// 防止SQL注入攻击
var allowedTokenTables = map[string]bool{
	"verification_tokens": true,
	"reset_tokens":        true,
}

// validateTableName 验证表名是否在白名单中
// 防止SQL注入攻击
func validateTableName(tableName string) error {
	if !allowedTokenTables[tableName] {
		return fmt.Errorf("invalid table name: %s", tableName)
	}
	return nil
}

// storeToken 通用令牌存储函数
// 先删除旧令牌，再插入新令牌
//
// 安全设计（T2）：入库前对令牌计算 SHA-256 hash（common.HashToken），
// 明文只存在于邮件链接中，数据库泄露时无法直接利用令牌
func (s *Store) storeToken(ctx context.Context, tableName, userID, token string, expiresAt time.Time) error {
	// 验证表名（防止SQL注入）
	if err := validateTableName(tableName); err != nil {
		return err
	}

	// 先删除旧令牌
	// nosec G201 -- tableName 已通过 validateTableName 验证，防止 SQL 注入
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, tableName), userID); err != nil {
		return fmt.Errorf("delete old token from %s failed: %w", tableName, err)
	}

	// 插入新令牌（仅存 hash，明文不落库）
	// #nosec G201 -- tableName 已通过 validateTableName 验证，防止 SQL 注入
	query := fmt.Sprintf(`INSERT INTO %s (user_id, token, expires_at, created_at) VALUES ($1, $2, $3, $4)`, tableName)
	_, err := s.db.ExecContext(ctx, query, userID, common.HashToken(token), expiresAt, time.Now())
	return err
}

// deleteToken 通用令牌删除函数
func (s *Store) deleteToken(ctx context.Context, tableName, userID string) error {
	// 验证表名（防止SQL注入）
	if err := validateTableName(tableName); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, tableName), userID)
	return err
}

// StoreVerificationToken 存储验证令牌
func (s *Store) StoreVerificationToken(ctx context.Context, userID string, token string, expiresAt time.Time) error {
	return s.storeToken(ctx, "verification_tokens", userID, token, expiresAt)
}

// GetVerificationToken 获取验证令牌
//
// 安全设计（T2）：返回的 Token 字段为令牌的 SHA-256 哈希值（64 位 hex），
// 非明文——调用方比对前须先对输入令牌计算 common.HashToken
func (s *Store) GetVerificationToken(ctx context.Context, userID string) (*store.VerificationToken, error) {
	query := `SELECT token, expires_at FROM verification_tokens WHERE user_id = $1`
	var token store.VerificationToken
	err := s.db.QueryRowContext(ctx, query, userID).Scan(&token.Token, &token.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &token, nil
}

// DeleteVerificationToken 删除验证令牌
func (s *Store) DeleteVerificationToken(ctx context.Context, userID string) error {
	return s.deleteToken(ctx, "verification_tokens", userID)
}

// StoreResetToken 存储重置令牌
func (s *Store) StoreResetToken(ctx context.Context, userID string, token string, expiresAt time.Time) error {
	return s.storeToken(ctx, "reset_tokens", userID, token, expiresAt)
}

// GetResetToken 获取重置令牌
//
// 安全设计（T2）：返回的 Token 字段为令牌的 SHA-256 哈希值（64 位 hex），
// 非明文——调用方比对前须先对输入令牌计算 common.HashToken
func (s *Store) GetResetToken(ctx context.Context, userID string) (*store.ResetToken, error) {
	query := `SELECT token, expires_at, used_at FROM reset_tokens WHERE user_id = $1`
	var token store.ResetToken
	err := s.db.QueryRowContext(ctx, query, userID).Scan(&token.Token, &token.ExpiresAt, &token.UsedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &token, nil
}

// MarkResetTokenUsed 标记重置令牌为已使用
func (s *Store) MarkResetTokenUsed(ctx context.Context, userID string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `UPDATE reset_tokens SET used_at = $1 WHERE user_id = $2 AND used_at IS NULL`
	result, err := s.db.ExecContext(ctx, query, time.Now(), userID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return store.ErrNotFound
	}

	return nil
}

// DeleteResetToken 删除重置令牌
func (s *Store) DeleteResetToken(ctx context.Context, userID string) error {
	return s.deleteToken(ctx, "reset_tokens", userID)
}
