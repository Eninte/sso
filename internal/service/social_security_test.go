// Package service_test 社交登录安全模块单元测试（阶段 2.3 专项）
package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数（仅在阶段 2.3 专项测试中使用）
// ============================================================================

// setupTestStore 创建并返回空的 mock store
func setupTestStore() *mock.Store {
	return mock.New()
}

// setupTestSocialService 创建并返回 social login service（无 auditSvc，验证 nil 安全）
func setupTestSocialService(storeInst *mock.Store) *service.SocialLoginService {
	jwtSvc := createTestJWTService()
	return service.NewSocialLoginService(storeInst, jwtSvc, "http://localhost:9000", "", "", "", "")
}

// createLocalUser 在 store 中创建一个本地账号（无 social_account 绑定）
func createLocalUser(storeInst *mock.Store, email string) *model.User {
	hashedPw, _ := crypto.NewPasswordService(4).HashPassword("Pass123!")
	now := time.Now()
	user := &model.User{
		ID:            "local-" + email,
		Email:         email,
		PasswordHash:  hashedPw,
		EmailVerified: true,
		Status:        model.UserStatusActive,
		Role:          model.UserRoleUser,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := storeInst.Create(context.Background(), user); err != nil {
		panic("createLocalUser failed: " + err.Error())
	}
	return user
}

// createBoundUserAndSocialAccount 创建已绑定 social_account 的用户
func createBoundUserAndSocialAccount(storeInst *mock.Store, provider, providerUserID string) (*model.User, *model.SocialAccount) {
	user := createLocalUser(storeInst, providerUserID+"@example.com")

	now := time.Now()
	account := &model.SocialAccount{
		ID:             "sa-" + providerUserID,
		Provider:       provider,
		ProviderUserID: providerUserID,
		UserID:         user.ID,
		ProviderEmail:  user.Email,
		EmailVerified:  true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	storeInst.AddSocialAccount(account)
	return user, account
}

// ============================================================================
// ExtractProviderIdentity 测试
// ============================================================================

func TestExtractProviderIdentity_UnsupportedProvider(t *testing.T) {
	t.Run("不支持的provider返回错误", func(t *testing.T) {
		_, err := service.ExtractProviderIdentity("facebook", map[string]interface{}{"id": "123"})
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrProviderNotSupported)
	})

	t.Run("空provider返回错误", func(t *testing.T) {
		_, err := service.ExtractProviderIdentity("", map[string]interface{}{"id": "123"})
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrProviderNotSupported)
	})
}

func TestExtractProviderIdentity_Google(t *testing.T) {
	t.Run("标准Google返回-sub字段+email_verified=true", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"sub":            "google-12345",
			"email":          "user@gmail.com",
			"email_verified": true,
			"name":           "Test User",
			"picture":        "https://google.com/avatar.png",
		}

		identity, err := service.ExtractProviderIdentity("google", userInfo)
		require.NoError(t, err)
		assert.Equal(t, "google-12345", identity.ProviderUserID)
		assert.Equal(t, "user@gmail.com", identity.Email)
		assert.True(t, identity.EmailVerified)
		assert.Equal(t, "Test User", identity.DisplayName)
		assert.Equal(t, "Test User", identity.Metadata["display_name"])
		assert.Equal(t, "https://google.com/avatar.png", identity.Metadata["avatar_url"])
	})

	t.Run("Google返回id而非sub字段", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"id":             "google-id-fallback",
			"email":          "user@gmail.com",
			"email_verified": true,
		}

		identity, err := service.ExtractProviderIdentity("google", userInfo)
		require.NoError(t, err)
		assert.Equal(t, "google-id-fallback", identity.ProviderUserID)
	})

	t.Run("Google返回email_verified为字符串true", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"sub":            "google-str-true",
			"email":          "user@gmail.com",
			"email_verified": "true",
		}

		identity, err := service.ExtractProviderIdentity("google", userInfo)
		require.NoError(t, err)
		assert.True(t, identity.EmailVerified)
	})

	t.Run("Google返回email_verified为字符串True大写", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"sub":            "google-str-True",
			"email":          "user@gmail.com",
			"email_verified": "True",
		}

		identity, err := service.ExtractProviderIdentity("google", userInfo)
		require.NoError(t, err)
		assert.True(t, identity.EmailVerified)
	})

	t.Run("Google返回email_verified为字符串false", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"sub":            "google-str-false",
			"email":          "user@gmail.com",
			"email_verified": "false",
		}

		identity, err := service.ExtractProviderIdentity("google", userInfo)
		require.NoError(t, err)
		assert.False(t, identity.EmailVerified)
	})

	t.Run("Google返回email_verified为false布尔", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"sub":            "google-bool-false",
			"email":          "user@gmail.com",
			"email_verified": false,
		}

		identity, err := service.ExtractProviderIdentity("google", userInfo)
		require.NoError(t, err)
		assert.False(t, identity.EmailVerified)
	})

	t.Run("Google缺少sub和id字段-拒绝", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"email":          "user@gmail.com",
			"email_verified": true,
		}

		_, err := service.ExtractProviderIdentity("google", userInfo)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrProviderUserIDMissing)
	})
}

