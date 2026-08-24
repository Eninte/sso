// Package service_test 管理员客户端管理单元测试
package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store/mock"
)

// ============================================================================
// 辅助函数
// ============================================================================

// newAdminClientService 构造带缓存与密码服务的 AdminService（不含审计）
func newAdminClientService(t *testing.T) (*service.AdminService, *mock.Store, cache.Cache) {
	t.Helper()
	store := mock.New()
	memCache := cache.NewMemoryCache()
	adminSvc := service.NewAdminServiceWithOptions(store,
		service.WithAdminCache(memCache),
		service.WithAdminPasswordService(crypto.NewPasswordService(4)),
	)
	return adminSvc, store, memCache
}

// newAdminClientServiceWithAudit 构造带缓存、密码服务与审计的 AdminService
func newAdminClientServiceWithAudit(t *testing.T) (*service.AdminService, *mock.Store, cache.Cache) {
	t.Helper()
	store := mock.New()
	memCache := cache.NewMemoryCache()
	auditSvc := service.NewAuditService(store)
	t.Cleanup(func() { auditSvc.Close() })
	adminSvc := service.NewAdminServiceWithOptions(store,
		service.WithAdminCache(memCache),
		service.WithAdminPasswordService(crypto.NewPasswordService(4)),
		service.WithAdminAudit(auditSvc),
	)
	return adminSvc, store, memCache
}

