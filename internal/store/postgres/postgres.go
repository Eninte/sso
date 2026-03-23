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
		INSERT INTO users (id, email, password_hash, email_verified, mfa_enabled, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := s.db.ExecContext(ctx, query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.EmailVerified,
		user.MFAEnabled,
		user.Status,
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
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT id, email, password_hash, email_verified, mfa_enabled, mfa_secret, 
		       status, login_attempts, locked_until, created_at, updated_at
		FROM users
		WHERE %s = $1
	`, field)

	user := &model.User{}
	var mfaSecret sql.NullString
	err := s.db.QueryRowContext(ctx, query, value).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.EmailVerified,
		&user.MFAEnabled,
		&mfaSecret,
		&user.Status,
		&user.LoginAttempts,
		&user.LockedUntil,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}

	if mfaSecret.Valid {
		user.MFASecret = mfaSecret.String
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
		    mfa_secret = $6, status = $7, login_attempts = $8, locked_until = $9, updated_at = $10
		WHERE id = $1
	`
	_, err := s.db.ExecContext(ctx, query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.EmailVerified,
		user.MFAEnabled,
		user.MFASecret,
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
		       status, login_attempts, locked_until, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := make([]*model.User, 0)
	for rows.Next() {
		user := &model.User{}
		var mfaSecret sql.NullString
		err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.PasswordHash,
			&user.EmailVerified,
			&user.MFAEnabled,
			&mfaSecret,
			&user.Status,
			&user.LoginAttempts,
			&user.LockedUntil,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		if mfaSecret.Valid {
			user.MFASecret = mfaSecret.String
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
func (s *Store) ValidateRedirectURI(ctx context.Context, clientID string, redirectURI string) bool {
	client, err := s.GetByClientID(ctx, clientID)
	if err != nil {
		return false
	}

	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			return true
		}
	}
	return false
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
	query := fmt.Sprintf(`
		SELECT id, access_token, refresh_token, user_id, client_id, scopes, expires_at, created_at, revoked_at
		FROM tokens
		WHERE %s = $1
	`, field)

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
func (s *Store) CleanupExpired(ctx context.Context) error {
	// 清理过期的Token
	_, err := s.db.ExecContext(ctx, `DELETE FROM tokens WHERE expires_at < $1`, time.Now())
	if err != nil {
		return err
	}

	// 清理过期的授权码
	_, err = s.db.ExecContext(ctx, `DELETE FROM authorization_codes WHERE expires_at < $1`, time.Now())
	if err != nil {
		return err
	}

	// 清理过期的验证令牌
	_, err = s.db.ExecContext(ctx, `DELETE FROM verification_tokens WHERE expires_at < $1`, time.Now())
	if err != nil {
		return err
	}

	// 清理过期的重置令牌
	_, err = s.db.ExecContext(ctx, `DELETE FROM reset_tokens WHERE expires_at < $1`, time.Now())
	return err
}

// ============================================================================
// 验证令牌存储实现
// ============================================================================

// StoreVerificationToken 存储验证令牌
func (s *Store) StoreVerificationToken(ctx context.Context, userID string, token string, expiresAt time.Time) error {
	// 先删除旧的验证令牌
	_, _ = s.db.ExecContext(ctx, `DELETE FROM verification_tokens WHERE user_id = $1`, userID)

	// 插入新的验证令牌
	query := `INSERT INTO verification_tokens (user_id, token, expires_at, created_at) VALUES ($1, $2, $3, $4)`
	_, err := s.db.ExecContext(ctx, query, userID, token, expiresAt, time.Now())
	return err
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
	_, err := s.db.ExecContext(ctx, `DELETE FROM verification_tokens WHERE user_id = $1`, userID)
	return err
}

// StoreResetToken 存储重置令牌
func (s *Store) StoreResetToken(ctx context.Context, userID string, token string, expiresAt time.Time) error {
	// 先删除旧的重置令牌
	_, _ = s.db.ExecContext(ctx, `DELETE FROM reset_tokens WHERE user_id = $1`, userID)

	// 插入新的重置令牌
	query := `INSERT INTO reset_tokens (user_id, token, expires_at, created_at) VALUES ($1, $2, $3, $4)`
	_, err := s.db.ExecContext(ctx, query, userID, token, expiresAt, time.Now())
	return err
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
	_, err := s.db.ExecContext(ctx, `DELETE FROM reset_tokens WHERE user_id = $1`, userID)
	return err
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

	logs := make([]*model.AuditLog, 0)
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
