// Package handler_test 浏览器登录页（/ui/login）单元测试
package handler_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/captcha"
	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助
// ============================================================================

// stubCaptchaVerifier 可配置的验证码 stub（同时实现生成能力）
type stubCaptchaVerifier struct {
	enabled       bool
	requireByIP   bool
	requireByAcct bool
	verifyOK      bool
	verifyErr     error
	generateErr   error
}

func (s *stubCaptchaVerifier) IsEnabled() bool { return s.enabled }
func (s *stubCaptchaVerifier) ShouldRequireCaptcha(_ context.Context, _ string) bool {
	return s.requireByIP
}
func (s *stubCaptchaVerifier) ShouldRequireCaptchaForAccount(_ context.Context, _ string) bool {
	return s.requireByAcct
}
func (s *stubCaptchaVerifier) Verify(_ context.Context, _, _ string) (bool, error) {
	return s.verifyOK, s.verifyErr
}
func (s *stubCaptchaVerifier) RecordFailure(_ context.Context, _ string)        {}
func (s *stubCaptchaVerifier) RecordAccountFailure(_ context.Context, _ string) {}
func (s *stubCaptchaVerifier) ClearFailures(_ context.Context, _ string)        {}
func (s *stubCaptchaVerifier) ClearAccountFailures(_ context.Context, _ string) {}
func (s *stubCaptchaVerifier) Generate(_ context.Context) (*captcha.Captcha, error) {
	if s.generateErr != nil {
		return nil, s.generateErr
	}
	return &captcha.Captcha{ID: "cap-1", Question: "3 + 5 = ?", TTL: 120}, nil
}

// createTestLoginPageHandler 创建测试用的浏览器登录页处理器
func createTestLoginPageHandler(t *testing.T, captchaSvc *stubCaptchaVerifier, secure bool) (*handler.LoginPageHandler, *mock.Store) {
	t.Helper()
	store := mock.New()
	passwordSvc := crypto.NewPasswordService(4)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwtSvc := crypto.NewJWTService(privateKey, &privateKey.PublicKey, "test-issuer", 15*time.Minute, 7*24*time.Hour)

	authSvc := service.NewAuthService(store, passwordSvc, jwtSvc, 5, 30*time.Minute)
	return handler.NewLoginPageHandler(authSvc, captchaSvc, secure), store
}

// seedLoginPageUser 注入一个已验证邮箱的测试用户（密码 Password123!）
func seedLoginPageUser(t *testing.T, store *mock.Store) {
	t.Helper()
	passwordSvc := crypto.NewPasswordService(4)
	hashed, err := passwordSvc.HashPassword("Password123!")
	require.NoError(t, err)
	store.AddUser(&model.User{
		ID:            "lp-user",
		Email:         "lp@example.com",
		PasswordHash:  hashed,
		EmailVerified: true,
		Status:        model.UserStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})
}

// newLoginPageForm 构造表单 POST 请求
func newLoginPageForm(form url.Values) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(http.MethodPost, "/ui/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return httptest.NewRecorder(), req
}

// ============================================================================
// GET /ui/login
// ============================================================================

func TestLoginPageHandler_HandlePage(t *testing.T) {
	t.Run("未触发验证码时不渲染验证码输入区", func(t *testing.T) {
		h, _ := createTestLoginPageHandler(t, &stubCaptchaVerifier{}, false)
		req := httptest.NewRequest(http.MethodGet, "/ui/login?redirect=/api/v1/authorize", nil)
		w := httptest.NewRecorder()

		h.HandlePage(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), `name="captcha_answer"`)
	})

	t.Run("IP维度触发时渲染验证码题目", func(t *testing.T) {
		h, _ := createTestLoginPageHandler(t, &stubCaptchaVerifier{enabled: true, requireByIP: true}, false)
		req := httptest.NewRequest(http.MethodGet, "/ui/login?redirect=/api/v1/authorize", nil)
		w := httptest.NewRecorder()

		h.HandlePage(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, `name="captcha_answer"`)
		assert.Contains(t, body, "3 + 5 = ?")
		assert.Contains(t, body, `name="captcha_id" value="cap-1"`)
	})

	t.Run("账号维度触发回跳captcha=1时也渲染验证码", func(t *testing.T) {
		h, _ := createTestLoginPageHandler(t, &stubCaptchaVerifier{enabled: true}, false)
		req := httptest.NewRequest(http.MethodGet, "/ui/login?redirect=/api/v1/authorize&captcha=1", nil)
		w := httptest.NewRecorder()

		h.HandlePage(w, req)

		assert.Contains(t, w.Body.String(), `name="captcha_answer"`)
	})

	t.Run("验证码生成失败降级为不渲染", func(t *testing.T) {
		h, _ := createTestLoginPageHandler(t, &stubCaptchaVerifier{
			enabled: true, requireByIP: true, generateErr: errors.New("store down"),
		}, false)
		req := httptest.NewRequest(http.MethodGet, "/ui/login?redirect=/api/v1/authorize", nil)
		w := httptest.NewRecorder()

		h.HandlePage(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), `name="captcha_answer"`)
	})
}