// seedAdminTestClient 向 mock store 直接注入一个客户端
func seedAdminTestClient(store *mock.Store, clientID string, public bool) *model.Client {
	client := &model.Client{
		ID:           "id-" + clientID,
		ClientID:     clientID,
		Name:         "Test Client " + clientID,
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

// seedClientToken 向 mock store 注入一个关联到指定客户端的 token
func seedClientToken(store *mock.Store, clientID string, accessToken string) *model.Token {
	token := &model.Token{
		ID:           "id-" + accessToken,
		AccessToken:  accessToken,
		RefreshToken: "refresh-" + accessToken,
		UserID:       "user-1",
		ClientID:     &clientID,
		Scopes:       []string{"openid"},
		ExpiresAt:    time.Now().Add(15 * time.Minute),
		CreatedAt:    time.Now(),
	}
	store.AddToken(token)
	return token
}

// presetClientCache 预置客户端缓存，用于断言操作后缓存已失效
func presetClientCache(t *testing.T, memCache cache.Cache, clientID string) {
	t.Helper()
	stale := &model.Client{ClientID: clientID, Name: "stale", IsActive: true}
	require.NoError(t, memCache.Set(context.Background(), cache.ClientKey(clientID), stale, time.Minute))
}

// assertCacheInvalidated 断言客户端缓存已失效
func assertCacheInvalidated(t *testing.T, memCache cache.Cache, clientID string) {
	t.Helper()
	var out model.Client
	err := memCache.Get(context.Background(), cache.ClientKey(clientID), &out)
	assert.ErrorIs(t, err, cache.ErrCacheMiss, "操作后客户端缓存应已失效")
}

// validCreateRequest 返回一个合法的创建请求
func validCreateRequest() model.ClientCreateRequest {
	return model.ClientCreateRequest{
		Name:         "内部 Wiki",
		RedirectURIs: []string{"https://wiki.example.com/oauth/callback"},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		Scopes:       []string{"openid", "profile", "email"},
		PublicClient: false,
	}
}

// ============================================================================
// CreateClient
// ============================================================================

func TestAdminService_CreateClient(t *testing.T) {
	ctx := context.Background()

	t.Run("机密客户端生成64位hex密钥且入库为哈希", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()

		client, secret, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.NoError(t, err)
		require.NotEmpty(t, secret)
		assert.Len(t, secret, 64, "密钥应为 64 位 hex")
		assert.Regexp(t, "^[0-9a-f]{64}$", secret)
		// 入库的是 bcrypt 哈希，绝非明文
		assert.NotEqual(t, secret, client.ClientSecret)
		// 新客户端默认启用（服务层显式设置）
		assert.True(t, client.IsActive)
		// 注：updated_at 由数据库 DEFAULT 填充，mock 不模拟该行为，真实行为由
		// store/postgres 的集成测试 TestClientStore_CreateGetRoundtrip 覆盖

		// 密钥可通过 VerifyPassword 验证
		pw := crypto.NewPasswordService(4)
		require.NoError(t, pw.VerifyPassword(client.ClientSecret, secret))

		// 返回的客户端不暴露哈希
		data, _ := json.Marshal(client)
		assert.NotContains(t, string(data), client.ClientSecret)
	})

	t.Run("公共客户端不生成密钥", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.PublicClient = true

		client, secret, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.NoError(t, err)
		assert.Empty(t, secret)
		assert.Empty(t, client.ClientSecret)

		// 入库确认
		got, err := store.GetByClientID(ctx, client.ClientID)
		require.NoError(t, err)
		assert.Empty(t, got.ClientSecret)
		assert.True(t, got.PublicClient)
	})

	t.Run("名称必填", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.Name = ""
		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, 400, apperrors.GetHTTPStatus(err))
	})

	t.Run("名称超长", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.Name = strings.Repeat("名", 256)
		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, 400, apperrors.GetHTTPStatus(err))
	})

	t.Run("重定向URI必填", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.RedirectURIs = nil
		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, 400, apperrors.GetHTTPStatus(err))
	})

	t.Run("重定向URI非法scheme被拒", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.RedirectURIs = []string{"ftp://example.com/cb"}
		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, 400, apperrors.GetHTTPStatus(err))
	})

	t.Run("重定向URI含fragment被拒", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.RedirectURIs = []string{"https://example.com/cb#frag"}
		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, 400, apperrors.GetHTTPStatus(err))
	})

	t.Run("重定向URI重复自动去重", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.RedirectURIs = []string{"https://example.com/cb", "https://example.com/cb", " https://example.com/cb "}
		client, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://example.com/cb"}, client.RedirectURIs)

		got, err := store.GetByClientID(ctx, client.ClientID)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://example.com/cb"}, got.RedirectURIs)
	})

	t.Run("重定向URI全为空白被拒", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.RedirectURIs = []string{"", "  "}
		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, 400, apperrors.GetHTTPStatus(err))
	})

	t.Run("重定向URI超过20条被拒", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		var uris []string
		for i := 0; i < 21; i++ {
			uris = append(uris, fmt.Sprintf("https://example%d.com/cb", i))
		}
		req.RedirectURIs = uris
		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, 400, apperrors.GetHTTPStatus(err))
	})

	t.Run("grant_types含client_credentials被拒", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.GrantTypes = []string{"authorization_code", "client_credentials"}
		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, apperrors.ErrCodeUnsupportedGrantType, apperrors.GetErrorCode(err))
		assert.Equal(t, 400, apperrors.GetHTTPStatus(err))
	})

	t.Run("grant_types未知值被拒", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.GrantTypes = []string{"implicit"}
		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, apperrors.ErrCodeUnsupportedGrantType, apperrors.GetErrorCode(err))
	})

	t.Run("grant_types缺authorization_code被拒", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.GrantTypes = []string{"refresh_token"}
		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, 400, apperrors.GetHTTPStatus(err))
	})

	t.Run("grant_types为空使用默认值", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.GrantTypes = nil
		client, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.NoError(t, err)
		assert.Equal(t, []string{"authorization_code", "refresh_token"}, client.GrantTypes)
	})

	t.Run("scopes不在白名单被拒", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.Scopes = []string{"openid", "admin"}
		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, apperrors.ErrCodeInvalidScope, apperrors.GetErrorCode(err))
	})

	t.Run("scopes为空使用默认值", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.Scopes = nil
		client, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.NoError(t, err)
		assert.Equal(t, []string{"openid", "profile", "email"}, client.Scopes)
	})

	t.Run("scopes重复自动去重", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := validCreateRequest()
		req.Scopes = []string{"openid", "openid", "profile", ""}
		client, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.NoError(t, err)
		assert.Equal(t, []string{"openid", "profile"}, client.Scopes)
	})

	t.Run("审计日志记录创建事件且不含密钥", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientServiceWithAudit(t)
		req := validCreateRequest()

		client, secret, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.NoError(t, err)

		waitForAuditLogs(ctx, t, store, "operator-1", string(model.EventClientCreated), 1)
		logs, _, err := store.ListAuditLogs(ctx, "operator-1", string(model.EventClientCreated), 0, 10)
		require.NoError(t, err)
		require.NotEmpty(t, logs)
		assert.Contains(t, logs[0].Details, client.ClientID)
		assert.NotContains(t, logs[0].Details, secret, "审计日志不得包含密钥明文或哈希")
		assert.NotContains(t, logs[0].Details, client.ClientSecret)
	})

	t.Run("密码服务未初始化返回500", func(t *testing.T) {
		store := mock.New()
		adminSvc := service.NewAdminServiceWithOptions(store) // 不注入 passwordSvc
		req := validCreateRequest()

		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, 500, apperrors.GetHTTPStatus(err))
	})

	t.Run("store创建失败返回内部错误", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		store.CreateClientErr = errors.New("db down")
		req := validCreateRequest()

		_, _, err := adminSvc.CreateClient(ctx, "operator-1", req)
		require.Error(t, err)
		assert.Equal(t, 500, apperrors.GetHTTPStatus(err))
	})
}

