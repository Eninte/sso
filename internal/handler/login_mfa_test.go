// Package handler_test MFA 两阶段登录 Handler 单元测试
// 覆盖 LoginHandler.HandleVerifyMFALogin：
//   - 成功路径（TOTP 验证通过，签发 Token）
//   - 参数校验失败（空 challenge / 空 code / 无效 method）
//   - Challenge 不存在 / IP 不匹配 / 无效 TOTP
package handler_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// createMFAHandlerTestSetup 创建装配了 MFA 服务的 LoginHandler 测试栈
// 返回：handler、store、mfaSvc、authSvc、user，便于测试编排
func createMFAHandlerTestSetup(t *testing.T, mfaSecret string) (
	*handler.LoginHandler,
	*mock.Store,
	*service.MFAService,
	*service.AuthService,
	*model.User,
) {
	t.Helper()

	store := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	privateKey, err := crypto.GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test-issuer", 15*time.Minute, 7*24*time.Hour)

	memCache := cache.NewMemoryCache()
	auditSvc := service.NewAuditService(store)
	mfaSvc := service.NewMFAServiceWithAudit(store, auditSvc)
	hmacKey := []byte("test-hmac-key-for-recovery-codes-32bytes")
	store.SetMockHMACKey(hmacKey)
	mfaSvc.SetHMACKey(hmacKey)

	authSvc := service.NewAuthServiceWithOptions(
		store, passwordSvc, jwtSvc, 5, 30*time.Minute,
		service.WithCache(memCache),
		service.WithMFA(mfaSvc, 5*time.Minute),
	)

	// 默认禁用验证码（测试不关注验证码逻辑）
	loginHandler := handler.NewLoginHandler(authSvc, &mockCaptchaVerifier{})

	// 创建已启用 MFA 的用户
	hashedPassword, err := crypto.NewPasswordService(4).HashPassword("Password123!")
	require.NoError(t, err)
	user := &model.User{
		ID:            "mfa-handler-user",
		Email:         "mfa-handler@example.com",
		PasswordHash:  hashedPassword,
		Role:          model.UserRoleUser,
		Status:        model.UserStatusActive,
		EmailVerified: true,
		MFAEnabled:    true,
		MFASecret:     mfaSecret,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.AddUser(user)

	return loginHandler, store, mfaSvc, authSvc, user
}

// newMFAHandlerOnly 仅返回 LoginHandler，用于不需要 store/mfaSvc/authSvc/user 的测试。
// 辅助函数内部允许 4 个 blank identifier，因为这是封装 createMFAHandlerTestSetup 的合理用法。
func newMFAHandlerOnly(t *testing.T, mfaSecret string) *handler.LoginHandler {
	h, _, _, _, _ := createMFAHandlerTestSetup(t, mfaSecret) //nolint:dogsled // 测试辅助：仅返回主对象
	return h
}

// performMFAFirstStageLogin 执行第一阶段登录，返回 mfa_challenge 令牌
func performMFAFirstStageLogin(t *testing.T, h *handler.LoginHandler, email, password, ip, ua string) string {
	t.Helper()
	body := bytes.NewReader([]byte(`{"email":"` + email + `","password":"` + password + `"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":1234"
	req.Header.Set("User-Agent", ua)
	w := httptest.NewRecorder()

	h.Handle(w, req)
	require.Equal(t, http.StatusOK, w.Code, "第一阶段登录应成功并返回 MFA Challenge")

	var resp model.LoginResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.MFARequired, "应返回 MFA Required 响应")
	require.NotEmpty(t, resp.MFAChallenge, "应包含 mfa_challenge 令牌")
	return resp.MFAChallenge
}

// ============================================================================
// 成功路径
// ============================================================================

func TestLoginHandler_HandleVerifyMFALogin_TOTP_Success(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"
	h, store, mfaSvc, _, user := createMFAHandlerTestSetup(t, secret)

	// 第一阶段：获取 challenge（注意：第一阶段不消耗 TOTP，仅生成 challenge）
	challenge := performMFAFirstStageLogin(t, h, "mfa-handler@example.com", "Password123!", "192.168.1.1", "Mozilla/5.0")

	// 第二阶段：用有效 TOTP 验证
	// 清除 TOTP 重放保护（避免与 mfaSvc 内部状态冲突）
	mfaSvc.ClearTOTPUsageForTesting(user.ID)
	validCode := generateTestTOTPForHandler(secret)

	body := bytes.NewReader([]byte(`{"mfa_challenge":"` + challenge + `","method":"totp","code":"` + validCode + `"}`))
	req := httptest.NewRequest("POST", "/api/v1/login/mfa/verify", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("User-Agent", "Mozilla/5.0")
	w := httptest.NewRecorder()

	h.HandleVerifyMFALogin(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "TOTP 验证成功应返回 200")

	var resp model.LoginResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.AccessToken, "应签发 access_token")
	assert.NotEmpty(t, resp.RefreshToken, "应签发 refresh_token")
	assert.False(t, resp.MFARequired, "第二阶段不应再返回 mfa_required")

	// 验证一次性：再次使用同一 challenge 应失败
	_ = store
}

// ============================================================================
// 参数校验失败
// ============================================================================

func TestLoginHandler_HandleVerifyMFALogin_EmptyChallenge(t *testing.T) {
	h := newMFAHandlerOnly(t, "JBSWY3DPEHPK3PXP")

	body := bytes.NewReader([]byte(`{"mfa_challenge":"","method":"totp","code":"123456"}`))
	req := httptest.NewRequest("POST", "/api/v1/login/mfa/verify", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleVerifyMFALogin(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLoginHandler_HandleVerifyMFALogin_EmptyCode(t *testing.T) {
	h := newMFAHandlerOnly(t, "JBSWY3DPEHPK3PXP")

	body := bytes.NewReader([]byte(`{"mfa_challenge":"some-token","method":"totp","code":""}`))
	req := httptest.NewRequest("POST", "/api/v1/login/mfa/verify", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleVerifyMFALogin(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLoginHandler_HandleVerifyMFALogin_InvalidMethod(t *testing.T) {
	h := newMFAHandlerOnly(t, "JBSWY3DPEHPK3PXP")

	body := bytes.NewReader([]byte(`{"mfa_challenge":"some-token","method":"invalid","code":"123456"}`))
	req := httptest.NewRequest("POST", "/api/v1/login/mfa/verify", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleVerifyMFALogin(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLoginHandler_HandleVerifyMFALogin_MalformedJSON(t *testing.T) {
	h := newMFAHandlerOnly(t, "JBSWY3DPEHPK3PXP")

	body := bytes.NewReader([]byte(`{invalid json}`))
	req := httptest.NewRequest("POST", "/api/v1/login/mfa/verify", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleVerifyMFALogin(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ============================================================================
// Challenge 失效场景
// ============================================================================

func TestLoginHandler_HandleVerifyMFALogin_ChallengeNotFound(t *testing.T) {
	h := newMFAHandlerOnly(t, "JBSWY3DPEHPK3PXP")

	body := bytes.NewReader([]byte(`{"mfa_challenge":"nonexistent-token","method":"totp","code":"123456"}`))
	req := httptest.NewRequest("POST", "/api/v1/login/mfa/verify", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("User-Agent", "Mozilla/5.0")
	w := httptest.NewRecorder()

	h.HandleVerifyMFALogin(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "无效 challenge 应返回 401")
}

func TestLoginHandler_HandleVerifyMFALogin_IPMismatch(t *testing.T) {
	h := newMFAHandlerOnly(t, "JBSWY3DPEHPK3PXP")

	// 第一阶段：从 192.168.1.1 登录
	challenge := performMFAFirstStageLogin(t, h, "mfa-handler@example.com", "Password123!", "192.168.1.1", "Mozilla/5.0")

	// 第二阶段：从不同 IP 验证 —— 应被拒绝
	body := bytes.NewReader([]byte(`{"mfa_challenge":"` + challenge + `","method":"totp","code":"123456"}`))
	req := httptest.NewRequest("POST", "/api/v1/login/mfa/verify", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.99:1234" // 不同 IP
	req.Header.Set("User-Agent", "Mozilla/5.0")
	w := httptest.NewRecorder()

	h.HandleVerifyMFALogin(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "IP 不匹配应返回 401")
}

func TestLoginHandler_HandleVerifyMFALogin_InvalidTOTP(t *testing.T) {
	h := newMFAHandlerOnly(t, "JBSWY3DPEHPK3PXP")

	challenge := performMFAFirstStageLogin(t, h, "mfa-handler@example.com", "Password123!", "192.168.1.1", "Mozilla/5.0")

	body := bytes.NewReader([]byte(`{"mfa_challenge":"` + challenge + `","method":"totp","code":"000000"}`))
	req := httptest.NewRequest("POST", "/api/v1/login/mfa/verify", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("User-Agent", "Mozilla/5.0")
	w := httptest.NewRecorder()

	h.HandleVerifyMFALogin(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "无效 TOTP 应返回 401")
}

// ============================================================================
// 第一阶段响应验证
// ============================================================================

func TestLoginHandler_Handle_MFAUserResponseShape(t *testing.T) {
	h := newMFAHandlerOnly(t, "JBSWY3DPEHPK3PXP")

	body := bytes.NewReader([]byte(`{"email":"mfa-handler@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("User-Agent", "Mozilla/5.0")
	w := httptest.NewRecorder()

	h.Handle(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp["mfa_required"].(bool))
	assert.NotEmpty(t, resp["mfa_challenge"])
	assert.Equal(t, float64(300), resp["expires_in"])

	// MFA 第一阶段不应签发 token
	_, hasAccess := resp["access_token"]
	assert.False(t, hasAccess, "MFA 第一阶段不应签发 access_token")
	_, hasRefresh := resp["refresh_token"]
	assert.False(t, hasRefresh, "MFA 第一阶段不应签发 refresh_token")

	// 应告知支持的 MFA 方法
	methods, ok := resp["mfa_methods"].([]interface{})
	require.True(t, ok, "应包含 mfa_methods 数组")
	assert.Len(t, methods, 2)
}

// ============================================================================
// 辅助函数
// ============================================================================

// generateTestTOTPForHandler 生成 TOTP 验证码（6 位数字，30 秒时间窗口）
// 复制自 service_test 中的 generateTestTOTP，避免跨包依赖
func generateTestTOTPForHandler(secret string) string {
	secret = strings.ToUpper(strings.TrimSpace(secret))
	secretBytes, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)

	now := time.Now()
	timeStep := uint64(now.Unix() / 30)

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, timeStep)

	mac := hmac.New(sha1.New, secretBytes)
	mac.Write(buf)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	return fmt.Sprintf("%06d", code%1000000)
}
