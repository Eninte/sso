package testutil_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/model"
	testutil "github.com/example/sso/internal/util/testutil"
)

// ============================================================================
// User fixtures 测试
// ============================================================================

func TestNewUser_Defaults(t *testing.T) {
	u := testutil.NewUser()

	require.NotEmpty(t, u.ID)
	assert.NotEqual(t, "00000000-0000-0000-0000-000000000000", u.ID, "ID 应为非零 uuid")
	assert.Equal(t, u.ID+"@example.com", u.Email)
	assert.Equal(t, "hashed-password", u.PasswordHash)
	assert.True(t, u.EmailVerified)
	assert.False(t, u.MFAEnabled)
	assert.Equal(t, model.UserRoleUser, u.Role)
	assert.Equal(t, model.UserStatusActive, u.Status)
	assert.Equal(t, 0, u.LoginAttempts)
	assert.Nil(t, u.LockedUntil)
	assert.False(t, u.CreatedAt.IsZero())
	assert.False(t, u.UpdatedAt.IsZero())
}

func TestNewUser_Options(t *testing.T) {
	locked := time.Now().Add(10 * time.Minute)
	u := testutil.NewUser(
		testutil.WithUserID("custom-id"),
		testutil.WithEmail("custom@test.com"),
		testutil.WithPasswordHash("$2a$12$abc"),
		testutil.WithEmailVerified(false),
		testutil.WithMFAEnabled(true),
		testutil.WithMFASecret("SECRET123"),
		testutil.WithRole(model.UserRoleAdmin),
		testutil.WithStatus(model.UserStatusLocked),
		testutil.WithLoginAttempts(3),
		testutil.WithLockedUntil(locked),
		testutil.WithCreatedAt(time.UnixMilli(0)),
		testutil.WithUpdatedAt(time.UnixMilli(0)),
	)

	assert.Equal(t, "custom-id", u.ID)
	assert.Equal(t, "custom@test.com", u.Email)
	assert.Equal(t, "$2a$12$abc", u.PasswordHash)
	assert.False(t, u.EmailVerified)
	assert.True(t, u.MFAEnabled)
	assert.Equal(t, "SECRET123", u.MFASecret)
	assert.Equal(t, model.UserRoleAdmin, u.Role)
	assert.Equal(t, model.UserStatusLocked, u.Status)
	assert.Equal(t, 3, u.LoginAttempts)
	require.NotNil(t, u.LockedUntil)
	assert.WithinDuration(t, locked, *u.LockedUntil, time.Second)
	assert.True(t, u.CreatedAt.Equal(time.UnixMilli(0)))
	assert.True(t, u.UpdatedAt.Equal(time.UnixMilli(0)))
}

func TestNewActiveUser(t *testing.T) {
	u := testutil.NewActiveUser()
	assert.Equal(t, model.UserStatusActive, u.Status)
	assert.True(t, u.EmailVerified)
	assert.Equal(t, model.UserRoleUser, u.Role)
}

func TestNewLockedUser(t *testing.T) {
	u := testutil.NewLockedUser()
	assert.Equal(t, model.UserStatusLocked, u.Status)
	require.NotNil(t, u.LockedUntil)
	assert.True(t, u.LockedUntil.After(time.Now()), "LockedUntil 应在未来")
}

func TestNewLockedUser_WithCustomOptions(t *testing.T) {
	u := testutil.NewLockedUser(testutil.WithEmail("x@y.com"))
	assert.Equal(t, "x@y.com", u.Email)
	assert.Equal(t, model.UserStatusLocked, u.Status)
}

