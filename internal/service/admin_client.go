// Package service 管理员客户端管理服务
// 提供 OAuth 客户端的创建、查询、更新、禁用/启用、删除、密钥轮换
package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/serviceutil"
)

// ============================================================================
// 常量
// ============================================================================

const (
	// maxClientNameLen 客户端名称最大长度（字符数）
	maxClientNameLen = 255
	// maxClientRedirectURIs 客户端重定向 URI 最大数量（去重后）
	maxClientRedirectURIs = 20
)

// defaultClientGrantTypes 客户端默认授权类型
var defaultClientGrantTypes = []string{model.GrantTypeAuthorizationCode, model.GrantTypeRefreshToken}

// defaultClientScopes 客户端默认 scope
var defaultClientScopes = []string{model.ScopeOpenID, model.ScopeProfile, model.ScopeEmail}

// ============================================================================
// 客户端管理
// ============================================================================

// normalizedClientConfig 校验并规范化后的客户端配置
type normalizedClientConfig struct {
	name         string
	redirectURIs []string
	grantTypes   []string
	scopes       []string
}

// validateClientRequest 校验并规范化客户端创建/更新请求
//
// 校验规则：
//   - name：必填，1-255 字符
//   - redirect_uris：必填 ≥1 条，每条通过 validateRedirectURI，去重后 ≤20 条
//   - grant_types：可选，默认 ["authorization_code","refresh_token"]；
//     必须是 {authorization_code, refresh_token} 子集且必须包含 authorization_code；
//     含未支持类型（如 client_credentials）→ ErrUnsupportedGrant
//
// validateClientName 校验客户端名称（必填，1-255 字符）
func validateClientName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", apperrors.ErrBadRequest.WithDetails("name is required")
	}
	if utf8.RuneCountInString(name) > maxClientNameLen {
		return "", apperrors.ErrBadRequest.WithDetails(fmt.Sprintf("name must be at most %d characters", maxClientNameLen))
	}
	return name, nil
}

// validateClientRedirectURIs 校验重定向 URI（必填 ≥1，去重后 ≤20，每条通过 validateRedirectURI）
func validateClientRedirectURIs(redirectURIs []string) ([]string, error) {
	if len(redirectURIs) == 0 {
		return nil, apperrors.ErrBadRequest.WithDetails("at least one redirect URI is required")
	}
	normalizedURIs := make([]string, 0, len(redirectURIs))
	seenURI := make(map[string]bool, len(redirectURIs))
	for _, uri := range redirectURIs {
		uri = strings.TrimSpace(uri)
		if uri == "" || seenURI[uri] {
			continue
		}
		if err := validateRedirectURI(uri); err != nil {
			return nil, apperrors.ErrBadRequest.WithDetails("invalid redirect URI: " + err.Error())
		}
		seenURI[uri] = true
		normalizedURIs = append(normalizedURIs, uri)
	}
	if len(normalizedURIs) == 0 {
		return nil, apperrors.ErrBadRequest.WithDetails("at least one valid redirect URI is required")
	}
	if len(normalizedURIs) > maxClientRedirectURIs {
		return nil, apperrors.ErrBadRequest.WithDetails(fmt.Sprintf("too many redirect URIs (max %d)", maxClientRedirectURIs))
	}
	return normalizedURIs, nil
}

