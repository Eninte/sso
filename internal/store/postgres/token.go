// Package postgres PostgreSQL Token存储实现
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
)

// ============================================================================
// Token存储实现
// ============================================================================

// StoreAuthorizationCode 存储授权码
func (s *Store) StoreAuthorizationCode(ctx context.Context, code *model.AuthorizationCode) error {
	query := `
		INSERT INTO authorization_codes (code, client_id, user_id, redirect_uri, scopes, code_challenge, code_challenge_method, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := s.db.ExecContext(ctx, query,
		code.Code,
		code.ClientID,
		code.UserID,
		code.RedirectURI,
		pq.Array(code.Scopes),
		code.CodeChallenge,
		code.CodeChallengeMethod,
		code.ExpiresAt,
		code.CreatedAt,
	)
	return err
}

// GetAuthorizationCode 获取授权码
func (s *Store) GetAuthorizationCode(ctx context.Context, code string) (*model.AuthorizationCode, error) {
	query := `
		SELECT code, client_id, user_id, redirect_uri, scopes, code_challenge, code_challenge_method, expires_at, created_at, used_at
		FROM authorization_codes
		WHERE code = $1
	`

	authCode := &model.AuthorizationCode{}
	err := s.db.QueryRowContext(ctx, query, code).Scan(
		&authCode.Code,
		&authCode.ClientID,
		&authCode.UserID,
		&authCode.RedirectURI,
		pq.Array(&authCode.Scopes),
		&authCode.CodeChallenge,
		&authCode.CodeChallengeMethod,
		&authCode.ExpiresAt,
		&authCode.CreatedAt,
		&authCode.UsedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}

	return authCode, nil
}

// MarkAuthorizationCodeUsed 标记授权码已使用
func (s *Store) MarkAuthorizationCodeUsed(ctx context.Context, code string) error {
	query := `UPDATE authorization_codes SET used_at = $2 WHERE code = $1`
	_, err := s.db.ExecContext(ctx, query, code, time.Now())
	return err
}

// UpdateAuthorizationCode 更新授权码
func (s *Store) UpdateAuthorizationCode(ctx context.Context, code *model.AuthorizationCode) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		UPDATE authorization_codes 
		SET used_at = $1
		WHERE code = $2`

	_, err := s.db.ExecContext(ctx, query, code.UsedAt, code.Code)
	return err
}

// StoreToken 存储Token记录
func (s *Store) StoreToken(ctx context.Context, token *model.Token) error {
	query := `
		INSERT INTO tokens (id, access_token, refresh_token, user_id, client_id, scopes, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := s.db.ExecContext(ctx, query,
		token.ID,
		token.AccessToken,
		token.RefreshToken,
		token.UserID,
		token.ClientID,
		pq.Array(token.Scopes),
		token.ExpiresAt,
		token.CreatedAt,
	)
	return err
}

// GetTokenByRefreshToken 根据刷新令牌获取Token记录
func (s *Store) GetTokenByRefreshToken(ctx context.Context, refreshToken string) (*model.Token, error) {
	return s.getTokenByField(ctx, "refresh_token", refreshToken)
}

// GetTokenByAccessToken 根据访问令牌获取Token记录
func (s *Store) GetTokenByAccessToken(ctx context.Context, accessToken string) (*model.Token, error) {
	return s.getTokenByField(ctx, "access_token", accessToken)
}

// getTokenByField 通用Token查询函数
func (s *Store) getTokenByField(ctx context.Context, field, value string) (*model.Token, error) {
	if !allowedTokenFields[field] {
		return nil, fmt.Errorf("%w: %s", ErrInvalidFieldName, field)
	}

	query := `
		SELECT id, access_token, refresh_token, user_id, client_id, scopes, expires_at, created_at, revoked_at
		FROM tokens
		WHERE ` + field + ` = $1`

	token := &model.Token{}
	err := s.db.QueryRowContext(ctx, query, value).Scan(
		&token.ID,
		&token.AccessToken,
		&token.RefreshToken,
		&token.UserID,
		&token.ClientID,
		pq.Array(&token.Scopes),
		&token.ExpiresAt,
		&token.CreatedAt,
		&token.RevokedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}

	return token, nil
}

// RevokeToken 撤销Token
func (s *Store) RevokeToken(ctx context.Context, accessToken string) error {
	query := `UPDATE tokens SET revoked_at = $2 WHERE access_token = $1`
	_, err := s.db.ExecContext(ctx, query, accessToken, time.Now())
	return err
}

// RevokeAllUserTokens 撤销用户所有Token
func (s *Store) RevokeAllUserTokens(ctx context.Context, userID string) error {
	query := `UPDATE tokens SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`
	_, err := s.db.ExecContext(ctx, query, userID, time.Now())
	return err
}

// CleanupExpired 清理过期的Token和授权码
// 使用分批删除避免长时间锁表和大量WAL日志
func (s *Store) CleanupExpired(ctx context.Context) error {
	now := time.Now()

	// 分批清理过期的Token
	if err := s.cleanupExpiredBatch(ctx, "tokens", now); err != nil {
		return fmt.Errorf("清理过期Token失败: %w", err)
	}

	// 分批清理过期的授权码
	if err := s.cleanupExpiredBatch(ctx, "authorization_codes", now); err != nil {
		return fmt.Errorf("清理过期授权码失败: %w", err)
	}

	// 分批清理过期的验证令牌
	if err := s.cleanupExpiredBatch(ctx, "verification_tokens", now); err != nil {
		return fmt.Errorf("清理过期验证令牌失败: %w", err)
	}

	// 分批清理过期的重置令牌
	if err := s.cleanupExpiredBatch(ctx, "reset_tokens", now); err != nil {
		return fmt.Errorf("清理过期重置令牌失败: %w", err)
	}

	return nil
}

// cleanupExpiredBatch 分批清理指定表中的过期数据
// 每次删除最多CleanupBatchSize条记录，避免长时间锁表
func (s *Store) cleanupExpiredBatch(ctx context.Context, tableName string, before time.Time) error {
	// 安全校验：仅允许白名单中的表名，防止SQL注入
	if !allowedCleanupTables[tableName] {
		return fmt.Errorf("不允许清理的表: %s", tableName)
	}

	// 获取表对应的主键列名
	pkColumn, ok := getPrimaryKeyColumn(tableName)
	if !ok {
		return fmt.Errorf("表 %s 缺少主键列名配置", tableName)
	}

	// #nosec G201 -- 表名来自内部配置常量，不是用户输入
	query := fmt.Sprintf(`
		DELETE FROM %s 
		WHERE %s IN (
			SELECT %s FROM %s 
			WHERE expires_at < $1 
			LIMIT $2
		)
	`, tableName, pkColumn, pkColumn, tableName)

	totalDeleted := 0
	for {
		result, err := s.db.ExecContext(ctx, query, before, CleanupBatchSize)
		if err != nil {
			return err
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}

		totalDeleted += int(affected)

		// 如果删除的记录数小于批量大小，说明已经没有更多过期记录
		if affected < CleanupBatchSize {
			break
		}

		// 避免过度占用数据库资源，短暂休息
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}

	_ = totalDeleted // 可以用于日志记录
	return nil
}
