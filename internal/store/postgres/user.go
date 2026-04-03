// Package postgres PostgreSQL用户存储实现
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
// 用户存储实现
// ============================================================================

// Create 创建新用户
func (s *Store) Create(ctx context.Context, user *model.User) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		INSERT INTO users (id, email, password_hash, email_verified, mfa_enabled, role, status, login_attempts, locked_until, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := s.db.ExecContext(ctx, query,
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
		// 检查是否为唯一约束冲突
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return store.ErrDuplicateEmail
		}
		return err
	}

	return nil
}

// GetByID 根据ID获取用户
func (s *Store) GetByID(ctx context.Context, id string) (*model.User, error) {
	return s.getUserByField(ctx, "id", id)
}

// GetByEmail 根据邮箱获取用户
func (s *Store) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	return s.getUserByField(ctx, "email", email)
}

// getUserByField 通用用户查询函数
func (s *Store) getUserByField(ctx context.Context, field, value string) (*model.User, error) {
	// 验证字段名是否在白名单中
	if !allowedUserFields[field] {
		return nil, fmt.Errorf("%w: %s", ErrInvalidFieldName, field)
	}

	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		SELECT id, email, password_hash, email_verified, mfa_enabled, mfa_secret, 
		       role, status, login_attempts, locked_until, created_at, updated_at
		FROM users
		WHERE ` + field + ` = $1`

	user, err := scanUser(s.db.QueryRowContext(ctx, query, value))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}

	return user, nil
}

// Update 更新用户信息
func (s *Store) Update(ctx context.Context, user *model.User) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		UPDATE users
		SET email = $2, password_hash = $3, email_verified = $4, mfa_enabled = $5,
		    mfa_secret = $6, role = $7, status = $8, login_attempts = $9, locked_until = $10, updated_at = $11
		WHERE id = $1
	`
	_, err := s.db.ExecContext(ctx, query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.EmailVerified,
		user.MFAEnabled,
		user.MFASecret,
		user.Role,
		user.Status,
		user.LoginAttempts,
		user.LockedUntil,
		time.Now(),
	)
	return err
}

// UpdateLoginAttempts 更新登录尝试次数
func (s *Store) UpdateLoginAttempts(ctx context.Context, userID string, attempts int, lockedUntil *time.Time) error {
	query := `
		UPDATE users
		SET login_attempts = $2, locked_until = $3, updated_at = $4
		WHERE id = $1
	`
	_, err := s.db.ExecContext(ctx, query, userID, attempts, lockedUntil, time.Now())
	return err
}

// IncrementLoginAttempts 原子递增登录尝试次数
// 使用数据库原子操作避免竞态条件
func (s *Store) IncrementLoginAttempts(ctx context.Context, userID string, maxAttempts int, lockoutDuration time.Duration) (attempts int, locked bool, lockedUntil *time.Time, err error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		UPDATE users
		SET login_attempts = login_attempts + 1,
			locked_until = CASE
				WHEN login_attempts + 1 >= $2 THEN NOW() + $3::INTERVAL
				ELSE locked_until
			END,
			status = CASE
				WHEN login_attempts + 1 >= $2 THEN 'locked'
				ELSE status
			END,
			updated_at = NOW()
		WHERE id = $1
		RETURNING login_attempts, status, locked_until
	`

	var status string
	err = s.db.QueryRowContext(ctx, query, userID, maxAttempts, lockoutDuration.String()).Scan(&attempts, &status, &lockedUntil)
	if err != nil {
		return 0, false, nil, err
	}

	locked = status == "locked"
	return attempts, locked, lockedUntil, nil
}

// ResetLoginAttempts 重置登录尝试次数
func (s *Store) ResetLoginAttempts(ctx context.Context, userID string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		UPDATE users
		SET login_attempts = 0,
			locked_until = NULL,
			status = CASE
				WHEN status = 'locked' THEN 'active'
				ELSE status
			END,
			updated_at = NOW()
		WHERE id = $1
	`
	_, err := s.db.ExecContext(ctx, query, userID)
	return err
}

// UnlockExpiredAccount 解锁已过期的锁定账户
// 仅当locked_until < NOW()时才解锁，避免竞态条件
func (s *Store) UnlockExpiredAccount(ctx context.Context, userID string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		UPDATE users
		SET login_attempts = 0,
			status = 'active',
			updated_at = NOW()
		WHERE id = $1
			AND status = 'locked'
			AND locked_until IS NOT NULL
			AND locked_until < NOW()
	`
	result, err := s.db.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	// 如果没有行被更新，说明账户未锁定或锁定时间未过期
	if rows == 0 {
		return store.ErrNotFound
	}

	return nil
}

// Delete 删除用户
func (s *Store) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM users WHERE id = $1`
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

// ListUsers 列出用户（支持分页）
func (s *Store) ListUsers(ctx context.Context, offset, limit int) ([]*model.User, int, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 获取总数
	var total int
	countQuery := `SELECT COUNT(*) FROM users`
	if err := s.db.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	query := `
		SELECT id, email, password_hash, email_verified, mfa_enabled, mfa_secret,
		       role, status, login_attempts, locked_until, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	// 预分配slice容量以减少内存重新分配
	users := make([]*model.User, 0, limit)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}