// ============================================================================
// GetClient / ListClients
// ============================================================================

func TestAdminService_GetClient(t *testing.T) {
	ctx := context.Background()
	adminSvc, store, _ := newAdminClientService(t)
	seedAdminTestClient(store, "get-client-1", false)

	t.Run("存在返回客户端", func(t *testing.T) {
		client, err := adminSvc.GetClient(ctx, "get-client-1")
		require.NoError(t, err)
		assert.Equal(t, "get-client-1", client.ClientID)
	})

	t.Run("不存在返回ClientNotFound", func(t *testing.T) {
		_, err := adminSvc.GetClient(ctx, "missing-client")
		require.Error(t, err)
		assert.True(t, errors.Is(err, apperrors.ErrClientNotFound))
		assert.Equal(t, 404, apperrors.GetHTTPStatus(err))
	})
}

func TestAdminService_ListClients(t *testing.T) {
	ctx := context.Background()
	adminSvc, store, _ := newAdminClientService(t)
	seedAdminTestClient(store, "list-client-1", false)
	seedAdminTestClient(store, "list-client-2", true)

	clients, total, err := adminSvc.ListClients(ctx, 0, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, 2)
	assert.GreaterOrEqual(t, len(clients), 2)
}

// ============================================================================
// UpdateClient
// ============================================================================

func TestAdminService_UpdateClient(t *testing.T) {
	ctx := context.Background()

	t.Run("更新成功且失效缓存", func(t *testing.T) {
		adminSvc, store, memCache := newAdminClientService(t)
		seedAdminTestClient(store, "upd-1", false)
		presetClientCache(t, memCache, "upd-1")

		req := model.ClientUpdateRequest{
			Name:         "内部 Wiki v2",
			RedirectURIs: []string{"https://wiki.example.com/oauth/callback", "https://wiki2.example.com/cb"},
			GrantTypes:   []string{"authorization_code", "refresh_token"},
			Scopes:       []string{"openid", "email"},
		}
		updated, err := adminSvc.UpdateClient(ctx, "operator-1", "upd-1", req)
		require.NoError(t, err)
		assert.Equal(t, "内部 Wiki v2", updated.Name)
		assert.Equal(t, []string{"openid", "email"}, updated.Scopes)
		assert.False(t, updated.UpdatedAt.IsZero())

		got, err := store.GetByClientID(ctx, "upd-1")
		require.NoError(t, err)
		assert.Equal(t, "内部 Wiki v2", got.Name)

		assertCacheInvalidated(t, memCache, "upd-1")
	})

	t.Run("客户端不存在返回NotFound", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		req := model.ClientUpdateRequest{Name: "x", RedirectURIs: []string{"https://example.com/cb"}}
		_, err := adminSvc.UpdateClient(ctx, "operator-1", "missing", req)
		require.Error(t, err)
		assert.Equal(t, 404, apperrors.GetHTTPStatus(err))
	})

	t.Run("修改public_client被拒", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		seedAdminTestClient(store, "upd-pub", true)

		req := model.ClientUpdateRequest{Name: "x", RedirectURIs: []string{"https://example.com/cb"}}
		pub := false
		req.PublicClient = &pub
		_, err := adminSvc.UpdateClient(ctx, "operator-1", "upd-pub", req)
		require.Error(t, err)
		assert.Equal(t, apperrors.ErrCodePublicImmutable, apperrors.GetErrorCode(err))
		assert.Equal(t, 400, apperrors.GetHTTPStatus(err))
	})

	t.Run("public_client与当前值一致允许更新", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		seedAdminTestClient(store, "upd-same", false)

		req := model.ClientUpdateRequest{Name: "y", RedirectURIs: []string{"https://example.com/cb"}}
		pub := false
		req.PublicClient = &pub
		_, err := adminSvc.UpdateClient(ctx, "operator-1", "upd-same", req)
		require.NoError(t, err)
	})

	t.Run("审计日志记录更新事件", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientServiceWithAudit(t)
		seedAdminTestClient(store, "upd-audit", false)

		req := model.ClientUpdateRequest{Name: "更新后", RedirectURIs: []string{"https://example.com/cb"}}
		_, err := adminSvc.UpdateClient(ctx, "operator-1", "upd-audit", req)
		require.NoError(t, err)

		waitForAuditLogs(ctx, t, store, "operator-1", string(model.EventClientUpdated), 1)
	})

	t.Run("store更新失败返回内部错误", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		seedAdminTestClient(store, "upd-err", false)
		store.UpdateClientErr = errors.New("db down")

		req := model.ClientUpdateRequest{Name: "x", RedirectURIs: []string{"https://example.com/cb"}}
		_, err := adminSvc.UpdateClient(ctx, "operator-1", "upd-err", req)
		require.Error(t, err)
		assert.Equal(t, 500, apperrors.GetHTTPStatus(err))
	})
}

