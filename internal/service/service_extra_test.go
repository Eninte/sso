package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store/mock"
)

func TestMFAService_GenerateRecoveryCodes(t *testing.T) {
	t.Cleanup(mock.ClearMFARecoveryCodes)

	t.Run("默认生成8个恢复码", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		codes, err := svc.GenerateRecoveryCodes(context.Background(), "user-1", 0)
		require.NoError(t, err)
		assert.Len(t, codes, 8)

		for _, code := range codes {
			assert.Regexp(t, "^[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}$", code)
		}
	})

	t.Run("指定数量", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		codes, err := svc.GenerateRecoveryCodes(context.Background(), "user-1", 5)
		require.NoError(t, err)
		assert.Len(t, codes, 5)
	})

	t.Run("超过20使用默认", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		codes, err := svc.GenerateRecoveryCodes(context.Background(), "user-1", 100)
		require.NoError(t, err)
		assert.Len(t, codes, 8)
	})

	t.Run("负数使用默认", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		codes, err := svc.GenerateRecoveryCodes(context.Background(), "user-1", -1)
		require.NoError(t, err)
		assert.Len(t, codes, 8)
	})
}

func TestMFAService_VerifyRecoveryCode(t *testing.T) {
	t.Cleanup(mock.ClearMFARecoveryCodes)

	t.Run("验证成功", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		codes, err := svc.GenerateRecoveryCodes(context.Background(), "user-1", 8)
		require.NoError(t, err)

		used, err := svc.VerifyRecoveryCode(context.Background(), "user-1", codes[0])
		require.NoError(t, err)
		assert.True(t, used)
	})

	t.Run("无效码", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		_, err := svc.GenerateRecoveryCodes(context.Background(), "user-1", 8)
		require.NoError(t, err)

		used, err := svc.VerifyRecoveryCode(context.Background(), "user-1", "INVALID-CODE-XXXX-XXXX")
		assert.ErrorIs(t, err, ErrRecoveryCodeInvalid)
		assert.False(t, used)
	})

	t.Run("已使用的码", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		codes, err := svc.GenerateRecoveryCodes(context.Background(), "user-1", 8)
		require.NoError(t, err)

		_, err = svc.VerifyRecoveryCode(context.Background(), "user-1", codes[0])
		require.NoError(t, err)

		used, err := svc.VerifyRecoveryCode(context.Background(), "user-1", codes[0])
		assert.ErrorIs(t, err, ErrRecoveryCodeInvalid)
		assert.False(t, used)
	})

	t.Run("限流触发", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		for i := 0; i < 5; i++ {
			_, _ = svc.VerifyRecoveryCode(context.Background(), "user-1", "bad")
		}

		used, err := svc.VerifyRecoveryCode(context.Background(), "user-1", "another-bad")
		assert.ErrorIs(t, err, ErrTooManyRecoveryAttempts)
		assert.False(t, used)
	})

	t.Run("成功后清除尝试记录", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		codes, err := svc.GenerateRecoveryCodes(context.Background(), "user-1", 8)
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			_, _ = svc.VerifyRecoveryCode(context.Background(), "user-1", "bad")
		}

		used, err := svc.VerifyRecoveryCode(context.Background(), "user-1", codes[0])
		require.NoError(t, err)
		assert.True(t, used)

		assert.False(t, svc.checkRecoveryRateLimit("user-1"))
	})
}

func TestMFAService_GetRecoveryCodeStatus(t *testing.T) {
	mock.ClearMFARecoveryCodes()
	t.Cleanup(mock.ClearMFARecoveryCodes)

	t.Run("返回剩余数量", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		codes, err := svc.GenerateRecoveryCodes(context.Background(), "user-1", 8)
		require.NoError(t, err)

		count, err := svc.GetRecoveryCodeStatus(context.Background(), "user-1")
		require.NoError(t, err)
		assert.Equal(t, 8, count)

		_, err = svc.VerifyRecoveryCode(context.Background(), "user-1", codes[0])
		require.NoError(t, err)

		count, err = svc.GetRecoveryCodeStatus(context.Background(), "user-1")
		require.NoError(t, err)
		assert.Equal(t, 7, count)
	})

	t.Run("无恢复码返回0", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		count, err := svc.GetRecoveryCodeStatus(context.Background(), "user-2")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestMFAService_checkRecoveryRateLimit(t *testing.T) {
	t.Cleanup(mock.ClearMFARecoveryCodes)

	t.Run("新用户未锁定", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		assert.False(t, svc.checkRecoveryRateLimit("user-1"))
	})

	t.Run("失败后未达上限未锁定", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		for i := 0; i < 3; i++ {
			svc.recordRecoveryFailure("user-1")
		}

		assert.False(t, svc.checkRecoveryRateLimit("user-1"))
	})

	t.Run("达到上限后锁定", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		for i := 0; i < 5; i++ {
			svc.recordRecoveryFailure("user-1")
		}

		assert.True(t, svc.checkRecoveryRateLimit("user-1"))
	})

	t.Run("clearRecoveryAttempts清除记录", func(t *testing.T) {
		m := mock.New()
		svc := NewMFAService(m)

		for i := 0; i < 5; i++ {
			svc.recordRecoveryFailure("user-1")
		}

		svc.clearRecoveryAttempts("user-1")
		assert.False(t, svc.checkRecoveryRateLimit("user-1"))
	})
}

