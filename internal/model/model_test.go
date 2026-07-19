package model_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/model"
)

func TestToken_GetClientID(t *testing.T) {
	t.Run("ClientID非nil返回实际值", func(t *testing.T) {
		clientID := "test-client-123"
		token := &model.Token{ClientID: &clientID}
		assert.Equal(t, "test-client-123", token.GetClientID())
	})

	t.Run("ClientID为nil返回空字符串", func(t *testing.T) {
		token := &model.Token{ClientID: nil}
		assert.Equal(t, "", token.GetClientID())
	})
}

func TestUser_JSONMarshaling(t *testing.T) {
	t.Run("敏感字段PasswordHash不出现在JSON中", func(t *testing.T) {
		user := &model.User{
			ID:            "1",
			Email:         "test@example.com",
			PasswordHash:  "$2a$10$secret",
			MFASecret:     "TOTPSECRET",
			LoginAttempts: 3,
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
		}

		data, err := json.Marshal(user)
		require.NoError(t, err)

		jsonStr := string(data)
		assert.NotContains(t, jsonStr, "password_hash")
		assert.NotContains(t, jsonStr, "$2a$10$secret")
		assert.NotContains(t, jsonStr, "mfa_secret")
		assert.NotContains(t, jsonStr, "TOTPSECRET")
		assert.NotContains(t, jsonStr, "login_attempts")
		assert.NotContains(t, jsonStr, "3")
	})

	t.Run("基本字段出现在JSON中", func(t *testing.T) {
		user := &model.User{
			ID:            "1",
			Email:         "test@example.com",
			EmailVerified: true,
			MFAEnabled:    false,
			Role:          model.UserRoleUser,
			Status:        model.UserStatusActive,
		}

		data, err := json.Marshal(user)
		require.NoError(t, err)

		jsonStr := string(data)
		assert.Contains(t, jsonStr, `"id":"1"`)
		assert.Contains(t, jsonStr, `"email":"test@example.com"`)
		assert.Contains(t, jsonStr, `"email_verified":true`)
		assert.Contains(t, jsonStr, `"mfa_enabled":false`)
		assert.Contains(t, jsonStr, `"role":"user"`)
		assert.Contains(t, jsonStr, `"status":"active"`)
	})

	t.Run("LockedUntil为nil时不出现在JSON中", func(t *testing.T) {
		user := &model.User{
			ID:          "1",
			Email:       "test@example.com",
			LockedUntil: nil,
		}

		data, err := json.Marshal(user)
		require.NoError(t, err)

		assert.NotContains(t, string(data), "locked_until")
	})

	t.Run("LockedUntil非nil时出现在JSON中", func(t *testing.T) {
		lockedTime := time.Now().Add(30 * time.Minute)
		user := &model.User{
			ID:          "1",
			Email:       "test@example.com",
			LockedUntil: &lockedTime,
		}

		data, err := json.Marshal(user)
		require.NoError(t, err)

		assert.Contains(t, string(data), "locked_until")
	})

	t.Run("JSON反序列化", func(t *testing.T) {
		jsonStr := `{"id":"2","email":"user@test.com","email_verified":true,"mfa_enabled":true,"role":"admin","status":"active","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}`

		var user model.User
		err := json.Unmarshal([]byte(jsonStr), &user)
		require.NoError(t, err)

		assert.Equal(t, "2", user.ID)
		assert.Equal(t, "user@test.com", user.Email)
		assert.True(t, user.EmailVerified)
		assert.True(t, user.MFAEnabled)
		assert.Equal(t, "admin", user.Role)
		assert.Equal(t, "active", user.Status)
	})
}

func TestClient_JSONMarshaling(t *testing.T) {
	t.Run("ClientSecret不出现在JSON中", func(t *testing.T) {
		client := &model.Client{
			ID:           "1",
			ClientID:     "my-client",
			ClientSecret: "super-secret",
			Name:         "Test Client",
		}

		data, err := json.Marshal(client)
		require.NoError(t, err)

		jsonStr := string(data)
		assert.NotContains(t, jsonStr, "client_secret")
		assert.NotContains(t, jsonStr, "super-secret")
		assert.Contains(t, jsonStr, `"client_id":"my-client"`)
		assert.Contains(t, jsonStr, `"name":"Test Client"`)
	})

	t.Run("JSON反序列化", func(t *testing.T) {
		jsonStr := `{"id":"1","client_id":"app1","name":"My App","redirect_uris":["http://localhost/callback"],"grant_types":["authorization_code"],"scopes":["openid","profile"],"public_client":true}`

		var client model.Client
		err := json.Unmarshal([]byte(jsonStr), &client)
		require.NoError(t, err)

		assert.Equal(t, "1", client.ID)
		assert.Equal(t, "app1", client.ClientID)
		assert.Equal(t, "My App", client.Name)
		assert.Equal(t, []string{"http://localhost/callback"}, client.RedirectURIs)
		assert.True(t, client.PublicClient)
	})
}