// 验证用户传入的选项会覆盖预设值（函数式选项惯例）
func TestPresetVariants_UserOptionsOverridePreset(t *testing.T) {
	t.Run("LockedUser 用户覆盖 status", func(t *testing.T) {
		u := testutil.NewLockedUser(testutil.WithStatus(model.UserStatusActive))
		assert.Equal(t, model.UserStatusActive, u.Status, "用户选项应覆盖预设")
	})

	t.Run("DisabledUser 用户覆盖 loginAttempts", func(t *testing.T) {
		u := testutil.NewDisabledUser(testutil.WithLoginAttempts(2))
		assert.Equal(t, 2, u.LoginAttempts, "用户选项应覆盖预设")
	})

	t.Run("UnverifiedUser 用户覆盖 emailVerified", func(t *testing.T) {
		u := testutil.NewUnverifiedUser(testutil.WithEmailVerified(true))
		assert.True(t, u.EmailVerified, "用户选项应覆盖预设")
	})

	t.Run("AdminUser 用户覆盖 role", func(t *testing.T) {
		u := testutil.NewAdminUser(testutil.WithRole(model.UserRoleUser))
		assert.Equal(t, model.UserRoleUser, u.Role, "用户选项应覆盖预设")
	})

	t.Run("PublicClient 用户覆盖 secret", func(t *testing.T) {
		c := testutil.NewPublicClient(testutil.WithClientSecret("custom-secret"))
		assert.Equal(t, "custom-secret", c.ClientSecret, "用户选项应覆盖预设")
		assert.True(t, c.PublicClient, "未被覆盖的预设应保留")
	})
}

func TestNewDisabledUser(t *testing.T) {
	u := testutil.NewDisabledUser()
	assert.Equal(t, model.UserStatusDisabled, u.Status)
	assert.Equal(t, 5, u.LoginAttempts)
}

func TestNewUnverifiedUser(t *testing.T) {
	u := testutil.NewUnverifiedUser()
	assert.Equal(t, model.UserStatusPending, u.Status)
	assert.False(t, u.EmailVerified)
}

func TestNewAdminUser(t *testing.T) {
	u := testutil.NewAdminUser()
	assert.Equal(t, model.UserRoleAdmin, u.Role)
	assert.Equal(t, model.UserStatusActive, u.Status)
}

func TestNewUser_UniqueID(t *testing.T) {
	ids := make(map[string]struct{}, 10)
	for i := 0; i < 10; i++ {
		u := testutil.NewUser()
		assert.NotContains(t, ids, u.ID, "每次生成的 ID 应唯一")
		ids[u.ID] = struct{}{}
	}
}

// ============================================================================
// Client fixtures 测试
// ============================================================================

func TestNewClient_Defaults(t *testing.T) {
	c := testutil.NewClient()

	require.NotEmpty(t, c.ID)
	assert.Equal(t, c.ID, c.ClientID, "默认 ID 与 ClientID 一致")
	assert.Equal(t, "test-client-secret", c.ClientSecret)
	assert.Equal(t, "Test Client", c.Name)
	assert.Equal(t, []string{"http://localhost:3000/callback"}, c.RedirectURIs)
	assert.Equal(t,
		[]string{model.GrantTypeAuthorizationCode, model.GrantTypeRefreshToken}, c.GrantTypes)
	assert.Equal(t, []string{"openid", "profile", "email"}, c.Scopes)
	assert.False(t, c.PublicClient)
	assert.False(t, c.CreatedAt.IsZero())
}

func TestNewClient_Options(t *testing.T) {
	c := testutil.NewClient(
		testutil.WithClientDBID("db-id"),
		testutil.WithClientID("biz-id"),
		testutil.WithClientSecret("secret-xyz"),
		testutil.WithClientName("My App"),
		testutil.WithRedirectURIs([]string{"https://a.com/cb", "https://b.com/cb"}),
		testutil.WithGrantTypes([]string{model.GrantTypeClientCredentials}),
		testutil.WithScopes([]string{"read"}),
		testutil.WithPublicClient(true),
	)

	assert.Equal(t, "db-id", c.ID)
	assert.Equal(t, "biz-id", c.ClientID)
	assert.Equal(t, "secret-xyz", c.ClientSecret)
	assert.Equal(t, "My App", c.Name)
	assert.Equal(t, []string{"https://a.com/cb", "https://b.com/cb"}, c.RedirectURIs)
	assert.Equal(t, []string{model.GrantTypeClientCredentials}, c.GrantTypes)
	assert.Equal(t, []string{"read"}, c.Scopes)
	assert.True(t, c.PublicClient)
}

