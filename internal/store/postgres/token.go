// Package postgres PostgreSQL Token存储实现
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/example/sso/internal/common"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
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
		code.Scopes,
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
		scanTextArray(&authCode.Scopes),
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

// UpdateAuthorizationCode 原子地标记授权码为已使用
// 通过 WHERE used_at IS NULL 条件保证并发安全，防止 TOCTOU 重放攻击
// 如果授权码已被使用（并发竞争或重复兑换），返回 ErrAuthorizationCodeUsed
func (s *Store) UpdateAuthorizationCode(ctx context.Context, code *model.AuthorizationCode) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		UPDATE authorization_codes
		SET used_at = $1
		WHERE code = $2 AND used_at IS NULL`

	result, err := s.db.ExecContext(ctx, query, code.UsedAt, code.Code)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		// 授权码不存在或已被使用（并发竞争）
		return store.ErrAuthorizationCodeUsed
	}

	return nil
}

// StoreToken 存储Token记录
//
// 安全设计（T1）：仅存 hash，明文不落库
//   - access_token / refresh_token 明文列始终写入 NULL
//   - 查询仅通过 access_token_hash / refresh_token_hash（SHA-256 hex）
//   - 明文只在签发时由 service 层持有并返回给客户端
func (s *Store) StoreToken(ctx context.Context, token *model.Token) error {
	// 计算 hash（若调用方未设置）
	if token.AccessTokenHash == "" && token.AccessToken != "" {
		token.AccessTokenHash = common.HashToken(token.AccessToken)
	}
	if token.RefreshTokenHash == "" && token.RefreshToken != "" {
		token.RefreshTokenHash = common.HashToken(token.RefreshToken)
	}

	// 明文列不写入（迁移 019 起两列允许 NULL）
	query := `
		INSERT INTO tokens (id, user_id, client_id, scopes, expires_at, created_at, refresh_expires_at, access_token_hash, refresh_token_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := s.db.ExecContext(ctx, query,
		token.ID,
		token.UserID,
		token.ClientID,
		token.Scopes,
		token.ExpiresAt,
		token.CreatedAt,
		token.RefreshExpiresAt,
		token.AccessTokenHash,
		token.RefreshTokenHash,
	)
	return err
}

// GetTokenByRefreshToken 根据刷新令牌获取Token记录
//
// 安全设计（T1）：仅通过 refresh_token_hash 查询，明文不落库也不出现在 WHERE 中；
// hash 未命中直接返回 store.ErrNotFound（无明文回退）
func (s *Store) GetTokenByRefreshToken(ctx context.Context, refreshToken string) (*model.Token, error) {
	hash := common.HashToken(refreshToken)
	return s.getTokenByField(ctx, "refresh_token_hash", hash)
}

// GetTokenByAccessToken 根据访问令牌获取Token记录
//
// 安全设计（T1）：仅通过 access_token_hash 查询，明文不落库也不出现在 WHERE 中；
// hash 未命中直接返回 store.ErrNotFound（无明文回退）
func (s *Store) GetTokenByAccessToken(ctx context.Context, accessToken string) (*model.Token, error) {
	hash := common.HashToken(accessToken)
	return s.getTokenByField(ctx, "access_token_hash", hash)
}

// getTokenByField 通用Token查询函数
//
// 安全设计（T1）：SELECT 列表不含 access_token / refresh_token 明文列，
// 返回的 model.Token 中这两个字段保持空字符串（明文只在签发时由 service 层持有）
func (s *Store) getTokenByField(ctx context.Context, field, value string) (*model.Token, error) {
	if !allowedTokenFields[field] {
		return nil, fmt.Errorf("%w: %s", ErrInvalidFieldName, field)
	}

	query := `
		SELECT id, user_id, client_id, scopes, expires_at, created_at, revoked_at, rotated_at, replaced_by_token_id, refresh_expires_at, access_token_hash, refresh_token_hash
		FROM tokens
		WHERE ` + field + ` = $1`

	token := &model.Token{}
	var accessTokenHash, refreshTokenHash sql.NullString
	err := s.db.QueryRowContext(ctx, query, value).Scan(
		&token.ID,
		&token.UserID,
		&token.ClientID,
		scanTextArray(&token.Scopes),
		&token.ExpiresAt,
		&token.CreatedAt,
		&token.RevokedAt,
		&token.RotatedAt,
		&token.ReplacedByTokenID,
		&token.RefreshExpiresAt,
		&accessTokenHash,
		&refreshTokenHash,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}

	// 旧数据 hash 字段可能为 NULL（迁移期间兼容）
	if accessTokenHash.Valid {
		token.AccessTokenHash = accessTokenHash.String
	}
	if refreshTokenHash.Valid {
		token.RefreshTokenHash = refreshTokenHash.String
	}

	return token, nil
}