// ============================================================================
// RotateClientSecret
// ============================================================================

func TestAdminService_RotateClientSecret(t *testing.T) {
	ctx := context.Background()

	t.Run("轮换成功：新旧密钥不同且撤销全部Token", func(t *testing.T) {
		adminSvc, store, memCache := newAdminClientService(t)
		seedAdminTestClient(store, "rot-1", false)
		seedClientToken(store, "rot-1", "rot-token-1")
		seedClientToken(store, "rot-1", "rot-token-2")
		presetClientCache(t, memCache, "rot-1")

		newSecret, err := adminSvc.RotateClientSecret(ctx, "operator-1", "rot-1")
		require.NoError(t, err)
		assert.Len(t, newSecret, 64)
		assert.Regexp(t, "^[0-9a-f]{64}$", newSecret)

		// 入库为哈希，且与旧 secret 不同
		client, err := store.GetByClientID(ctx, "rot-1")
		require.NoError(t, err)
		pw := crypto.NewPasswordService(4)
		require.NoError(t, pw.VerifyPassword(client.ClientSecret, newSecret))

		// 全部 Token 被撤销
		tok1, err := store.GetTokenByAccessToken(ctx, "rot-token-1")
		require.NoError(t, err)
		assert.NotNil(t, tok1.RevokedAt)
		tok2, err := store.GetTokenByAccessToken(ctx, "rot-token-2")
		require.NoError(t, err)
		assert.NotNil(t, tok2.RevokedAt)

		assertCacheInvalidated(t, memCache, "rot-1")
	})

	t.Run("公共客户端不支持轮换", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		seedAdminTestClient(store, "rot-pub", true)

		_, err := adminSvc.RotateClientSecret(ctx, "operator-1", "rot-pub")
		require.Error(t, err)
		assert.Equal(t, apperrors.ErrCodeNoSecretForPublic, apperrors.GetErrorCode(err))
		assert.Equal(t, 400, apperrors.GetHTTPStatus(err))
	})

	t.Run("客户端不存在返回NotFound", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		_, err := adminSvc.RotateClientSecret(ctx, "operator-1", "missing")
		require.Error(t, err)
		assert.Equal(t, 404, apperrors.GetHTTPStatus(err))
	})

	t.Run("密码服务未初始化返回500", func(t *testing.T) {
		store := mock.New()
		adminSvc := service.NewAdminServiceWithOptions(store) // 不注入 passwordSvc
		seedAdminTestClient(store, "rot-nopw", false)

		_, err := adminSvc.RotateClientSecret(ctx, "operator-1", "rot-nopw")
		require.Error(t, err)
		assert.Equal(t, 500, apperrors.GetHTTPStatus(err))
	})

	t.Run("撤销Token失败中止轮换", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		seedAdminTestClient(store, "rot-err", false)
		store.RevokeAllClientTokensErr = errors.New("db down")

		_, err := adminSvc.RotateClientSecret(ctx, "operator-1", "rot-err")
		require.Error(t, err)
	})
}

// ============================================================================
// DisableClient / EnableClient
// ============================================================================

