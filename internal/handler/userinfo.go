// Package handler 用户信息处理器
// 处理用户信息查询（阶段 2.2：按 scope 过滤返回字段，符合 OIDC 规范）
package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/example/sso/internal/cache"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
)

// ============================================================================
// UserInfoHandler 用户信息处理器
// ============================================================================

// UserInfoHandler 用户信息处理器
type UserInfoHandler struct {
	store store.Store
	cache cache.Cache
}

// NewUserInfoHandler 创建用户信息处理器
func NewUserInfoHandler(store store.Store, cache ...cache.Cache) *UserInfoHandler {
	h := &UserInfoHandler{store: store}
	if len(cache) > 0 {
		h.cache = cache[0]
	}
	return h
}

// Handle 处理获取用户信息请求
// GET /api/v1/userinfo
// 需要有效的Access Token
//
// 阶段 2.2：按 access_token 的 scope 过滤返回字段（OIDC 规范）
//   - openid：返回 sub（始终返回）
//   - profile：返回 role 等基础 profile 字段
//   - email：返回 email、email_verified
//   - 未授权 scope 的字段不会返回，防止信息泄露
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
			writeJSON(w, http.StatusOK, filterUserInfoByScope(cached.ID, cached.Email, cached.EmailVerified, cached.Role, userScopes))
			return
		}
	}

	// 缓存未命中，查询数据库
	user, err := h.store.GetByID(r.Context(), userID)
	if err != nil {
		slog.Error("查询用户信息失败", "error", err, "userID", userID)
		// 数据库查询失败时仍按 scope 过滤返回上下文中已有的信息
		writeJSON(w, http.StatusOK, filterUserInfoByScope(userID, userEmail, false, "", userScopes))
		return
	}

	// 写入缓存（5分钟TTL）
	if h.cache != nil {
		if err := h.cache.Set(r.Context(), cacheKey, user, 5*time.Minute); err != nil {
			slog.Debug("缓存用户信息失败", "error", err, "userID", userID)
		}
	}

	// 按 scope 过滤返回字段
	writeJSON(w, http.StatusOK, filterUserInfoByScope(user.ID, user.Email, user.EmailVerified, user.Role, userScopes))
}

// filterUserInfoByScope 按 OIDC scope 规范过滤返回字段
//
// scope 字段映射（OIDC 标准）：
//   - sub：始终返回（OIDC 核心）
//   - openid：基础标识，返回 sub
//   - profile：返回 role 等基础 profile 字段
//   - email：返回 email、email_verified
//
// 安全设计：未授权的 scope 对应字段不会返回，防止信息泄露
func filterUserInfoByScope(userID, email string, emailVerified bool, role string, scopes []string) map[string]interface{} {
	result := map[string]interface{}{
		"sub": userID,
	}

	if hasScope(scopes, model.ScopeEmail) {
		result["email"] = email
		result["email_verified"] = emailVerified
	}

	if hasScope(scopes, model.ScopeProfile) {
		// profile scope 返回用户基础信息（项目当前仅有 role 字段）
		result["role"] = role
	}

	return result
}

// hasScope 检查 scopes 中是否包含指定 scope
func hasScope(scopes []string, target string) bool {
	for _, sc := range scopes {
		if strings.TrimSpace(sc) == target {
			return true
		}
	}
	return false
}
