// Package testutil 提供跨测试包复用的测试辅助函数
//
// 本文件提供测试数据 fixtures 工厂，用于减少测试中重复的 model 构造代码。
//
// 设计原则：
//   - 函数式选项模式（functional options）覆盖所有字段，便于按需定制
//   - 预设变体（NewActiveUser / NewLockedUser 等）封装常见用例，零参数即可使用
//   - 默认值合理：ID 用 uuid.New()，Email 用 "<id>@example.com"，时间用 time.Now()
//   - 不引入新依赖：复用项目已有的 google/uuid
//
// 使用示例：
//
//	// 默认活跃用户
//	user := testutil.NewActiveUser()
//
//	// 自定义邮箱的锁定用户
//	user := testutil.NewLockedUser(testutil.WithEmail("locked@my.com"))
//
//	// 自定义任意字段
//	user := testutil.NewUser(
//	    testutil.WithEmail("admin@test.com"),
//	    testutil.WithRole(model.UserRoleAdmin),
//	    testutil.WithMFAEnabled(true),
//	)
package testutil

import (
	"time"

	"github.com/google/uuid"

	"github.com/example/sso/internal/model"
)

// ============================================================================
// User fixtures
// ============================================================================

// UserOption 自定义 User 字段的函数选项
type UserOption func(*model.User)

