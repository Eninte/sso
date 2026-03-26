// Package store 数据存储层接口
// 定义数据访问的抽象接口
package store

import (
	"context"
	"time"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

var (
	ErrNotFound        = apperrors.ErrNotFound
	ErrDuplicateEmail  = apperrors.ErrEmailExists
	ErrDuplicateClient = apperrors.ErrConflict
)

// ============================================================================
// Store 存储接口
// ============================================================================

type Store interface {
	UserStore
	ClientStore
	TokenStore
	AuditLogStore
	KeyStore
	Close() error
	Ping(ctx context.Context) error
}

// ============================================================================
// UserStore 用户存储接口
// ============================================================================

type UserStore interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id string) error
	ListUsers(ctx context.Context, offset, limit int) ([]*model.User, int, error)
	UpdateLoginAttempts(ctx context.Context, userID string, attempts int, lockedUntil *time.Time) error
	IncrementLoginAttempts(ctx context.Context, userID string, maxAttempts int, lockoutDuration time.Duration) (attempts int, locked bool, lockedUntil *time.Time, err error)
	ResetLoginAttempts(ctx context.Context, userID string) error
	UnlockExpiredAccount(ctx context.Context, userID string) error
}

// ============================================================================
// ClientStore 客户端存储接口
// ============================================================================

type ClientStore interface {
	CreateClient(ctx context.Context, client *model.Client) error
	GetByClientID(ctx context.Context, clientID string) (*model.Client, error)
	ValidateRedirectURI(ctx context.Context, clientID, redirectURI string) bool
}

// ============================================================================
// TokenStore Token存储接口
// ============================================================================

type TokenStore interface {
	StoreToken(ctx context.Context, token *model.Token) error
	GetTokenByAccessToken(ctx context.Context, accessToken string) (*model.Token, error)
	GetTokenByRefreshToken(ctx context.Context, refreshToken string) (*model.Token, error)
	RevokeToken(ctx context.Context, accessToken string) error
	RevokeAllUserTokens(ctx context.Context, userID string) error
	StoreAuthorizationCode(ctx context.Context, code *model.AuthorizationCode) error
	GetAuthorizationCode(ctx context.Context, code string) (*model.AuthorizationCode, error)
	UpdateAuthorizationCode(ctx context.Context, code *model.AuthorizationCode) error
	StoreVerificationToken(ctx context.Context, userID, token string, expiresAt time.Time) error
	GetVerificationToken(ctx context.Context, userID string) (*VerificationToken, error)
	DeleteVerificationToken(ctx context.Context, userID string) error
	StoreResetToken(ctx context.Context, userID, token string, expiresAt time.Time) error
	GetResetToken(ctx context.Context, userID string) (*ResetToken, error)
	DeleteResetToken(ctx context.Context, userID string) error
	CleanupExpired(ctx context.Context) error
}

// ============================================================================
// AuditLogStore 审计日志存储接口
// ============================================================================

type AuditLogStore interface {
	StoreAuditLog(ctx context.Context, log *model.AuditLog) error
	ListAuditLogs(ctx context.Context, userID, eventType string, offset, limit int) ([]*model.AuditLog, int, error)
}

type KeyStore interface {
	StoreKey(ctx context.Context, key *model.KeyVersion) error
	GetActiveKey(ctx context.Context) (*model.KeyVersion, error)
	GetKeyByID(ctx context.Context, keyID string) (*model.KeyVersion, error)
	ListActiveKeys(ctx context.Context) ([]*model.KeyVersion, error)
	ListAllKeys(ctx context.Context) ([]*model.KeyVersion, error)
	DeprecateKey(ctx context.Context, keyID string, expiresAt time.Time) error
	RevokeKey(ctx context.Context, keyID string) error
	DeleteKey(ctx context.Context, keyID string) error
}

// ============================================================================
// 辅助类型
// ============================================================================

type VerificationToken struct {
	UserID    string
	Token     string
	ExpiresAt time.Time
}

type ResetToken struct {
	UserID    string
	Token     string
	ExpiresAt time.Time
}
