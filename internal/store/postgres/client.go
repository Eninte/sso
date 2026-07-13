// Package postgres PostgreSQL OAuth客户端存储实现
package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
)

// ============================================================================
// 客户端存储实现
// ============================================================================

// GetByClientID 根据客户端ID获取客户端
func (s *Store) GetByClientID(ctx context.Context, clientID string) (*model.Client, error) {
	query := `
		SELECT id, client_id, client_secret, name, redirect_uris, grant_types, scopes, public_client, created_at
		FROM oauth_clients
		WHERE client_id = $1
	`

	client := &model.Client{}
	err := s.db.QueryRowContext(ctx, query, clientID).Scan(
		&client.ID,
		&client.ClientID,
		&client.ClientSecret,
		&client.Name,
		scanTextArray(&client.RedirectURIs),
		scanTextArray(&client.GrantTypes),
		scanTextArray(&client.Scopes),
		&client.PublicClient,
		&client.CreatedAt,
	)

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
		INSERT INTO oauth_clients (id, client_id, client_secret, name, redirect_uris, grant_types, scopes, public_client, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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
