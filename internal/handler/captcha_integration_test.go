// Package handler_test 验证码集成测试
// 测试验证码在HTTP层面的完整流程：自适应触发 → 生成 → 传递 → 验证
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

	"github.com/your-org/sso/internal/cache"
	"github.com/your-org/sso/internal/captcha"
	"github.com/your-org/sso/internal/crypto"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/handler"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/service"
	"github.com/your-org/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助
// ============================================================================

// captchaTestEnv 验证码测试环境，持有服务实例和缓存引用
type captchaTestEnv struct {
	captchaSvc *captcha.Service
	memCache   cache.Cache
}

// createCaptchaTestEnv 创建启用验证码的测试环境（阈值=3，窗口=15分钟）
func createCaptchaTestEnv(t *testing.T) *captchaTestEnv {
	t.Helper()
	c := cache.NewMemoryCache()
	t.Cleanup(func() { c.Close() })
	svc := captcha.NewServiceWithAdaptive(c, true, 5*time.Minute, 3, 15*time.Minute)
	return &captchaTestEnv{captchaSvc: svc, memCache: c}
}

// createDisabledCaptchaEnv 创建禁用验证码的测试环境
func createDisabledCaptchaEnv(t *testing.T) *captchaTestEnv {
	t.Helper()
	c := cache.NewMemoryCache()
	t.Cleanup(func() { c.Close() })
	svc := captcha.NewServiceWithAdaptive(c, false, 5*time.Minute, 3, 15*time.Minute)
	return &captchaTestEnv{captchaSvc: svc, memCache: c}
}

// generateAndSolve 生成验证码并获取正确答案
func (env *captchaTestEnv) generateAndSolve(t *testing.T) (string, string) {
	t.Helper()
	c, err := env.captchaSvc.Generate(context.Background())
	require.NoError(t, err)

	var data struct {
		Answer string `json:"answer"`
	}
	cacheKey := captcha.CaptchaCachePrefix + c.ID
	err = env.memCache.Get(context.Background(), cacheKey, &data)
	require.NoError(t, err)

	return c.ID, data.Answer
}

// simulateFailures 模拟指定IP的N次失败记录，使验证码触发
func (env *captchaTestEnv) simulateFailures(t *testing.T, ip string, count int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < count; i++ {
		env.captchaSvc.RecordFailure(ctx, ip)
	}
}

// generateCaptchaHTTP 通过HTTP请求生成验证码
func generateCaptchaHTTP(t *testing.T, env *captchaTestEnv) *captcha.Captcha {
	t.Helper()
	h := handler.NewCaptchaHandler(env.captchaSvc)

	req := httptest.NewRequest("GET", "/api/v1/captcha", nil)
	w := httptest.NewRecorder()
	h.Handle(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// 响应格式为 {message: "", data: {id, type, question, ttl}}
	dataRaw, ok := resp["data"]
	require.True(t, ok, "response should contain 'data' field")

	dataBytes, err := json.Marshal(dataRaw)
	require.NoError(t, err)

	var c captcha.Captcha
	err = json.Unmarshal(dataBytes, &c)
	require.NoError(t, err)

	return &c
}

// createTestAuthSvc 创建测试用AuthService
func createTestAuthSvc() (service.AuthServiceInterface, *mock.Store) {
	store := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	privateKey, _ := crypto.GenerateRSAKeyPair(2048)
	jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test-issuer", 15*time.Minute, 7*24*time.Hour)
	authSvc := service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute)
	return authSvc, store
}

// createTestUserSvc 创建测试用UserService
func createTestUserSvc() (service.UserServiceInterface, *mock.Store) {
	store := mock.New()
	passwordSvc := crypto.NewPasswordService(4)
	userSvc := service.NewUserService(store, passwordSvc, nil, "http://localhost:9090")
	return userSvc, store
}

// ============================================================================
// CaptchaHandler 测试
// ============================================================================