func TestExtractProviderIdentity_GitHub(t *testing.T) {
	t.Run("标准GitHub返回-id为float64", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"id":         float64(12345678),
			"login":      "testuser",
			"name":       "Test User",
			"email":      "user@users.noreply.github.com",
			"avatar_url": "https://github.com/avatar.png",
		}

		identity, err := service.ExtractProviderIdentity("github", userInfo)
		require.NoError(t, err)
		assert.Equal(t, "12345678", identity.ProviderUserID)
		assert.Equal(t, "user@users.noreply.github.com", identity.Email)
		// 安全设计：GitHub email 默认视为未验证
		assert.False(t, identity.EmailVerified)
		assert.Equal(t, "testuser", identity.Metadata["login"])
		assert.Equal(t, "Test User", identity.DisplayName)
		assert.Equal(t, "Test User", identity.Metadata["display_name"])
		assert.Equal(t, "https://github.com/avatar.png", identity.Metadata["avatar_url"])
	})

	t.Run("GitHub id为int类型", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"id":    int(99999),
			"login": "testuser",
		}

		identity, err := service.ExtractProviderIdentity("github", userInfo)
		require.NoError(t, err)
		assert.Equal(t, "99999", identity.ProviderUserID)
	})

	t.Run("GitHub id为int64类型", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"id":    int64(88888),
			"login": "testuser",
		}

		identity, err := service.ExtractProviderIdentity("github", userInfo)
		require.NoError(t, err)
		assert.Equal(t, "88888", identity.ProviderUserID)
	})

	t.Run("GitHub id为string类型", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"id":    "77777",
			"login": "testuser",
		}

		identity, err := service.ExtractProviderIdentity("github", userInfo)
		require.NoError(t, err)
		assert.Equal(t, "77777", identity.ProviderUserID)
	})

	t.Run("GitHub缺少id字段-拒绝", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"login": "testuser",
		}

		_, err := service.ExtractProviderIdentity("github", userInfo)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrProviderUserIDMissing)
	})

	t.Run("GitHub无email-EmailVerified为false", func(t *testing.T) {
		// 模拟用户未公开 email 的场景
		userInfo := map[string]interface{}{
			"id":    float64(123),
			"login": "nouser",
			"email": "",
		}

		identity, err := service.ExtractProviderIdentity("github", userInfo)
		require.NoError(t, err)
		assert.Equal(t, "", identity.Email)
		assert.False(t, identity.EmailVerified)
		// 缺少 name 时使用 login 作为 DisplayName
		assert.Equal(t, "nouser", identity.DisplayName)
	})

	t.Run("GitHub缺少name-使用login作为DisplayName", func(t *testing.T) {
		userInfo := map[string]interface{}{
			"id":    float64(456),
			"login": "fallback-login",
		}

		identity, err := service.ExtractProviderIdentity("github", userInfo)
		require.NoError(t, err)
		assert.Equal(t, "fallback-login", identity.DisplayName)
	})
}

// ============================================================================
// findOrCreateSocialUser 测试（直接调用，不通过 HandleCallback）
// ============================================================================

