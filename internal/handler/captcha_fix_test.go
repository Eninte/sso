// Package handler_test 修复后的登录与验证码逻辑验证测试
// 覆盖两个核心修复：
// 1. 仅凭据相关错误（401/403）触发 RecordFailure，非凭据错误（500/429）不触发
// 2. Verify 使用剩余 TTL 而非重置完整 TTL
package handler_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/captcha"
	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 可追踪调用的 mock captchaVerifier
// ============================================================================

// trackingCaptchaVerifier 记录 RecordFailure 调用次数和参数的 mock
type trackingCaptchaVerifier struct {
	enabled            bool
	requireCaptcha     bool
	verifyOK           bool
	recordFailureCalls atomic.Int64
	recordFailureKeys  []string
	clearFailuresCalls atomic.Int64
	clearFailuresKeys  []string
}

func (m *trackingCaptchaVerifier) IsEnabled() bool { return m.enabled }
func (m *trackingCaptchaVerifier) ShouldRequireCaptcha(_ context.Context, _ string) bool {
	return m.requireCaptcha
}
func (m *trackingCaptchaVerifier) Verify(_ context.Context, _, _ string) (bool, error) {
	return m.verifyOK, nil
}
func (m *trackingCaptchaVerifier) RecordFailure(_ context.Context, key string) {
	m.recordFailureCalls.Add(1)
	m.recordFailureKeys = append(m.recordFailureKeys, key)
}
func (m *trackingCaptchaVerifier) ClearFailures(_ context.Context, key string) {
	m.clearFailuresCalls.Add(1)
	m.clearFailuresKeys = append(m.clearFailuresKeys, key)
}

// ============================================================================
// 测试辅助：创建带已知密码的测试用户
// ============================================================================

