// Package middleware_test GetClientIP 客户端IP解析测试
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/sso/internal/middleware"

	"github.com/stretchr/testify/assert"
)

// withTrustedProxies 设置受信代理列表并在测试结束后重置
func withTrustedProxies(t *testing.T, proxies ...string) {
	t.Helper()
	middleware.SetTrustedProxies(proxies)
	t.Cleanup(func() { middleware.SetTrustedProxies(nil) })
}

func TestGetClientIP(t *testing.T) {
	t.Run("无代理头时返回RemoteAddr", func(t *testing.T) {
		withTrustedProxies(t, "10.0.0.0/8")

		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "203.0.113.5:54321"

		assert.Equal(t, "203.0.113.5", middleware.GetClientIP(req))
	})

	t.Run("默认无受信代理时忽略全部代理头", func(t *testing.T) {
		withTrustedProxies(t) // 空列表

		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.0.2.10:443"
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		req.Header.Set("X-Real-IP", "1.2.3.4")

		assert.Equal(t, "192.0.2.10", middleware.GetClientIP(req))
	})

	t.Run("非受信来源伪造XFF被忽略", func(t *testing.T) {
		withTrustedProxies(t, "10.0.0.0/8")

		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "198.51.100.7:1234"
		req.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")

		// 攻击者直接伪造 XFF 不得影响限流/审计取 IP
		assert.Equal(t, "198.51.100.7", middleware.GetClientIP(req))
	})

	t.Run("受信代理单跳XFF取真实客户端", func(t *testing.T) {
		withTrustedProxies(t, "10.0.0.0/8")

		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:443"
		req.Header.Set("X-Forwarded-For", "203.0.113.5")

		assert.Equal(t, "203.0.113.5", middleware.GetClientIP(req))
	})

	t.Run("多跳XFF从右向左跳过受信代理链", func(t *testing.T) {
		withTrustedProxies(t, "10.0.0.0/8", "192.168.0.0/16")

		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:443"
		// 左侧 6.6.6.6 为客户端伪造，右侧 192.168.1.2 为内部可信代理
		req.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9, 192.168.1.2")

		assert.Equal(t, "203.0.113.9", middleware.GetClientIP(req))
	})

	t.Run("XFF全部为受信代理时回退RemoteAddr", func(t *testing.T) {
		withTrustedProxies(t, "10.0.0.0/8")

		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.9:443"
		req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")

		assert.Equal(t, "10.0.0.9", middleware.GetClientIP(req))
	})

	t.Run("兼容X-Real-IP单跳代理", func(t *testing.T) {
		withTrustedProxies(t, "172.16.0.5")

		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "172.16.0.5:443"
		req.Header.Set("X-Real-IP", "203.0.113.77")

		assert.Equal(t, "203.0.113.77", middleware.GetClientIP(req))
	})

	t.Run("XFF存在时优先于X-Real-IP", func(t *testing.T) {
		withTrustedProxies(t, "172.16.0.5")

		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "172.16.0.5:443"
		req.Header.Set("X-Forwarded-For", "203.0.113.1")
		req.Header.Set("X-Real-IP", "203.0.113.2")

		assert.Equal(t, "203.0.113.1", middleware.GetClientIP(req))
	})

	t.Run("XFF含非法条目时跳过继续查找", func(t *testing.T) {
		withTrustedProxies(t, "10.0.0.0/8")

		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:443"
		req.Header.Set("X-Forwarded-For", "203.0.113.5, not-an-ip, unknown")

		assert.Equal(t, "203.0.113.5", middleware.GetClientIP(req))
	})

	t.Run("RemoteAddr无法解析时返回原始值", func(t *testing.T) {
		withTrustedProxies(t, "10.0.0.0/8")

		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "unix-socket"

		assert.Equal(t, "unix-socket", middleware.GetClientIP(req))
	})
}

func TestGetClientIP_HTTPThroughMiddleware(t *testing.T) {
	// 通过中间件链路验证：429 响应的限流 key 应按 XFF 最右非受信 IP 分桶
	withTrustedProxies(t, "10.0.0.0/8")

	rl := middleware.NewRateLimiter(2, time.Minute) // 窗口内最多2次
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 两个不同真实客户端经同一受信代理访问，应各自独立计数
	for i := 0; i < 2; i++ {
		for _, client := range []string{"203.0.113.5", "203.0.113.6"} {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "10.0.0.1:443"
			req.Header.Set("X-Forwarded-For", client)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code, "client=%s iter=%d", client, i)
		}
	}

	// 伪造最左 XFF 不得偷取他人配额：仍受自身真实 IP 限制
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 203.0.113.5")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}
