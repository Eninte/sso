// Package handler_test T15 验证码账号维度计数的 HTTP 层测试
// 场景：攻击者不断更换 IP 对同一账号尝试登录，
// IP 维度各自未达阈值，但账号维度累计达到阈值后，
// 无论来源 IP 如何都要求验证码
package handler_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/handler"
)

// doLogin 发起一次登录请求（不带验证码头），返回响应记录器
func doLogin(t *testing.T, h *handler.LoginHandler, email, password, ip string) *httptest.ResponseRecorder {
	t.Helper()
	body := bytes.NewReader([]byte(`{"email":"` + email + `","password":"` + password + `"}`))
	req := httptest.NewRequest("POST", "/api/v1/login", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":1234"
	w := httptest.NewRecorder()
	h.Handle(w, req)
	return w
}

// TestLoginHandler_AccountDimension_MultiIPTrigger 同一账号多 IP 分散尝试触发验证码
// 这是 T15 的核心场景（审计 L7）：每个 IP 的失败次数都低于 IP 维度阈值，
// 但账号维度累计达到阈值后，来自全新 IP 的请求也必须提供验证码
func TestLoginHandler_AccountDimension_MultiIPTrigger(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, store := createTestAuthSvc()
	setupFixTestUser(t, store)
	h := handler.NewLoginHandler(authSvc, env.captchaSvc)
	ctx := context.Background()

	email := "fix-active@example.com"
	attackIPs := []string{"30.0.0.1", "30.0.0.2", "30.0.0.3"}

	// 3 个不同 IP 各失败 1 次（每个 IP 的 IP 维度计数=1，远低于阈值 3）
	for _, ip := range attackIPs {
		w := doLogin(t, h, email, "WrongPassword!", ip)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "凭据错误应返回 401")
	}

	// IP 维度：每个 IP 都未触发
	for _, ip := range attackIPs {
		assert.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, ip),
			"IP %s 仅失败 1 次，IP 维度不应触发", ip)
	}

	// 账号维度：累计 3 次失败，已触发
	assert.True(t, env.captchaSvc.ShouldRequireCaptchaForAccount(ctx, email),
		"同一账号跨 3 个 IP 累计 3 次失败，账号维度应触发")

	// 来自第 4 个全新 IP 的请求：即使密码正确，也要求验证码（400）
	newIP := "30.0.0.4"
	assert.False(t, env.captchaSvc.ShouldRequireCaptcha(ctx, newIP), "全新 IP 的 IP 维度无记录")
	w := doLogin(t, h, email, "Password123!", newIP)
	assert.Equal(t, http.StatusBadRequest, w.Code,
		"账号维度已触发时，全新 IP 不提供验证码应返回 400")

	// 同一新 IP 换其他账号：不受该账号维度影响，请求到达凭据校验（403 账户禁用）
	w = doLogin(t, h, "fix-disabled@example.com", "Password123!", newIP)
	assert.Equal(t, http.StatusForbidden, w.Code,
		"其他账号不受 victim 账号维度计数影响，应到达凭据校验阶段")
}

// TestLoginHandler_AccountDimension_SuccessClearsCounter 登录成功清除账号维度计数
func TestLoginHandler_AccountDimension_SuccessClearsCounter(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, store := createTestAuthSvc()
	setupFixTestUser(t, store)
	h := handler.NewLoginHandler(authSvc, env.captchaSvc)
	ctx := context.Background()

	email := "fix-active@example.com"

	// 2 个 IP 各失败 1 次：账号计数=2，未达阈值
	doLogin(t, h, email, "WrongPassword!", "31.0.0.1")
	doLogin(t, h, email, "WrongPassword!", "31.0.0.2")
	require.False(t, env.captchaSvc.ShouldRequireCaptchaForAccount(ctx, email))

	// 从第 3 个 IP 成功登录：账号维度计数清零
	w := doLogin(t, h, email, "Password123!", "31.0.0.3")
	require.Equal(t, http.StatusOK, w.Code, "正确凭据应登录成功")

	// 再从 2 个新 IP 各失败 1 次：若计数未清零将累计到 4 触发，清零后仅为 2
	doLogin(t, h, email, "WrongPassword!", "31.0.0.4")
	doLogin(t, h, email, "WrongPassword!", "31.0.0.5")
	assert.False(t, env.captchaSvc.ShouldRequireCaptchaForAccount(ctx, email),
		"成功登录应清零账号维度计数，后续 2 次失败不应触发验证码")

	// 正确凭据从全新 IP 登录仍可直接成功（无需验证码）
	w = doLogin(t, h, email, "Password123!", "31.0.0.6")
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestLoginHandler_AccountDimension_EmailNormalization 登录请求的邮箱大小写差异归一化计数
func TestLoginHandler_AccountDimension_EmailNormalization(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, store := createTestAuthSvc()
	setupFixTestUser(t, store)
	h := handler.NewLoginHandler(authSvc, env.captchaSvc)
	ctx := context.Background()

	// 用邮箱的大小写变体各失败 1 次（归一化后计为同一账号）
	doLogin(t, h, "FIX-ACTIVE@example.com", "WrongPassword!", "32.0.0.1")
	doLogin(t, h, "Fix-Active@Example.Com", "WrongPassword!", "32.0.0.2")
	doLogin(t, h, "fix-active@example.com", "WrongPassword!", "32.0.0.3")

	assert.True(t, env.captchaSvc.ShouldRequireCaptchaForAccount(ctx, "fix-active@example.com"),
		"邮箱写法差异应归一化为同一账号累计计数")

	// 全新 IP + 规范写法邮箱：要求验证码
	w := doLogin(t, h, "fix-active@example.com", "Password123!", "32.0.0.4")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestRegisterHandler_AccountDimensionTrigger 注册端点同样按账号维度触发验证码
// （注册失败不计数，但登录累计的账号维度计数对注册生效，防止换端点绕过）
func TestRegisterHandler_AccountDimensionTrigger(t *testing.T) {
	env := createCaptchaTestEnv(t)
	authSvc, store := createTestAuthSvc()
	setupFixTestUser(t, store)
	loginH := handler.NewLoginHandler(authSvc, env.captchaSvc)
	registerH := handler.NewRegisterHandler(authSvc, env.captchaSvc)
	ctx := context.Background()

	email := "fix-active@example.com"

	// 通过登录失败使账号维度触发
	for _, ip := range []string{"33.0.0.1", "33.0.0.2", "33.0.0.3"} {
		doLogin(t, loginH, email, "WrongPassword!", ip)
	}
	require.True(t, env.captchaSvc.ShouldRequireCaptchaForAccount(ctx, email))

	// 从全新 IP 访问注册端点（同一邮箱）：要求验证码
	body := bytes.NewReader([]byte(`{"email":"` + email + `","password":"Password123!"}`))
	req := httptest.NewRequest("POST", "/api/v1/register", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "33.0.0.4:1234"
	w := httptest.NewRecorder()
	registerH.Handle(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code,
		"账号维度已触发时，注册端点同样要求验证码，防止换端点绕过")
}
