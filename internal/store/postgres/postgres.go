// Package postgres PostgreSQL存储实现
// 实现store.Store接口，提供PostgreSQL数据库访问
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
// Store PostgreSQL存储实现
// ============================================================================

// 默认查询超时时间
const DefaultQueryTimeout = 10 * time.Second

// CleanupBatchSize 清理过期数据时的批量大小
// 使用分批删除避免长时间锁表和大量WAL日志
const CleanupBatchSize = 1000

// AllowedCleanupTables 允许清理操作的表名白名单
// 用于防止SQL注入攻击，仅允许预定义的安全表名
// 这些表包含有过期时间字段(expires_at)，支持安全清理
var AllowedCleanupTables = map[string]bool{
	"tokens":              true, // OAuth令牌表
	"authorization_codes": true, // OAuth授权码表
	"verification_tokens": true, // 邮箱验证令牌表
	"reset_tokens":        true, // 密码重置令牌表
}

// allowedCleanupTables 是AllowedCleanupTables的内部别名
// 保持向后兼容性
var allowedCleanupTables = AllowedCleanupTables

// cleanupTableKeys 各清理表的主键列名（只读，通过getPrimaryKeyColumn访问）
var cleanupTableKeys = map[string]string{
	"tokens":              "id",
	"authorization_codes": "code",
	"verification_tokens": "token",
	"reset_tokens":        "token",
}

// getPrimaryKeyColumn 返回指定表的主键列名
// 如果表不在映射中，返回空字符串和false
func getPrimaryKeyColumn(table string) (string, bool) {
	pk, ok := cleanupTableKeys[table]
	return pk, ok
}

// Store PostgreSQL存储实现
type Store struct {
	db      *sql.DB
	timeout time.Duration
}

// New 创建PostgreSQL存储实例
func New(db *sql.DB) *Store {
	return &Store{
		db:      db,
		timeout: DefaultQueryTimeout,
	}
}

// NewFromURL 从URL创建PostgreSQL存储实例
func NewFromURL(databaseURL string) (*Store, error) {
	return NewFromURLWithTimeout(databaseURL, DefaultQueryTimeout)
}

// NewFromURLWithTimeout 从URL创建PostgreSQL存储实例，支持自定义超时
// 注意：连接池配置由调用方通过 sql.DB 设置，此函数不覆盖
func NewFromURLWithTimeout(databaseURL string, timeout time.Duration) (*Store, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return &Store{
		db:      db,
		timeout: timeout,
	}, nil
}

// NewFromConfig 从URL和配置创建PostgreSQL存储实例
func NewFromConfig(databaseURL string, maxOpenConns, maxIdleConns int, connMaxLifetime, queryTimeout time.Duration) (*Store, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	// 使用配置参数设置连接池
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return &Store{
		db:      db,
		timeout: queryTimeout,
	}, nil
}

// withTimeout 创建带超时的上下文
func (s *Store) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	// 如果上下文已有超时，使用原有超时
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < s.timeout {
			return context.WithDeadline(ctx, deadline)
		}
	}
	return context.WithTimeout(ctx, s.timeout)
}

// Close 关闭数据库连接
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping 检查数据库连接
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

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

// ============================================================================
// 允许的查询字段白名单
// ============================================================================

// allowedUserFields 允许用于用户查询的字段白名单
var allowedUserFields = map[string]bool{
	"id":    true,
	"email": true,
}

// allowedTokenFields 允许用于Token查询的字段白名单
var allowedTokenFields = map[string]bool{
	"id":            true,
	"access_token":  true,
	"refresh_token": true,
	"user_id":       true,
}

// ErrInvalidFieldName 无效的字段名错误
var ErrInvalidFieldName = errors.New("invalid field name")

// scanUser 从数据库行扫描用户数据
// 消除重复的用户扫描代码
func scanUser(scanner interface {
	Scan(dest ...interface{}) error
}) (*model.User, error) {
	user := &model.User{}
	var mfaSecret sql.NullString
	err := scanner.Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.EmailVerified,
		&user.MFAEnabled,
		&mfaSecret,
		&user.Role,
		&user.Status,
		&user.LoginAttempts,
		&user.LockedUntil,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if mfaSecret.Valid {
		user.MFASecret = mfaSecret.String
	}
	return user, nil
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
		pq.Array(&client.RedirectURIs),
		pq.Array(&client.GrantTypes),
		pq.Array(&client.Scopes),
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
		pq.Array(client.RedirectURIs),
		pq.Array(client.GrantTypes),
		pq.Array(client.Scopes),
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

	return nil
}

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

// ============================================================================
// 审计日志存储实现
// ============================================================================

// StoreAuditLog 存储审计日志
func (s *Store) StoreAuditLog(ctx context.Context, log *model.AuditLog) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		INSERT INTO audit_logs (id, event_type, user_id, client_id, ip_address, user_agent, details, success, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := s.db.ExecContext(ctx, query,
		log.ID,
		log.EventType,
		log.UserID,
		log.ClientID,
		log.IPAddress,
		log.UserAgent,
		log.Details,
		log.Success,
		log.Timestamp,
	)
	return err
}

// ListAuditLogs 列出审计日志（支持分页和过滤）
// 注意: SQL格式化是安全的，因为whereClause只包含固定的SQL片段
// 用户输入通过参数化查询（$1, $2...）传递，不存在SQL注入风险
func (s *Store) ListAuditLogs(ctx context.Context, userID string, eventType string, offset, limit int) ([]*model.AuditLog, int, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 构建查询条件
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if userID != "" {
		whereClause += fmt.Sprintf(" AND user_id = $%d", argIndex)
		args = append(args, userID)
		argIndex++
	}

	if eventType != "" {
		whereClause += fmt.Sprintf(" AND event_type = $%d", argIndex)
		args = append(args, eventType)
		argIndex++
	}

	// 获取总数
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs %s", whereClause)
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	query := fmt.Sprintf(`
		SELECT id, event_type, user_id, client_id, ip_address, user_agent, details, success, timestamp
		FROM audit_logs
		%s
		ORDER BY timestamp DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	// 预分配slice容量以减少内存重新分配
	logs := make([]*model.AuditLog, 0, limit)
	for rows.Next() {
		log := &model.AuditLog{}
		err := rows.Scan(
			&log.ID,
			&log.EventType,
			&log.UserID,
			&log.ClientID,
			&log.IPAddress,
			&log.UserAgent,
			&log.Details,
			&log.Success,
			&log.Timestamp,
		)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return logs, total, nil
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
