// Package handler_test AdminHandler 客户端管理单元测试
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// createTestAdminClientHandler 创建带密码服务的管理员处理器（客户端管理需要）
func createTestAdminClientHandler() (*handler.AdminHandler, *mock.Store) {
	store := mock.New()
	adminSvc := service.NewAdminServiceWithOptions(store,
		service.WithAdminPasswordService(crypto.NewPasswordService(4)),
	)
	return handler.NewAdminHandler(adminSvc), store
}

// newClientRouter 构造带路径参数的 mux 路由器并注册单一路由
func newClientRouter(h *handler.AdminHandler, path string, fn func(http.ResponseWriter, *http.Request)) *mux.Router {
	router := mux.NewRouter()
	router.HandleFunc(path, fn)
	return router
}

// doAdminClientRequest 执行带路径参数的客户端管理请求
func doAdminClientRequest(t *testing.T, h *handler.AdminHandler, router *mux.Router, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req = addAdminContext(req, "admin@example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// seedHandlerClient 向 mock store 注入客户端
func seedHandlerClient(store *mock.Store, clientID string, public bool) *model.Client {
	client := &model.Client{
		ID:           "id-" + clientID,
		ClientID:     clientID,
		Name:         "Handler Client " + clientID,
		RedirectURIs: []string{"https://example.com/callback"},
		GrantTypes:   []string{model.GrantTypeAuthorizationCode, model.GrantTypeRefreshToken},
		Scopes:       []string{"openid", "profile", "email"},
		PublicClient: public,
		IsActive:     true,
		CreatedAt:    time.Now(),
	}
	store.AddClient(client)
	return client
}

func validHandlerCreateBody() map[string]interface{} {
	return map[string]interface{}{
		"name":          "内部 Wiki",
		"redirect_uris": []string{"https://wiki.example.com/oauth/callback"},
		"grant_types":   []string{"authorization_code", "refresh_token"},
		"scopes":        []string{"openid", "profile", "email"},
		"public_client": false,
	}
}

// ============================================================================
// HandleCreateClient
// ============================================================================

func TestAdminHandler_HandleCreateClient(t *testing.T) {
	h, _ := createTestAdminClientHandler()
	router := newClientRouter(h, "/admin/clients", h.HandleCreateClient)

	t.Run("机密客户端创建成功返回201且含一次性secret", func(t *testing.T) {
		w := doAdminClientRequest(t, h, router, "POST", "/admin/clients", validHandlerCreateBody())
		assert.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		secret, ok := resp["client_secret"].(string)
		require.True(t, ok, "机密客户端响应应包含 client_secret")
		assert.Len(t, secret, 64)
		assert.NotEmpty(t, resp["client_id"])
		assert.Equal(t, true, resp["is_active"])
	})

	t.Run("公共客户端创建成功无client_secret字段", func(t *testing.T) {
		body := validHandlerCreateBody()
		body["public_client"] = true
		w := doAdminClientRequest(t, h, router, "POST", "/admin/clients", body)
		assert.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		_, hasSecret := resp["client_secret"]
		assert.False(t, hasSecret, "公共客户端响应不应包含 client_secret")
		assert.Equal(t, true, resp["public_client"])
	})

	t.Run("无效JSON返回400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/clients", bytes.NewReader([]byte("invalid")))
		req = addAdminContext(req, "admin@example.com")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("参数校验失败返回400", func(t *testing.T) {
		body := validHandlerCreateBody()
		body["redirect_uris"] = []string{"ftp://example.com/cb"}
		w := doAdminClientRequest(t, h, router, "POST", "/admin/clients", body)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("含client_credentials返回400 UNSUPPORTED_GRANT_TYPE", func(t *testing.T) {
		body := validHandlerCreateBody()
		body["grant_types"] = []string{"authorization_code", "client_credentials"}
		w := doAdminClientRequest(t, h, router, "POST", "/admin/clients", body)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp, "code")
		assert.Equal(t, "UNSUPPORTED_GRANT_TYPE", resp["code"])
	})

	t.Run("响应绝不包含密钥哈希", func(t *testing.T) {
		w := doAdminClientRequest(t, h, router, "POST", "/admin/clients", validHandlerCreateBody())
		body := w.Body.String()
		assert.NotContains(t, body, "$2a$", "响应不应包含 bcrypt 哈希")
	})
}

// ============================================================================
// HandleListClients / HandleGetClient
// ============================================================================

func TestAdminHandler_HandleListClients(t *testing.T) {
	h, store := createTestAdminClientHandler()
	router := newClientRouter(h, "/admin/clients", h.HandleListClients)
	seedHandlerClient(store, "list-1", false)
	seedHandlerClient(store, "list-2", true)

	w := doAdminClientRequest(t, h, router, "GET", "/admin/clients?page=1&pageSize=10", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	clients, ok := resp["clients"].([]interface{})
	require.True(t, ok)
	require.GreaterOrEqual(t, len(clients), 2)
	assert.NotEmpty(t, resp["total"])
	assert.NotEmpty(t, resp["page"])
	assert.NotEmpty(t, resp["total_pages"])

	// 列表中绝不包含 secret
	body := w.Body.String()
	assert.NotContains(t, body, "client_secret")
	assert.NotContains(t, body, "$2a$")
}

func TestAdminHandler_HandleGetClient(t *testing.T) {
	h, store := createTestAdminClientHandler()
	router := newClientRouter(h, "/admin/clients/{client_id}", h.HandleGetClient)
	seedHandlerClient(store, "get-1", false)

	t.Run("存在返回客户端", func(t *testing.T) {
		w := doAdminClientRequest(t, h, router, "GET", "/admin/clients/get-1", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "get-1", resp["client_id"])
		assert.NotContains(t, w.Body.String(), "client_secret")
	})

	t.Run("不存在返回404", func(t *testing.T) {
		w := doAdminClientRequest(t, h, router, "GET", "/admin/clients/missing", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ============================================================================
// HandleUpdateClient
// ============================================================================

func TestAdminHandler_HandleUpdateClient(t *testing.T) {
	h, store := createTestAdminClientHandler()
	router := newClientRouter(h, "/admin/clients/{client_id}", h.HandleUpdateClient)
	seedHandlerClient(store, "upd-1", false)

	t.Run("更新成功", func(t *testing.T) {
		body := map[string]interface{}{
			"name":          "内部 Wiki v2",
			"redirect_uris": []string{"https://wiki.example.com/oauth/callback"},
			"grant_types":   []string{"authorization_code", "refresh_token"},
			"scopes":        []string{"openid", "email"},
		}
		w := doAdminClientRequest(t, h, router, "PUT", "/admin/clients/upd-1", body)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "内部 Wiki v2", resp["name"])
		assert.NotContains(t, w.Body.String(), "client_secret")
	})

	t.Run("修改public_client返回400", func(t *testing.T) {
		body := map[string]interface{}{
			"name":          "x",
			"redirect_uris": []string{"https://example.com/cb"},
			"public_client": true, // 当前为 false，试图修改
		}
		w := doAdminClientRequest(t, h, router, "PUT", "/admin/clients/upd-1", body)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "PUBLIC_CLIENT_IMMUTABLE", resp["code"])
	})

	t.Run("不存在返回404", func(t *testing.T) {
		body := map[string]interface{}{"name": "x", "redirect_uris": []string{"https://example.com/cb"}}
		w := doAdminClientRequest(t, h, router, "PUT", "/admin/clients/missing", body)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ============================================================================
// HandleDisableClient / HandleEnableClient
// ============================================================================

func TestAdminHandler_HandleDisableEnableClient(t *testing.T) {
	h, store := createTestAdminClientHandler()
	disableRouter := newClientRouter(h, "/admin/clients/{client_id}/disable", h.HandleDisableClient)
	enableRouter := newClientRouter(h, "/admin/clients/{client_id}/enable", h.HandleEnableClient)
	seedHandlerClient(store, "toggle-1", false)

	t.Run("禁用成功", func(t *testing.T) {
		w := doAdminClientRequest(t, h, disableRouter, "POST", "/admin/clients/toggle-1/disable", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "客户端已禁用", resp["message"])
	})

	t.Run("禁用幂等", func(t *testing.T) {
		w := doAdminClientRequest(t, h, disableRouter, "POST", "/admin/clients/toggle-1/disable", nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("启用成功", func(t *testing.T) {
		w := doAdminClientRequest(t, h, enableRouter, "POST", "/admin/clients/toggle-1/enable", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "客户端已启用", resp["message"])
	})

	t.Run("禁用不存在返回404", func(t *testing.T) {
		w := doAdminClientRequest(t, h, disableRouter, "POST", "/admin/clients/missing/disable", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ============================================================================
// HandleRotateClientSecret
// ============================================================================

func TestAdminHandler_HandleRotateClientSecret(t *testing.T) {
	h, store := createTestAdminClientHandler()
	router := newClientRouter(h, "/admin/clients/{client_id}/rotate-secret", h.HandleRotateClientSecret)
	seedHandlerClient(store, "rot-1", false)

	t.Run("轮换成功返回一次性新secret", func(t *testing.T) {
		w := doAdminClientRequest(t, h, router, "POST", "/admin/clients/rot-1/rotate-secret", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		secret, ok := resp["client_secret"].(string)
		require.True(t, ok)
		assert.Len(t, secret, 64)
	})

	t.Run("公共客户端返回400", func(t *testing.T) {
		seedHandlerClient(store, "rot-pub", true)
		w := doAdminClientRequest(t, h, router, "POST", "/admin/clients/rot-pub/rotate-secret", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "SECRET_NOT_SUPPORTED_FOR_PUBLIC_CLIENT", resp["code"])
	})

	t.Run("不存在返回404", func(t *testing.T) {
		w := doAdminClientRequest(t, h, router, "POST", "/admin/clients/missing/rotate-secret", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ============================================================================
// HandleDeleteClient
// ============================================================================

func TestAdminHandler_HandleDeleteClient(t *testing.T) {
	h, store := createTestAdminClientHandler()
	router := newClientRouter(h, "/admin/clients/{client_id}", h.HandleDeleteClient)
	seedHandlerClient(store, "del-1", false)

	t.Run("删除成功", func(t *testing.T) {
		w := doAdminClientRequest(t, h, router, "DELETE", "/admin/clients/del-1", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "客户端已删除", resp["message"])
	})

	t.Run("不存在返回404", func(t *testing.T) {
		w := doAdminClientRequest(t, h, router, "DELETE", "/admin/clients/missing", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ============================================================================
// 响应序列化安全性
// ============================================================================

func TestAdminHandler_ClientResponseNeverLeaksSecretHash(t *testing.T) {
	client := &model.Client{
		ClientID:     "leak-test",
		ClientSecret: "$2a$10$supersecrethashvalue1234567890",
		Name:         "Leak Test",
		RedirectURIs: []string{"https://example.com/cb"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"openid"},
		IsActive:     true,
	}
	// clientResponse 是 handler 包私有函数，通过真实 handler 路径验证：
	// 注入含哈希的客户端，列表/查询响应不得出现哈希
	h, store := createTestAdminClientHandler()
	store.AddClient(client)

	router := newClientRouter(h, "/admin/clients/{client_id}", h.HandleGetClient)
	w := doAdminClientRequest(t, h, router, "GET", "/admin/clients/leak-test", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.NotContains(t, body, "$2a$10$supersecrethashvalue")
	assert.NotContains(t, body, strings.ToLower("supersecret"), "不得泄露密钥相关内容")
}
