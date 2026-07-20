// Package postgres PostgreSQL用户存储实现
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
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
		if isUniqueViolation(err) {
			return store.ErrDuplicateEmail
		}
		return err
	}

	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
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

// ExistsUserByRole 检查是否存在指定角色的用户
// 使用 EXISTS 子查询，仅扫描索引即可返回结果，避免全表数据加载
func (s *Store) ExistsUserByRole(ctx context.Context, role string) (bool, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE role = $1 LIMIT 1)`
	err := s.db.QueryRowContext(ctx, query, role).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// CreateAdminAtomic 原子地创建初始管理员账户
//
// 流程：
//  1. 开启事务
//  2. 获取事务级 advisory lock（pg_try_advisory_xact_lock）
//     - 锁 ID 由 hashtext('sso_init_admin') 计算，全局唯一
//     - 锁在事务提交/回滚后自动释放，无需显式释放
//  3. 再次检查管理员是否已存在（防御性）
//  4. 插入管理员用户
//  5. 提交事务
//
// 并发行为：
//   - 多个并发请求同时调用：第一个获取锁的事务执行插入并提交；
//     其余获取锁失败，立即返回 ErrForbidden（不阻塞）
//   - 锁获取失败时不会查询数据库，避免无效的连接占用
//
// 安全性：
//   - 解决传统 AdminExists + Create 的 TOCTOU 竞态
//   - 即使攻击者绕过应用层 AdminExists 检查，数据库层仍会拒绝
//   - 邮箱重复由 users.email 唯一约束保证，返回 ErrDuplicateEmail
func (s *Store) CreateAdminAtomic(ctx context.Context, user *model.User) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	// defer Rollback 确保 panic 或提前返回时事务回滚
	// 显式 Commit 后 Rollback 是 no-op（sql.Tx 已处理）
	defer func() { _ = tx.Rollback() }()

	// 尝试获取事务级 advisory lock（非阻塞）
	// hashtext 返回 int4，pg_try_advisory_xact_lock 接受 int4 参数
	var lockAcquired bool
	err = tx.QueryRowContext(ctx,
		"SELECT pg_try_advisory_xact_lock(hashtext('sso_init_admin'))").Scan(&lockAcquired)
	if err != nil {
		return fmt.Errorf("acquire advisory lock failed: %w", err)
	}
	if !lockAcquired {
		// 并发创建：另一个事务正在创建管理员
		return store.ErrForbidden
	}

	// 在锁内再次检查管理员是否已存在
	// 即使创建者绕过 service 层的 AdminExists 检查，DB 层仍拒绝
	var exists bool
	err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE role = $1 LIMIT 1)",
		model.UserRoleAdmin,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check admin exists failed: %w", err)
	}
	if exists {
		return store.ErrForbidden
	}

	// 插入管理员用户
	query := `
		INSERT INTO users (id, email, password_hash, email_verified, mfa_enabled, role, status, login_attempts, locked_until, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err = tx.ExecContext(ctx, query,
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
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction failed: %w", err)
	}

	return nil
}
