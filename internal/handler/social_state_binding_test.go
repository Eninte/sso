// Package handler_test T11 社交登录 state 会话绑定测试（M2：login CSRF 防护）
package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// stateBindingTestKey 测试用 HMAC 密钥（模拟 MFA_RECOVERY_HMAC_KEY）
var stateBindingTestKey = []byte("test-state-binding-hmac-key-32b")

// newMockSocialService 创建指向 httptest mock 服务器的社交登录服务
func newMockSocialService(t *testing.T, server *httptest.Server) service.SocialLoginServiceInterface {
	t.Helper()
	providers := map[string]*service.OAuthProvider{
		"google": {
			Name:         "google",
			ClientID:     "g-id",
			ClientSecret: "g-secret",
			AuthURL:      server.URL + "/auth",
			TokenURL:     server.URL + "/token",
			UserInfoURL:  server.URL + "/userinfo",
			Scopes:       []string{"email", "profile"},
		},
	}
	return service.NewSocialLoginServiceWithProviders(mock.New(), createTestJWTService(), providers, http.DefaultClient)
}

// newSocialMockServer 创建 mock OAuth 服务器（token + userinfo 端点）
func newSocialMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token",
			"token_type":   "bearer",
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"sub":            "google-user-1",
			"email":          "binding@gmail.com",
			"email_verified": true,
			"name":           "Binding User",
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func setProviderVar(req *http.Request, provider string) *http.Request {
	return mux.SetURLVars(req, map[string]string{"provider": provider})
}

// findStateCookie 从响应中提取 state 指纹 Cookie
func findStateCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == "sso_social_state" {
			return c
		}
	}
	return nil
}

// extractState 从重定向 Location 中提取 state 参数
func extractState(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	u, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	return u.Query().Get("state")
}

// ============================================================================
// 发起端测试
// ============================================================================

func TestSocialLogin_StateBinding_Initiator(t *testing.T) {
	server := newSocialMockServer(t)

	t.Run("绑定开启_忽略客户端state_下发指纹Cookie", func(t *testing.T) {
		h := handler.NewSocialLoginHandler(newMockSocialService(t, server)).
			WithStateBinding(true, stateBindingTestKey, false)

		req := setProviderVar(httptest.NewRequest("GET", "/auth/google?state=client-controlled-state", nil), "google")
		w := httptest.NewRecorder()
		h.HandleLogin(w, req)

		require.Equal(t, http.StatusTemporaryRedirect, w.Code)
		state := extractState(t, w)
		assert.NotEqual(t, "client-controlled-state", state, "客户端传入的 state 必须被忽略")
		assert.GreaterOrEqual(t, len(state), 40, "服务端生成的 state 应具有足够熵（32 字节 base64url）")

		cookie := findStateCookie(t, w)
		require.NotNil(t, cookie, "绑定开启时必须下发 state 指纹 Cookie")
		assert.True(t, cookie.HttpOnly)
		assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
		assert.Equal(t, "/auth", cookie.Path)
		assert.False(t, cookie.Secure, "非生产环境不带 Secure")
		assert.NotEmpty(t, cookie.Value)
	})

	t.Run("绑定开启_生产模式_Cookie带Secure", func(t *testing.T) {
		h := handler.NewSocialLoginHandler(newMockSocialService(t, server)).
			WithStateBinding(true, stateBindingTestKey, true)

		req := setProviderVar(httptest.NewRequest("GET", "/auth/google", nil), "google")
		w := httptest.NewRecorder()
		h.HandleLogin(w, req)

		cookie := findStateCookie(t, w)
		require.NotNil(t, cookie)
		assert.True(t, cookie.Secure, "生产环境必须带 Secure")
	})

	t.Run("绑定关闭_回显客户端state_无Cookie", func(t *testing.T) {
		h := handler.NewSocialLoginHandler(newMockSocialService(t, server)) // 未启用绑定（旧行为）

		req := setProviderVar(httptest.NewRequest("GET", "/auth/google?state=client-state-legacy", nil), "google")
		w := httptest.NewRecorder()
		h.HandleLogin(w, req)

		require.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "client-state-legacy", extractState(t, w), "绑定关闭时应保持旧行为（回显客户端 state）")
		assert.Nil(t, findStateCookie(t, w), "绑定关闭时不下发指纹 Cookie")
	})
}

// ============================================================================
// 回调端测试
// ============================================================================

func TestSocialLogin_StateBinding_Callback(t *testing.T) {
	server := newSocialMockServer(t)

	// initiate 完成发起端流程，返回 (state, fingerprintCookie)
	initiate := func(t *testing.T, h *handler.SocialLoginHandler) (string, *http.Cookie) {
		t.Helper()
		req := setProviderVar(httptest.NewRequest("GET", "/auth/google", nil), "google")
		w := httptest.NewRecorder()
		h.HandleLogin(w, req)
		require.Equal(t, http.StatusTemporaryRedirect, w.Code)
		return extractState(t, w), findStateCookie(t, w)
	}

	doCallback := func(h *handler.SocialLoginHandler, state string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := setProviderVar(httptest.NewRequest("GET", "/auth/google/callback?code=mock-code&state="+url.QueryEscape(state), nil), "google")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.HandleCallback(w, req)
		return w
	}

	t.Run("无Cookie_拒绝", func(t *testing.T) {
		h := handler.NewSocialLoginHandler(newMockSocialService(t, server)).
			WithStateBinding(true, stateBindingTestKey, false)
		state, _ := initiate(t, h)

		w := doCallback(h, state, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "OAUTH_STATE_INVALID")
	})

	t.Run("指纹不匹配_拒绝", func(t *testing.T) {
		h := handler.NewSocialLoginHandler(newMockSocialService(t, server)).
			WithStateBinding(true, stateBindingTestKey, false)
		state, cookie := initiate(t, h)

		tampered := &http.Cookie{Name: cookie.Name, Value: "tampered-fingerprint", Path: cookie.Path}
		w := doCallback(h, state, tampered)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "OAUTH_STATE_INVALID")
	})

	t.Run("伪造state_即使有合法Cookie_拒绝", func(t *testing.T) {
		h := handler.NewSocialLoginHandler(newMockSocialService(t, server)).
			WithStateBinding(true, stateBindingTestKey, false)
		_, cookie := initiate(t, h)

		// 攻击者用别的会话的 cookie + 自己构造的 state
		w := doCallback(h, "attacker-forged-state", cookie)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "OAUTH_STATE_INVALID")
	})

	t.Run("正常流程_指纹匹配_登录成功", func(t *testing.T) {
		h := handler.NewSocialLoginHandler(newMockSocialService(t, server)).
			WithStateBinding(true, stateBindingTestKey, false)
		state, cookie := initiate(t, h)
		require.NotNil(t, cookie)

		w := doCallback(h, state, cookie)
		assert.Equal(t, http.StatusOK, w.Code, "指纹匹配且 state 有效时应完成登录")
		assert.Contains(t, w.Body.String(), "access_token")
	})

	t.Run("绑定关闭_无Cookie_兼容旧行为", func(t *testing.T) {
		svc := newMockSocialService(t, server)
		h := handler.NewSocialLoginHandler(svc) // 未启用绑定

		// 旧行为：客户端可携带自己的 state，回调无需 Cookie
		authURL, err := svc.GetAuthorizationURL("google", "legacy-client-state")
		require.NoError(t, err)
		require.True(t, strings.Contains(authURL, "state=legacy-client-state"))

		w := doCallback(h, "legacy-client-state", nil)
		assert.Equal(t, http.StatusOK, w.Code, "绑定关闭时无 Cookie 应兼容旧行为")
	})
}
