// Package serviceutil 提供服务层通用工具函数
package serviceutil

import (
	"context"
	"log/slog"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/logging"
	"github.com/example/sso/internal/store"
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
// 实现说明（按用户精确失效）：
//   - token 缓存键为 token:<access_token_hash>（cache.TokenKey），读取发生在
//     JWT 解析之前（认证中间件尚未解析出 userID），无法改用用户维度的读键
//   - 因此以数据库为索引：查询该用户在 tokens 表中的全部 access_token_hash，
//     逐个删除对应缓存键——精确、无竞态、不再影响其他用户的缓存命中
//   - 查询失败时回退 DeletePattern("token:*") 全量清理：
//     宁可让其他用户多一次缓存 miss 回源，也不放过本应立即失效的键
func InvalidateUserTokenCache(ctx context.Context, storeSvc store.Store, cacheSvc cache.Cache, userID string) {
	if cacheSvc == nil {
		return
	}

	hashes, err := storeSvc.ListTokenHashesByUser(ctx, userID)
	if err != nil {
		// store 错误可能含 DSN
		slog.Warn("查询用户Token哈希失败，回退全量清理Token缓存",
			"error", logging.SanitizeDBURL(err.Error()),
			"user_id", userID,
		)
		if delErr := cacheSvc.DeletePattern(ctx, cache.TokenCachePrefix+"*"); delErr != nil {
			slog.Warn("全量清理Token缓存失败",
				"error", logging.SanitizeDBURL(delErr.Error()),
				"user_id", userID,
			)
		}
		return
	}

	for _, hash := range hashes {
		if hash == "" {
			continue
		}
		if err := cacheSvc.Delete(ctx, cache.TokenKeyByHash(hash)); err != nil {
			slog.Warn("清除用户Token缓存失败",
				"error", logging.SanitizeDBURL(err.Error()),
				"user_id", userID,
			)
		}
	}
}

// InvalidateClientTokenCache 失效指定客户端所有 token 的缓存
//
// 设计目的：与 InvalidateUserTokenCache 同理（避免 service 间循环依赖），
// 但维度是客户端。认证中间件按 token:<access_token_hash> 缓存撤销状态
// （TTL 15 分钟），客户端级撤销只改 DB 不清缓存的话，已撤销 token 在缓存
// 窗口内仍被接受。以下路径在撤销客户端 Token 后都应调用：
//   - AdminService.DisableClient（禁用客户端）
//   - AdminService.RotateClientSecret（轮换密钥）
//   - AdminService.DeleteClient（删除客户端；须在删除前调用，
//     删除后 tokens.client_id 已被 FK SET NULL 置空，无法再按客户端定位哈希）
//
// 实现说明（按客户端精确失效）：
//   - 以数据库为索引：查询该客户端在 tokens 表中的全部 access_token_hash，逐个删键
//   - 查询失败时回退 DeletePattern("token:*") 全量清理（同用户维度的兜底策略）
func InvalidateClientTokenCache(ctx context.Context, storeSvc store.Store, cacheSvc cache.Cache, clientID string) {
	if cacheSvc == nil {
		return
	}

	hashes, err := storeSvc.ListTokenHashesByClient(ctx, clientID)
	if err != nil {
		// store 错误可能含 DSN
		slog.Warn("查询客户端Token哈希失败，回退全量清理Token缓存",
			"error", logging.SanitizeDBURL(err.Error()),
			"client_id", clientID,
		)
		if delErr := cacheSvc.DeletePattern(ctx, cache.TokenCachePrefix+"*"); delErr != nil {
			slog.Warn("全量清理Token缓存失败",
				"error", logging.SanitizeDBURL(delErr.Error()),
				"client_id", clientID,
			)
		}
		return
	}

	for _, hash := range hashes {
		if hash == "" {
			continue
		}
		if err := cacheSvc.Delete(ctx, cache.TokenKeyByHash(hash)); err != nil {
			slog.Warn("清除客户端Token缓存失败",
				"error", logging.SanitizeDBURL(err.Error()),
				"client_id", clientID,
			)
		}
	}
}

// InvalidateUserInfoCache 失效指定用户的用户信息缓存
//
// 覆盖两个读取路径：
//   - userinfo:<userID>：GET /api/v1/userinfo（handler/userinfo.go，TTL 5 分钟）
//   - user:<userID>：AdminService.GetUser（TTL 5 分钟）
//
// 任何改变上述缓存内容的用户变更（邮箱验证成功、禁用/启用/删除用户）
// 都必须调用，否则变更最多延迟一个 TTL 才对外可见。
func InvalidateUserInfoCache(ctx context.Context, cacheSvc cache.Cache, userID string) {
	if cacheSvc == nil {
		return
	}
	for _, key := range []string{cache.UserInfoKey(userID), cache.UserIDKey(userID)} {
		if err := cacheSvc.Delete(ctx, key); err != nil {
			slog.Warn("清除用户信息缓存失败",
				"error", logging.SanitizeDBURL(err.Error()),
				"user_id", userID,
			)
		}
	}
}

// InvalidateClientCache 失效客户端缓存（客户端配置变更后必须调用）
//
// 设计目的：与 InvalidateUserTokenCache 同理，避免 service 之间的循环依赖。
// 客户端缓存由 OAuthService.getClient 读取（cache.ClientKey，TTL 为 ClientTTL），
// 任何修改客户端配置的路径（更新/禁用/启用/轮换/删除）都必须失效该缓存，
// 否则旧配置最多存活 ClientTTL（1 小时），导致变更延迟生效。
func InvalidateClientCache(ctx context.Context, cacheSvc cache.Cache, clientID string) {
	if cacheSvc == nil {
		return
	}
	if err := cacheSvc.Delete(ctx, cache.ClientKey(clientID)); err != nil {
		slog.Warn("清除客户端缓存失败",
			"error", err,
			"client_id", clientID,
		)
	}
}
