// Package service_test 用户服务单元测试
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestUserService 创建测试用的用户服务
func createTestUserService() (*service.UserService, *mock.Store) {
	mockStore := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	// 使用nil的emailSvc，测试时不实际发送邮件
	var emailSvc *service.EmailService
	userSvc := service.NewUserService(mockStore, passwordSvc, emailSvc, "http://localhost:9090")
	return userSvc, mockStore
}

// createTestUserServiceWithEmail 创建带mock邮件服务的用户服务
func createTestUserServiceWithEmail() (*service.UserService, *mock.Store, *mockMailSender) {
	mockStore := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	mockSender := &mockMailSender{}
	emailSvc, err := service.NewEmailService(&service.EmailConfig{
		SMTPHost: "localhost",
		SMTPPort: 587,
		From:     "noreply@example.com",
	}, mockSender)
	if err != nil {
		panic(err)
	}
	userSvc := service.NewUserService(mockStore, passwordSvc, emailSvc, "http://localhost:9090")
	return userSvc, mockStore, mockSender
}

// ============================================================================
// SendVerificationEmail 测试
// ============================================================================

func TestUserService_SendVerificationEmail(t *testing.T) {
	userSvc, mockStore := createTestUserService()
	ctx := context.Background()

	t.Run("邮箱已验证", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:            "user-123",
			Email:         "test@example.com",
			EmailVerified: true,
		}
		mockStore.AddUser(user)

		err := userSvc.SendVerificationEmail(ctx, "user-123")

		assert.ErrorIs(t, err, service.ErrEmailAlreadyVerified)
	})

	t.Run("用户不存在", func(t *testing.T) {
		mockStore.Reset()

		err := userSvc.SendVerificationEmail(ctx, "nonexistent")

		assert.Error(t, err)
	})
}

// ============================================================================
// VerifyEmail 测试
// ============================================================================

func TestUserService_VerifyEmail(t *testing.T) {
	userSvc, mockStore := createTestUserService()
	ctx := context.Background()

	t.Run("令牌不存在", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:            "user-123",
			Email:         "test@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		err := userSvc.VerifyEmail(ctx, "user-123", "invalid-token")

		assert.ErrorIs(t, err, service.ErrVerificationCodeInvalid)
	})

	t.Run("令牌不匹配", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:            "user-123",
			Email:         "test@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		// 存储一个令牌
		mockStore.StoreVerificationToken(ctx, "user-123", "correct-token", time.Now().Add(1*time.Hour))

		err := userSvc.VerifyEmail(ctx, "user-123", "wrong-token")

		assert.ErrorIs(t, err, service.ErrVerificationCodeInvalid)
	})

	t.Run("令牌已过期", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:            "user-123",
			Email:         "test@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		// 存储一个过期的令牌
		mockStore.StoreVerificationToken(ctx, "user-123", "expired-token", time.Now().Add(-1*time.Hour))

		err := userSvc.VerifyEmail(ctx, "user-123", "expired-token")

		assert.ErrorIs(t, err, service.ErrVerificationCodeExpired)
	})

	t.Run("验证成功", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:            "user-verify-ok",
			Email:         "verify@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		mockStore.StoreVerificationToken(ctx, "user-verify-ok", "valid-token", time.Now().Add(1*time.Hour))

		err := userSvc.VerifyEmail(ctx, "user-verify-ok", "valid-token")

		assert.NoError(t, err)

		// 验证用户邮箱已标记为已验证
		updatedUser, _ := mockStore.GetByID(ctx, "user-verify-ok")
		assert.True(t, updatedUser.EmailVerified)
	})
}

// ============================================================================
// ForgotPassword 测试
// ============================================================================

func TestUserService_ForgotPassword(t *testing.T) {
	userSvc, mockStore := createTestUserService()
	ctx := context.Background()

	t.Run("用户不存在-应返回成功", func(t *testing.T) {
		mockStore.Reset()

		// 为了安全，即使用户不存在也返回成功
		err := userSvc.ForgotPassword(ctx, "nonexistent@example.com")

		assert.NoError(t, err)
	})

	t.Run("用户存在", func(t *testing.T) {
		// 使用带mock邮件服务的UserService
		userSvc, mockStore, mockSender := createTestUserServiceWithEmail()

		user := &model.User{
			ID:    "user-123",
			Email: "test@example.com",
		}
		mockStore.AddUser(user)

		ctx := context.Background()

		err := userSvc.ForgotPassword(ctx, "test@example.com")

		assert.NoError(t, err)
		// 验证邮件已发送
		assert.Len(t, mockSender.sentMessages, 1)
		assert.Contains(t, string(mockSender.sentMessages[0].Msg), "重置密码")
	})
}