// validateClientGrantTypes 校验授权类型
// 可选，默认 ["authorization_code","refresh_token"]；必须是 {authorization_code, refresh_token}
// 子集且必须包含 authorization_code；含未支持类型（如 client_credentials）→ ErrUnsupportedGrant
func validateClientGrantTypes(grantTypes []string) ([]string, error) {
	normalizedGrants := make([]string, 0, len(grantTypes))
	seenGrant := make(map[string]bool, len(grantTypes))
	for _, g := range grantTypes {
		g = strings.TrimSpace(g)
		if g == "" || seenGrant[g] {
			continue
		}
		if !model.IsSupportedGrantType(g) {
			// 含 client_credentials 等未实现类型 → 400 UNSUPPORTED_GRANT_TYPE
			return nil, apperrors.ErrUnsupportedGrant
		}
		seenGrant[g] = true
		normalizedGrants = append(normalizedGrants, g)
	}
	if len(normalizedGrants) == 0 {
		normalizedGrants = defaultClientGrantTypes
	}
	hasAuthCode := false
	for _, g := range normalizedGrants {
		if g == model.GrantTypeAuthorizationCode {
			hasAuthCode = true
			break
		}
	}
	if !hasAuthCode {
		return nil, apperrors.ErrBadRequest.WithDetails("grant_types must include authorization_code")
	}
	return normalizedGrants, nil
}

// validateClientScopes 校验 scope（可选，默认 ["openid","profile","email"]；非空、每项在白名单内、去重）
func validateClientScopes(scopes []string) ([]string, error) {
	normalizedScopes := model.NormalizeScopes(scopes)
	if len(normalizedScopes) == 0 {
		normalizedScopes = defaultClientScopes
	}
	for _, sc := range normalizedScopes {
		if !model.IsSupportedScope(sc) {
			return nil, apperrors.ErrInvalidScope
		}
	}
	return normalizedScopes, nil
}

// validateClientRequest 校验并规范化客户端创建/更新请求
//
// 校验规则：
//   - name：必填，1-255 字符
//   - redirect_uris：必填 ≥1 条，每条通过 validateRedirectURI，去重后 ≤20 条
//   - grant_types：可选，默认 ["authorization_code","refresh_token"]；
//     必须是 {authorization_code, refresh_token} 子集且必须包含 authorization_code；
//     含未支持类型（如 client_credentials）→ ErrUnsupportedGrant
//   - scopes：可选，默认 ["openid","profile","email"]；非空、每项须在全局白名单内、去重
//
// 返回规范化的配置（去重后），错误均为 apperrors（调用方直接透传）
func validateClientRequest(name string, redirectURIs, grantTypes, scopes []string) (*normalizedClientConfig, error) {
	validName, err := validateClientName(name)
	if err != nil {
		return nil, err
	}
	normalizedURIs, err := validateClientRedirectURIs(redirectURIs)
	if err != nil {
		return nil, err
	}
	normalizedGrants, err := validateClientGrantTypes(grantTypes)
	if err != nil {
		return nil, err
	}
	normalizedScopes, err := validateClientScopes(scopes)
	if err != nil {
		return nil, err
	}

	return &normalizedClientConfig{
		name:         validName,
		redirectURIs: normalizedURIs,
		grantTypes:   normalizedGrants,
		scopes:       normalizedScopes,
	}, nil
}

// ListClients 列出客户端（分页）
func (s *AdminService) ListClients(ctx context.Context, offset, limit int) ([]*model.Client, int, error) {
	return s.store.ListClients(ctx, offset, limit)
}

// GetClient 获取单个客户端
func (s *AdminService) GetClient(ctx context.Context, clientID string) (*model.Client, error) {
	client, err := s.store.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, serviceutil.HandleStoreError(err, apperrors.ErrClientNotFound)
	}
	return client, nil
}

