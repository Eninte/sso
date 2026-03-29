// Package handler 用户信息处理器
// 处理用户信息查询
package handler

import (
	"log/slog"
	"net/http"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/store"
)

// ============================================================================
// UserInfoHandler 用户信息处理器
// ============================================================================

// UserInfoHandler 用户信息处理器
type UserInfoHandler struct {
	store store.Store
}

// NewUserInfoHandler 创建用户信息处理器
func NewUserInfoHandler(store store.Store) *UserInfoHandler {
	return &UserInfoHandler{store: store}
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

	// 查询用户完整信息（包含email_verified等字段）
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

	// 返回完整用户信息
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sub":            user.ID,
		"email":          user.Email,
		"scope":          userScopes,
		"email_verified": user.EmailVerified,
	})
}