// ============================================================================
// POST /ui/login
// ============================================================================

func TestLoginPageHandler_HandleSubmit(t *testing.T) {
	t.Run("登录成功：302回跳并下發会话Cookie", func(t *testing.T) {
		h, store := createTestLoginPageHandler(t, &stubCaptchaVerifier{}, false)
		seedLoginPageUser(t, store)

		w, req := newLoginPageForm(url.Values{
			"email":    {"lp@example.com"},
			"password": {"Password123!"},
			"redirect": {"/api/v1/authorize?client_id=test-client"},
		})
		h.HandleSubmit(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/api/v1/authorize?client_id=test-client", w.Header().Get("Location"))

		var session *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == "sso_session" {
				session = c
			}
		}
		require.NotNil(t, session, "登录成功应下發 sso_session cookie")
		assert.NotEmpty(t, session.Value)
		assert.True(t, session.HttpOnly)
		assert.False(t, session.Secure, "非生产环境不带 Secure")
		assert.Equal(t, http.SameSiteLaxMode, session.SameSite)
	})

	t.Run("生产模式Cookie带Secure", func(t *testing.T) {
		h, store := createTestLoginPageHandler(t, &stubCaptchaVerifier{}, true)
		seedLoginPageUser(t, store)

		w, req := newLoginPageForm(url.Values{
			"email":    {"lp@example.com"},
			"password": {"Password123!"},
			"redirect": {"/api/v1/authorize"},
		})
		h.HandleSubmit(w, req)

		require.Equal(t, http.StatusFound, w.Code)
		for _, c := range w.Result().Cookies() {
			if c.Name == "sso_session" {
				assert.True(t, c.Secure, "生产环境必须带 Secure")
				return
			}
		}
		t.Fatal("登录成功应下發 sso_session cookie")
	})

	t.Run("开放重定向被拦截", func(t *testing.T) {
		h, store := createTestLoginPageHandler(t, &stubCaptchaVerifier{}, false)
		seedLoginPageUser(t, store)

		for _, evil := range []string{"https://evil.com/callback", "//evil.com/callback"} {
			w, req := newLoginPageForm(url.Values{
				"email":    {"lp@example.com"},
				"password": {"Password123!"},
				"redirect": {evil},
			})
			h.HandleSubmit(w, req)

			assert.Equal(t, http.StatusFound, w.Code)
			assert.Equal(t, "/", w.Header().Get("Location"), "站外重定向应回落到 /: %s", evil)
		}
	})

	t.Run("密码错误：302回登录页且不下發Cookie", func(t *testing.T) {
		h, store := createTestLoginPageHandler(t, &stubCaptchaVerifier{}, false)
		seedLoginPageUser(t, store)

		w, req := newLoginPageForm(url.Values{
			"email":    {"lp@example.com"},
			"password": {"WrongPassword!"},
			"redirect": {"/api/v1/authorize"},
		})
		h.HandleSubmit(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		location := w.Header().Get("Location")
		assert.Contains(t, location, "/ui/login")
		assert.Contains(t, location, "error=")
		for _, c := range w.Result().Cookies() {
			assert.NotEqual(t, "sso_session", c.Name, "登录失败不应下發会话 cookie")
		}
	})

	t.Run("验证码触发但未提交：302回登录页并强制渲染验证码", func(t *testing.T) {
		h, store := createTestLoginPageHandler(t, &stubCaptchaVerifier{enabled: true, requireByAcct: true}, false)
		seedLoginPageUser(t, store)

		w, req := newLoginPageForm(url.Values{
			"email":    {"lp@example.com"},
			"password": {"Password123!"},
			"redirect": {"/api/v1/authorize"},
		})
		h.HandleSubmit(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		location := w.Header().Get("Location")
		assert.Contains(t, location, "captcha=1")
		assert.Contains(t, location, "error=")
	})

	t.Run("验证码错误：302回登录页", func(t *testing.T) {
		h, store := createTestLoginPageHandler(t, &stubCaptchaVerifier{
			enabled: true, requireByIP: true, verifyOK: false,
		}, false)
		seedLoginPageUser(t, store)

		w, req := newLoginPageForm(url.Values{
			"email":          {"lp@example.com"},
			"password":       {"Password123!"},
			"redirect":       {"/api/v1/authorize"},
			"captcha_id":     {"cap-1"},
			"captcha_answer": {"999"},
		})
		h.HandleSubmit(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		location := w.Header().Get("Location")
		assert.Contains(t, location, "captcha=1")
		assert.Contains(t, location, "error=")
	})

	t.Run("验证码正确：正常登录", func(t *testing.T) {
		h, store := createTestLoginPageHandler(t, &stubCaptchaVerifier{
			enabled: true, requireByIP: true, verifyOK: true,
		}, false)
		seedLoginPageUser(t, store)

		w, req := newLoginPageForm(url.Values{
			"email":          {"lp@example.com"},
			"password":       {"Password123!"},
			"redirect":       {"/api/v1/authorize"},
			"captcha_id":     {"cap-1"},
			"captcha_answer": {"8"},
		})
		h.HandleSubmit(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/api/v1/authorize", w.Header().Get("Location"))
	})
}