func TestFindOrCreateSocialUser_ExistingAccount(t *testing.T) {
	t.Run("已存在social_account-复用用户-active状态", func(t *testing.T) {
		storeInst := setupTestStore()
		svc := setupTestSocialService(storeInst)

		// 预先创建用户 + social_account
		user, _ := createBoundUserAndSocialAccount(storeInst, "google", "google-existing-123")

		identity := &service.ProviderIdentity{
			ProviderUserID: "google-existing-123",
			Email:          "existing@gmail.com",
			EmailVerified:  true,
		}

		result, err := svc.FindOrCreateSocialUserForTest(context.Background(), "google", identity)
		require.NoError(t, err)
		assert.Equal(t, user.ID, result.ID)
		assert.Equal(t, user.Email, result.Email)
	})

	t.Run("已存在social_account-用户被禁用-拒绝", func(t *testing.T) {
		storeInst := setupTestStore()
		svc := setupTestSocialService(storeInst)

		user, account := createBoundUserAndSocialAccount(storeInst, "google", "google-disabled-123")
		// 修改用户状态为禁用
		user.Status = model.UserStatusDisabled
		require.NoError(t, storeInst.Update(context.Background(), user))
		_ = account

		identity := &service.ProviderIdentity{
			ProviderUserID: "google-disabled-123",
			Email:          user.Email,
			EmailVerified:  true,
		}

		_, err := svc.FindOrCreateSocialUserForTest(context.Background(), "google", identity)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrAccountDisabled)
	})

	t.Run("已存在social_account-用户被锁定-拒绝", func(t *testing.T) {
		storeInst := setupTestStore()
		svc := setupTestSocialService(storeInst)

		user, _ := createBoundUserAndSocialAccount(storeInst, "google", "google-locked-123")
		user.Status = model.UserStatusLocked
		require.NoError(t, storeInst.Update(context.Background(), user))

		identity := &service.ProviderIdentity{
			ProviderUserID: "google-locked-123",
			Email:          user.Email,
			EmailVerified:  true,
		}

		_, err := svc.FindOrCreateSocialUserForTest(context.Background(), "google", identity)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrAccountLocked)
	})
}

func TestFindOrCreateSocialUser_NewAccount_EmailNotVerified(t *testing.T) {
	t.Run("provider未验证email-拒绝创建用户", func(t *testing.T) {
		storeInst := setupTestStore()
		svc := setupTestSocialService(storeInst)

		identity := &service.ProviderIdentity{
			ProviderUserID: "google-new-unverified",
			Email:          "unverified@gmail.com",
			EmailVerified:  false,
		}

		_, err := svc.FindOrCreateSocialUserForTest(context.Background(), "google", identity)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrProviderEmailNotVerified)

		// 验证不会创建 user
		users, _, _ := storeInst.ListUsers(context.Background(), 0, 100)
		assert.Len(t, users, 0)
	})
}

func TestFindOrCreateSocialUser_NewAccount_EmailConflictWithLocal(t *testing.T) {
	t.Run("provider_email与本地账号冲突-拒绝自动合并", func(t *testing.T) {
		storeInst := setupTestStore()
		svc := setupTestSocialService(storeInst)

		// 预先创建本地账号（无 social_account 绑定）
		localUser := createLocalUser(storeInst, "local@gmail.com")

		// 用相同的 email 但不同的 provider_user_id 登录（应拒绝）
		identity := &service.ProviderIdentity{
			ProviderUserID: "google-attacker-id",
			Email:          localUser.Email,
			EmailVerified:  true,
		}

		_, err := svc.FindOrCreateSocialUserForTest(context.Background(), "google", identity)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrEmailConflictWithLocal)

		// 验证不会创建第二个用户
		users, _, _ := storeInst.ListUsers(context.Background(), 0, 100)
		assert.Len(t, users, 1)
	})
}

func TestFindOrCreateSocialUser_NewAccount_Success(t *testing.T) {
	t.Run("新用户创建成功-原子事务", func(t *testing.T) {
		storeInst := setupTestStore()
		svc := setupTestSocialService(storeInst)

		identity := &service.ProviderIdentity{
			ProviderUserID: "google-new-success",
			Email:          "newuser@gmail.com",
			EmailVerified:  true,
			DisplayName:    "New User",
		}

		user, err := svc.FindOrCreateSocialUserForTest(context.Background(), "google", identity)
		require.NoError(t, err)
		assert.NotEmpty(t, user.ID)
		assert.Equal(t, "newuser@gmail.com", user.Email)
		assert.True(t, user.EmailVerified)
		assert.Equal(t, model.UserStatusActive, user.Status)
		assert.Equal(t, model.UserRoleUser, user.Role)

		// 验证 social_account 已创建并正确关联
		account, err := storeInst.GetSocialAccount(context.Background(), "google", "google-new-success")
		require.NoError(t, err)
		assert.Equal(t, user.ID, account.UserID)
		assert.Equal(t, "google", account.Provider)
		assert.Equal(t, "google-new-success", account.ProviderUserID)
		assert.Equal(t, "newuser@gmail.com", account.ProviderEmail)
		assert.True(t, account.EmailVerified)
	})
}

