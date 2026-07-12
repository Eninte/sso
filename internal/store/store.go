// Package store 数据存储层接口
// 定义数据访问的抽象接口
package store

import (
	"context"
	"time"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

var (
	ErrNotFound              = apperrors.ErrNotFound
	ErrDuplicateEmail        = apperrors.ErrEmailExists
	ErrDuplicateClient       = apperrors.ErrConflict
	ErrAuthorizationCodeUsed = apperrors.ErrCodeUsedErr
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
	MFARecoveryCodeStore
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

	// ExistsUserByRole 检查是否存在指定角色的用户
	// 用于管理员存在性检查等场景，避免全表扫描
	ExistsUserByRole(ctx context.Context, role string) (bool, error)
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
	MarkResetTokenUsed(ctx context.Context, userID string) error
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
// MFARecoveryCodeStore MFA恢复码存储接口
// ============================================================================

type MFARecoveryCodeStore interface {
	StoreMFARecoveryCodes(ctx context.Context, userID string, codeHashes []string) error
	GetUnusedMFARecoveryCodes(ctx context.Context, userID string) ([]string, error)
	VerifyAndUseMFARecoveryCode(ctx context.Context, userID, codeHash string) (bool, error)
	DeleteUsedMFARecoveryCodes(ctx context.Context, userID string) error
	DeleteAllMFARecoveryCodes(ctx context.Context, userID string) error

	// DisableMFAAndClearRecoveryCodes 原子地禁用MFA并清除所有恢复码
	// 必须在单个事务中执行 Update(user) + DeleteAllMFARecoveryCodes，
	// 防止出现"用户MFA已禁用但恢复码残留"的不一致状态
	DisableMFAAndClearRecoveryCodes(ctx context.Context, user *model.User) error
}

// ============================================================================
// QualityMetricsStore 质量指标存储接口
// ============================================================================

type QualityMetricsStore interface {
	StoreMetrics(ctx context.Context, m *QualityMetrics) error
	GetLatestMetrics(ctx context.Context) (*QualityMetrics, error)
	GetMetricsRange(ctx context.Context, from, to time.Time) ([]QualityMetrics, error)
	GetWeeklyComparison(ctx context.Context) (*WeeklyComparison, error)
}

// QualityMetrics 质量指标数据
type QualityMetrics struct {
	ID                string                 `json:"id"`
	RecordedAt        time.Time              `json:"recorded_at"`
	GitCommitSHA      string                 `json:"git_commit_sha,omitempty"`
	CoveragePercent   float64                `json:"coverage_percent"`
	TestPassRate      float64                `json:"test_pass_rate"`
	TotalTests        int                    `json:"total_tests"`
	PassedTests       int                    `json:"passed_tests"`
	FailedTests       int                    `json:"failed_tests"`
	LintViolations    int                    `json:"lint_violations"`
	GosecViolations   int                    `json:"gosec_violations"`
	GocycloViolations int                    `json:"gocyclo_violations"`
	QualityScore      float64                `json:"quality_score"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
}

// WeeklyComparison 周对比数据
type WeeklyComparison struct {
	Current  *QualityMetrics `json:"current"`
	Previous *QualityMetrics `json:"previous,omitempty"`
	Delta    *QualityDelta   `json:"delta,omitempty"`
}

// QualityDelta 质量指标变化
type QualityDelta struct {
	CoverageDelta float64 `json:"coverage_delta"`
	PassRateDelta float64 `json:"pass_rate_delta"`
	ScoreDelta    float64 `json:"score_delta"`
	LintDelta     int     `json:"lint_delta"`
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
	UsedAt    *time.Time // 使用时间；NULL表示未使用
}