func TestCaptchaHandler_Handle_Success(t *testing.T) {
	env := createCaptchaTestEnv(t)
	h := handler.NewCaptchaHandler(env.captchaSvc)

	req := httptest.NewRequest("GET", "/api/v1/captcha", nil)
	w := httptest.NewRecorder()
	h.Handle(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	// 响应格式为 {message: "", data: {id, type, question, ttl}}
	data, ok := resp["data"].(map[string]interface{})
	require.True(t, ok, "response should contain 'data' object")

	assert.NotEmpty(t, data["id"])
	assert.Equal(t, "math", data["type"])
	assert.NotEmpty(t, data["question"])
	assert.Equal(t, float64(300), data["ttl"])
}

func TestCaptchaHandler_Handle_Disabled(t *testing.T) {
	env := createDisabledCaptchaEnv(t)
	h := handler.NewCaptchaHandler(env.captchaSvc)

	req := httptest.NewRequest("GET", "/api/v1/captcha", nil)
	w := httptest.NewRecorder()
	h.Handle(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCaptchaHandler_Handle_UniqueIDs(t *testing.T) {
	env := createCaptchaTestEnv(t)
	h := handler.NewCaptchaHandler(env.captchaSvc)

	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/api/v1/captcha", nil)
		w := httptest.NewRecorder()
		h.Handle(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)

		data, ok := resp["data"].(map[string]interface{})
		require.True(t, ok)

		id := data["id"].(string)
		assert.False(t, ids[id], "captcha ID should be unique")
		ids[id] = true
	}
}

// ============================================================================
// 自适应触发核心测试：正常用户无需验证码
// ============================================================================

func TestLoginHandler_NoCaptchaForNormalUser(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, _ := createTestAuthSvc()
	h := handler.NewLoginHandler(authSvc, env.captchaSvc)

	// 正常用户（无失败记录）不应被要求验证码
	body := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.Handle(w, req)

	// 不应返回验证码错误（可能返回密码错误等其他错误，但不是验证码相关）
	assert.NotContains(t, w.Body.String(), "验证码不能为空")
	assert.NotContains(t, w.Body.String(), "验证码无效或已过期")
}

func TestRegisterHandler_NoCaptchaForNormalUser(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, _ := createTestAuthSvc()
	h := handler.NewRegisterHandler(authSvc, env.captchaSvc)

	body := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/register", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.Handle(w, req)

	assert.NotContains(t, w.Body.String(), "验证码不能为空")
	assert.NotContains(t, w.Body.String(), "验证码无效或已过期")
}

func TestForgotPassword_NoCaptchaForNormalUser(t *testing.T) {
	env := createCaptchaTestEnv(t)
	userSvc, _ := createTestUserSvc()
	h := handler.NewUserHandler(userSvc, env.captchaSvc)

	body := bytes.NewReader([]byte(`{"email":"test@example.com"}`))
	req := httptest.NewRequest("POST", "/api/v1/forgot-password", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.HandleForgotPassword(w, req)

	assert.NotContains(t, w.Body.String(), "验证码不能为空")
}

func TestResetPassword_NoCaptchaForNormalUser(t *testing.T) {
	env := createCaptchaTestEnv(t)
	userSvc, _ := createTestUserSvc()
	h := handler.NewUserHandler(userSvc, env.captchaSvc)

	body := bytes.NewReader([]byte(`{"token":"abc","user_id":"123","new_password":"NewPass123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.HandleResetPassword(w, req)

	assert.NotContains(t, w.Body.String(), "验证码不能为空")
}

// ============================================================================
// 自适应触发：失败达到阈值后要求验证码
// ============================================================================

func TestLoginHandler_CaptchaRequiredAfterFailures(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, _ := createTestAuthSvc()
	h := handler.NewLoginHandler(authSvc, env.captchaSvc)

	// 模拟3次失败（达到阈值）
	env.simulateFailures(t, "1.2.3.4", 3)

	body := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.Handle(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "验证码不能为空")
}

func TestLoginHandler_CaptchaNotRequiredBelowThreshold(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, _ := createTestAuthSvc()
	h := handler.NewLoginHandler(authSvc, env.captchaSvc)

	// 模拟2次失败（未达阈值3）
	env.simulateFailures(t, "1.2.3.4", 2)

	body := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.Handle(w, req)

	// 未达阈值，不应要求验证码
	assert.NotContains(t, w.Body.String(), "验证码不能为空")
}

func TestRegisterHandler_CaptchaRequiredAfterFailures(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, _ := createTestAuthSvc()
	h := handler.NewRegisterHandler(authSvc, env.captchaSvc)

	env.simulateFailures(t, "1.2.3.4", 3)

	body := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/register", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.Handle(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "验证码不能为空")
}

func TestForgotPassword_CaptchaRequiredAfterFailures(t *testing.T) {
	env := createCaptchaTestEnv(t)
	userSvc, _ := createTestUserSvc()
	h := handler.NewUserHandler(userSvc, env.captchaSvc)

	env.simulateFailures(t, "1.2.3.4", 3)

	body := bytes.NewReader([]byte(`{"email":"test@example.com"}`))
	req := httptest.NewRequest("POST", "/api/v1/forgot-password", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.HandleForgotPassword(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "验证码不能为空")
}

func TestResetPassword_CaptchaRequiredAfterFailures(t *testing.T) {
	env := createCaptchaTestEnv(t)
	userSvc, _ := createTestUserSvc()
	h := handler.NewUserHandler(userSvc, env.captchaSvc)

	env.simulateFailures(t, "1.2.3.4", 3)

	body := bytes.NewReader([]byte(`{"token":"abc","user_id":"123","new_password":"NewPass123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.HandleResetPassword(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "验证码不能为空")
}

// ============================================================================
// 验证码禁用时
// ============================================================================

func TestLoginHandler_CaptchaDisabled(t *testing.T) {
	env := createDisabledCaptchaEnv(t)
	authSvc, _ := createTestAuthSvc()
	h := handler.NewLoginHandler(authSvc, env.captchaSvc)

	// 即使模拟失败，禁用时也不要求验证码
	env.simulateFailures(t, "1.2.3.4", 10)

	body := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.Handle(w, req)

	assert.NotContains(t, w.Body.String(), "验证码不能为空")
}

// ============================================================================
// 验证码无效测试（达到阈值后提供错误验证码）
// ============================================================================

func TestLoginHandler_CaptchaInvalid(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, _ := createTestAuthSvc()
	h := handler.NewLoginHandler(authSvc, env.captchaSvc)

	env.simulateFailures(t, "1.2.3.4", 3)

	body := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Captcha-ID", "fake-id")
	req.Header.Set("X-Captcha-Answer", "wrong")
	req.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()

	h.Handle(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "验证码无效或已过期")
}

// ============================================================================
// 端到端：失败触发 → 生成验证码 → 使用验证码通过
// ============================================================================

func TestCaptcha_EndToEnd_AdaptiveTrigger(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, _ := createTestAuthSvc()
	loginHandler := handler.NewLoginHandler(authSvc, env.captchaSvc)

	// Step 1: 正常用户无需验证码
	body1 := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req1 := httptest.NewRequest("POST", "/api/v1/login", body1)
	req1.Header.Set("Content-Type", "application/json")
	req1.RemoteAddr = "5.6.7.8:1234"
	w1 := httptest.NewRecorder()
	loginHandler.Handle(w1, req1)
	assert.NotContains(t, w1.Body.String(), "验证码")

	// Step 2: 模拟3次失败
	env.simulateFailures(t, "5.6.7.8", 3)

	// Step 3: 现在需要验证码了
	body2 := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req2 := httptest.NewRequest("POST", "/api/v1/login", body2)
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "5.6.7.8:1234"
	w2 := httptest.NewRecorder()
	loginHandler.Handle(w2, req2)
	assert.Contains(t, w2.Body.String(), "验证码不能为空")

	// Step 4: 生成验证码并使用
	captchaID, answer := env.generateAndSolve(t)
	body3 := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req3 := httptest.NewRequest("POST", "/api/v1/login", body3)
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-Captcha-ID", captchaID)
	req3.Header.Set("X-Captcha-Answer", answer)
	req3.RemoteAddr = "5.6.7.8:1234"
	w3 := httptest.NewRecorder()
	loginHandler.Handle(w3, req3)
	assert.NotContains(t, w3.Body.String(), "验证码")
}

func TestCaptcha_EndToEnd_OneTimeUse(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, _ := createTestAuthSvc()
	loginHandler := handler.NewLoginHandler(authSvc, env.captchaSvc)

	// 模拟失败达到阈值
	env.simulateFailures(t, "1.2.3.4", 3)

	// 生成验证码
	captchaID, answer := env.generateAndSolve(t)

	// 第一次使用 - 验证码通过
	body1 := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req1 := httptest.NewRequest("POST", "/api/v1/login", body1)
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Captcha-ID", captchaID)
	req1.Header.Set("X-Captcha-Answer", answer)
	req1.RemoteAddr = "1.2.3.4:1234"
	w1 := httptest.NewRecorder()
	loginHandler.Handle(w1, req1)
	assert.NotContains(t, w1.Body.String(), "验证码无效")

	// 第二次使用同一验证码 - 应失败
	env.simulateFailures(t, "1.2.3.4", 3) // 重新模拟失败以触发验证码要求
	// 用已消费的验证码
	body2 := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req2 := httptest.NewRequest("POST", "/api/v1/login", body2)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Captcha-ID", captchaID)
	req2.Header.Set("X-Captcha-Answer", answer)
	req2.RemoteAddr = "1.2.3.4:1234"
	w2 := httptest.NewRecorder()
	loginHandler.Handle(w2, req2)
	assert.Contains(t, w2.Body.String(), "验证码无效或已过期")
}

func TestCaptcha_EndToEnd_DifferentIPsIndependent(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, _ := createTestAuthSvc()
	loginHandler := handler.NewLoginHandler(authSvc, env.captchaSvc)

	// IP-A 失败3次，触发验证码
	env.simulateFailures(t, "10.0.0.1", 3)

	// IP-A 需要验证码
	body1 := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req1 := httptest.NewRequest("POST", "/api/v1/login", body1)
	req1.Header.Set("Content-Type", "application/json")
	req1.RemoteAddr = "10.0.0.1:1234"
	w1 := httptest.NewRecorder()
	loginHandler.Handle(w1, req1)
	assert.Contains(t, w1.Body.String(), "验证码不能为空")

	// IP-B 无失败记录，不需要验证码
	body2 := bytes.NewReader([]byte(`{"email":"test@example.com","password":"Password123!"}`))
	req2 := httptest.NewRequest("POST", "/api/v1/login", body2)
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "10.0.0.2:1234"
	w2 := httptest.NewRecorder()
	loginHandler.Handle(w2, req2)
	assert.NotContains(t, w2.Body.String(), "验证码不能为空")
}

// ============================================================================
// Captcha Service 单元测试（自适应触发）
// ============================================================================

func TestCaptchaService_ShouldRequireCaptcha(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()
	svc := captcha.NewServiceWithAdaptive(c, true, 5*time.Minute, 3, 15*time.Minute)
	ctx := context.Background()

	// 无记录时不需要
	assert.False(t, svc.ShouldRequireCaptcha(ctx, "1.2.3.4"))

	// 1次失败
	svc.RecordFailure(ctx, "1.2.3.4")
	assert.False(t, svc.ShouldRequireCaptcha(ctx, "1.2.3.4"))

	// 2次失败
	svc.RecordFailure(ctx, "1.2.3.4")
	assert.False(t, svc.ShouldRequireCaptcha(ctx, "1.2.3.4"))

	// 3次失败（达到阈值）
	svc.RecordFailure(ctx, "1.2.3.4")
	assert.True(t, svc.ShouldRequireCaptcha(ctx, "1.2.3.4"))

	// 清除后不需要
	svc.ClearFailures(ctx, "1.2.3.4")
	assert.False(t, svc.ShouldRequireCaptcha(ctx, "1.2.3.4"))
}

func TestCaptchaService_DifferentKeysIndependent(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()
	svc := captcha.NewServiceWithAdaptive(c, true, 5*time.Minute, 3, 15*time.Minute)
	ctx := context.Background()

	// IP-A 失败3次
	for i := 0; i < 3; i++ {
		svc.RecordFailure(ctx, "10.0.0.1")
	}
	assert.True(t, svc.ShouldRequireCaptcha(ctx, "10.0.0.1"))

	// IP-B 无记录
	assert.False(t, svc.ShouldRequireCaptcha(ctx, "10.0.0.2"))

	// 清除IP-A不影响IP-B（IP-B本来就没记录）
	svc.ClearFailures(ctx, "10.0.0.1")
	assert.False(t, svc.ShouldRequireCaptcha(ctx, "10.0.0.1"))
}

func TestCaptchaService_DisabledNoTracking(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()
	svc := captcha.NewServiceWithAdaptive(c, false, 5*time.Minute, 3, 15*time.Minute)
	ctx := context.Background()

	// 禁用时RecordFailure不生效
	for i := 0; i < 10; i++ {
		svc.RecordFailure(ctx, "1.2.3.4")
	}
	assert.False(t, svc.ShouldRequireCaptcha(ctx, "1.2.3.4"))
}

// ============================================================================
// 修复验证：仅凭据相关错误触发 RecordFailure
// ============================================================================

// setupFixTestUser 创建用于修复验证的测试用户
func setupFixTestUser(t *testing.T, store *mock.Store) {
	t.Helper()
	passwordSvc := crypto.NewPasswordService(4)
	hashedPassword, err := passwordSvc.HashPassword("Password123!")
	require.NoError(t, err)

	// 正常活跃用户
	store.AddUser(&model.User{
		ID:            "fix-user-active",
		Email:         "fix-active@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})

	// 被禁用的用户
	store.AddUser(&model.User{
		ID:            "fix-user-disabled",
		Email:         "fix-disabled@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusDisabled,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})

	// 被锁定的用户（设置未来锁定时间，否则 validateUserCredentials 会自动解锁）
	futureTime := time.Now().Add(1 * time.Hour)
	store.AddUser(&model.User{
		ID:            "fix-user-locked",
		Email:         "fix-locked@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusLocked,
		LockedUntil:   &futureTime,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})

	// 邮箱未验证的用户
	store.AddUser(&model.User{
		ID:            "fix-user-unverified",
		Email:         "fix-unverified@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: false,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})
}

func TestLoginHandler_CredentialErrorTriggersRecordFailure(t *testing.T) {
	// 验证：凭据相关错误（401/403）应递增验证码失败计数
	env := createCaptchaTestEnv(t)
	authSvc, store := createTestAuthSvc()
	setupFixTestUser(t, store)
	h := handler.NewLoginHandler(authSvc, env.captchaSvc)
	ctx := context.Background()

	t.Run("密码错误(401)递增失败计数", func(t *testing.T) {
		ip := "20.0.0.1"
		require.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip))

		// 连续3次密码错误
		for i := 0; i < 3; i++ {
			body := bytes.NewReader([]byte(`{"email":"fix-active@example.com","password":"WrongPassword!"}`))
			req := httptest.NewRequest("POST", "/api/v1/login", body)
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = ip + ":1234"
			w := httptest.NewRecorder()
			h.Handle(w, req)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		}

		// 3次凭据错误后应触发验证码
		assert.True(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip),
			"3次凭据错误后应触发验证码要求")
	})

	t.Run("账户被禁用(403)递增失败计数", func(t *testing.T) {
		ip := "20.0.0.2"
		require.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip))

		for i := 0; i < 3; i++ {
			body := bytes.NewReader([]byte(`{"email":"fix-disabled@example.com","password":"Password123!"}`))
			req := httptest.NewRequest("POST", "/api/v1/login", body)
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = ip + ":1234"
			w := httptest.NewRecorder()
			h.Handle(w, req)
			assert.Equal(t, http.StatusForbidden, w.Code)
		}

		assert.True(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip),
			"账户禁用(403)属于凭据相关错误，应递增失败计数")
	})

	t.Run("账户被锁定(403)递增失败计数", func(t *testing.T) {
		ip := "20.0.0.3"
		require.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip))

		for i := 0; i < 3; i++ {
			body := bytes.NewReader([]byte(`{"email":"fix-locked@example.com","password":"Password123!"}`))
			req := httptest.NewRequest("POST", "/api/v1/login", body)
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = ip + ":1234"
			w := httptest.NewRecorder()
			h.Handle(w, req)
			assert.Equal(t, http.StatusForbidden, w.Code)
		}

		assert.True(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip),
			"账户锁定(403)属于凭据相关错误，应递增失败计数")
	})

	t.Run("邮箱未验证(401)递增失败计数", func(t *testing.T) {
		ip := "20.0.0.4"
		require.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip))

		for i := 0; i < 3; i++ {
			body := bytes.NewReader([]byte(`{"email":"fix-unverified@example.com","password":"Password123!"}`))
			req := httptest.NewRequest("POST", "/api/v1/login", body)
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = ip + ":1234"
			w := httptest.NewRecorder()
			h.Handle(w, req)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		}

		assert.True(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip),
			"邮箱未验证(401)属于凭据相关错误，应递增失败计数")
	})
}