func TestToken_JSONMarshaling(t *testing.T) {
	t.Run("ClientID为nil时omitempty", func(t *testing.T) {
		token := &model.Token{
			ID:           "1",
			AccessToken:  "access-123",
			RefreshToken: "refresh-456",
			UserID:       "user-1",
			ClientID:     nil,
		}

		data, err := json.Marshal(token)
		require.NoError(t, err)

		assert.NotContains(t, string(data), "client_id")
	})

	t.Run("ClientID非nil时出现在JSON中", func(t *testing.T) {
		clientID := "client-123"
		token := &model.Token{
			ID:       "1",
			ClientID: &clientID,
		}

		data, err := json.Marshal(token)
		require.NoError(t, err)

		assert.Contains(t, string(data), `"client_id":"client-123"`)
	})

	t.Run("RevokedAt为nil时出现在JSON中为null", func(t *testing.T) {
		token := &model.Token{
			ID:           "1",
			AccessToken:  "access",
			RefreshToken: "refresh",
			UserID:       "user-1",
			RevokedAt:    nil,
		}

		data, err := json.Marshal(token)
		require.NoError(t, err)

		jsonStr := string(data)
		assert.Contains(t, jsonStr, `"revoked_at":null`)
	})

	t.Run("RevokedAt非nil时出现在JSON中", func(t *testing.T) {
		revokedTime := time.Now()
		token := &model.Token{
			ID:        "1",
			RevokedAt: &revokedTime,
		}

		data, err := json.Marshal(token)
		require.NoError(t, err)

		assert.Contains(t, string(data), "revoked_at")
	})
}

func TestAuthorizationCode_JSONMarshaling(t *testing.T) {
	t.Run("UsedAt为nil时出现在JSON中为null", func(t *testing.T) {
		code := &model.AuthorizationCode{
			Code:     "auth-code-123",
			ClientID: "client-1",
			UserID:   "user-1",
			UsedAt:   nil,
		}

		data, err := json.Marshal(code)
		require.NoError(t, err)

		jsonStr := string(data)
		assert.Contains(t, jsonStr, `"used_at":null`)
	})

	t.Run("UsedAt非nil时出现在JSON中", func(t *testing.T) {
		usedTime := time.Now()
		code := &model.AuthorizationCode{
			Code:   "auth-code-123",
			UsedAt: &usedTime,
		}

		data, err := json.Marshal(code)
		require.NoError(t, err)

		assert.Contains(t, string(data), "used_at")
	})
}