// NewUser 创建一个 User，默认字段：
//   - ID: 随机 uuid
//   - Email: "<id>@example.com"
//   - PasswordHash: "hashed-password"（测试占位符，避免裸 bcrypt）
//   - EmailVerified: true
//   - Role: UserRoleUser
//   - Status: UserStatusActive
//   - CreatedAt/UpdatedAt: time.Now()
func NewUser(opts ...UserOption) *model.User {
	id := uuid.New().String()
	now := time.Now()
	u := &model.User{
		ID:            id,
		Email:         id + "@example.com",
		PasswordHash:  "hashed-password",
		EmailVerified: true,
		MFAEnabled:    false,
		Role:          model.UserRoleUser,
		Status:        model.UserStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

// 预设变体：封装常见的用户状态场景

// NewActiveUser 创建一个活跃且邮箱已验证的普通用户
func NewActiveUser(opts ...UserOption) *model.User {
	return NewUser(opts...)
}

// NewLockedUser 创建一个被锁定的用户（LockedUntil 默认 30 分钟后）
// 用户传入的选项会覆盖预设值（函数式选项惯例）
func NewLockedUser(opts ...UserOption) *model.User {
	preset := []UserOption{
		WithStatus(model.UserStatusLocked),
		WithLockedUntil(time.Now().Add(30 * time.Minute)),
	}
	return NewUser(append(preset, opts...)...)
}

// NewDisabledUser 创建一个被禁用的用户（LoginAttempts=5）
// 用户传入的选项会覆盖预设值（函数式选项惯例）
func NewDisabledUser(opts ...UserOption) *model.User {
	preset := []UserOption{
		WithStatus(model.UserStatusDisabled),
		WithLoginAttempts(5),
	}
	return NewUser(append(preset, opts...)...)
}

// NewUnverifiedUser 创建一个邮箱未验证的用户（Status=pending，EmailVerified=false）
// 用户传入的选项会覆盖预设值（函数式选项惯例）
func NewUnverifiedUser(opts ...UserOption) *model.User {
	preset := []UserOption{
		WithStatus(model.UserStatusPending),
		WithEmailVerified(false),
	}
	return NewUser(append(preset, opts...)...)
}

// NewAdminUser 创建一个管理员用户
// 用户传入的选项会覆盖预设值（函数式选项惯例）
func NewAdminUser(opts ...UserOption) *model.User {
	preset := []UserOption{WithRole(model.UserRoleAdmin)}
	return NewUser(append(preset, opts...)...)
}

// === User 字段选项 ===

// WithUserID 设置用户 ID
func WithUserID(id string) UserOption {
	return func(u *model.User) { u.ID = id }
}

// WithEmail 设置邮箱
func WithEmail(email string) UserOption {
	return func(u *model.User) { u.Email = email }
}

// WithPasswordHash 设置密码哈希
func WithPasswordHash(hash string) UserOption {
	return func(u *model.User) { u.PasswordHash = hash }
}

// WithEmailVerified 设置邮箱验证状态
func WithEmailVerified(verified bool) UserOption {
	return func(u *model.User) { u.EmailVerified = verified }
}

// WithMFAEnabled 设置是否启用 MFA
func WithMFAEnabled(enabled bool) UserOption {
	return func(u *model.User) { u.MFAEnabled = enabled }
}

// WithMFASecret 设置 MFA 密钥
func WithMFASecret(secret string) UserOption {
	return func(u *model.User) { u.MFASecret = secret }
}

// WithRole 设置用户角色
func WithRole(role string) UserOption {
	return func(u *model.User) { u.Role = role }
}

// WithStatus 设置用户状态
func WithStatus(status string) UserOption {
	return func(u *model.User) { u.Status = status }
}

// WithLoginAttempts 设置登录失败次数
func WithLoginAttempts(n int) UserOption {
	return func(u *model.User) { u.LoginAttempts = n }
}

// WithLockedUntil 设置锁定截止时间
func WithLockedUntil(t time.Time) UserOption {
	return func(u *model.User) {
		locked := t
		u.LockedUntil = &locked
	}
}

// WithCreatedAt 设置创建时间
func WithCreatedAt(t time.Time) UserOption {
	return func(u *model.User) { u.CreatedAt = t }
}

// WithUpdatedAt 设置更新时间
func WithUpdatedAt(t time.Time) UserOption {
	return func(u *model.User) { u.UpdatedAt = t }
}

// ============================================================================
// Client fixtures
// ============================================================================

// ClientOption 自定义 Client 字段的函数选项
type ClientOption func(*model.Client)

// NewClient 创建一个机密（confidential）OAuth 客户端，默认字段：
//   - ID/ClientID: 随机 uuid
//   - ClientSecret: "test-client-secret"
//   - Name: "Test Client"
//   - RedirectURIs: ["http://localhost:3000/callback"]
//   - GrantTypes: [authorization_code, refresh_token]
//   - Scopes: [openid, profile, email]
//   - PublicClient: false
func NewClient(opts ...ClientOption) *model.Client {
	id := uuid.New().String()
	c := &model.Client{
		ID:           id,
		ClientID:     id,
		ClientSecret: "test-client-secret",
		Name:         "Test Client",
		RedirectURIs: []string{"http://localhost:3000/callback"},
		GrantTypes:   []string{model.GrantTypeAuthorizationCode, model.GrantTypeRefreshToken},
		Scopes:       []string{"openid", "profile", "email"},
		PublicClient: false,
		CreatedAt:    time.Now(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewPublicClient 创建一个公开客户端（PublicClient=true，无 ClientSecret）
// 用户传入的选项会覆盖预设值（函数式选项惯例）
func NewPublicClient(opts ...ClientOption) *model.Client {
	preset := []ClientOption{
		WithPublicClient(true),
		WithClientSecret(""),
	}
	return NewClient(append(preset, opts...)...)
}

// === Client 字段选项 ===

// WithClientDBID 设置 Client.ID（数据库主键，区别于业务用的 ClientID）
func WithClientDBID(id string) ClientOption {
	return func(c *model.Client) { c.ID = id }
}

// WithClientID 设置业务用的 client_id
func WithClientID(id string) ClientOption {
	return func(c *model.Client) { c.ClientID = id }
}

// WithClientSecret 设置 client_secret
func WithClientSecret(secret string) ClientOption {
	return func(c *model.Client) { c.ClientSecret = secret }
}

// WithClientName 设置客户端名称
func WithClientName(name string) ClientOption {
	return func(c *model.Client) { c.Name = name }
}

// WithRedirectURIs 设置回调地址列表
func WithRedirectURIs(uris []string) ClientOption {
	return func(c *model.Client) { c.RedirectURIs = uris }
}

// WithGrantTypes 设置授权类型列表
func WithGrantTypes(types []string) ClientOption {
	return func(c *model.Client) { c.GrantTypes = types }
}

// WithScopes 设置 scope 列表
func WithScopes(scopes []string) ClientOption {
	return func(c *model.Client) { c.Scopes = scopes }
}

// WithPublicClient 设置是否为公开客户端
func WithPublicClient(isPublic bool) ClientOption {
	return func(c *model.Client) { c.PublicClient = isPublic }
}

// ============================================================================
// Token fixtures
// ============================================================================

// TokenOption 自定义 Token 字段的函数选项
type TokenOption func(*model.Token)

// NewToken 创建一个有效 Token，默认字段：
//   - ID: 随机 uuid
//   - AccessToken/RefreshToken: 随机 uuid
//   - UserID: 随机 uuid（如需绑定到具体用户，用 WithUserID 覆盖）
//   - ClientID: nil（无关联客户端）
//   - Scopes: [openid, profile]
//   - ExpiresAt: 15 分钟后
//   - RevokedAt: nil
func NewToken(opts ...TokenOption) *model.Token {
	id := uuid.New().String()
	now := time.Now()
	t := &model.Token{
		ID:           id,
		AccessToken:  uuid.New().String(),
		RefreshToken: uuid.New().String(),
		UserID:       uuid.New().String(),
		ClientID:     nil,
		Scopes:       []string{"openid", "profile"},
		ExpiresAt:    now.Add(15 * time.Minute),
		CreatedAt:    now,
		RevokedAt:    nil,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// NewRevokedToken 创建一个已撤销的 Token（RevokedAt 默认为 time.Now()）
func NewRevokedToken(opts ...TokenOption) *model.Token {
	opts = append(opts, WithRevokedAt(time.Now()))
	return NewToken(opts...)
}

// NewExpiredToken 创建一个已过期的 Token（ExpiresAt 默认为 1 小时前）
func NewExpiredToken(opts ...TokenOption) *model.Token {
	opts = append(opts, WithExpiresAt(time.Now().Add(-1*time.Hour)))
	return NewToken(opts...)
}

// === Token 字段选项 ===

// WithTokenID 设置 Token.ID
func WithTokenID(id string) TokenOption {
	return func(t *model.Token) { t.ID = id }
}

// WithAccessToken 设置 access_token
func WithAccessToken(token string) TokenOption {
	return func(t *model.Token) { t.AccessToken = token }
}

// WithRefreshToken 设置 refresh_token
func WithRefreshToken(token string) TokenOption {
	return func(t *model.Token) { t.RefreshToken = token }
}

// WithUserID 设置关联的用户 ID
func WithUserIDForToken(id string) TokenOption {
	return func(t *model.Token) { t.UserID = id }
}

// WithTokenClientID 设置关联的 client_id（处理 *string 类型）
func WithTokenClientID(id string) TokenOption {
	return func(t *model.Token) {
		if id == "" {
			t.ClientID = nil
			return
		}
		cid := id
		t.ClientID = &cid
	}
}

// WithTokenScopes 设置 scope 列表
func WithTokenScopes(scopes []string) TokenOption {
	return func(t *model.Token) { t.Scopes = scopes }
}

// WithExpiresAt 设置过期时间
func WithExpiresAt(at time.Time) TokenOption {
	return func(t *model.Token) { t.ExpiresAt = at }
}

// WithRevokedAt 设置撤销时间
func WithRevokedAt(at time.Time) TokenOption {
	return func(t *model.Token) {
		revoked := at
		t.RevokedAt = &revoked
	}
}

// WithTokenCreatedAt 设置创建时间
func WithTokenCreatedAt(at time.Time) TokenOption {
	return func(t *model.Token) { t.CreatedAt = at }
}