// CreateClient 创建客户端
//
// 机密客户端：生成 32 字节随机密钥并以 bcrypt 哈希存储，明文仅在此返回一次；
// 公共客户端（public_client=true）：不生成密钥（无 client_secret 字段）。
// 创建成功后写后重读返回数据库真实值（updated_at/is_active 由 DB DEFAULT 填充）。
func (s *AdminService) CreateClient(ctx context.Context, operatorID string, req model.ClientCreateRequest) (*model.Client, string, error) {
	normalized, err := validateClientRequest(req.Name, req.RedirectURIs, req.GrantTypes, req.Scopes)
	if err != nil {
		return nil, "", err
	}

	clientID := uuid.New().String()
	client := &model.Client{
		ID:           uuid.New().String(),
		ClientID:     clientID,
		Name:         normalized.name,
		RedirectURIs: normalized.redirectURIs,
		GrantTypes:   normalized.grantTypes,
		Scopes:       normalized.scopes,
		PublicClient: req.PublicClient,
		IsActive:     true,
		CreatedAt:    time.Now(),
	}

	var clientSecret string
	if !req.PublicClient {
		if s.passwordSvc == nil {
			return nil, "", apperrors.New(apperrors.ErrCodeInternal, "密码服务未初始化", 500)
		}
		clientSecret, err = generateRandomHex(32)
		if err != nil {
			return nil, "", serviceutil.WrapServiceError("生成客户端密钥", err)
		}
		secretHash, err := s.passwordSvc.HashPassword(clientSecret)
		if err != nil {
			return nil, "", serviceutil.WrapServiceError("密钥哈希", err)
		}
		client.ClientSecret = secretHash
	}

	if err := s.store.CreateClient(ctx, client); err != nil {
		return nil, "", serviceutil.WrapServiceError("创建客户端", err)
	}

	// 写后重读：从数据库取真实值
	created, err := s.store.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, "", serviceutil.WrapServiceError("读取新建客户端", err)
	}

	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventClientCreated), operatorID, map[string]interface{}{
		"client_id":     clientID,
		"client_name":   created.Name,
		"public_client": created.PublicClient,
		"ip_address":    clientIPFromContext(ctx),
	})

	return created, clientSecret, nil
}

// UpdateClient 更新客户端配置（全量替换语义，public_client 不可变）
func (s *AdminService) UpdateClient(ctx context.Context, operatorID, clientID string, req model.ClientUpdateRequest) (*model.Client, error) {
	existing, err := s.store.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, serviceutil.HandleStoreError(err, apperrors.ErrClientNotFound)
	}

	// public_client 创建后不可变（D5）
	if req.PublicClient != nil && *req.PublicClient != existing.PublicClient {
		return nil, apperrors.ErrPublicClientImmutable
	}

	normalized, err := validateClientRequest(req.Name, req.RedirectURIs, req.GrantTypes, req.Scopes)
	if err != nil {
		return nil, err
	}

	update := &model.Client{
		ClientID:     clientID,
		Name:         normalized.name,
		RedirectURIs: normalized.redirectURIs,
		GrantTypes:   normalized.grantTypes,
		Scopes:       normalized.scopes,
	}
	if err := s.store.UpdateClient(ctx, update); err != nil {
		return nil, serviceutil.HandleStoreError(err, apperrors.ErrClientNotFound)
	}

	// 写后重读：从数据库取真实值
	updated, err := s.store.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, serviceutil.HandleStoreError(err, apperrors.ErrClientNotFound)
	}

	serviceutil.InvalidateClientCache(ctx, s.cache, clientID)

	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventClientUpdated), operatorID, map[string]interface{}{
		"client_id":   clientID,
		"client_name": updated.Name,
		"ip_address":  clientIPFromContext(ctx),
	})

	return updated, nil
}

// DisableClient 禁用客户端（幂等）
//
// 顺序：先撤销全部未撤销 Token，再置为禁用（D4）。
// 若 SetClientActive 失败，最多是 Token 已死但客户端仍可重新授权（与接口报错一致），
// 不会出现"报告已禁用但 refresh token 仍可用"的假安全感。
func (s *AdminService) DisableClient(ctx context.Context, operatorID, clientID string) error {
	if err := s.store.RevokeAllClientTokens(ctx, clientID); err != nil {
		return serviceutil.WrapServiceError("撤销客户端Token", err)
	}
	if err := s.store.SetClientActive(ctx, clientID, false); err != nil {
		return serviceutil.HandleStoreError(err, apperrors.ErrClientNotFound)
	}

	serviceutil.InvalidateClientCache(ctx, s.cache, clientID)

	if err := auditutil.CriticalAuditLog(ctx, s.auditSvc, string(model.EventClientDisabled), operatorID, map[string]interface{}{
		"client_id":  clientID,
		"ip_address": clientIPFromContext(ctx),
	}); err != nil {
		slog.Error("禁用客户端审计日志记录失败（操作已生效，无法回滚）", "error", err, "client_id", clientID)
	}
	return nil
}

