// Package service 管理员服务
// 提供管理员相关的业务逻辑
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/serviceutil"
)

// ============================================================================
// 上下文辅助（用于在不破坏接口的情况下传递请求元数据）
// ============================================================================

type adminContextKey string

const clientIPContextKey adminContextKey = "admin.client_ip"

// WithClientIP 将客户端 IP 注入上下文，供 AdminService 审计日志使用
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPContextKey, ip)
}

// clientIPFromContext 从上下文中获取客户端 IP
func clientIPFromContext(ctx context.Context) string {
	if ip, ok := ctx.Value(clientIPContextKey).(string); ok {
		return ip
	}
	return ""
}

// ============================================================================
// 管理员服务接口
// ============================================================================

// AdminServiceInterface 管理员服务接口
type AdminServiceInterface interface {
	// 用户管理
	ListUsers(ctx context.Context, offset, limit int) ([]*model.User, int, error)
	GetUser(ctx context.Context, userID string) (*model.User, error)
	DisableUser(ctx context.Context, userID string) error
	EnableUser(ctx context.Context, userID string) error
	DeleteUser(ctx context.Context, userID string) error

	// 审计日志
	GetAuditLogs(ctx context.Context, offset, limit int, eventType string) ([]*model.AuditLog, int, error)

	// 系统管理
	SystemHealth(ctx context.Context) (*SystemHealthInfo, error)
	CleanupExpired(ctx context.Context) error
}

// ============================================================================
// 系统健康信息
// ============================================================================

// SystemHealthInfo 系统健康信息
type SystemHealthInfo struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Database  string    `json:"database"`
	Version   string    `json:"version"`
	BuildTime string    `json:"build_time"`
}

// ============================================================================
// 管理员服务实现
// ============================================================================

// AdminService 管理员服务实现
type AdminService struct {
	store     store.Store
	cache     cache.Cache
	auditSvc  *AuditService
	version   string
	buildTime string
}

// AdminServiceOption AdminService 配置选项
type AdminServiceOption func(*AdminService)

// WithAdminCache 设置缓存
func WithAdminCache(cacheSvc cache.Cache) AdminServiceOption {
	return func(s *AdminService) { s.cache = cacheSvc }
}

// WithAdminAudit 设置审计服务
func WithAdminAudit(auditSvc *AuditService) AdminServiceOption {
	return func(s *AdminService) { s.auditSvc = auditSvc }
}

// WithAdminVersion 设置版本信息
func WithAdminVersion(version, buildTime string) AdminServiceOption {
	return func(s *AdminService) { s.version = version; s.buildTime = buildTime }
}

// NewAdminServiceWithOptions 创建带选项的管理员服务
func NewAdminServiceWithOptions(store store.Store, options ...AdminServiceOption) *AdminService {
	svc := &AdminService{store: store, version: "dev", buildTime: "unknown"}
	for _, opt := range options {
		opt(svc)
	}
	return svc
}

// NewAdminService 创建管理员服务（兼容旧调用，等价于 NewAdminServiceWithOptions）
func NewAdminService(store store.Store) *AdminService {
	return NewAdminServiceWithOptions(store)
}

// NewAdminServiceWithCache 创建带缓存的管理员服务（兼容旧调用）
func NewAdminServiceWithCache(store store.Store, cacheSvc cache.Cache) *AdminService {
	return NewAdminServiceWithOptions(store, WithAdminCache(cacheSvc))
}

// NewAdminServiceWithVersion 创建带版本号的管理员服务（兼容旧调用）
func NewAdminServiceWithVersion(store store.Store, cacheSvc cache.Cache, version string, buildTime string) *AdminService {
	return NewAdminServiceWithOptions(store, WithAdminCache(cacheSvc), WithAdminVersion(version, buildTime))
}

// ============================================================================
// 用户管理方法
// ============================================================================

// ListUsers 列出用户
func (s *AdminService) ListUsers(ctx context.Context, offset, limit int) ([]*model.User, int, error) {
	return s.store.ListUsers(ctx, offset, limit)
}

// GetUser 获取用户
func (s *AdminService) GetUser(ctx context.Context, userID string) (*model.User, error) {
	// 检查缓存
	if s.cache != nil {
		var cachedUser model.User
		cacheKey := cache.UserIDKey(userID)
		if err := s.cache.Get(ctx, cacheKey, &cachedUser); err == nil {
			return &cachedUser, nil
		}
	}

	// 缓存未命中，查询数据库
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return nil, serviceutil.WrapServiceError("查询用户", err)
	}

	// 缓存结果（失败不影响主流程）
	if s.cache != nil {
		cacheKey := cache.UserIDKey(userID)
		if err := s.cache.Set(ctx, cacheKey, user, cache.DefaultTTL); err != nil {
			slog.Warn("缓存用户信息失败", "error", err, "user_id", userID)
		}
	}

	return user, nil
}

