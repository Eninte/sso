// Package service 初始化服务
// 提供系统初始化相关的业务逻辑（管理员创建、OAuth客户端创建）
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/serviceutil"
	"github.com/example/sso/internal/validator"
)

// InitServiceInterface 初始化服务接口
type InitServiceInterface interface {
	AdminExists(ctx context.Context) (bool, error)
	CreateAdmin(ctx context.Context, email, password string) (*model.User, error)
	CreateOAuthClient(ctx context.Context, name, redirectURI string) (*model.Client, string, error)
}

// InitService 初始化服务实现
type InitService struct {
	store       store.Store
	passwordSvc *crypto.PasswordService
	auditSvc    auditutil.AuditService
}

// NewInitService 创建初始化服务
func NewInitService(store store.Store, passwordSvc *crypto.PasswordService, auditSvc auditutil.AuditService) *InitService {
	return &InitService{
		store:       store,
		passwordSvc: passwordSvc,
		auditSvc:    auditSvc,
	}
}

// AdminExists 检查是否已存在管理员用户
// 委托给 Store.ExistsUserByRole，使用数据库 EXISTS 查询，避免全表扫描
func (s *InitService) AdminExists(ctx context.Context) (bool, error) {
	exists, err := s.store.ExistsUserByRole(ctx, model.UserRoleAdmin)
	if err != nil {
		return false, serviceutil.WrapServiceError("查询管理员状态", err)
	}
	return exists, nil
}

// CreateAdmin 创建管理员账户
func (s *InitService) CreateAdmin(ctx context.Context, email, password string) (*model.User, error) {
	if err := validator.ValidateRegisterRequest(email, password); err != nil {
		return nil, err
	}

	exists, err := s.AdminExists(ctx)
	if err != nil {
		return nil, serviceutil.WrapServiceError("检查管理员状态", err)
	}
	if exists {
		return nil, apperrors.ErrForbidden
	}

	// 注意：这里提前检查邮箱是为了提供更好的用户体验（快速失败）
	// 但仍然依赖数据库唯一约束来处理并发场景下的竞态条件
	existingUser, err := s.store.GetByEmail(ctx, email)
	if err != nil && !apperrors.Is(err, store.ErrNotFound) {
		return nil, serviceutil.WrapServiceError("检查邮箱", err)
	}
	if existingUser != nil {
		return nil, apperrors.ErrEmailExists
	}

	hash, err := s.passwordSvc.HashPassword(password)
	if err != nil {
		return nil, serviceutil.WrapServiceError("哈希密码", err)
	}

	now := time.Now()
	user := &model.User{
		ID:            uuid.New().String(),
		Email:         email,
		PasswordHash:  hash,
		EmailVerified: true,
		Role:          model.UserRoleAdmin,
		Status:        model.UserStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.store.Create(ctx, user); err != nil {
		// 处理并发场景下的重复邮箱错误（数据库唯一约束）
		if apperrors.Is(err, store.ErrDuplicateEmail) {
			return nil, apperrors.ErrEmailExists
		}
		return nil, serviceutil.WrapServiceError("创建管理员", err)
	}

	auditutil.SafeAuditLog(ctx, s.auditSvc, "admin_created", user.ID, map[string]interface{}{
		"email":   user.Email,
		"success": true,
	})

	return user, nil
}

// CreateOAuthClient 创建默认OAuth客户端
func (s *InitService) CreateOAuthClient(ctx context.Context, name, redirectURI string) (*model.Client, string, error) {
	exists, err := s.AdminExists(ctx)
	if err != nil {
		return nil, "", serviceutil.WrapServiceError("检查管理员状态", err)
	}
	if !exists {
		return nil, "", apperrors.ErrForbidden
	}

	if name == "" {
		return nil, "", apperrors.ErrBadRequest
	}

	if err := validateRedirectURI(redirectURI); err != nil {
		return nil, "", apperrors.ErrBadRequest.WithDetails("无效的重定向URI: " + err.Error())
	}

	clientID := uuid.New().String()
	clientSecret, err := generateRandomHex(32)
	if err != nil {
		return nil, "", serviceutil.WrapServiceError("生成客户端密钥", err)
	}

	secretHash, err := s.passwordSvc.HashPassword(clientSecret)
	if err != nil {
		return nil, "", serviceutil.WrapServiceError("密钥哈希", err)
	}

	client := &model.Client{
		ID:           uuid.New().String(),
		ClientID:     clientID,
		ClientSecret: secretHash,
		Name:         name,
		RedirectURIs: []string{redirectURI},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		Scopes:       []string{"openid", "profile", "email"},
		PublicClient: false,
		CreatedAt:    time.Now(),
	}

	if err := s.store.CreateClient(ctx, client); err != nil {
		return nil, "", serviceutil.WrapServiceError("创建客户端", err)
	}

	auditutil.SafeAuditLog(ctx, s.auditSvc, "oauth_client_created", "", map[string]interface{}{
		"client_id":   clientID,
		"client_name": name,
		"success":     true,
	})

	return client, clientSecret, nil
}

// validateRedirectURI 验证重定向URI格式
func validateRedirectURI(rawURI string) error {
	if rawURI == "" {
		return fmt.Errorf("重定向URI不能为空")
	}

	parsed, err := url.Parse(rawURI)
	if err != nil {
		return fmt.Errorf("URI格式无效: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("仅支持http或https协议")
	}

	if parsed.Host == "" {
		return fmt.Errorf("URI必须包含主机名")
	}

	if parsed.Fragment != "" {
		return fmt.Errorf("重定向URI不能包含片段（#）")
	}

	return nil
}

// generateRandomHex 生成指定字节数的随机hex字符串
func generateRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
