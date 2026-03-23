// Package handler_test User和MFA Handler单元测试
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
	"github.com/stretchr/testify/require"

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

// createContextWithUserID 创建带有用户ID的请求上下文
func createContextWithUserID(userID string) context.Context {
	return context.WithValue(context.Background(), middleware.UserIDKey, userID)
}

// createUserHandler 创建测试用的用户处理器
func createUserHandler(t *testing.T) (*handler.UserHandler, *mock.MockStore) {
	storeInst := mock.New()
	passwordSvc := crypto.NewPasswordService(10)
	emailSvc := service.NewEmailService(&service.EmailConfig{
		SMTPHost: "localhost",
		SMTPPort: 1,
		From:     "test@example.com",
	})
	userSvc := service.NewUserService(storeInst, passwordSvc, emailSvc, "http://localhost:9090")
	return handler.NewUserHandler(userSvc), storeInst
}

// createMFAHandler 创建测试用的MFA处理器
func createMFAHandler(t *testing.T) (*handler.MFAHandler, *mock.MockStore) {
	storeInst := mock.New()
	mfaSvc := service.NewMFAService(storeInst)
	return handler.NewMFAHandler(mfaSvc), storeInst
}

// ============================================================================
// UserHandler 测试
// ============================================================================

func TestUserHandler_HandleVerifyEmail(t *testing.T) {
	userHandler, storeInst := createUserHandler(t)

	t.Run("缺少参数", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/verify-email", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("仅缺少token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/verify-email?user_id=user1", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("仅缺少user_id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/verify-email?token=abc", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("无效的验证token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/verify-email?token=wrong&user_id=user1", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("验证token不匹配", func(t *testing.T) {
		storeInst.Reset()

		storeInst.AddUser(&model.User{
			ID:           "verify-user",
			Email:        "verify@example.com",
			Status:       model.UserStatusActive,
			PasswordHash: "dummy",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		})
		_ = storeInst.StoreVerificationToken(context.Background(), "verify-user", "correct-token", time.Now().Add(24*time.Hour))

		req := httptest.NewRequest("GET", "/api/v1/verify-email?token=wrong-token&user_id=verify-user", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("验证成功", func(t *testing.T) {
		storeInst.Reset()

		storeInst.AddUser(&model.User{
			ID:           "verify-user-ok",
			Email:        "verifyok@example.com",
			Status:       model.UserStatusActive,
			PasswordHash: "dummy",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		})
		_ = storeInst.StoreVerificationToken(context.Background(), "verify-user-ok", "valid-token", time.Now().Add(24*time.Hour))

		req := httptest.NewRequest("GET", "/api/v1/verify-email?token=valid-token&user_id=verify-user-ok", nil)
		w := httptest.NewRecorder()

		userHandler.HandleVerifyEmail(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestUserHandler_HandleSendVerificationEmail(t *testing.T) {
	userHandler, storeInst := createUserHandler(t)

	t.Run("未认证", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/verify-email/send", nil)
		w := httptest.NewRecorder()

		userHandler.HandleSendVerificationEmail(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("用户不存在", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/verify-email/send", nil)
		req = req.WithContext(createContextWithUserID("nonexistent"))
		w := httptest.NewRecorder()

		userHandler.HandleSendVerificationEmail(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("邮箱已验证", func(t *testing.T) {
		storeInst.Reset()

		storeInst.AddUser(&model.User{
			ID:            "verified-user",
			Email:         "verified@example.com",
			EmailVerified: true,
			PasswordHash:  "dummy",
			Status:        model.UserStatusActive,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})

		req := httptest.NewRequest("POST", "/api/v1/verify-email/send", nil)
		req = req.WithContext(createContextWithUserID("verified-user"))
		w := httptest.NewRecorder()

		userHandler.HandleSendVerificationEmail(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestUserHandler_HandleForgotPassword(t *testing.T) {
	userHandler, _ := createUserHandler(t)

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/forgot-password", bytes.NewReader([]byte("invalid")))
		w := httptest.NewRecorder()

		userHandler.HandleForgotPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("空邮箱", func(t *testing.T) {
		body := map[string]string{"email": ""}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/forgot-password", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleForgotPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("邮箱不存在-安全返回成功", func(t *testing.T) {
		body := map[string]string{"email": "nonexistent@example.com"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/forgot-password", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		userHandler.HandleForgotPassword(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestUserHandler_HandleResetPassword(t *testing.T) {
	userHandler, _ := createUserHandler(t)

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/reset-password", bytes.NewReader([]byte("invalid")))
		w := httptest.NewRecorder()

		userHandler.HandleResetPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少token", func(t *testing.T) {
		body := map[string]string{"user_id": "user1", "new_password": "NewPass123!"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/reset-password", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()

		userHandler.HandleResetPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少user_id", func(t *testing.T) {
		body := map[string]string{"token": "abc", "new_password": "NewPass123!"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/reset-password", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()

		userHandler.HandleResetPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少new_password", func(t *testing.T) {
		body := map[string]string{"token": "abc", "user_id": "user1"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/reset-password", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()

		userHandler.HandleResetPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("无效的重置token", func(t *testing.T) {
		body := map[string]string{"token": "wrong", "user_id": "user1", "new_password": "NewPass123!"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/reset-password", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()

		userHandler.HandleResetPassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestUserHandler_HandleChangePassword(t *testing.T) {
	userHandler, storeInst := createUserHandler(t)

	t.Run("未认证", func(t *testing.T) {
		body := map[string]string{"old_password": "Old1234!", "new_password": "New1234!"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/change-password", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()

		userHandler.HandleChangePassword(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/change-password", bytes.NewReader([]byte("invalid")))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		userHandler.HandleChangePassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少旧密码", func(t *testing.T) {
		body := map[string]string{"old_password": "", "new_password": "New1234!"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/change-password", bytes.NewReader(bodyBytes))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		userHandler.HandleChangePassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("缺少新密码", func(t *testing.T) {
		body := map[string]string{"old_password": "Old1234!", "new_password": ""}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/change-password", bytes.NewReader(bodyBytes))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		userHandler.HandleChangePassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("旧密码错误", func(t *testing.T) {
		storeInst.Reset()

		hashedPw, _ := crypto.NewPasswordService(10).HashPassword("CorrectPass1!")
		storeInst.AddUser(&model.User{
			ID:           "pwd-user",
			Email:        "pwd@example.com",
			PasswordHash: hashedPw,
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		})

		body := map[string]string{"old_password": "WrongPass1!", "new_password": "NewPass123!"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/change-password", bytes.NewReader(bodyBytes))
		req = req.WithContext(createContextWithUserID("pwd-user"))
		w := httptest.NewRecorder()

		userHandler.HandleChangePassword(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ============================================================================
// MFAHandler 测试
// ============================================================================

func TestMFAHandler_HandleSetupMFA(t *testing.T) {
	mfaHandler, storeInst := createMFAHandler(t)

	t.Run("未认证", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/mfa/setup", nil)
		w := httptest.NewRecorder()

		mfaHandler.HandleSetupMFA(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("用户不存在", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/mfa/setup", nil)
		req = req.WithContext(createContextWithUserID("nonexistent"))
		w := httptest.NewRecorder()

		mfaHandler.HandleSetupMFA(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("MFA已启用-返回409", func(t *testing.T) {
		storeInst.Reset()

		storeInst.AddUser(&model.User{
			ID:           "mfa-enabled-user",
			Email:        "mfa@example.com",
			MFAEnabled:   true,
			MFASecret:    "existing-secret",
			PasswordHash: "dummy",
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		})

		req := httptest.NewRequest("POST", "/api/v1/mfa/setup", nil)
		req = req.WithContext(createContextWithUserID("mfa-enabled-user"))
		w := httptest.NewRecorder()

		mfaHandler.HandleSetupMFA(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("MFA设置成功", func(t *testing.T) {
		storeInst.Reset()

		storeInst.AddUser(&model.User{
			ID:           "mfa-setup-user",
			Email:        "mfasetup@example.com",
			MFAEnabled:   false,
			PasswordHash: "dummy",
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		})

		req := httptest.NewRequest("POST", "/api/v1/mfa/setup", nil)
		req = req.WithContext(createContextWithUserID("mfa-setup-user"))
		w := httptest.NewRecorder()

		mfaHandler.HandleSetupMFA(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp["secret"])
		assert.NotEmpty(t, resp["qr_code_url"])
	})
}

func TestMFAHandler_HandleVerifyMFA(t *testing.T) {
	mfaHandler, _ := createMFAHandler(t)

	t.Run("未认证", func(t *testing.T) {
		body := map[string]string{"code": "123456"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/mfa/verify", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()

		mfaHandler.HandleVerifyMFA(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/mfa/verify", bytes.NewReader([]byte("invalid")))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		mfaHandler.HandleVerifyMFA(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("验证码为空", func(t *testing.T) {
		body := map[string]string{"code": ""}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/mfa/verify", bytes.NewReader(bodyBytes))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		mfaHandler.HandleVerifyMFA(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestMFAHandler_HandleDisableMFA(t *testing.T) {
	mfaHandler, storeInst := createMFAHandler(t)

	t.Run("未认证", func(t *testing.T) {
		body := map[string]string{"code": "123456"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/mfa/disable", bytes.NewReader(bodyBytes))
		w := httptest.NewRecorder()

		mfaHandler.HandleDisableMFA(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("无效JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/mfa/disable", bytes.NewReader([]byte("invalid")))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		mfaHandler.HandleDisableMFA(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("验证码为空", func(t *testing.T) {
		body := map[string]string{"code": ""}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/mfa/disable", bytes.NewReader(bodyBytes))
		req = req.WithContext(createContextWithUserID("user1"))
		w := httptest.NewRecorder()

		mfaHandler.HandleDisableMFA(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("MFA未启用", func(t *testing.T) {
		storeInst.Reset()

		storeInst.AddUser(&model.User{
			ID:           "no-mfa-user",
			Email:        "nomfa@example.com",
			MFAEnabled:   false,
			PasswordHash: "dummy",
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		})

		body := map[string]string{"code": "123456"}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/mfa/disable", bytes.NewReader(bodyBytes))
		req = req.WithContext(createContextWithUserID("no-mfa-user"))
		w := httptest.NewRecorder()

		mfaHandler.HandleDisableMFA(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestMFAHandler_HandleMFAStatus(t *testing.T) {
	t.Run("未认证", func(t *testing.T) {
		mfaHandler, _ := createMFAHandler(t)

		req := httptest.NewRequest("GET", "/api/v1/mfa/status", nil)
		w := httptest.NewRecorder()

		mfaHandler.HandleMFAStatus(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("获取MFA状态-MFA未启用", func(t *testing.T) {
		storeInst := mock.New()
		mfaSvc := service.NewMFAService(storeInst)
		h := handler.NewMFAHandler(mfaSvc)

		storeInst.AddUser(&model.User{
			ID:           "status-user",
			Email:        "status@example.com",
			MFAEnabled:   false,
			PasswordHash: "dummy",
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		})

		req := httptest.NewRequest("GET", "/api/v1/mfa/status", nil)
		req = req.WithContext(createContextWithUserID("status-user"))
		w := httptest.NewRecorder()

		h.HandleMFAStatus(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp model.MFAStatusResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Enabled)
	})

	t.Run("获取MFA状态-MFA已启用", func(t *testing.T) {
		storeInst := mock.New()
		mfaSvc := service.NewMFAService(storeInst)
		h := handler.NewMFAHandler(mfaSvc)

		storeInst.AddUser(&model.User{
			ID:           "mfa-on-user",
			Email:        "mfaon@example.com",
			MFAEnabled:   true,
			PasswordHash: "dummy",
			Status:       model.UserStatusActive,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		})

		req := httptest.NewRequest("GET", "/api/v1/mfa/status", nil)
		req = req.WithContext(createContextWithUserID("mfa-on-user"))
		w := httptest.NewRecorder()

		h.HandleMFAStatus(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp model.MFAStatusResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Enabled)
	})
}