func TestNewPublicClient(t *testing.T) {
	c := testutil.NewPublicClient()

	assert.True(t, c.PublicClient)
	assert.Empty(t, c.ClientSecret, "公开客户端不应有 secret")
}

// ============================================================================
// Token fixtures 测试
// ============================================================================

func TestNewToken_Defaults(t *testing.T) {
	tok := testutil.NewToken()

	require.NotEmpty(t, tok.ID)
	require.NotEmpty(t, tok.AccessToken)
	require.NotEmpty(t, tok.RefreshToken)
	require.NotEmpty(t, tok.UserID)
	assert.NotEqual(t, tok.ID, tok.AccessToken, "ID 与 AccessToken 应不同")
	assert.NotEqual(t, tok.AccessToken, tok.RefreshToken, "AccessToken 与 RefreshToken 应不同")
	assert.Nil(t, tok.ClientID)
	assert.Equal(t, []string{"openid", "profile"}, tok.Scopes)
	assert.True(t, tok.ExpiresAt.After(time.Now()), "默认 Token 未过期")
	assert.Nil(t, tok.RevokedAt)
	assert.False(t, tok.CreatedAt.IsZero())
}

func TestNewToken_Options(t *testing.T) {
	cid := "client-abc"
	tok := testutil.NewToken(
		testutil.WithTokenID("tok-1"),
		testutil.WithAccessToken("access-xyz"),
		testutil.WithRefreshToken("refresh-xyz"),
		testutil.WithUserIDForToken("user-1"),
		testutil.WithTokenClientID(cid),
		testutil.WithTokenScopes([]string{"admin"}),
		testutil.WithExpiresAt(time.UnixMilli(0)),
		testutil.WithRevokedAt(time.UnixMilli(1)),
		testutil.WithTokenCreatedAt(time.UnixMilli(2)),
	)

	assert.Equal(t, "tok-1", tok.ID)
	assert.Equal(t, "access-xyz", tok.AccessToken)
	assert.Equal(t, "refresh-xyz", tok.RefreshToken)
	assert.Equal(t, "user-1", tok.UserID)
	require.NotNil(t, tok.ClientID)
	assert.Equal(t, cid, *tok.ClientID)
	assert.Equal(t, []string{"admin"}, tok.Scopes)
	assert.True(t, tok.ExpiresAt.Equal(time.UnixMilli(0)))
	require.NotNil(t, tok.RevokedAt)
	assert.True(t, tok.RevokedAt.Equal(time.UnixMilli(1)))
	assert.True(t, tok.CreatedAt.Equal(time.UnixMilli(2)))
}

func TestNewToken_WithEmptyClientID(t *testing.T) {
	tok := testutil.NewToken(testutil.WithTokenClientID(""))
	assert.Nil(t, tok.ClientID, "空字符串应将 ClientID 设为 nil")
}

func TestNewRevokedToken(t *testing.T) {
	tok := testutil.NewRevokedToken()
	require.NotNil(t, tok.RevokedAt)
	assert.WithinDuration(t, time.Now(), *tok.RevokedAt, time.Second)
}

func TestNewExpiredToken(t *testing.T) {
	tok := testutil.NewExpiredToken()
	assert.True(t, tok.ExpiresAt.Before(time.Now()), "ExpiredToken 应已过期")
}

// ============================================================================
// 集成场景：User 与 Token 绑定
// ============================================================================

func TestUserTokenBinding(t *testing.T) {
	// 演示典型用例：创建用户后再创建绑定该用户的 token
	user := testutil.NewActiveUser(testutil.WithEmail("alice@test.com"))
	tok := testutil.NewToken(testutil.WithUserIDForToken(user.ID))

	assert.Equal(t, user.ID, tok.UserID)
	assert.True(t, strings.HasSuffix(user.Email, "@test.com"))
}
