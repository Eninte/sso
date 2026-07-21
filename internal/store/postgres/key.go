// Package postgres PostgreSQL 密钥版本存储实现
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
)

// ============================================================================
// 密钥版本存储实现
// ============================================================================

// StoreKey 存储密钥版本
func (s *Store) StoreKey(ctx context.Context, key *model.KeyVersion) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		INSERT INTO key_versions (id, public_key, private_key, status, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := s.db.ExecContext(ctx, query,
		key.ID,
		key.PublicKey,
		key.PrivateKey,
		key.Status,
		key.CreatedAt,
		key.ExpiresAt,
	)
	return err
}

// GetActiveKey 获取当前活跃的密钥
func (s *Store) GetActiveKey(ctx context.Context) (*model.KeyVersion, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		SELECT id, public_key, private_key, status, created_at, expires_at
		FROM key_versions
		WHERE status = 'active'
		ORDER BY created_at DESC
		LIMIT 1
	`

	key := &model.KeyVersion{}
	err := s.db.QueryRowContext(ctx, query).Scan(
		&key.ID,
		&key.PublicKey,
		&key.PrivateKey,
		&key.Status,
		&key.CreatedAt,
		&key.ExpiresAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}

	return key, nil
}

// GetKeyByID 根据ID获取密钥
func (s *Store) GetKeyByID(ctx context.Context, keyID string) (*model.KeyVersion, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		SELECT id, public_key, private_key, status, created_at, expires_at
		FROM key_versions
		WHERE id = $1
	`

	key := &model.KeyVersion{}
	err := s.db.QueryRowContext(ctx, query, keyID).Scan(
		&key.ID,
		&key.PublicKey,
		&key.PrivateKey,
		&key.Status,
		&key.CreatedAt,
		&key.ExpiresAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}

	return key, nil
}

// scanKeyVersions 扫描密钥版本结果集
// 提取公共逻辑，避免代码重复
func (s *Store) scanKeyVersions(rows *sql.Rows) ([]*model.KeyVersion, error) {
	keys := make([]*model.KeyVersion, 0)
	for rows.Next() {
		key := &model.KeyVersion{}
		err := rows.Scan(
			&key.ID,
			&key.PublicKey,
			&key.PrivateKey,
			&key.Status,
			&key.CreatedAt,
			&key.ExpiresAt,
		)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return keys, nil
}

// ListActiveKeys 列出活跃的密钥（包括已弃用的）
func (s *Store) ListActiveKeys(ctx context.Context) ([]*model.KeyVersion, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		SELECT id, public_key, private_key, status, created_at, expires_at
		FROM key_versions
		WHERE status IN ('active', 'deprecated')
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanKeyVersions(rows)
}

// ListAllKeys 列出所有密钥
func (s *Store) ListAllKeys(ctx context.Context) ([]*model.KeyVersion, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		SELECT id, public_key, private_key, status, created_at, expires_at
		FROM key_versions
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanKeyVersions(rows)
}

// DeprecateKey 弃用密钥
func (s *Store) DeprecateKey(ctx context.Context, keyID string, expiresAt time.Time) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `UPDATE key_versions SET status = 'deprecated', expires_at = $2 WHERE id = $1`
	result, err := s.db.ExecContext(ctx, query, keyID, expiresAt)
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

// RevokeKey 撤销密钥
func (s *Store) RevokeKey(ctx context.Context, keyID string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `UPDATE key_versions SET status = 'revoked' WHERE id = $1`
	result, err := s.db.ExecContext(ctx, query, keyID)
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

// DeleteKey 删除密钥
func (s *Store) DeleteKey(ctx context.Context, keyID string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `DELETE FROM key_versions WHERE id = $1`
	result, err := s.db.ExecContext(ctx, query, keyID)
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

// UpdateKeyPrivateKey 更新密钥的私钥字段
// T7：懒加密回写密文时使用；store 层不做加解密，仅原样写入
func (s *Store) UpdateKeyPrivateKey(ctx context.Context, keyID string, privateKey []byte) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `UPDATE key_versions SET private_key = $2 WHERE id = $1`
	result, err := s.db.ExecContext(ctx, query, keyID, privateKey)
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
