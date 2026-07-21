// Package app 路由装配单元测试
package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

// sensitiveRouteMarker 标记请求是否经过了敏感限流子路由的中间件
const sensitiveRouteMarker = "X-Sensitive-Route"

// newTestRouters 构造与 initHandlers 相同层级的测试路由：
// root -> api(/api/v1) -> sensitive（带标记中间件，模拟敏感限流器）
func newTestRouters() (root, api, sensitive *mux.Router) {
	root = mux.NewRouter()
	api = root.PathPrefix("/api/v1").Subrouter()
	sensitive = api.PathPrefix("").Subrouter()
	sensitive.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(sensitiveRouteMarker, "on")
			next.ServeHTTP(w, r)
		})
	})
	return root, api, sensitive
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// TestRegisterTokenEndpoint_SensitiveEnabled T13：敏感限流启用时 /token 必须注册到敏感子路由
func TestRegisterTokenEndpoint_SensitiveEnabled(t *testing.T) {
	root, api, sensitive := newTestRouters()
	registerTokenEndpoint(api, sensitive, okHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/token", nil)
	w := httptest.NewRecorder()
	root.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "on", w.Header().Get(sensitiveRouteMarker), "/token 应经过敏感限流子路由")
}

// TestRegisterTokenEndpoint_SensitiveDisabled T13：敏感限流未启用时回退注册到全局限流路由
func TestRegisterTokenEndpoint_SensitiveDisabled(t *testing.T) {
	root, api, _ := newTestRouters()
	registerTokenEndpoint(api, nil, okHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/token", nil)
	w := httptest.NewRecorder()
	root.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get(sensitiveRouteMarker), "敏感限流未启用时不应经过敏感子路由")
}

// TestRegisterTokenEndpoint_GeneralPublicEndpointUnaffected T13：一般公开端点不进入敏感子路由
func TestRegisterTokenEndpoint_GeneralPublicEndpointUnaffected(t *testing.T) {
	root, api, sensitive := newTestRouters()
	registerTokenEndpoint(api, sensitive, okHandler)
	api.HandleFunc("/captcha", okHandler).Methods("GET")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/captcha", nil)
	w := httptest.NewRecorder()
	root.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get(sensitiveRouteMarker), "/captcha 应使用全局限流而非敏感限流")
}

// TestRegisterTokenEndpoint_OnlyPostMethod T13：/token 仅注册 POST 方法
func TestRegisterTokenEndpoint_OnlyPostMethod(t *testing.T) {
	root, api, sensitive := newTestRouters()
	registerTokenEndpoint(api, sensitive, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/token", nil)
	w := httptest.NewRecorder()
	root.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