// RotateRefreshToken 原子地轮换 refresh token
//
// 在单个事务内：
//  1. UPDATE tokens SET revoked_at=NOW(), rotated_at=NOW(), replaced_by_token_id=$newID
//     WHERE refresh_token_hash=$hash AND revoked_at IS NULL AND rotated_at IS NULL
//  2. 若 RowsAffected == 0 → 返回 store.ErrTokenRotated（重放或不存在）
//  3. INSERT new token（含 hash）
//
// 安全设计：UPDATE + INSERT 在同一事务内，避免 TOCTOU 竞态
// 安全设计（T1）：仅通过 refresh_token_hash 定位，hash 未命中即视为已轮换/不存在（无明文回退）
func (s *Store) RotateRefreshToken(ctx context.Context, oldRefreshToken string, newToken *model.Token) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 原子地标记旧 token 为已轮换 + 已撤销
	// 通过 WHERE rotated_at IS NULL AND revoked_at IS NULL 保证只能轮换一次
	// 多个并发请求中只有一个会成功
	oldHash := common.HashToken(oldRefreshToken)
	updateQuery := `
		UPDATE tokens
		SET revoked_at = NOW(),
		    rotated_at = NOW(),
		    replaced_by_token_id = $2
		WHERE refresh_token_hash = $1
		  AND revoked_at IS NULL
		  AND rotated_at IS NULL`
	result, err := tx.ExecContext(ctx, updateQuery, oldHash, newToken.ID)
	if err != nil {
		return fmt.Errorf("rotate refresh token failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get affected rows failed: %w", err)
	}

	if rowsAffected == 0 {
		// token 不存在，或已被撤销/已轮换
		// 这是重放攻击的典型特征：已被轮换的 refresh token 再次出现
		return store.ErrTokenRotated
	}

	// 2. 计算新 token 的 hash（若调用方未设置）
	if newToken.AccessTokenHash == "" && newToken.AccessToken != "" {
		newToken.AccessTokenHash = common.HashToken(newToken.AccessToken)
	}
	if newToken.RefreshTokenHash == "" && newToken.RefreshToken != "" {
		newToken.RefreshTokenHash = common.HashToken(newToken.RefreshToken)
	}

	// 3. 插入新的 token 记录（仅存 hash，明文不落库）
	insertQuery := `
		INSERT INTO tokens (id, user_id, client_id, scopes, expires_at, created_at, refresh_expires_at, access_token_hash, refresh_token_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err = tx.ExecContext(ctx, insertQuery,
		newToken.ID,
		newToken.UserID,
		newToken.ClientID,
		newToken.Scopes,
		newToken.ExpiresAt,
		newToken.CreatedAt,
		newToken.RefreshExpiresAt,
		newToken.AccessTokenHash,
		newToken.RefreshTokenHash,
	)
	if err != nil {
		return fmt.Errorf("insert new token failed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rotation transaction failed: %w", err)
	}

	return nil
}

// RevokeToken 撤销Token
//
// 阶段 2.4：添加 WHERE revoked_at IS NULL 条件
//   - 避免覆盖首次撤销时间戳（审计友好）
//   - token 不存在或已撤销时不报错（与 Mock 实现对齐）
//   - 调用方需通过 AuthMiddleware 缓存失效感知撤销生效
//
// 安全设计（T1）：仅通过 access_token_hash 定位，无明文回退；
// hash 未命中（token 不存在或已撤销）时不报错（与 Mock 实现对齐）
func (s *Store) RevokeToken(ctx context.Context, accessToken string) error {
	hash := common.HashToken(accessToken)
	now := time.Now()

	query := `UPDATE tokens SET revoked_at = $2 WHERE access_token_hash = $1 AND revoked_at IS NULL`
	_, err := s.db.ExecContext(ctx, query, hash, now)
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
