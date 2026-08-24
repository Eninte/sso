// Package postgres PostgreSQL OAuth客户端存储实现
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
)

// ============================================================================
// 客户端存储实现
// ============================================================================

// GetByClientID 根据客户端ID获取客户端
func (s *Store) GetByClientID(ctx context.Context, clientID string) (*model.Client, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT `+clientSelectColumns+`
		FROM oauth_clients
		WHERE client_id = $1
	`, clientID)

	client, err := scanClient(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}

	return client, nil
}

// CreateClient 创建新客户端
func (s *Store) CreateClient(ctx context.Context, client *model.Client) error {
	query := `
		INSERT INTO oauth_clients (id, client_id, client_secret, name, redirect_uris, grant_types, scopes, public_client, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := s.db.ExecContext(ctx, query,
		client.ID,
		client.ClientID,
		client.ClientSecret,
		client.Name,
		client.RedirectURIs,
		client.GrantTypes,
		client.Scopes,
		client.PublicClient,
		client.IsActive,
		client.CreatedAt,
	)
	return err
}

// ValidateRedirectURI 验证重定向URI是否在允许列表中
// 优化：使用EXISTS子查询，避免加载整个客户端对象
func (s *Store) ValidateRedirectURI(ctx context.Context, clientID string, redirectURI string) bool {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `SELECT EXISTS(SELECT 1 FROM oauth_clients WHERE client_id = $1 AND $2 = ANY(redirect_uris))`
	var exists bool
	err := s.db.QueryRowContext(ctx, query, clientID, redirectURI).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

// ============================================================================
// 客户端管理（管理员 API）
// ============================================================================

const clientSelectColumns = `id, client_id, client_secret, name, redirect_uris, grant_types, scopes, public_client, is_active, created_at, updated_at`

// scanClient 扫描一行客户端记录到 model.Client
// client_secret 允许 NULL（公共客户端无密钥），NULL 映射为空字符串
func scanClient(scanner interface {
	Scan(dest ...interface{}) error
}) (*model.Client, error) {
	client := &model.Client{}
	var secret sql.NullString
	err := scanner.Scan(
		&client.ID,
		&client.ClientID,
		&secret,
		&client.Name,
		scanTextArray(&client.RedirectURIs),
		scanTextArray(&client.GrantTypes),
		scanTextArray(&client.Scopes),
		&client.PublicClient,
		&client.IsActive,
		&client.CreatedAt,
		&client.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	client.ClientSecret = secret.String
	return client, nil
}

// ListClients 分页列出客户端（按创建时间倒序）
func (s *Store) ListClients(ctx context.Context, offset, limit int) ([]*model.Client, int, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+clientSelectColumns+`
		FROM oauth_clients
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	clients := make([]*model.Client, 0)
	for rows.Next() {
		client, err := scanClient(rows)
		if err != nil {
			return nil, 0, err
		}
		clients = append(clients, client)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM oauth_clients`).Scan(&total); err != nil {
		return nil, 0, err
	}

	return clients, total, nil
}

// UpdateClient 更新客户端配置（name/redirect_uris/grant_types/scopes）
func (s *Store) UpdateClient(ctx context.Context, client *model.Client) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	result, err := s.db.ExecContext(ctx, `
		UPDATE oauth_clients
		SET name = $2, redirect_uris = $3, grant_types = $4, scopes = $5, updated_at = NOW()
		WHERE client_id = $1
	`, client.ClientID, client.Name, client.RedirectURIs, client.GrantTypes, client.Scopes)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return store.ErrNotFound
	}
	return nil
}

// RotateClientSecret 轮换客户端密钥（存储 bcrypt 哈希）
func (s *Store) RotateClientSecret(ctx context.Context, clientID, secretHash string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	result, err := s.db.ExecContext(ctx, `
		UPDATE oauth_clients
		SET client_secret = $2, updated_at = NOW()
		WHERE client_id = $1
	`, clientID, secretHash)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return store.ErrNotFound
	}
	return nil
}

// SetClientActive 设置客户端启用/禁用状态
func (s *Store) SetClientActive(ctx context.Context, clientID string, active bool) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	result, err := s.db.ExecContext(ctx, `
		UPDATE oauth_clients
		SET is_active = $2, updated_at = NOW()
		WHERE client_id = $1
	`, clientID, active)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return store.ErrNotFound
	}
	return nil
}

// RevokeAllClientTokens 撤销客户端全部未撤销 Token
func (s *Store) RevokeAllClientTokens(ctx context.Context, clientID string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE tokens
		SET revoked_at = NOW()
		WHERE client_id = $1 AND revoked_at IS NULL
	`, clientID)
	return err
}

// DeleteClient 删除客户端（单事务）
//
// 顺序（与 021 迁移的 FK 语义配合）：
//  1. 撤销该客户端全部未撤销 Token
//  2. 删除其授权码（显式删除，FK CASCADE 兜底）
//  3. 删除客户端行；RowsAffected==0 回滚并返回 store.ErrNotFound
//
// 事务保证：任一步失败整体回滚，不会出现"客户端已删但 Token 未撤销"的中间态
func (s *Store) DeleteClient(ctx context.Context, clientID string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 撤销该客户端全部未撤销 Token
	if _, err := tx.ExecContext(ctx, `
		UPDATE tokens
		SET revoked_at = NOW()
		WHERE client_id = $1 AND revoked_at IS NULL
	`, clientID); err != nil {
		return fmt.Errorf("revoke client tokens failed: %w", err)
	}

	// 2. 删除授权码（FK CASCADE 兜底，显式删除保持语义清晰）
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM authorization_codes WHERE client_id = $1
	`, clientID); err != nil {
		return fmt.Errorf("delete authorization codes failed: %w", err)
	}

	// 3. 删除客户端行
	result, err := tx.ExecContext(ctx, `
		DELETE FROM oauth_clients WHERE client_id = $1
	`, clientID)
	if err != nil {
		return fmt.Errorf("delete client failed: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get affected rows failed: %w", err)
	}
	if rows == 0 {
		return store.ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete client transaction failed: %w", err)
	}
	return nil
}
