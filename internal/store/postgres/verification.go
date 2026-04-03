// Package postgres PostgreSQL 验证令牌存储实现
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/your-org/sso/internal/store"
)

// ============================================================================
// 验证令牌存储实现
// ============================================================================

// storeToken 通用令牌存储函数
// 先删除旧令牌，再插入新令牌
func (s *Store) storeToken(ctx context.Context, tableName, userID, token string, expiresAt time.Time) error {
	// 先删除旧令牌
	_, _ = s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, tableName), userID)

	// 插入新令牌
	query := fmt.Sprintf(`INSERT INTO %s (user_id, token, expires_at, created_at) VALUES ($1, $2, $3, $4)`, tableName) // #nosec G201 -- 表名来自内部配置常量，不是用户输入
	_, err := s.db.ExecContext(ctx, query, userID, token, expiresAt, time.Now())
	return err
}

// deleteToken 通用令牌删除函数
func (s *Store) deleteToken(ctx context.Context, tableName, userID string) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, tableName), userID)
	return err
}

// StoreVerificationToken 存储验证令牌
func (s *Store) StoreVerificationToken(ctx context.Context, userID string, token string, expiresAt time.Time) error {
	return s.storeToken(ctx, "verification_tokens", userID, token, expiresAt)
}

// GetVerificationToken 获取验证令牌
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
func (s *Store) GetResetToken(ctx context.Context, userID string) (*store.ResetToken, error) {
	query := `SELECT token, expires_at FROM reset_tokens WHERE user_id = $1`
	var token store.ResetToken
	err := s.db.QueryRowContext(ctx, query, userID).Scan(&token.Token, &token.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return &token, nil
}

// DeleteResetToken 删除重置令牌
func (s *Store) DeleteResetToken(ctx context.Context, userID string) error {
	return s.deleteToken(ctx, "reset_tokens", userID)
}
