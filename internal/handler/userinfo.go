// Package handler 用户信息处理器
// 处理用户信息查询
package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
)

// ============================================================================
// UserInfoHandler 用户信息处理器
// ============================================================================

// UserInfoHandler 用户信息处理器
type UserInfoHandler struct {
	store store.Store
	cache Cache
}

// Cache 缓存接口（与internal/cache/redis.go的Cache接口一致）
type Cache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// NewUserInfoHandler 创建用户信息处理器
func NewUserInfoHandler(store store.Store, cache ...Cache) *UserInfoHandler {
	h := &UserInfoHandler{store: store}
	if len(cache) > 0 {
		h.cache = cache[0]
	}
	return h
}

// Handle 处理获取用户信息请求
// GET /api/v1/userinfo
// 需要有效的Access Token
func (h *UserInfoHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// 从上下文获取用户信息 (由认证中间件设置)
	userID := middleware.GetUserIDFromContext(r.Context())
	userEmail := middleware.GetUserEmailFromContext(r.Context())
	userScopes := middleware.GetUserScopesFromContext(r.Context())

	if userID == "" {
		writeError(w, http.StatusUnauthorized, getMessage(r, apperrors.ErrCodeUnauthorized))
		return
	}

	// 尝试从缓存获取用户信息
	cacheKey := "userinfo:" + userID
	if h.cache != nil {
		var cached model.User
		if err := h.cache.Get(r.Context(), cacheKey, &cached); err == nil && cached.ID != "" {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"sub":            cached.ID,
				"email":          cached.Email,
				"scope":          userScopes,
				"email_verified": cached.EmailVerified,
			})
			return
		}
	}

	// 缓存未命中，查询数据库
	user, err := h.store.GetByID(r.Context(), userID)
	if err != nil {
		slog.Error("查询用户信息失败", "error", err, "userID", userID)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"sub":   userID,
			"email": userEmail,
			"scope": userScopes,
		})
		return
	}

	// 写入缓存（5分钟TTL）
	if h.cache != nil {
		if err := h.cache.Set(r.Context(), cacheKey, user, 5*time.Minute); err != nil {
			slog.Debug("缓存用户信息失败", "error", err, "userID", userID)
		}
	}

	// 返回完整用户信息
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sub":            user.ID,
		"email":          user.Email,
		"scope":          userScopes,
		"email_verified": user.EmailVerified,
	})
}
