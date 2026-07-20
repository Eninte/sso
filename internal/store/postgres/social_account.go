// Package postgres 社交账号身份存储实现（阶段 2.3）
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
)

// ============================================================================
// 社交账号身份存储实现
// ============================================================================

// CreateSocialAccount 创建社交账号绑定记录
//
// 唯一约束：
//   - (provider, provider_user_id) — 同一社交账号不能绑定到多个用户
//   - (user_id, provider) — 同一用户在同一 provider 下不能绑定多个账号
//
// 冲突时返回 store.ErrSocialAccountConflict
func (s *Store) CreateSocialAccount(ctx context.Context, account *model.SocialAccount) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	metadataJSON, _ := json.Marshal(account.ProviderMetadata)

	query := `
		INSERT INTO social_accounts
			(id, provider, provider_user_id, user_id, provider_email, email_verified, provider_metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := s.db.ExecContext(ctx, query,
		account.ID,
		account.Provider,
		account.ProviderUserID,
		account.UserID,
		account.ProviderEmail,
		account.EmailVerified,
		metadataJSON,
		account.CreatedAt,
		account.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return store.ErrSocialAccountConflict
		}
		return err
	}
	return nil
}

// GetSocialAccount 通过 (provider, provider_user_id) 查找社交账号
// 不存在返回 store.ErrNotFound
func (s *Store) GetSocialAccount(ctx context.Context, provider, providerUserID string) (*model.SocialAccount, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		SELECT id, provider, provider_user_id, user_id, provider_email, email_verified, provider_metadata, created_at, updated_at
		FROM social_accounts
		WHERE provider = $1 AND provider_user_id = $2
	`
	var account model.SocialAccount
	var metadataJSON []byte
	var providerEmail sql.NullString

	err := s.db.QueryRowContext(ctx, query, provider, providerUserID).Scan(
		&account.ID,
		&account.Provider,
		&account.ProviderUserID,
		&account.UserID,
		&providerEmail,
		&account.EmailVerified,
		&metadataJSON,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		// 检查 pgx 的 ErrNoRows 等价错误
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, fmt.Errorf("query social account: %w", err)
		}
		return nil, fmt.Errorf("query social account: %w", err)
	}

	if providerEmail.Valid {
		account.ProviderEmail = providerEmail.String
	}
	account.ProviderMetadata = model.ProviderMetadataFromJSON(metadataJSON)

	return &account, nil
}

// ListSocialAccountsByUserID 列出用户绑定的所有社交账号
func (s *Store) ListSocialAccountsByUserID(ctx context.Context, userID string) ([]*model.SocialAccount, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		SELECT id, provider, provider_user_id, user_id, provider_email, email_verified, provider_metadata, created_at, updated_at
		FROM social_accounts
		WHERE user_id = $1
		ORDER BY created_at ASC
	`
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list social accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*model.SocialAccount
	for rows.Next() {
		var account model.SocialAccount
		var metadataJSON []byte
		var providerEmail sql.NullString

		if err := rows.Scan(
			&account.ID,
			&account.Provider,
			&account.ProviderUserID,
			&account.UserID,
			&providerEmail,
			&account.EmailVerified,
			&metadataJSON,
			&account.CreatedAt,
			&account.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan social account: %w", err)
		}
		if providerEmail.Valid {
			account.ProviderEmail = providerEmail.String
		}
		account.ProviderMetadata = model.ProviderMetadataFromJSON(metadataJSON)
		accounts = append(accounts, &account)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate social accounts: %w", err)
	}

	return accounts, nil
}

// DeleteSocialAccount 解绑社交账号
func (s *Store) DeleteSocialAccount(ctx context.Context, provider, providerUserID string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `DELETE FROM social_accounts WHERE provider = $1 AND provider_user_id = $2`
	result, err := s.db.ExecContext(ctx, query, provider, providerUserID)
	if err != nil {
		return fmt.Errorf("delete social account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return store.ErrNotFound
	}

	return nil
}

// UpdateSocialAccount 更新社交账号绑定信息
//
// 阶段 D 修复（L2）：原 updateSocialAccountIfNeeded 仅修改内存对象未持久化
//
// 仅更新以下字段：
//   - provider_email
//   - email_verified
//   - provider_metadata
//   - updated_at
//
// 不修改 user_id 关联，防止通过修改 provider 端 email 接管其他用户账号
// 通过 (provider, provider_user_id) 定位记录，不存在返回 ErrNotFound
func (s *Store) UpdateSocialAccount(ctx context.Context, account *model.SocialAccount) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	metadataJSON, _ := json.Marshal(account.ProviderMetadata)

	query := `
		UPDATE social_accounts
		SET provider_email = $3,
		    email_verified = $4,
		    provider_metadata = $5,
		    updated_at = $6
		WHERE provider = $1 AND provider_user_id = $2
	`
	result, err := s.db.ExecContext(ctx, query,
		account.Provider,
		account.ProviderUserID,
		account.ProviderEmail,
		account.EmailVerified,
		metadataJSON,
		account.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update social account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return store.ErrNotFound
	}

	return nil
}

// CreateSocialAccountAtomic 原子地创建用户 + 社交账号绑定
//
// 在单个事务中执行：
//  1. INSERT users
//  2. INSERT social_accounts
//
// 若 (provider, provider_user_id) 已存在则返回 store.ErrSocialAccountConflict
// 若 email 已存在则返回 store.ErrDuplicateEmail
func (s *Store) CreateSocialAccountAtomic(ctx context.Context, user *model.User, account *model.SocialAccount) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// 1. 插入用户
	userQuery := `
		INSERT INTO users (id, email, password_hash, email_verified, mfa_enabled, role, status, login_attempts, locked_until, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err = tx.ExecContext(ctx, userQuery,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.EmailVerified,
		user.MFAEnabled,
		user.Role,
		user.Status,
		user.LoginAttempts,
		user.LockedUntil,
		user.CreatedAt,
		user.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return store.ErrDuplicateEmail
		}
		return fmt.Errorf("insert user: %w", err)
	}

	// 2. 插入社交账号
	metadataJSON, _ := json.Marshal(account.ProviderMetadata)
	socialQuery := `
		INSERT INTO social_accounts
			(id, provider, provider_user_id, user_id, provider_email, email_verified, provider_metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err = tx.ExecContext(ctx, socialQuery,
		account.ID,
		account.Provider,
		account.ProviderUserID,
		account.UserID,
		account.ProviderEmail,
		account.EmailVerified,
		metadataJSON,
		account.CreatedAt,
		account.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return store.ErrSocialAccountConflict
		}
		return fmt.Errorf("insert social account: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// 确保 time 包被使用（防止某些 lint 误报）
var _ = time.Now