func TestLoginHandler_NonCredentialErrorNoRecordFailure(t *testing.T) {
	// 验证：服务器内部错误(500)不应递增验证码失败计数
	env := createCaptchaTestEnv(t)
	store := mock.New()
	// 注入 GetUserByEmail 错误，模拟数据库故障
	store.GetUserByEmailErr = apperrors.ErrInternal
	authSvc := service.NewAuthService(
		store,
		crypto.NewPasswordService(4),
		func() *crypto.JWTService {
			pk, _ := crypto.GenerateRSAKeyPair(2048)
			return crypto.NewJWTService(pk, &pk.PublicKey, "test-issuer", 15*time.Minute, 7*24*time.Hour)
		}(),
		5, 30*time.Minute,
	)
	h := handler.NewLoginHandler(authSvc, env.captchaSvc)
	ctx := context.Background()

	ip := "20.0.0.50"
	require.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip))

	// 连续3次服务器错误
	for i := 0; i < 3; i++ {
		body := bytes.NewReader([]byte(`{"email":"any@example.com","password":"Password123!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		h.Handle(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	}

	// 服务器内部错误不应递增失败计数
	assert.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip),
		"服务器内部错误(500)不应递增验证码失败计数，避免影响合法用户")
}

func TestLoginHandler_SuccessClearsFailures(t *testing.T) {
	// 验证：登录成功后应清除失败计数
	env := createCaptchaTestEnv(t)
	authSvc, store := createTestAuthSvc()
	setupFixTestUser(t, store)
	h := handler.NewLoginHandler(authSvc, env.captchaSvc)
	ctx := context.Background()

	ip := "20.0.0.100"

	// 先制造2次失败（未达阈值，无需验证码即可登录）
	env.simulateFailures(t, ip, 2)
	assert.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip))

	// 登录成功应清除失败计数
	body := bytes.NewReader([]byte(`{"email":"fix-active@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":1234"
	w := httptest.NewRecorder()
	h.Handle(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip),
		"登录成功后应清除失败计数")
}

// ============================================================================
// 修复验证：ForgotPassword/ResetPassword 的 RecordFailure 行为
// ============================================================================

func TestForgotPassword_NoRecordFailureForNonexistentEmail(t *testing.T) {
	// ForgotPassword 对不存在的邮箱返回 nil（安全设计：防枚举）
	// 因此不会触发 RecordFailure
	env := createCaptchaTestEnv(t)
	userSvc, _ := createTestUserSvc()
	h := handler.NewUserHandler(userSvc, env.captchaSvc)
	ctx := context.Background()

	ip := "30.0.0.1"
	require.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip))

	// ForgotPassword 对不存在的邮箱返回成功响应（防枚举），且不调用 RecordFailure
	for i := 0; i < 5; i++ {
		body := bytes.NewReader([]byte(`{"email":"nonexistent@example.com"}`))
		req := httptest.NewRequest("POST", "/api/v1/forgot-password", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		h.HandleForgotPassword(w, req)
		// 响应始终为200（防枚举）
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 不存在的邮箱不触发 RecordFailure（服务层返回 nil）
	assert.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip),
		"ForgotPassword 对不存在的邮箱不应递增失败计数")
}