func TestLoginResponse_JSONMarshaling(t *testing.T) {
	t.Run("Scopes为空时omitempty", func(t *testing.T) {
		resp := &model.LoginResponse{
			AccessToken:  "access-123",
			RefreshToken: "refresh-456",
			TokenType:    "Bearer",
			ExpiresIn:    900,
			Scopes:       []string{},
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		assert.NotContains(t, string(data), "scopes")
	})

	t.Run("Scopes非空时出现在JSON中", func(t *testing.T) {
		resp := &model.LoginResponse{
			AccessToken:  "access-123",
			RefreshToken: "refresh-456",
			TokenType:    "Bearer",
			ExpiresIn:    900,
			Scopes:       []string{"openid", "profile"},
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		assert.Contains(t, string(data), `"scopes":["openid","profile"]`)
	})
}

func TestErrorResponse_JSONMarshaling(t *testing.T) {
	t.Run("序列化", func(t *testing.T) {
		resp := model.ErrorResponse{Error: "INVALID_CREDENTIALS"}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		assert.Equal(t, `{"error":"INVALID_CREDENTIALS"}`, string(data))
	})

	t.Run("反序列化", func(t *testing.T) {
		jsonStr := `{"error":"SOME_ERROR"}`

		var resp model.ErrorResponse
		err := json.Unmarshal([]byte(jsonStr), &resp)
		require.NoError(t, err)

		assert.Equal(t, "SOME_ERROR", resp.Error)
	})
}

func TestSuccessResponse_JSONMarshaling(t *testing.T) {
	t.Run("Data为nil时omitempty", func(t *testing.T) {
		resp := model.SuccessResponse{Message: "ok", Data: nil}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		assert.NotContains(t, string(data), "data")
	})

	t.Run("Data非nil时出现在JSON中", func(t *testing.T) {
		resp := model.SuccessResponse{
			Message: "success",
			Data:    map[string]string{"id": "1"},
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		assert.Contains(t, string(data), `"data"`)
	})
}

func TestMFAModels_JSONMarshaling(t *testing.T) {
	t.Run("MFASetupResponse序列化", func(t *testing.T) {
		resp := model.MFASetupResponse{
			Secret:      "JBSWY3DPEHPK3PXP",
			QRCodeURL:   "otpauth://totp/SSO:user@example.com?secret=JBSWY3DPEHPK3PXP",
			ManualEntry: "JBSW Y3DP EHPK 3PXP",
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		jsonStr := string(data)
		assert.Contains(t, jsonStr, `"secret":"JBSWY3DPEHPK3PXP"`)
		assert.Contains(t, jsonStr, `"qr_code_url"`)
		assert.Contains(t, jsonStr, `"manual_entry"`)
	})

	t.Run("MFAStatusResponse序列化", func(t *testing.T) {
		resp := model.MFAStatusResponse{Enabled: true}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		assert.Equal(t, `{"enabled":true}`, string(data))
	})
}

func TestAuditLog_JSONMarshaling(t *testing.T) {
	t.Run("序列化", func(t *testing.T) {
		log := model.AuditLog{
			ID:        "log-1",
			EventType: string(model.EventUserLogin),
			UserID:    "user-1",
			IPAddress: "192.168.1.1",
			Success:   true,
		}

		data, err := json.Marshal(log)
		require.NoError(t, err)

		jsonStr := string(data)
		assert.Contains(t, jsonStr, `"event_type":"user.login"`)
		assert.Contains(t, jsonStr, `"user_id":"user-1"`)
		assert.Contains(t, jsonStr, `"ip_address":"192.168.1.1"`)
		assert.Contains(t, jsonStr, `"success":true`)
	})
}

func TestUserStatusConstants(t *testing.T) {
	t.Run("UserStatus常量值", func(t *testing.T) {
		assert.Equal(t, "active", model.UserStatusActive)
		assert.Equal(t, "locked", model.UserStatusLocked)
		assert.Equal(t, "disabled", model.UserStatusDisabled)
	})

	t.Run("UserRole常量值", func(t *testing.T) {
		assert.Equal(t, "user", model.UserRoleUser)
		assert.Equal(t, "admin", model.UserRoleAdmin)
	})

	t.Run("GrantType常量值", func(t *testing.T) {
		assert.Equal(t, "authorization_code", model.GrantTypeAuthorizationCode)
		assert.Equal(t, "refresh_token", model.GrantTypeRefreshToken)
		assert.Equal(t, "client_credentials", model.GrantTypeClientCredentials)
	})
}

func TestAuditEventTypeConstants(t *testing.T) {
	t.Run("User事件类型", func(t *testing.T) {
		assert.Equal(t, "user.register", string(model.EventUserRegister))
		assert.Equal(t, "user.login", string(model.EventUserLogin))
		assert.Equal(t, "user.login_failed", string(model.EventUserLoginFailed))
		assert.Equal(t, "user.logout", string(model.EventUserLogout))
		assert.Equal(t, "user.locked", string(model.EventUserLocked))
		assert.Equal(t, "user.unlocked", string(model.EventUserUnlocked))
		assert.Equal(t, "user.logout_all", string(model.EventLogoutAll))
	})

	t.Run("Token事件类型", func(t *testing.T) {
		assert.Equal(t, "token.issued", string(model.EventTokenIssued))
		assert.Equal(t, "token.refresh", string(model.EventTokenRefresh))
		assert.Equal(t, "token.revoke", string(model.EventTokenRevoke))
	})

	t.Run("OAuth事件类型", func(t *testing.T) {
		assert.Equal(t, "oauth.code_created", string(model.EventAuthCodeCreated))
		assert.Equal(t, "oauth.code_used", string(model.EventAuthCodeUsed))
		assert.Equal(t, "oauth.code_invalid", string(model.EventAuthCodeInvalid))
	})

	t.Run("Security事件类型", func(t *testing.T) {
		assert.Equal(t, "security.rate_limit", string(model.EventRateLimitExceeded))
		assert.Equal(t, "security.suspicious", string(model.EventSuspiciousActivity))
		assert.Equal(t, "security.password_changed", string(model.EventPasswordChanged))
		assert.Equal(t, "security.password_reset", string(model.EventPasswordReset))
		assert.Equal(t, "security.account_locked", string(model.EventAccountLocked))
		assert.Equal(t, "security.account_unlocked", string(model.EventAccountUnlocked))
	})

	t.Run("MFA事件类型", func(t *testing.T) {
		assert.Equal(t, "mfa.setup", string(model.EventMFASetup))
		assert.Equal(t, "mfa.enabled", string(model.EventMFAEnabled))
		assert.Equal(t, "mfa.disabled", string(model.EventMFADisabled))
	})

	t.Run("Key事件类型", func(t *testing.T) {
		assert.Equal(t, "key.rotated", string(model.EventKeyRotated))
		assert.Equal(t, "key.revoked", string(model.EventKeyRevoked))
	})

	t.Run("System事件类型", func(t *testing.T) {
		assert.Equal(t, "system.start", string(model.EventSystemStart))
		assert.Equal(t, "system.stop", string(model.EventSystemStop))
	})
}

func TestRequestModels_JSONMarshaling(t *testing.T) {
	t.Run("RegisterRequest", func(t *testing.T) {
		req := model.RegisterRequest{Email: "test@example.com", Password: "secret"}
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"email":"test@example.com"`)

		var decoded model.RegisterRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, req.Email, decoded.Email)
	})

	t.Run("LoginRequest", func(t *testing.T) {
		req := model.LoginRequest{Email: "test@example.com", Password: "secret"}
		data, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded model.LoginRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, req.Email, decoded.Email)
	})

	t.Run("AuthorizeRequest", func(t *testing.T) {
		req := model.AuthorizeRequest{
			ClientID:            "client-1",
			RedirectURI:         "http://localhost/callback",
			ResponseType:        "code",
			Scope:               "openid profile",
			State:               "random-state",
			CodeChallenge:       "challenge",
			CodeChallengeMethod: "S256",
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded model.AuthorizeRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, req.ClientID, decoded.ClientID)
		assert.Equal(t, req.CodeChallengeMethod, decoded.CodeChallengeMethod)
	})

	t.Run("TokenRequest", func(t *testing.T) {
		req := model.TokenRequest{
			GrantType:    "authorization_code",
			Code:         "auth-code",
			RedirectURI:  "http://localhost/callback",
			ClientID:     "client-1",
			ClientSecret: "secret",
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded model.TokenRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, req.GrantType, decoded.GrantType)
	})
}

func TestUser_DBTags(t *testing.T) {
	t.Run("User结构体字段可访问", func(t *testing.T) {
		user := model.User{
			ID:            "1",
			Email:         "test@example.com",
			PasswordHash:  "hash",
			EmailVerified: true,
			MFAEnabled:    false,
			MFASecret:     "",
			Role:          "user",
			Status:        "active",
			LoginAttempts: 0,
			LockedUntil:   nil,
		}

		assert.Equal(t, "1", user.ID)
		assert.Equal(t, "test@example.com", user.Email)
		assert.Equal(t, "hash", user.PasswordHash)
		assert.True(t, user.EmailVerified)
		assert.False(t, user.MFAEnabled)
		assert.Equal(t, "", user.MFASecret)
		assert.Equal(t, "user", user.Role)
		assert.Equal(t, "active", user.Status)
		assert.Equal(t, 0, user.LoginAttempts)
		assert.Nil(t, user.LockedUntil)
	})
}

func TestJSONNoSensitiveDataLeak(t *testing.T) {
	t.Run("User完整JSON输出不包含敏感数据", func(t *testing.T) {
		user := &model.User{
			ID:            "1",
			Email:         "user@test.com",
			PasswordHash:  "$2a$10$hash",
			EmailVerified: true,
			MFAEnabled:    true,
			MFASecret:     "secret123",
			Role:          "admin",
			Status:        "active",
			LoginAttempts: 5,
		}

		data, err := json.Marshal(user)
		require.NoError(t, err)

		jsonStr := string(data)
		for _, sensitive := range []string{"$2a$10", "secret123", "password_hash", "mfa_secret", "login_attempts"} {
			assert.False(t, strings.Contains(jsonStr, sensitive), "JSON包含敏感数据: %s", sensitive)
		}
	})

	t.Run("Client完整JSON输出不包含密钥", func(t *testing.T) {
		client := &model.Client{
			ID:           "1",
			ClientID:     "app1",
			ClientSecret: "very-secret-value",
			Name:         "Test App",
		}

		data, err := json.Marshal(client)
		require.NoError(t, err)

		assert.NotContains(t, string(data), "very-secret-value")
		assert.NotContains(t, string(data), "client_secret")
	})
}