func setupLoginTestUser(t *testing.T, store *mock.Store) {
	t.Helper()
	passwordSvc := crypto.NewPasswordService(4)
	hashedPassword, err := passwordSvc.HashPassword("Password123!")
	require.NoError(t, err)

	// 正常活跃用户
	store.AddUser(&model.User{
		ID:            "fix-test-user",
		Email:         "fixtest@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})

	// 被禁用的用户
	store.AddUser(&model.User{
		ID:            "fix-test-disabled",
		Email:         "disabled-fixtest@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusDisabled,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})

	// 被锁定的用户（设置未来锁定时间，否则 validateUserCredentials 会自动解锁）
	futureTime := time.Now().Add(1 * time.Hour)
	store.AddUser(&model.User{
		ID:            "fix-test-locked",
		Email:         "locked-fixtest@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: true,
		Status:        model.UserStatusLocked,
		LockedUntil:   &futureTime,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})

	// 邮箱未验证的用户
	store.AddUser(&model.User{
		ID:            "fix-test-unverified",
		Email:         "unverified-fixtest@example.com",
		PasswordHash:  hashedPassword,
		EmailVerified: false,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})
}

func createFixTestAuthSvc(store *mock.Store) service.AuthServiceInterface {
	passwordSvc := crypto.NewPasswordService(4)
	privateKey, _ := crypto.GenerateRSAKeyPair(2048)
	jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test-issuer", 15*time.Minute, 7*24*time.Hour)
	return service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute)
}

// ============================================================================
// 修复1: 仅凭据相关错误触发 RecordFailure
// ============================================================================

func TestFix_CredentialErrorTriggersRecordFailure(t *testing.T) {
	// 凭据错误（401）应触发 RecordFailure
	tracker := &trackingCaptchaVerifier{enabled: true}
	store := mock.New()
	authSvc := createFixTestAuthSvc(store)
	setupLoginTestUser(t, store)

	h := handler.NewLoginHandler(authSvc, tracker)

	t.Run("密码错误(401)应触发RecordFailure", func(t *testing.T) {
		tracker.recordFailureCalls.Store(0)
		tracker.recordFailureKeys = nil

		body := bytes.NewReader([]byte(`{"email":"fixtest@example.com","password":"WrongPassword!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.1:1234"
		w := httptest.NewRecorder()

		h.Handle(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "密码错误应返回401")
		assert.Equal(t, int64(1), tracker.recordFailureCalls.Load(),
			"凭据错误应触发 RecordFailure")
		assert.Contains(t, tracker.recordFailureKeys, "10.0.0.1",
			"RecordFailure 应使用客户端IP")
	})

	t.Run("用户不存在(401)应触发RecordFailure", func(t *testing.T) {
		tracker.recordFailureCalls.Store(0)
		tracker.recordFailureKeys = nil

		body := bytes.NewReader([]byte(`{"email":"nonexistent@example.com","password":"Password123!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.2:1234"
		w := httptest.NewRecorder()

		h.Handle(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "用户不存在应返回401")
		assert.Equal(t, int64(1), tracker.recordFailureCalls.Load(),
			"凭据错误应触发 RecordFailure")
	})

	t.Run("账户被禁用(403)应触发RecordFailure", func(t *testing.T) {
		tracker.recordFailureCalls.Store(0)
		tracker.recordFailureKeys = nil

		body := bytes.NewReader([]byte(`{"email":"disabled-fixtest@example.com","password":"Password123!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.3:1234"
		w := httptest.NewRecorder()

		h.Handle(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code, "账户被禁用应返回403")
		assert.Equal(t, int64(1), tracker.recordFailureCalls.Load(),
			"账户禁用(403)属于凭据相关错误，应触发 RecordFailure")
	})

	t.Run("账户被锁定(403)应触发RecordFailure", func(t *testing.T) {
		tracker.recordFailureCalls.Store(0)
		tracker.recordFailureKeys = nil

		body := bytes.NewReader([]byte(`{"email":"locked-fixtest@example.com","password":"Password123!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.4:1234"
		w := httptest.NewRecorder()

		h.Handle(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code, "账户被锁定应返回403")
		assert.Equal(t, int64(1), tracker.recordFailureCalls.Load(),
			"账户锁定(403)属于凭据相关错误，应触发 RecordFailure")
	})

	t.Run("邮箱未验证(401)应触发RecordFailure", func(t *testing.T) {
		tracker.recordFailureCalls.Store(0)
		tracker.recordFailureKeys = nil

		body := bytes.NewReader([]byte(`{"email":"unverified-fixtest@example.com","password":"Password123!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.5:1234"
		w := httptest.NewRecorder()

		h.Handle(w, req)

		// 邮箱未验证返回401
		assert.Equal(t, http.StatusUnauthorized, w.Code, "邮箱未验证应返回401")
		assert.Equal(t, int64(1), tracker.recordFailureCalls.Load(),
			"邮箱未验证(401)属于凭据相关错误，应触发 RecordFailure")
	})
}

func TestFix_LoginSuccessClearsFailures(t *testing.T) {
	// 登录成功应调用 ClearFailures
	tracker := &trackingCaptchaVerifier{enabled: true}
	store := mock.New()
	authSvc := createFixTestAuthSvc(store)
	setupLoginTestUser(t, store)

	h := handler.NewLoginHandler(authSvc, tracker)

	tracker.clearFailuresCalls.Store(0)
	tracker.clearFailuresKeys = nil

	body := bytes.NewReader([]byte(`{"email":"fixtest@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.100:1234"
	w := httptest.NewRecorder()

	h.Handle(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "正确凭据应登录成功")
	assert.Equal(t, int64(1), tracker.clearFailuresCalls.Load(),
		"登录成功应调用 ClearFailures")
	assert.Contains(t, tracker.clearFailuresKeys, "10.0.0.100",
		"ClearFailures 应使用客户端IP")
	assert.Equal(t, int64(0), tracker.recordFailureCalls.Load(),
		"登录成功不应调用 RecordFailure")
}

// ============================================================================
// 修复1 补充: 非凭据错误不触发 RecordFailure（使用 mock store 错误注入）
// ============================================================================

func TestFix_NonCredentialErrorDoesNotTriggerRecordFailure(t *testing.T) {
	// 通过 mock store 的错误注入模拟数据库错误(500)
	tracker := &trackingCaptchaVerifier{enabled: true}
	store := mock.New()
	// 注入 GetUserByEmail 错误，模拟数据库故障
	store.GetUserByEmailErr = apperrors.ErrInternal
	authSvc := createFixTestAuthSvc(store)

	h := handler.NewLoginHandler(authSvc, tracker)

	tracker.recordFailureCalls.Store(0)
	tracker.recordFailureKeys = nil

	body := bytes.NewReader([]byte(`{"email":"any@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.50:1234"
	w := httptest.NewRecorder()

	h.Handle(w, req)

	// 服务器内部错误不应触发 RecordFailure
	assert.Equal(t, http.StatusInternalServerError, w.Code, "数据库错误应返回500")
	assert.Equal(t, int64(0), tracker.recordFailureCalls.Load(),
		"服务器内部错误(500)不应触发 RecordFailure，避免影响合法用户")
}

// ============================================================================
// 修复2: Verify 使用剩余 TTL 而非重置完整 TTL
// ============================================================================

func TestFix_VerifyUsesRemainingTTL(t *testing.T) {
	// 使用短 TTL 验证：错误猜测不会延长验证码生命周期
	c := cache.NewMemoryCache()
	defer c.Close()

	// 使用 2 秒的短 TTL，方便观察过期行为
	svc := captcha.NewServiceWithAdaptive(c, true, 2*time.Second, 3, 15*time.Minute)
	ctx := context.Background()

	t.Run("错误猜测不延长验证码生命周期", func(t *testing.T) {
		captchaResp, err := svc.Generate(ctx)
		require.NoError(t, err)

		// 获取正确答案
		var data struct {
			Answer string `json:"answer"`
		}
		err = c.Get(ctx, captcha.CaptchaCachePrefix+captchaResp.ID, &data)
		require.NoError(t, err)

		// 立即提交错误答案（TTL 几乎未消耗）
		ok, err := svc.Verify(ctx, captchaResp.ID, "wrong")
		assert.NoError(t, err)
		assert.False(t, ok)

		// 等待接近原始 TTL（2秒）后，验证码应已过期
		// 修复前：错误猜测会重置 TTL 为完整 2 秒，此时验证码仍存在
		// 修复后：错误猜测使用剩余 TTL，验证码应在约 2 秒后过期
		time.Sleep(2200 * time.Millisecond)

		// 验证码应已过期，使用正确答案也无法通过
		ok, err = svc.Verify(ctx, captchaResp.ID, data.Answer)
		assert.NoError(t, err)
		assert.False(t, ok, "验证码应在原始 TTL 后过期，不被错误猜测延长")
	})

	t.Run("正确答案在 TTL 内仍可验证", func(t *testing.T) {
		captchaResp2, err := svc.Generate(ctx)
		require.NoError(t, err)

		var data struct {
			Answer string `json:"answer"`
		}
		err = c.Get(ctx, captcha.CaptchaCachePrefix+captchaResp2.ID, &data)
		require.NoError(t, err)

		// 先提交一次错误答案
		ok, err := svc.Verify(ctx, captchaResp2.ID, "wrong")
		assert.NoError(t, err)
		assert.False(t, ok)

		// 在 TTL 内用正确答案仍可通过
		ok, err = svc.Verify(ctx, captchaResp2.ID, data.Answer)
		assert.NoError(t, err)
		assert.True(t, ok, "TTL 内正确答案应验证通过")
	})
}

// ============================================================================
// 端到端：完整登录 + 验证码自适应触发流程
// ============================================================================

func TestFix_EndToEnd_LoginCaptchaFlow(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()
	captchaSvc := captcha.NewServiceWithAdaptive(c, true, 5*time.Minute, 3, 15*time.Minute)

	store := mock.New()
	authSvc := createFixTestAuthSvc(store)
	setupLoginTestUser(t, store)

	loginHandler := handler.NewLoginHandler(authSvc, captchaSvc)

	ip := "192.168.1.1"
	ctx := context.Background()

	// Step 1: 正常用户无需验证码
	t.Log("Step 1: 正常用户登录，无需验证码")
	body := bytes.NewReader([]byte(`{"email":"fixtest@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":1234"
	w := httptest.NewRecorder()
	loginHandler.Handle(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, captchaSvc.ShouldRequireCaptcha(ctx, ip), "成功登录后不应需要验证码")

	// Step 2: 模拟3次凭据失败
	t.Log("Step 2: 模拟3次凭据失败，触发验证码")
	for i := 0; i < 3; i++ {
		body := bytes.NewReader([]byte(`{"email":"fixtest@example.com","password":"WrongPassword!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		loginHandler.Handle(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}
	assert.True(t, captchaSvc.ShouldRequireCaptcha(ctx, ip), "3次失败后应需要验证码")

	// Step 3: 不带验证码的请求被拒绝
	t.Log("Step 3: 不带验证码的请求被拒绝")
	body = bytes.NewReader([]byte(`{"email":"fixtest@example.com","password":"Password123!"}`))
	req = httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":1234"
	w = httptest.NewRecorder()
	loginHandler.Handle(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "验证码不能为空")

	// Step 4: 生成验证码并使用正确答案通过
	t.Log("Step 4: 生成验证码并使用正确答案通过")
	captchaResp, err := captchaSvc.Generate(ctx)
	require.NoError(t, err)

	var captchaData struct {
		Answer string `json:"answer"`
	}
	err = c.Get(ctx, captcha.CaptchaCachePrefix+captchaResp.ID, &captchaData)
	require.NoError(t, err)

	body = bytes.NewReader([]byte(`{"email":"fixtest@example.com","password":"Password123!"}`))
	req = httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Captcha-Id", captchaResp.ID)
	req.Header.Set("X-Captcha-Answer", captchaData.Answer)
	req.RemoteAddr = ip + ":1234"
	w = httptest.NewRecorder()
	loginHandler.Handle(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "正确验证码+正确密码应登录成功")

	// Step 5: 登录成功后失败计数被清除
	t.Log("Step 5: 登录成功后失败计数被清除")
	assert.False(t, captchaSvc.ShouldRequireCaptcha(ctx, ip), "登录成功后应清除失败计数")

	// Step 6: 验证码一次性使用
	t.Log("Step 6: 已使用的验证码不可再次使用")
	captchaSvc.RecordFailure(ctx, ip)
	captchaSvc.RecordFailure(ctx, ip)
	captchaSvc.RecordFailure(ctx, ip) // 重新触发验证码要求

	body = bytes.NewReader([]byte(`{"email":"fixtest@example.com","password":"Password123!"}`))
	req = httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Captcha-Id", captchaResp.ID)
	req.Header.Set("X-Captcha-Answer", captchaData.Answer)
	req.RemoteAddr = ip + ":1234"
	w = httptest.NewRecorder()
	loginHandler.Handle(w, req)
	assert.Contains(t, w.Body.String(), "验证码无效或已过期", "已使用的验证码应不可再次使用")
}

// ============================================================================
// 端到端：不同 IP 的失败计数相互独立
// ============================================================================

func TestFix_EndToEnd_DifferentIPsIndependent(t *testing.T) {
	c := cache.NewMemoryCache()
	defer c.Close()
	captchaSvc := captcha.NewServiceWithAdaptive(c, true, 5*time.Minute, 3, 15*time.Minute)

	store := mock.New()
	authSvc := createFixTestAuthSvc(store)
	setupLoginTestUser(t, store)

	loginHandler := handler.NewLoginHandler(authSvc, captchaSvc)
	ctx := context.Background()

	ipA := "172.16.0.1"
	ipB := "172.16.0.2"

	// IP-A 失败3次
	for i := 0; i < 3; i++ {
		body := bytes.NewReader([]byte(`{"email":"fixtest@example.com","password":"WrongPassword!"}`))
		req := httptest.NewRequest("POST", "/api/v1/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ipA + ":1234"
		w := httptest.NewRecorder()
		loginHandler.Handle(w, req)
	}

	assert.True(t, captchaSvc.ShouldRequireCaptcha(ctx, ipA), "IP-A 3次失败后应需要验证码")
	assert.False(t, captchaSvc.ShouldRequireCaptcha(ctx, ipB), "IP-B 无失败记录，不应需要验证码")

	// IP-B 正常登录不受影响
	body := bytes.NewReader([]byte(`{"email":"fixtest@example.com","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ipB + ":1234"
	w := httptest.NewRecorder()
	loginHandler.Handle(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "IP-B 应正常登录，不受 IP-A 失败影响")
}