func TestAdminService_DisableClient(t *testing.T) {
	ctx := context.Background()

	t.Run("禁用：状态翻转+撤销Token+失效缓存", func(t *testing.T) {
		adminSvc, store, memCache := newAdminClientService(t)
		seedAdminTestClient(store, "dis-1", false)
		seedClientToken(store, "dis-1", "dis-token-1")
		presetClientCache(t, memCache, "dis-1")

		err := adminSvc.DisableClient(ctx, "operator-1", "dis-1")
		require.NoError(t, err)

		client, err := store.GetByClientID(ctx, "dis-1")
		require.NoError(t, err)
		assert.False(t, client.IsActive)

		tok, err := store.GetTokenByAccessToken(ctx, "dis-token-1")
		require.NoError(t, err)
		assert.NotNil(t, tok.RevokedAt, "禁用应撤销该客户端全部未撤销 Token")

		assertCacheInvalidated(t, memCache, "dis-1")
	})

	t.Run("禁用幂等", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		seedAdminTestClient(store, "dis-idem", false)

		require.NoError(t, adminSvc.DisableClient(ctx, "operator-1", "dis-idem"))
		require.NoError(t, adminSvc.DisableClient(ctx, "operator-1", "dis-idem"))
	})

	t.Run("禁用不存在的客户端返回NotFound", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		err := adminSvc.DisableClient(ctx, "operator-1", "missing")
		require.Error(t, err)
		assert.Equal(t, 404, apperrors.GetHTTPStatus(err))
	})

	t.Run("撤销Token失败中止禁用", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		seedAdminTestClient(store, "dis-err", false)
		store.RevokeAllClientTokensErr = errors.New("db down")

		err := adminSvc.DisableClient(ctx, "operator-1", "dis-err")
		require.Error(t, err)
	})
}

func TestAdminService_EnableClient(t *testing.T) {
	ctx := context.Background()

	t.Run("启用：状态翻转+失效缓存", func(t *testing.T) {
		adminSvc, store, memCache := newAdminClientService(t)
		seedAdminTestClient(store, "ena-1", false)
		presetClientCache(t, memCache, "ena-1")

		require.NoError(t, adminSvc.DisableClient(ctx, "operator-1", "ena-1"))
		require.NoError(t, adminSvc.EnableClient(ctx, "operator-1", "ena-1"))

		client, err := store.GetByClientID(ctx, "ena-1")
		require.NoError(t, err)
		assert.True(t, client.IsActive)

		assertCacheInvalidated(t, memCache, "ena-1")
	})

	t.Run("启用幂等", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		seedAdminTestClient(store, "ena-idem", false)

		require.NoError(t, adminSvc.EnableClient(ctx, "operator-1", "ena-idem"))
		require.NoError(t, adminSvc.EnableClient(ctx, "operator-1", "ena-idem"))
	})

	t.Run("启用不存在的客户端返回NotFound", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		err := adminSvc.EnableClient(ctx, "operator-1", "missing")
		require.Error(t, err)
		assert.Equal(t, 404, apperrors.GetHTTPStatus(err))
	})
}

// ============================================================================
// DeleteClient
// ============================================================================

func TestAdminService_DeleteClient(t *testing.T) {
	ctx := context.Background()

	t.Run("删除成功：客户端消失+缓存失效+审计", func(t *testing.T) {
		adminSvc, store, memCache := newAdminClientServiceWithAudit(t)
		seedAdminTestClient(store, "del-1", false)
		presetClientCache(t, memCache, "del-1")

		err := adminSvc.DeleteClient(ctx, "operator-1", "del-1")
		require.NoError(t, err)

		_, err = store.GetByClientID(ctx, "del-1")
		require.ErrorIs(t, err, apperrors.ErrNotFound)

		assertCacheInvalidated(t, memCache, "del-1")

		waitForAuditLogs(ctx, t, store, "operator-1", string(model.EventClientDeleted), 1)
	})

	t.Run("删除不存在的客户端返回NotFound", func(t *testing.T) {
		adminSvc, _, _ := newAdminClientService(t)
		err := adminSvc.DeleteClient(ctx, "operator-1", "missing")
		require.Error(t, err)
		assert.Equal(t, 404, apperrors.GetHTTPStatus(err))
	})

	t.Run("store删除失败返回内部错误", func(t *testing.T) {
		adminSvc, store, _ := newAdminClientService(t)
		seedAdminTestClient(store, "del-err", false)
		store.DeleteClientErr = errors.New("db down")

		err := adminSvc.DeleteClient(ctx, "operator-1", "del-err")
		require.Error(t, err)
		assert.Equal(t, 500, apperrors.GetHTTPStatus(err))
	})
}