// EnableClient 启用客户端（幂等，不恢复已撤销的 Token）
func (s *AdminService) EnableClient(ctx context.Context, operatorID, clientID string) error {
	if err := s.store.SetClientActive(ctx, clientID, true); err != nil {
		return serviceutil.HandleStoreError(err, apperrors.ErrClientNotFound)
	}

	serviceutil.InvalidateClientCache(ctx, s.cache, clientID)

	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventClientEnabled), operatorID, map[string]interface{}{
		"client_id":  clientID,
		"ip_address": clientIPFromContext(ctx),
	})
	return nil
}

// RotateClientSecret 轮换客户端密钥
//
// 顺序：先撤销全部未撤销 Token，再换钥（D3）。
// 若第二步失败，旧 secret 仍有效、Token 已死，管理员重试即可；
// 反过来会留下"新 secret 已生效但旧 Token 仍存活"的更危险状态。
func (s *AdminService) RotateClientSecret(ctx context.Context, operatorID, clientID string) (string, error) {
	client, err := s.store.GetByClientID(ctx, clientID)
	if err != nil {
		return "", serviceutil.HandleStoreError(err, apperrors.ErrClientNotFound)
	}
	if client.PublicClient {
		return "", apperrors.ErrNoSecretForPublic
	}
	if s.passwordSvc == nil {
		return "", apperrors.New(apperrors.ErrCodeInternal, "密码服务未初始化", 500)
	}

	clientSecret, err := generateRandomHex(32)
	if err != nil {
		return "", serviceutil.WrapServiceError("生成客户端密钥", err)
	}
	secretHash, err := s.passwordSvc.HashPassword(clientSecret)
	if err != nil {
		return "", serviceutil.WrapServiceError("密钥哈希", err)
	}

	if err := s.store.RevokeAllClientTokens(ctx, clientID); err != nil {
		return "", serviceutil.WrapServiceError("撤销客户端Token", err)
	}
	if err := s.store.RotateClientSecret(ctx, clientID, secretHash); err != nil {
		return "", serviceutil.HandleStoreError(err, apperrors.ErrClientNotFound)
	}

	serviceutil.InvalidateClientCache(ctx, s.cache, clientID)

	if err := auditutil.CriticalAuditLog(ctx, s.auditSvc, string(model.EventClientSecretRotated), operatorID, map[string]interface{}{
		"client_id":  clientID,
		"ip_address": clientIPFromContext(ctx),
	}); err != nil {
		slog.Error("轮换客户端密钥审计日志记录失败（操作已生效，无法回滚）", "error", err, "client_id", clientID)
	}

	return clientSecret, nil
}

// DeleteClient 删除客户端（硬删除，store 单事务完成 Token 撤销与授权码清理）
func (s *AdminService) DeleteClient(ctx context.Context, operatorID, clientID string) error {
	if err := s.store.DeleteClient(ctx, clientID); err != nil {
		return serviceutil.HandleStoreError(err, apperrors.ErrClientNotFound)
	}

	serviceutil.InvalidateClientCache(ctx, s.cache, clientID)

	if err := auditutil.CriticalAuditLog(ctx, s.auditSvc, string(model.EventClientDeleted), operatorID, map[string]interface{}{
		"client_id":  clientID,
		"ip_address": clientIPFromContext(ctx),
	}); err != nil {
		slog.Error("删除客户端审计日志记录失败（操作已生效，无法回滚）", "error", err, "client_id", clientID)
	}
	return nil
}
