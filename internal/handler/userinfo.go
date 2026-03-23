// Package handler 用户信息处理器
// 处理用户信息查询
package handler

import (
	"net/http"

	"github.com/your-org/sso/internal/middleware"
	"github.com/your-org/sso/internal/service"
)

// ============================================================================
// UserInfoHandler 用户信息处理器
// ============================================================================

// UserInfoHandler 用户信息处理器
type UserInfoHandler struct {
	authSvc service.AuthServiceInterface
}

// NewUserInfoHandler 创建用户信息处理器
func NewUserInfoHandler(authSvc service.AuthServiceInterface) *UserInfoHandler {
	return &UserInfoHandler{authSvc: authSvc}
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
		writeError(w, http.StatusUnauthorized, "未认证")
		return
	}

	// 返回用户信息
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sub":   userID,
		"email": userEmail,
		"scope": userScopes,
	})
}