func TestResetPassword_RecordFailureOnServiceError(t *testing.T) {
	// ResetPassword 在服务层失败时无条件调用 RecordFailure
	env := createCaptchaTestEnv(t)
	userSvc, _ := createTestUserSvc()
	h := handler.NewUserHandler(userSvc, env.captchaSvc)
	ctx := context.Background()

	ip := "30.0.0.2"
	require.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip))

	// 使用无效 token 重置密码
	for i := 0; i < 3; i++ {
		body := bytes.NewReader([]byte(`{"token":"invalid","user_id":"fake","new_password":"NewPass123!"}`))
		req := httptest.NewRequest("POST", "/api/v1/reset-password", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		h.HandleResetPassword(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	}

	assert.True(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip),
		"ResetPassword 服务层失败应递增验证码失败计数")
}

// ============================================================================
// 修复验证：Verify TTL 不被错误猜测延长（端到端）
// ============================================================================

func TestCaptcha_EndToEnd_VerifyTTLNotExtended(t *testing.T) {
	// 端到端验证：错误猜测不会延长验证码生命周期
	// 使用短 TTL 的 captchaTestEnv
	c := cache.NewMemoryCache()
	defer c.Close()
	shortTTLSvc := captcha.NewServiceWithAdaptive(c, true, 2*time.Second, 3, 15*time.Minute)
	env := &captchaTestEnv{captchaSvc: shortTTLSvc, memCache: c}

	authSvc, store := createTestAuthSvc()
	setupFixTestUser(t, store)
	loginHandler := handler.NewLoginHandler(authSvc, env.captchaSvc)

	ip := "40.0.0.1"
	ctx := context.Background()

	// 模拟失败达到阈值
	env.simulateFailures(t, ip, 3)
	require.True(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip))

	// 生成验证码
	captchaID, answer := env.generateAndSolve(t)

	// 提交错误答案的验证码（TTL 几乎未消耗）
	{
		body := bytes.NewReader([]byte(`{"email":"fix-active@example.com","password":"Password123!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Captcha-ID", captchaID)
		req.Header.Set("X-Captcha-Answer", "wrong")
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		loginHandler.Handle(w, req)
		assert.Contains(t, w.Body.String(), "验证码无效或已过期")
	}

	// 等待超过原始 TTL（2秒），验证码应已过期
	// 修复前：错误猜测会重置 TTL，此时验证码仍存在
	// 修复后：错误猜测使用剩余 TTL，验证码应已过期
	time.Sleep(2200 * time.Millisecond)

	// 重新模拟失败以触发验证码要求
	env.simulateFailures(t, ip, 3)

	// 使用正确答案但验证码已过期
	{
		body := bytes.NewReader([]byte(`{"email":"fix-active@example.com","password":"Password123!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Captcha-ID", captchaID)
		req.Header.Set("X-Captcha-Answer", answer)
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		loginHandler.Handle(w, req)
		assert.Contains(t, w.Body.String(), "验证码无效或已过期",
			"验证码应在原始 TTL 后过期，不被错误猜测延长")
	}
}

// ============================================================================
// 修复验证：完整登录 + 验证码自适应触发 + 凭据错误过滤 端到端
// ============================================================================

func TestCaptcha_EndToEnd_CredentialErrorFiltering(t *testing.T) {
	// 端到端验证：凭据错误触发验证码，服务器错误不触发
	env := createCaptchaTestEnv(t)
	authSvc, store := createTestAuthSvc()
	setupFixTestUser(t, store)
	loginHandler := handler.NewLoginHandler(authSvc, env.captchaSvc)
	ctx := context.Background()

	ip := "50.0.0.1"

	// Step 1: 正常登录成功，无需验证码
	t.Log("Step 1: 正常登录成功")
	{
		body := bytes.NewReader([]byte(`{"email":"fix-active@example.com","password":"Password123!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		loginHandler.Handle(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip))
	}

	// Step 2: 2次凭据错误，仍未达阈值
	t.Log("Step 2: 2次凭据错误")
	for i := 0; i < 2; i++ {
		body := bytes.NewReader([]byte(`{"email":"fix-active@example.com","password":"WrongPassword!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		loginHandler.Handle(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}
	assert.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip), "2次失败未达阈值")

	// Step 3: 第3次凭据错误，达到阈值
	t.Log("Step 3: 第3次凭据错误，触发验证码")
	{
		body := bytes.NewReader([]byte(`{"email":"fix-active@example.com","password":"WrongPassword!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		loginHandler.Handle(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}
	assert.True(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip), "3次凭据错误后应触发验证码")

	// Step 4: 不带验证码的请求被拒绝
	t.Log("Step 4: 不带验证码被拒绝")
	{
		body := bytes.NewReader([]byte(`{"email":"fix-active@example.com","password":"Password123!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		loginHandler.Handle(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "验证码不能为空")
	}

	// Step 5: 生成验证码并使用正确答案通过
	t.Log("Step 5: 生成验证码并登录成功")
	captchaID, answer := env.generateAndSolve(t)
	{
		body := bytes.NewReader([]byte(`{"email":"fix-active@example.com","password":"Password123!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Captcha-ID", captchaID)
		req.Header.Set("X-Captcha-Answer", answer)
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		loginHandler.Handle(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Step 6: 登录成功后失败计数被清除
	t.Log("Step 6: 登录成功后失败计数被清除")
	assert.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip),
		"登录成功后应清除失败计数")
}
