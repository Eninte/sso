// Package handler_test Handler层边界条件测试
// 测试邮箱验证、Token刷新、MFA等Handler的边界条件
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/your-org/sso/internal/crypto"
	"github.com/your-org/sso/internal/handler"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestUserHandler 创建测试用的用户处理器
func createTestUserHandler(t *testing.T) (*handler.UserHandler, *mock.Store) {
	store := mock.New()
	passwordSvc := crypto.NewPasswordService(4)

	// 使用nil的emailSvc，测试时不实际发送邮件
	var emailSvc *service.EmailService

	userSvc := service.NewUserService(store, passwordSvc, emailSvc, "http://localhost:9090")
	userHandler := handler.NewUserHandler(userSvc, testCaptchaSvc)

	return userHandler, store
}

// createTestMFAHandler 创建测试用的MFA处理器
func createTestMFAHandler(t *testing.T) (*handler.MFAHandler, *mock.Store) {
	store := mock.New()
	mfaSvc := service.NewMFAService(store)
	mfaHandler := handler.NewMFAHandler(mfaSvc)

	return mfaHandler, store
}

// ============================================================================
// 邮箱验证Handler边界条件测试
// ============================================================================

func TestUserHandler_VerifyEmail_BoundaryConditions(t *testing.T) {
	userHandler, store := createTestUserHandler(t)

	// 创建测试用户
	testUser := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  "hash",
		EmailVerified: false,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(testUser)

	t.Run("无效token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/verify-email?token=invalid-token&user_id=test-user-id", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少token参数", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/verify-email?user_id=test-user-id", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少user_id参数", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/verify-email?token=some-token", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空token参数", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/verify-email?token=&user_id=test-user-id", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空user_id参数", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/verify-email?token=some-token&user_id=", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("用户不存在", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/verify-email?token=some-token&user_id=nonexistent-user", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ============================================================================
// Token刷新Handler边界条件测试
// ============================================================================

func TestTokenHandler_RefreshToken_BoundaryConditions(t *testing.T) {
	tokenHandler, store := createTestTokenHandler(t)

	// 创建测试用户
	passwordSvc := crypto.NewPasswordService(4)
	hashedPassword, _ := passwordSvc.HashPassword("Password123!")

	user := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(user)

	t.Run("空refresh_token", func(t *testing.T) {
		body := map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": "",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("格式错误的refresh_token", func(t *testing.T) {
		body := map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": "not-a-valid-token-format",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("超长refresh_token", func(t *testing.T) {
		// 创建一个超长的token（正常token应该是固定长度）
		longToken := string(make([]byte, 10000))
		body := map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": longToken,
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("malformed请求体-无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/token", bytes.NewReader([]byte("not valid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("malformed请求体-空请求", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/token", nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleToken(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ============================================================================
// MFA Handler边界条件测试
// ============================================================================

func TestMFAHandler_VerifyMFA_BoundaryConditions(t *testing.T) {
	mfaHandler, store := createTestMFAHandler(t)

	// 创建测试用户
	testUser := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  "hash",
		EmailVerified: true,
		Status:        model.UserStatusActive,
		MFAEnabled:    false,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(testUser)

	t.Run("无效TOTP代码-空字符串", func(t *testing.T) {
		body := map[string]string{
			"code": "",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/mfa/verify", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		mfaHandler.HandleVerifyMFA(w, req.WithContext(ctx))

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("无效TOTP代码-错误格式", func(t *testing.T) {
		body := map[string]string{
			"code": "abc123", // TOTP应该是6位数字
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/mfa/verify", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		mfaHandler.HandleVerifyMFA(w, req.WithContext(ctx))

		// 应该返回错误（具体错误码取决于验证逻辑）
		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("无效TOTP代码-长度不正确", func(t *testing.T) {
		body := map[string]string{
			"code": "12345", // 只有5位
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/mfa/verify", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		mfaHandler.HandleVerifyMFA(w, req.WithContext(ctx))

		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("malformed请求体-无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/mfa/verify", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		mfaHandler.HandleVerifyMFA(w, req.WithContext(ctx))

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("malformed请求体-空请求", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/mfa/verify", nil)
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		mfaHandler.HandleVerifyMFA(w, req.WithContext(ctx))

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("未认证用户", func(t *testing.T) {
		body := map[string]string{
			"code": "123456",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/mfa/verify", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mfaHandler.HandleVerifyMFA(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestMFAHandler_DisableMFA_BoundaryConditions(t *testing.T) {
	mfaHandler, store := createTestMFAHandler(t)

	// 创建启用了MFA的测试用户
	testUser := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  "hash",
		EmailVerified: true,
		Status:        model.UserStatusActive,
		MFAEnabled:    true,
		MFASecret:     "JBSWY3DPEHPK3PXP",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(testUser)

	t.Run("无效TOTP代码", func(t *testing.T) {
		body := map[string]string{
			"code": "000000", // 错误的代码
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/mfa/disable", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		mfaHandler.HandleDisableMFA(w, req.WithContext(ctx))

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空TOTP代码", func(t *testing.T) {
		body := map[string]string{
			"code": "",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/mfa/disable", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		mfaHandler.HandleDisableMFA(w, req.WithContext(ctx))

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("malformed请求体", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/mfa/disable", bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		mfaHandler.HandleDisableMFA(w, req.WithContext(ctx))

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ============================================================================
// 其他Handler的malformed请求体测试
// ============================================================================

func TestUserHandler_ForgotPassword_MalformedRequest(t *testing.T) {
	userHandler, _ := createTestUserHandler(t)

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/forgot-password", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleForgotPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空请求体", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/forgot-password", nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleForgotPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少email字段", func(t *testing.T) {
		body := map[string]string{}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/forgot-password", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleForgotPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空email字段", func(t *testing.T) {
		body := map[string]string{
			"email": "",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/forgot-password", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleForgotPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestUserHandler_ResetPassword_MalformedRequest(t *testing.T) {
	userHandler, _ := createTestUserHandler(t)

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/reset-password", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleResetPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空请求体", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/reset-password", nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleResetPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少token字段", func(t *testing.T) {
		body := map[string]string{
			"user_id":      "test-user",
			"new_password": "NewPass123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/reset-password", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleResetPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少user_id字段", func(t *testing.T) {
		body := map[string]string{
			"token":        "some-token",
			"new_password": "NewPass123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/reset-password", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleResetPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少new_password字段", func(t *testing.T) {
		body := map[string]string{
			"token":   "some-token",
			"user_id": "test-user",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/reset-password", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleResetPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestUserHandler_ChangePassword_MalformedRequest(t *testing.T) {
	userHandler, _ := createTestUserHandler(t)

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/change-password", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		userHandler.HandleChangePassword(w, req.WithContext(ctx))

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空请求体", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/change-password", nil)
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		userHandler.HandleChangePassword(w, req.WithContext(ctx))

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少old_password字段", func(t *testing.T) {
		body := map[string]string{
			"new_password": "NewPass123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/change-password", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		userHandler.HandleChangePassword(w, req.WithContext(ctx))

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少new_password字段", func(t *testing.T) {
		body := map[string]string{
			"old_password": "OldPass123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/change-password", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
		w := httptest.NewRecorder()

		userHandler.HandleChangePassword(w, req.WithContext(ctx))

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("未认证用户", func(t *testing.T) {
		body := map[string]string{
			"old_password": "OldPass123!",
			"new_password": "NewPass123!",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/change-password", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleChangePassword(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestTokenHandler_Revoke_MalformedRequest(t *testing.T) {
	tokenHandler, _ := createTestTokenHandler(t)

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/token/revoke", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleRevoke(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空请求体", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/token/revoke", nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleRevoke(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空token字段", func(t *testing.T) {
		body := map[string]string{
			"token": "",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/token/revoke", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tokenHandler.HandleRevoke(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestMFAHandler_SetupMFA_MalformedRequest(t *testing.T) {
	mfaHandler, store := createTestMFAHandler(t)

	// 创建测试用户
	testUser := &model.User{
		ID:            "test-user-id",
		Email:         "test@example.com",
		PasswordHash:  "hash",
		EmailVerified: true,
		Status:        model.UserStatusActive,
		MFAEnabled:    false,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(testUser)

	t.Run("未认证用户", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/mfa/setup", nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mfaHandler.HandleSetupMFA(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