// DisableUser 禁用用户
func (s *AdminService) DisableUser(ctx context.Context, userID string) error {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return serviceutil.WrapServiceError("查询用户", err)
	}

	user.Status = model.UserStatusDisabled
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return serviceutil.WrapServiceError("更新用户", err)
	}

	// 撤销所有Token（失败不影响主流程）
	if err := s.store.RevokeAllUserTokens(ctx, userID); err != nil {
		slog.Warn("撤销用户Token失败", "error", err, "user_id", userID)
	}

	// 阶段 2.4：统一清 token 缓存（撤销后立即生效，避免 15 分钟延迟窗口）
	serviceutil.InvalidateUserTokenCache(ctx, s.cache, userID)

	// 失效缓存，确保用户状态变更立即生效
	if s.cache != nil {
		cacheKey := cache.UserIDKey(userID)
		if err := s.cache.Delete(ctx, cacheKey); err != nil {
			slog.Warn("失效用户缓存失败", "error", err, "user_id", userID)
		}
	}

	// 记录管理员操作审计日志（失败不影响主流程）
	if s.auditSvc != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventUserDisabled), userID, map[string]interface{}{
			"ip_address": clientIPFromContext(ctx),
		})
	}

	return nil
}

// EnableUser 启用用户
func (s *AdminService) EnableUser(ctx context.Context, userID string) error {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return serviceutil.WrapServiceError("查询用户", err)
	}

	user.Status = model.UserStatusActive
	user.LoginAttempts = 0
	user.LockedUntil = nil
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return serviceutil.WrapServiceError("更新用户", err)
	}

	// 失效缓存，确保用户状态变更立即生效
	if s.cache != nil {
		cacheKey := cache.UserIDKey(userID)
		if err := s.cache.Delete(ctx, cacheKey); err != nil {
			slog.Warn("失效用户缓存失败", "error", err, "user_id", userID)
		}
	}

	// 记录管理员操作审计日志（失败不影响主流程）
	if s.auditSvc != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventUserEnabled), userID, map[string]interface{}{
			"ip_address": clientIPFromContext(ctx),
		})
	}

	return nil
}

// DeleteUser 删除用户
func (s *AdminService) DeleteUser(ctx context.Context, userID string) error {
	// 先撤销所有Token
	if err := s.store.RevokeAllUserTokens(ctx, userID); err != nil {
		slog.Warn("撤销用户Token失败", "error", err, "user_id", userID)
	}

	// 删除用户
	if err := s.store.Delete(ctx, userID); err != nil {
		return serviceutil.WrapServiceError("删除用户", err)
	}

	// 阶段 2.4：统一清 token 缓存（与 DisableUser 行为一致）
	serviceutil.InvalidateUserTokenCache(ctx, s.cache, userID)

	// 失效缓存，确保删除后立即返回404
	if s.cache != nil {
		cacheKey := cache.UserIDKey(userID)
		if err := s.cache.Delete(ctx, cacheKey); err != nil {
			slog.Warn("失效用户缓存失败", "error", err, "user_id", userID)
		}
	}

	// 记录管理员操作审计日志（失败不影响主流程）
	if s.auditSvc != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventUserDeleted), userID, map[string]interface{}{
			"ip_address": clientIPFromContext(ctx),
		})
	}

	return nil
}

// GetAuditLogs 获取审计日志
func (s *AdminService) GetAuditLogs(ctx context.Context, offset, limit int, eventType string) ([]*model.AuditLog, int, error) {
	return s.store.ListAuditLogs(ctx, "", eventType, offset, limit)
}

// ============================================================================
// 系统管理方法
// ============================================================================

// SystemHealth 系统健康检查
func (s *AdminService) SystemHealth(ctx context.Context) (*SystemHealthInfo, error) {
	dbStatus := "ok"
	if err := s.store.Ping(ctx); err != nil {
		dbStatus = "error"
	}

	// 整体状态：数据库故障时为 error，否则为 ok
	status := "ok"
	if dbStatus != "ok" {
		status = "error"
	}

	return &SystemHealthInfo{
		Status:    status,
		Timestamp: time.Now(),
		Database:  dbStatus,
		Version:   s.version,
		BuildTime: s.buildTime,
	}, nil
}

// CleanupExpired 清理过期数据
func (s *AdminService) CleanupExpired(ctx context.Context) error {
	if err := s.store.CleanupExpired(ctx); err != nil {
		return err
	}

	// 记录管理员操作审计日志（失败不影响主流程）
	if s.auditSvc != nil {
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventSystemCleanup), "", map[string]interface{}{
			"ip_address": clientIPFromContext(ctx),
		})
	}

	return nil
}