// ============================================================================
// ResetPassword 测试
// ============================================================================

func TestUserService_ResetPassword(t *testing.T) {
	userSvc, mockStore := createTestUserService()
	ctx := context.Background()

	t.Run("令牌不存在", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:           "user-123",
			Email:        "test@example.com",
			PasswordHash: "old-hash",
		}
		mockStore.AddUser(user)

		err := userSvc.ResetPassword(ctx, "user-123", "invalid-token", "NewPassword123!")

		assert.ErrorIs(t, err, service.ErrResetTokenInvalid)
	})

	t.Run("令牌不匹配", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:           "user-123",
			Email:        "test@example.com",
			PasswordHash: "old-hash",
		}
		mockStore.AddUser(user)

		mockStore.StoreResetToken(ctx, "user-123", "correct-token", time.Now().Add(1*time.Hour))

		err := userSvc.ResetPassword(ctx, "user-123", "wrong-token", "NewPassword123!")

		assert.ErrorIs(t, err, service.ErrResetTokenInvalid)
	})

	t.Run("令牌已过期", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:           "user-123",
			Email:        "test@example.com",
			PasswordHash: "old-hash",
		}
		mockStore.AddUser(user)

		mockStore.StoreResetToken(ctx, "user-123", "expired-token", time.Now().Add(-1*time.Hour))

		err := userSvc.ResetPassword(ctx, "user-123", "expired-token", "NewPassword123!")

		assert.ErrorIs(t, err, service.ErrResetTokenExpired)
	})

	t.Run("密码太短", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:           "user-123",
			Email:        "test@example.com",
			PasswordHash: "old-hash",
		}
		mockStore.AddUser(user)

		mockStore.StoreResetToken(ctx, "user-123", "valid-token", time.Now().Add(1*time.Hour))

		err := userSvc.ResetPassword(ctx, "user-123", "valid-token", "short")

		assert.Error(t, err)
		assert.ErrorIs(t, err, crypto.ErrPasswordTooShort)
	})

	t.Run("成功重置密码", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:           "user-reset",
			Email:        "reset@example.com",
			PasswordHash: "old-hash",
		}
		mockStore.AddUser(user)

		mockStore.StoreResetToken(ctx, "user-reset", "valid-token", time.Now().Add(1*time.Hour))

		err := userSvc.ResetPassword(ctx, "user-reset", "valid-token", "NewPassword123!")

		assert.NoError(t, err)

		// 验证密码已更新
		updatedUser, _ := mockStore.GetByID(ctx, "user-reset")
		assert.NotEqual(t, "old-hash", updatedUser.PasswordHash)
	})
}

// ============================================================================
// ChangePassword 测试
// ============================================================================

func TestUserService_ChangePassword(t *testing.T) {
	userSvc, mockStore := createTestUserService()
	ctx := context.Background()

	// 创建密码哈希
	passwordSvc := crypto.NewPasswordService(4)
	hashedPassword, _ := passwordSvc.HashPassword("OldPassword123!")

	t.Run("用户不存在", func(t *testing.T) {
		mockStore.Reset()

		err := userSvc.ChangePassword(ctx, "nonexistent", "OldPassword123!", "NewPassword123!")

		assert.Error(t, err)
	})

	t.Run("旧密码错误", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:           "user-123",
			Email:        "test@example.com",
			PasswordHash: hashedPassword,
		}
		mockStore.AddUser(user)

		err := userSvc.ChangePassword(ctx, "user-123", "WrongPassword!", "NewPassword123!")

		assert.ErrorIs(t, err, service.ErrInvalidCredentials)
	})

	t.Run("新密码太短", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:           "user-123",
			Email:        "test@example.com",
			PasswordHash: hashedPassword,
		}
		mockStore.AddUser(user)

		err := userSvc.ChangePassword(ctx, "user-123", "OldPassword123!", "short")

		assert.Error(t, err)
		// 新的错误格式是AppError
		assert.Contains(t, err.Error(), "密码长度不能少于8个字符")
	})

	t.Run("成功修改密码", func(t *testing.T) {
		mockStore.Reset()
		user := &model.User{
			ID:           "user-123",
			Email:        "test@example.com",
			PasswordHash: hashedPassword,
		}
		mockStore.AddUser(user)

		err := userSvc.ChangePassword(ctx, "user-123", "OldPassword123!", "NewPassword123!")

		assert.NoError(t, err)

		// 验证新密码
		updatedUser, _ := mockStore.GetByID(ctx, "user-123")
		err = passwordSvc.VerifyPassword(updatedUser.PasswordHash, "NewPassword123!")
		assert.NoError(t, err)
	})
}

