// Package serviceutil 提供服务层通用工具函数
package serviceutil

import (
	"context"
	"log/slog"

	"github.com/example/sso/internal/cache"
)

// InvalidateUserTokenCache 失效指定用户所有 token 的缓存
//
// 阶段 2.4：统一撤销路径的缓存清理方法
//
// 设计目的：
//   - 解决 AuthService/AdminService/UserService 之间循环依赖问题
//     （这些 service 都需要清 token 缓存但不能互相调用）
//   - 所有需要立即让该用户所有 token 失效的路径都应调用：
//   - AuthService.LogoutAllWithAudit（登出所有设备）
//   - AuthService.handleRefreshTokenReplay（refresh token 重放攻击）
//   - UserService.ChangePasswordWithAudit（修改密码后强制重新登录）
//   - UserService.ResetPasswordWithAudit（密码重置）
//   - AdminService.DisableUser/DeleteUser
//   - IncrementLoginAttempts 触发账户锁定时
//
// 实现说明：
//   - 当前使用 DeletePattern("token:*") 清除全部 token 缓存
//     Redis SCAN 性能可接受，且 15 分钟 TTL 会自动过期
//   - TODO: 后续优化为按 user_id 维度清理（cache.UserTokenKey + 中间件配合）
//     避免影响其他用户的缓存命中率
func InvalidateUserTokenCache(ctx context.Context, cacheSvc cache.Cache, userID string) {
	if cacheSvc == nil {
		return
	}
	if err := cacheSvc.DeletePattern(ctx, cache.TokenCachePrefix+"*"); err != nil {
		slog.Warn("清除用户Token缓存失败",
			"error", err,
			"user_id", userID,
		)
	}
}