// ============================================================================
// getClient 禁用拦截（D6）
// ============================================================================

// newOAuthServiceWithCache 构造带缓存的 OAuthService（用于 getClient 两条路径测试）
func newOAuthServiceWithCache(t *testing.T) (*service.OAuthService, *mock.Store, cache.Cache) {
	t.Helper()
	store := mock.New()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwtSvc := crypto.NewJWTService(
		privateKey,
		&privateKey.PublicKey,
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)
	tokenSvc := service.NewTokenService(jwtSvc, store)
	memCache := cache.NewMemoryCache()
	oauthSvc := service.NewOAuthService(store, tokenSvc,
		service.WithOAuthPassword(crypto.NewPasswordService(4)),
		service.WithOAuthCache(memCache),
	)
	return oauthSvc, store, memCache
}

// storeDisabledClient 向 mock store 注入一个已禁用的客户端
func storeDisabledClient(store *mock.Store, clientID string) *model.Client {
	client := &model.Client{
		ID:           "id-" + clientID,
		ClientID:     clientID,
		Name:         "Disabled",
		RedirectURIs: []string{"https://example.com/callback"},
		GrantTypes:   []string{model.GrantTypeAuthorizationCode},
		Scopes:       []string{"openid"},
		IsActive:     false,
		CreatedAt:    time.Now(),
	}
	store.AddClient(client)
	return client
}

func TestOAuthService_GetClient_DisabledClientBlocked(t *testing.T) {
	ctx := context.Background()

	t.Run("DB路径：授权码签发被拒（禁用客户端）", func(t *testing.T) {
		oauthSvc, store, _ := newOAuthServiceWithCache(t)
		storeDisabledClient(store, "dis-db")

		_, err := oauthSvc.CreateAuthorizationCode(ctx, "dis-db", "user-1",
			"https://example.com/callback", []string{"openid"}, "challenge", "S256")
		require.Error(t, err)
		assert.True(t, errors.Is(err, service.ErrInvalidClient), "禁用客户端应返回 ErrInvalidClient")
	})

	t.Run("DB路径：授权码交换被拒（禁用客户端）", func(t *testing.T) {
		oauthSvc, store, _ := newOAuthServiceWithCache(t)
		storeDisabledClient(store, "dis-exchange")

		code := &model.AuthorizationCode{
			Code:        "code-1",
			ClientID:    "dis-exchange",
			UserID:      "user-1",
			RedirectURI: "https://example.com/callback",
			Scopes:      []string{"openid"},
			ExpiresAt:   time.Now().Add(10 * time.Minute),
			CreatedAt:   time.Now(),
		}
		require.NoError(t, store.StoreAuthorizationCode(ctx, code))

		_, err := oauthSvc.ExchangeAuthorizationCode(ctx, "code-1", "dis-exchange",
			"", "https://example.com/callback", "")
		require.Error(t, err)
		assert.True(t, errors.Is(err, service.ErrInvalidClient), "禁用客户端交换授权码应返回 ErrInvalidClient")
	})

	t.Run("缓存路径：缓存命中禁用客户端被拒", func(t *testing.T) {
		oauthSvc, _, memCache := newOAuthServiceWithCache(t)
		// 不放入 store，仅预置缓存（走缓存命中路径）
		disabled := &model.Client{
			ClientID:     "dis-cache",
			Name:         "Disabled Cache",
			RedirectURIs: []string{"https://example.com/callback"},
			GrantTypes:   []string{model.GrantTypeAuthorizationCode},
			Scopes:       []string{"openid"},
			IsActive:     false,
		}
		require.NoError(t, memCache.Set(ctx, cache.ClientKey("dis-cache"), disabled, time.Minute))

		_, err := oauthSvc.CreateAuthorizationCode(ctx, "dis-cache", "user-1",
			"https://example.com/callback", []string{"openid"}, "challenge", "S256")
		require.Error(t, err)
		assert.True(t, errors.Is(err, service.ErrInvalidClient))
	})
}