func TestFindOrCreateSocialUser_StoreError(t *testing.T) {
	t.Run("GetSocialAccount数据库错误-返回包装错误", func(t *testing.T) {
		storeInst := setupTestStore()
		svc := setupTestSocialService(storeInst)

		// 注入数据库错误
		storeInst.GetSocialAccountErr = assert.AnError

		identity := &service.ProviderIdentity{
			ProviderUserID: "google-db-err",
			Email:          "dberr@gmail.com",
			EmailVerified:  true,
		}

		_, err := svc.FindOrCreateSocialUserForTest(context.Background(), "google", identity)
		require.Error(t, err)
		// 不暴露内部细节
		assert.NotEqual(t, assert.AnError, err)
	})
}

// ============================================================================
// 阶段 D 审查修复（L2）：updateSocialAccountIfNeeded 实际持久化测试
// ============================================================================

func TestFindOrCreateSocialUser_UpdateSocialAccountPersisted(t *testing.T) {
	t.Run("provider_email变化-实际持久化到DB", func(t *testing.T) {
		storeInst := setupTestStore()
		svc := setupTestSocialService(storeInst)

		// 预创建用户 + social_account，provider_email 为旧值
		user, account := createBoundUserAndSocialAccount(storeInst, "google", "google-update-123")
		account.ProviderEmail = "old-email@gmail.com"
		account.EmailVerified = false
		require.NoError(t, storeInst.UpdateSocialAccount(context.Background(), account))

		// 记录调用前的 updated_at，用于验证后续更新
		updatedBefore, err := storeInst.GetSocialAccount(context.Background(), "google", "google-update-123")
		require.NoError(t, err)
		oldUpdatedAt := updatedBefore.UpdatedAt

		// 模拟 provider 端 email 变化 + verified 变为 true
		identity := &service.ProviderIdentity{
			ProviderUserID: "google-update-123",
			Email:          "new-email@gmail.com",
			EmailVerified:  true,
		}

		// 确保 time.Now() 推进以使 updated_at 严格大于 oldUpdatedAt
		time.Sleep(10 * time.Millisecond)
		result, err := svc.FindOrCreateSocialUserForTest(context.Background(), "google", identity)
		require.NoError(t, err)
		assert.Equal(t, user.ID, result.ID)

		// 阶段 D 修复（L2）：验证 DB 中的 social_account 已实际更新
		updated, err := storeInst.GetSocialAccount(context.Background(), "google", "google-update-123")
		require.NoError(t, err)
		assert.Equal(t, "new-email@gmail.com", updated.ProviderEmail, "provider_email 应已持久化更新")
		assert.True(t, updated.EmailVerified, "email_verified 应已持久化更新")
		assert.True(t, updated.UpdatedAt.After(oldUpdatedAt), "updated_at 应已更新")
		// user_id 不应被修改
		assert.Equal(t, user.ID, updated.UserID, "user_id 关联不应被修改")
	})

	t.Run("provider_email未变化-不调用Update", func(t *testing.T) {
		storeInst := setupTestStore()
		svc := setupTestSocialService(storeInst)

		_, account := createBoundUserAndSocialAccount(storeInst, "google", "google-nochange-123")

		// 注入 UpdateSocialAccount 错误，验证未被调用
		storeInst.UpdateSocialAccountErr = assert.AnError

		identity := &service.ProviderIdentity{
			ProviderUserID: "google-nochange-123",
			Email:          account.ProviderEmail, // 相同 email
			EmailVerified:  account.EmailVerified, // 相同 verified
		}

		_, err := svc.FindOrCreateSocialUserForTest(context.Background(), "google", identity)
		require.NoError(t, err, "未变化时不应调用 Update，故注入错误也不应影响")
	})

	t.Run("UpdateSocialAccount失败-登录主流程不受影响", func(t *testing.T) {
		storeInst := setupTestStore()
		svc := setupTestSocialService(storeInst)

		user, account := createBoundUserAndSocialAccount(storeInst, "google", "google-update-fail-123")
		account.ProviderEmail = "old@gmail.com"
		account.EmailVerified = false
		require.NoError(t, storeInst.UpdateSocialAccount(context.Background(), account))

		// 注入 UpdateSocialAccount 错误
		storeInst.UpdateSocialAccountErr = assert.AnError

		identity := &service.ProviderIdentity{
			ProviderUserID: "google-update-fail-123",
			Email:          "new@gmail.com",
			EmailVerified:  true,
		}

		// 阶段 D 修复（L2）：更新失败不影响登录主流程（已通过身份校验）
		result, err := svc.FindOrCreateSocialUserForTest(context.Background(), "google", identity)
		require.NoError(t, err, "Update 失败不应影响登录主流程")
		assert.Equal(t, user.ID, result.ID)
	})
}
