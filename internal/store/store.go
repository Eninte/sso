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
	// ErrForbidden 用于存储层拒绝禁止的操作（如并发创建初始管理员）
	// 与 apperrors.ErrForbidden 等价，重新导出避免 store 包直接依赖 apperrors
	ErrForbidden = apperrors.ErrForbidden
	// ErrTokenRotated Refresh Token 已被轮换（rotated_at 非空）又再次出现
	// 重新导出，避免 store 包用户直接依赖 apperrors
	ErrTokenRotated = apperrors.ErrTokenRotated
	// 阶段 2.3：社交账号相关错误
	ErrSocialAccountConflict = apperrors.ErrSocialAccountConflict
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
	SocialAccountStore
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

	// CountActiveAdmins 统计活跃状态的管理员数量
	// 用于"末位管理员保护"：禁用/删除最后一个活跃管理员前必须拦截
	CountActiveAdmins(ctx context.Context) (int, error)

	// CreateAdminAtomic 原子地创建初始管理员账户
	// 使用事务级 advisory lock + 数据库 EXISTS 检查，确保全局只能创建一个初始管理员
	// 并发调用时，仅第一个获取锁的事务会执行插入，其余返回 ErrForbidden
	// 同时避免"检查-插入"竞态：传统模式 AdminExists + Create 之间存在 TOCTOU 窗口
	// 实现要求：PostgreSQL 9.6+（pg_advisory_xact_lock / hashtext）
	CreateAdminAtomic(ctx context.Context, user *model.User) error
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

	// RotateRefreshToken 原子地轮换 refresh token
	//
	// 在单个事务内完成：
	//  1. 标记旧 token 为已轮换（rotated_at = NOW()）+ 已撤销（revoked_at = NOW()）
	//     仅当旧 token 当前未撤销且未轮换时才执行更新（WHERE rotated_at IS NULL AND revoked_at IS NULL）
	//  2. 若 RowsAffected == 0，说明 token 不存在或已被使用，返回 ErrTokenRotated
	//     （这是重放攻击的典型特征：已被轮换的 refresh token 再次出现）
	//  3. 插入新的 token 记录（newToken），ReplacedByTokenID 已指向旧 token 的 ID
	//
	// 安全设计：
	//   - 原子性：UPDATE + INSERT 在同一事务内，避免 TOCTOU 竞态
	//   - 一次性：通过 WHERE rotated_at IS NULL 保证只能轮换一次
	//   - 重放检测：RowsAffected == 0 时调用方应视为 token 被盗用
	RotateRefreshToken(ctx context.Context, oldRefreshToken string, newToken *model.Token) error
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
	// UpdateKeyPrivateKey 更新密钥的私钥字段（T7：懒加密回写密文时使用）
	UpdateKeyPrivateKey(ctx context.Context, keyID string, privateKey []byte) error
}

// ============================================================================
// MFARecoveryCodeStore MFA恢复码存储接口
// ============================================================================

type MFARecoveryCodeStore interface {
	StoreMFARecoveryCodes(ctx context.Context, userID string, codeHashes []string) error
	GetUnusedMFARecoveryCodes(ctx context.Context, userID string) ([]string, error)
	VerifyAndUseMFARecoveryCode(ctx context.Context, userID string, codeHash string) (bool, error)
	DeleteUsedMFARecoveryCodes(ctx context.Context, userID string) error
	DeleteAllMFARecoveryCodes(ctx context.Context, userID string) error

	// DisableMFAAndClearRecoveryCodes 原子地禁用MFA并清除所有恢复码
	// 必须在单个事务中执行 Update(user) + DeleteAllMFARecoveryCodes，
	// 防止出现"用户MFA已禁用但恢复码残留"的不一致状态
	DisableMFAAndClearRecoveryCodes(ctx context.Context, user *model.User) error
}

// ============================================================================
// SocialAccountStore 社交账号身份存储接口（阶段 2.3）
// ============================================================================

type SocialAccountStore interface {
	// CreateSocialAccount 创建社交账号绑定记录
	// 唯一约束：(provider, provider_user_id) 与 (user_id, provider)
	// 冲突时返回 ErrSocialAccountConflict
	CreateSocialAccount(ctx context.Context, account *model.SocialAccount) error

	// GetSocialAccount 通过 (provider, provider_user_id) 查找社交账号
	// 用于社交登录回调时查找已绑定的用户
	// 不存在返回 ErrNotFound
	GetSocialAccount(ctx context.Context, provider, providerUserID string) (*model.SocialAccount, error)

	// ListSocialAccountsByUserID 列出用户绑定的所有社交账号
	// 用于用户在个人中心查看/解绑社交账号
	ListSocialAccountsByUserID(ctx context.Context, userID string) ([]*model.SocialAccount, error)

	// UpdateSocialAccount 更新社交账号绑定信息
	// 仅更新 provider_email / email_verified / provider_metadata / updated_at 字段
	// 不修改 user_id 关联，防止通过修改 provider 端 email 接管其他用户账号
	// 阶段 D 修复（L2）：原 updateSocialAccountIfNeeded 仅修改内存对象未持久化
	UpdateSocialAccount(ctx context.Context, account *model.SocialAccount) error

	// DeleteSocialAccount 解绑社交账号
	// 用于用户主动解绑
	DeleteSocialAccount(ctx context.Context, provider, providerUserID string) error

	// CreateSocialAccountAtomic 原子地创建用户 + 社交账号绑定
	// 必须在单个事务中执行：
	//   1. INSERT users
	//   2. INSERT social_accounts
	// 防止"用户已创建但社交账号未绑定"或"社交账号已绑定但用户不存在"的不一致状态
	// 若 (provider, provider_user_id) 已存在则返回 ErrSocialAccountConflict
	CreateSocialAccountAtomic(ctx context.Context, user *model.User, account *model.SocialAccount) error
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
