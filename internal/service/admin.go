// Package service 管理员服务
// 提供管理员相关的业务逻辑
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/your-org/sso/internal/cache"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
)

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
}

// ============================================================================
// 管理员服务实现
// ============================================================================

// AdminService 管理员服务实现
type AdminService struct {
	store   store.Store
	cache   cache.Cache
	version string
}

// NewAdminService 创建管理员服务
func NewAdminService(store store.Store) *AdminService {
	return &AdminService{store: store, version: "dev"}
}

// NewAdminServiceWithCache 创建带缓存的管理员服务
func NewAdminServiceWithCache(store store.Store, cacheSvc cache.Cache) *AdminService {
	return &AdminService{store: store, cache: cacheSvc, version: "dev"}
}

// NewAdminServiceWithVersion 创建带版本号的管理员服务
func NewAdminServiceWithVersion(store store.Store, cacheSvc cache.Cache, version string) *AdminService {
	return &AdminService{store: store, cache: cacheSvc, version: version}
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
		return nil, err
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
		return err
	}

	user.Status = model.UserStatusDisabled
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return err
	}

	// 撤销所有Token（失败不影响主流程）
	if err := s.store.RevokeAllUserTokens(ctx, userID); err != nil {
		slog.Warn("撤销用户Token失败", "error", err, "user_id", userID)
	}

	return nil
}

// EnableUser 启用用户
func (s *AdminService) EnableUser(ctx context.Context, userID string) error {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	user.Status = model.UserStatusActive
	user.LoginAttempts = 0
	user.LockedUntil = nil
	user.UpdatedAt = time.Now()

	return s.store.Update(ctx, user)
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

	return &SystemHealthInfo{
		Status:    "ok",
		Timestamp: time.Now(),
		Database:  dbStatus,
		Version:   s.version,
	}, nil
}

// CleanupExpired 清理过期数据
func (s *AdminService) CleanupExpired(ctx context.Context) error {
	return s.store.CleanupExpired(ctx)
}
