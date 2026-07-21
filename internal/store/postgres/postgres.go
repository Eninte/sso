// Package postgres PostgreSQL存储实现
// 实现store.Store接口，提供PostgreSQL数据库访问
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver

	"github.com/example/sso/internal/model"
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

// textArrayMap is initialized before concurrent request handling begins.
// Prewarming the slice mapping keeps subsequent scans read-only.
var textArrayMap = func() *pgtype.Map {
	typeMap := pgtype.NewMap()
	_, _ = typeMap.TypeForValue((*[]string)(nil))
	return typeMap
}()

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
	db, err := sql.Open("pgx", databaseURL)
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
func NewFromConfig(databaseURL string, maxOpenConns, maxIdleConns int, connMaxLifetime, connMaxIdleTime, queryTimeout time.Duration) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}

	// 使用配置参数设置连接池
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	if connMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(connMaxIdleTime)
	}

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

// scanTextArray adapts pgx's PostgreSQL array codec to database/sql.Scanner.
func scanTextArray(destination *[]string) sql.Scanner {
	return textArrayMap.SQLScanner(destination)
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
//
// 安全设计（T1）：明文列 access_token / refresh_token 已移除，
// 仅允许 hash 查询，防止明文出现在 WHERE 条件中
var allowedTokenFields = map[string]bool{
	"id":                 true,
	"user_id":            true,
	"access_token_hash":  true,
	"refresh_token_hash": true,
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