// ============================================================================
// VerifyEmail 错误路径测试
// 验证: 需求 8.5
// ============================================================================

func TestUserService_VerifyEmail_StoreError(t *testing.T) {
	userSvc, mockStore := createTestUserService()
	ctx := context.Background()

	// 设置存储错误
	mockStore.GetVerificationTokenErr = errors.New("database error")

	err := userSvc.VerifyEmail(ctx, "user-123", "token")

	assert.Error(t, err)
	assert.NotErrorIs(t, err, service.ErrVerificationCodeInvalid)
}

// TestUserService_VerifyEmail_ErrorPaths 测试VerifyEmail的各种错误场景
// 验证: 需求 8.5
func TestUserService_VerifyEmail_ErrorPaths(t *testing.T) {
	ctx := context.Background()

	// ==== 测试1: Token过期 ====
	// 验证: 需求 8.5
	t.Run("Token过期", func(t *testing.T) {
		userSvc, mockStore := createTestUserService()
		mockStore.Reset()

		user := &model.User{
			ID:            "user-expired",
			Email:         "expired@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		// 存储一个过期的令牌（过期时间在过去）
		expiredTime := time.Now().Add(-1 * time.Hour)
		mockStore.StoreVerificationToken(ctx, "user-expired", "expired-token", expiredTime)

		// 尝试验证
		err := userSvc.VerifyEmail(ctx, "user-expired", "expired-token")

		// 验证返回过期错误
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrVerificationCodeExpired)

		// 验证用户邮箱未被标记为已验证
		updatedUser, _ := mockStore.GetByID(ctx, "user-expired")
		assert.False(t, updatedUser.EmailVerified)
	})

	// ==== 测试2: 无效Token - Token不存在 ====
	// 验证: 需求 8.5
	t.Run("无效Token_不存在", func(t *testing.T) {
		userSvc, mockStore := createTestUserService()
		mockStore.Reset()

		user := &model.User{
			ID:            "user-no-token",
			Email:         "notoken@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		// 不存储任何令牌，直接尝试验证
		err := userSvc.VerifyEmail(ctx, "user-no-token", "nonexistent-token")

		// 验证返回无效令牌错误
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrVerificationCodeInvalid)

		// 验证用户邮箱未被标记为已验证
		updatedUser, _ := mockStore.GetByID(ctx, "user-no-token")
		assert.False(t, updatedUser.EmailVerified)
	})

	// ==== 测试3: 无效Token - Token不匹配 ====
	// 验证: 需求 8.5
	t.Run("无效Token_不匹配", func(t *testing.T) {
		userSvc, mockStore := createTestUserService()
		mockStore.Reset()

		user := &model.User{
			ID:            "user-mismatch",
			Email:         "mismatch@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		// 存储一个令牌
		mockStore.StoreVerificationToken(ctx, "user-mismatch", "correct-token", time.Now().Add(1*time.Hour))

		// 使用错误的令牌尝试验证
		err := userSvc.VerifyEmail(ctx, "user-mismatch", "wrong-token")

		// 验证返回无效令牌错误
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrVerificationCodeInvalid)

		// 验证用户邮箱未被标记为已验证
		updatedUser, _ := mockStore.GetByID(ctx, "user-mismatch")
		assert.False(t, updatedUser.EmailVerified)
	})

	// ==== 测试4: Store返回错误 - GetVerificationToken失败 ====
	// 验证: 需求 8.5
	t.Run("Store返回错误_GetVerificationToken", func(t *testing.T) {
		userSvc, mockStore := createTestUserService()
		mockStore.Reset()

		// 注入数据库错误
		mockStore.GetVerificationTokenErr = errors.New("database connection failed")

		// 尝试验证
		err := userSvc.VerifyEmail(ctx, "user-123", "some-token")

		// 验证返回错误（不是ErrVerificationCodeInvalid）
		assert.Error(t, err)
		assert.NotErrorIs(t, err, service.ErrVerificationCodeInvalid)
		assert.Contains(t, err.Error(), "database connection failed")
	})

	// ==== 测试5: Store返回错误 - GetByID失败 ====
	// 验证: 需求 8.5
	t.Run("Store返回错误_GetByID", func(t *testing.T) {
		userSvc, mockStore := createTestUserService()
		mockStore.Reset()

		// 存储一个有效的令牌
		mockStore.StoreVerificationToken(ctx, "user-getbyid-fail", "valid-token", time.Now().Add(1*time.Hour))

		// 注入GetByID错误
		mockStore.GetUserByIDErr = errors.New("database read error")

		// 尝试验证
		err := userSvc.VerifyEmail(ctx, "user-getbyid-fail", "valid-token")

		// 验证返回数据库错误
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database read error")
	})

	// ==== 测试6: Store返回错误 - Update失败 ====
	// 验证: 需求 8.5
	t.Run("Store返回错误_Update", func(t *testing.T) {
		userSvc, mockStore := createTestUserService()
		mockStore.Reset()

		user := &model.User{
			ID:            "user-update-fail",
			Email:         "updatefail@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		// 存储一个有效的令牌
		mockStore.StoreVerificationToken(ctx, "user-update-fail", "valid-token", time.Now().Add(1*time.Hour))

		// 注入Update错误
		mockStore.UpdateUserErr = errors.New("database write error")

		// 尝试验证
		err := userSvc.VerifyEmail(ctx, "user-update-fail", "valid-token")

		// 验证返回数据库写入错误
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database write error")

		// 注意: 由于mock store返回的是同一个user对象引用，
		// 即使Update失败，内存中的对象也会被修改。
		// 这是mock实现的限制，真实数据库不会有这个问题。
		// 主要验证点是Update错误被正确返回。
	})

	// ==== 测试7: 边界情况 - Token刚好过期 ====
	// 验证: 需求 8.5
	t.Run("Token刚好过期", func(t *testing.T) {
		userSvc, mockStore := createTestUserService()
		mockStore.Reset()

		user := &model.User{
			ID:            "user-just-expired",
			Email:         "justexpired@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		// 存储一个刚好过期的令牌（过期时间是1秒前）
		justExpiredTime := time.Now().Add(-1 * time.Second)
		mockStore.StoreVerificationToken(ctx, "user-just-expired", "just-expired-token", justExpiredTime)

		// 尝试验证
		err := userSvc.VerifyEmail(ctx, "user-just-expired", "just-expired-token")

		// 验证返回过期错误
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrVerificationCodeExpired)
	})

	// ==== 测试8: 边界情况 - Token刚好未过期 ====
	// 验证: 需求 8.5
	t.Run("Token刚好未过期", func(t *testing.T) {
		userSvc, mockStore := createTestUserService()
		mockStore.Reset()

		user := &model.User{
			ID:            "user-just-valid",
			Email:         "justvalid@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		// 存储一个刚好未过期的令牌（过期时间是1秒后）
		justValidTime := time.Now().Add(1 * time.Second)
		mockStore.StoreVerificationToken(ctx, "user-just-valid", "just-valid-token", justValidTime)

		// 尝试验证
		err := userSvc.VerifyEmail(ctx, "user-just-valid", "just-valid-token")

		// 验证成功
		assert.NoError(t, err)

		// 验证用户邮箱已标记为已验证
		updatedUser, _ := mockStore.GetByID(ctx, "user-just-valid")
		assert.True(t, updatedUser.EmailVerified)
	})

	// ==== 测试9: 空Token ====
	// 验证: 需求 8.5
	t.Run("空Token", func(t *testing.T) {
		userSvc, mockStore := createTestUserService()
		mockStore.Reset()

		user := &model.User{
			ID:            "user-empty-token",
			Email:         "emptytoken@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		// 存储一个有效的令牌
		mockStore.StoreVerificationToken(ctx, "user-empty-token", "valid-token", time.Now().Add(1*time.Hour))

		// 使用空Token尝试验证
		err := userSvc.VerifyEmail(ctx, "user-empty-token", "")

		// 验证返回无效令牌错误
		assert.Error(t, err)
		assert.ErrorIs(t, err, service.ErrVerificationCodeInvalid)
	})

	// ==== 测试10: 空UserID ====
	// 验证: 需求 8.5
	t.Run("空UserID", func(t *testing.T) {
		userSvc, mockStore := createTestUserService()
		mockStore.Reset()

		// 使用空UserID尝试验证
		err := userSvc.VerifyEmail(ctx, "", "some-token")

		// 验证返回错误
		assert.Error(t, err)
	})
}

// ============================================================================
// 带邮件服务的测试
// ============================================================================

func TestUserService_SendVerificationEmail_WithEmail(t *testing.T) {
	userSvc, mockStore, mockSender := createTestUserServiceWithEmail()
	ctx := context.Background()

	t.Run("成功发送验证邮件", func(t *testing.T) {
		mockStore.Reset()
		mockSender.sentMessages = nil
		user := &model.User{
			ID:            "user-email-ok",
			Email:         "verify@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		err := userSvc.SendVerificationEmail(ctx, "user-email-ok")

		assert.NoError(t, err)
		assert.Len(t, mockSender.sentMessages, 1)
	})

	t.Run("邮箱已验证-不发送", func(t *testing.T) {
		mockStore.Reset()
		mockSender.sentMessages = nil
		user := &model.User{
			ID:            "user-verified",
			Email:         "verified@example.com",
			EmailVerified: true,
		}
		mockStore.AddUser(user)

		err := userSvc.SendVerificationEmail(ctx, "user-verified")

		assert.ErrorIs(t, err, service.ErrEmailAlreadyVerified)
		assert.Len(t, mockSender.sentMessages, 0)
	})

	t.Run("邮件发送失败", func(t *testing.T) {
		mockStore.Reset()
		mockSender.sentMessages = nil
		mockSender.shouldError = true
		user := &model.User{
			ID:            "user-email-fail",
			Email:         "fail@example.com",
			EmailVerified: false,
		}
		mockStore.AddUser(user)

		err := userSvc.SendVerificationEmail(ctx, "user-email-fail")

		assert.Error(t, err)
		mockSender.shouldError = false
	})
}

func TestUserService_ForgotPassword_WithEmail(t *testing.T) {
	userSvc, mockStore, mockSender := createTestUserServiceWithEmail()
	ctx := context.Background()

	t.Run("用户存在-发送重置邮件", func(t *testing.T) {
		mockStore.Reset()
		mockSender.sentMessages = nil
		user := &model.User{
			ID:    "user-fp-ok",
			Email: "forgot@example.com",
		}
		mockStore.AddUser(user)

		err := userSvc.ForgotPassword(ctx, "forgot@example.com")

		assert.NoError(t, err)
		assert.Len(t, mockSender.sentMessages, 1)
	})

	t.Run("用户不存在-安全返回成功", func(t *testing.T) {
		mockStore.Reset()
		mockSender.sentMessages = nil

		err := userSvc.ForgotPassword(ctx, "nonexistent@example.com")

		assert.NoError(t, err)
		assert.Len(t, mockSender.sentMessages, 0)
	})

	t.Run("邮件发送失败", func(t *testing.T) {
		mockStore.Reset()
		mockSender.sentMessages = nil
		mockSender.shouldError = true
		user := &model.User{
			ID:    "user-fp-fail",
			Email: "fail@example.com",
		}
		mockStore.AddUser(user)

		// 安全设计：邮件发送失败也返回nil，不泄露内部错误
		// 但会记录错误日志以便排查
		err := userSvc.ForgotPassword(ctx, "fail@example.com")

		assert.NoError(t, err)
		mockSender.shouldError = false
	})
}