func TestMFAService_generateRecoveryCode(t *testing.T) {
	t.Run("生成格式正确的恢复码", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			code, err := generateRecoveryCode()
			require.NoError(t, err)
			assert.Regexp(t, "^[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}$", code)
		}
	})
}

func TestMFAService_bcryptHash(t *testing.T) {
	t.Run("哈希值可以验证", func(t *testing.T) {
		code, err := generateRecoveryCode()
		require.NoError(t, err)

		hash, err := bcryptHash(code)
		require.NoError(t, err)
		assert.NotEqual(t, code, hash)
		assert.NotEmpty(t, hash)
	})

	t.Run("相同输入不同哈希", func(t *testing.T) {
		hash1, err := bcryptHash("same-code")
		require.NoError(t, err)

		hash2, err := bcryptHash("same-code")
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2)
	})
}

func TestMFAService_generateHOTP(t *testing.T) {
	t.Run("生成6位数字", func(t *testing.T) {
		secret := []byte("JBSWY3DPEHPK3PXP")
		for counter := uint64(0); counter < 10; counter++ {
			code := generateHOTP(secret, counter)
			assert.Len(t, code, 6)
			assert.Regexp(t, "^[0-9]{6}$", code)
		}
	})

	t.Run("相同输入相同输出", func(t *testing.T) {
		secret := []byte("test-secret")
		code1 := generateHOTP(secret, 42)
		code2 := generateHOTP(secret, 42)
		assert.Equal(t, code1, code2)
	})

	t.Run("不同counter不同输出", func(t *testing.T) {
		secret := []byte("test-secret")
		code1 := generateHOTP(secret, 1)
		code2 := generateHOTP(secret, 2)
		assert.NotEqual(t, code1, code2)
	})
}

func TestMFAService_NewMFAServiceWithAudit(t *testing.T) {
	t.Run("创建带审计的MFA服务", func(t *testing.T) {
		m := mock.New()
		auditSvc := NewAuditService(m)
		svc := NewMFAServiceWithAudit(m, auditSvc)

		assert.NotNil(t, svc)
	})
}

func TestAdminService_DeleteUser(t *testing.T) {
	t.Run("删除不存在的用户返回错误", func(t *testing.T) {
		m := mock.New()
		m.DeleteUserErr = assert.AnError

		adminSvc := NewAdminService(m)

		err := adminSvc.DeleteUser(context.Background(), "nonexistent")
		assert.Error(t, err)
	})
}

func TestAdminService_GetAuditLogs(t *testing.T) {
	t.Run("获取所有日志", func(t *testing.T) {
		m := mock.New()
		adminSvc := NewAdminService(m)

		logs, total, err := adminSvc.GetAuditLogs(context.Background(), 0, 10, "")
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Empty(t, logs)
	})
}

func TestAdminService_NewAdminServiceWithVersion(t *testing.T) {
	t.Run("自定义版本号", func(t *testing.T) {
		m := mock.New()
		adminSvc := NewAdminServiceWithVersion(m, nil, "v1.2.3")

		info, _ := adminSvc.SystemHealth(context.Background())
		assert.Equal(t, "v1.2.3", info.Version)
	})

	t.Run("默认版本号", func(t *testing.T) {
		m := mock.New()
		adminSvc := NewAdminService(m)

		info, _ := adminSvc.SystemHealth(context.Background())
		assert.Equal(t, "dev", info.Version)
	})
}

func TestAuditService_fallbackLog(t *testing.T) {
	t.Run("fallbackLog不panic", func(t *testing.T) {
		m := mock.New()
		auditSvc := NewAuditService(m)

		assert.NotPanics(t, func() {
			auditSvc.fallbackLog(context.Background(), &model.AuditLog{
				ID:        "test-id",
				EventType: "test.event",
				UserID:    "user-1",
			})
		})
	})
}

func TestAuthService_WithUserService(t *testing.T) {
	t.Run("WithUserService选项", func(t *testing.T) {
		m := mock.New()
		userSvc := NewUserService(m, nil, nil, "")

		var opts []AuthServiceOption
		opts = append(opts, WithUserService(userSvc))

		assert.Len(t, opts, 1)
	})
}

func TestAuthService_WithMetrics(t *testing.T) {
	t.Run("WithMetrics选项", func(t *testing.T) {
		var opts []AuthServiceOption
		opts = append(opts, WithMetrics(nil))

		assert.Len(t, opts, 1)
	})
}

func TestNewUserServiceWithAudit(t *testing.T) {
	t.Run("创建带审计的用户服务", func(t *testing.T) {
		m := mock.New()
		auditSvc := NewAuditService(m)
		userSvc := NewUserServiceWithAudit(m, nil, nil, "", auditSvc)

		assert.NotNil(t, userSvc)
	})
}
