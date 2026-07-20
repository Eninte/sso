// Package service 社交登录安全增强（阶段 2.3）
//
// 核心安全修复：
//  1. 用 (provider, provider_user_id) 查找社交账号，避免 email 接管攻击
//  2. 校验 provider 返回的 email_verified 字段，未验证拒绝登录
//  3. provider 返回的 email 与本地账号冲突时，拒绝自动合并
//  4. state 改用 Redis 存储，支持多实例部署
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/serviceutil"
)

// ============================================================================
// 重新导出阶段 2.3 错误变量
// ============================================================================

var (
	ErrProviderEmailNotVerified = apperrors.ErrProviderEmailNotVerified
	ErrSocialAccountConflict    = apperrors.ErrSocialAccountConflict
	ErrEmailConflictWithLocal   = apperrors.ErrEmailConflictWithLocal
	ErrProviderUserIDMissing    = apperrors.ErrProviderUserIDMissing
)

// ============================================================================
// Provider 身份信息提取
// ============================================================================

// ProviderIdentity 提供商返回的用户身份信息
type ProviderIdentity struct {
	ProviderUserID string            // provider 返回的唯一用户 ID（sub/id/login）
	Email          string            // provider 返回的 email（可能为空）
	EmailVerified  bool              // provider 是否验证了 email
	DisplayName    string            // 用户在 provider 处的显示名
	Metadata       map[string]string // 其他元信息（avatar_url 等）
}

// ExtractProviderIdentity 从 provider 返回的 userInfo 中提取身份信息
//
// 安全设计：
//   - 严格按 provider 类型提取字段，避免字段混淆
//   - Google：sub 字段是 provider_user_id（永不变化），email 是用户主邮箱
//   - GitHub：id 字段是 provider_user_id（数字字符串），email 可能为空（GitHub 不公开 email 时）
//     此时不允许合成 email（旧实现用 login@github.com 是危险的）
func ExtractProviderIdentity(provider string, userInfo map[string]interface{}) (*ProviderIdentity, error) {
	if !model.IsSupportedProvider(provider) {
		return nil, ErrProviderNotSupported
	}

	identity := &ProviderIdentity{
		Metadata: make(map[string]string),
	}

	switch provider {
	case model.ProviderGoogle:
		// Google userinfo 标准返回字段：sub, email, email_verified, name, picture
		if sub, ok := userInfo["sub"].(string); ok {
			identity.ProviderUserID = sub
		} else if id, ok := userInfo["id"].(string); ok {
			// 部分 Google API 返回 id 而非 sub
			identity.ProviderUserID = id
		}

		if email, ok := userInfo["email"].(string); ok {
			identity.Email = email
		}

		// Google 返回 email_verified 是 bool 或 string
		switch v := userInfo["email_verified"].(type) {
		case bool:
			identity.EmailVerified = v
		case string:
			identity.EmailVerified = v == "true" || v == "True"
		}

		if name, ok := userInfo["name"].(string); ok {
			identity.DisplayName = name
			identity.Metadata["display_name"] = name
		}
		if picture, ok := userInfo["picture"].(string); ok {
			identity.Metadata["avatar_url"] = picture
		}

	case model.ProviderGitHub:
		// GitHub /user 返回：id（数字）, login, name, email, avatar_url
		// id 是 GitHub 用户唯一标识（数字），login 是用户名
		// email 字段在用户未公开 email 时为空字符串
		if id, ok := userInfo["id"]; ok {
			// GitHub id 可能是 float64（JSON 数字）或 int
			switch v := id.(type) {
			case float64:
				identity.ProviderUserID = fmt.Sprintf("%.0f", v)
			case int:
				identity.ProviderUserID = fmt.Sprintf("%d", v)
			case int64:
				identity.ProviderUserID = fmt.Sprintf("%d", v)
			case string:
				identity.ProviderUserID = v
			}
		}

		if email, ok := userInfo["email"].(string); ok && email != "" {
			identity.Email = email
			// GitHub /user 接口不返回 email_verified 字段
			// 安全设计：默认为未验证（fail-secure），由 HandleCallback 调用
			// enrichGitHubIdentity 通过 /user/emails API 补全真实 verified 状态
			// 阶段 D 审查修复（H2）：原实现硬编码 false 会导致已验证 GitHub 用户无法登录
			identity.EmailVerified = false
		}

		if login, ok := userInfo["login"].(string); ok {
			identity.Metadata["login"] = login
			if identity.DisplayName == "" {
				identity.DisplayName = login
			}
		}
		if name, ok := userInfo["name"].(string); ok && name != "" {
			identity.DisplayName = name
			identity.Metadata["display_name"] = name
		}
		if avatarURL, ok := userInfo["avatar_url"].(string); ok {
			identity.Metadata["avatar_url"] = avatarURL
		}
	}

	// 必须有 provider_user_id
	if identity.ProviderUserID == "" {
		return nil, ErrProviderUserIDMissing
	}

	return identity, nil
}

// ============================================================================
// 用户查找/创建核心逻辑
// ============================================================================

// findOrCreateSocialUser 阶段 2.3 安全改造版
//
// 流程：
//  1. 通过 (provider, provider_user_id) 查找 social_account
//  2. 若找到 → 通过 user_id 加载 user → 检查 user 状态 → 返回 user
//  3. 若未找到：
//     a. 校验 provider 返回的 email_verified 必须为 true（Google）
//        GitHub 默认视为未验证，要求用户走邮箱验证流程
//     b. 若 email 非空且与本地 user 冲突 → 拒绝自动合并（返回 ErrEmailConflictWithLocal）
//     c. 创建新 user + social_account（原子事务）
//
// 安全设计：
//   - 不再用 email 查找 user，避免账号接管
//   - provider email 未验证时拒绝登录（不自动标记为已验证）
//   - email 冲突时拒绝自动合并，要求用户主动登录本地账号后再绑定
func (s *SocialLoginService) findOrCreateSocialUser(
	ctx context.Context,
	provider string,
	identity *ProviderIdentity,
) (*model.User, error) {
	// 1. 通过 (provider, provider_user_id) 查找 social_account
	existingAccount, err := s.store.GetSocialAccount(ctx, provider, identity.ProviderUserID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			// 数据库错误，不暴露内部细节
			return nil, serviceutil.HandleStoreError(err, store.ErrNotFound)
		}
		// 进入新建分支
		existingAccount = nil
	}

	// 2. 已存在 social_account → 加载 user
	if existingAccount != nil {
		user, err := s.store.GetByID(ctx, existingAccount.UserID)
		if err != nil {
			return nil, serviceutil.HandleStoreError(err, store.ErrNotFound)
		}

		// 检查 user 状态
		if user.Status == model.UserStatusDisabled {
			return nil, ErrAccountDisabled
		}
		if user.Status == model.UserStatusLocked {
			return nil, ErrAccountLocked
		}

		// 更新 social_account 的 provider_email / email_verified / metadata
		// 用于跟踪 provider 端的 email 变化（如用户在 Google 修改了 primary email）
		s.updateSocialAccountIfNeeded(ctx, existingAccount, identity)

		return user, nil
	}

	// 3. 新建分支
	// 3a. 校验 provider 返回的 email_verified
	if !identity.EmailVerified {
		// 安全设计：provider 未验证 email 时拒绝登录
		// 防止攻击者用未验证 email 接管账号
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventSocialLoginRejected), "", map[string]interface{}{
			"provider":          provider,
			"provider_user_id":  identity.ProviderUserID,
			"email":             identity.Email,
			"reason":            "provider_email_not_verified",
		})
		return nil, ErrProviderEmailNotVerified
	}

	// 3b. 检查 email 是否与本地账号冲突
	if identity.Email != "" {
		existingUser, err := s.store.GetByEmail(ctx, identity.Email)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, serviceutil.HandleStoreError(err, store.ErrNotFound)
		}
		if existingUser != nil {
			// email 已被本地账号占用 → 拒绝自动合并
			// 安全设计：要求用户先登录本地账号，再在个人中心绑定社交账号
			auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventSocialLoginRejected), existingUser.ID, map[string]interface{}{
				"provider":          provider,
				"provider_user_id":  identity.ProviderUserID,
				"email":             identity.Email,
				"reason":            "email_conflict_with_local",
			})
			return nil, ErrEmailConflictWithLocal
		}
	}

	// 3c. 创建新 user + social_account（原子事务）
	now := time.Now()
	userID := uuid.New().String()
	user := &model.User{
		ID:            userID,
		Email:         identity.Email,
		EmailVerified: true, // provider 已验证 email
		Status:        model.UserStatusActive,
		Role:          model.UserRoleUser,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	account := &model.SocialAccount{
		ID:               uuid.New().String(),
		Provider:         provider,
		ProviderUserID:   identity.ProviderUserID,
		UserID:           userID,
		ProviderEmail:    identity.Email,
		EmailVerified:    identity.EmailVerified,
		ProviderMetadata: identity.Metadata,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.store.CreateSocialAccountAtomic(ctx, user, account); err != nil {
		if errors.Is(err, store.ErrSocialAccountConflict) {
			// 并发：另一个请求已绑定了该社交账号，重新查找
			return s.findOrCreateSocialUser(ctx, provider, identity)
		}
		if errors.Is(err, store.ErrDuplicateEmail) {
			// 并发：另一个请求已用该 email 创建了 user
			return nil, ErrEmailConflictWithLocal
		}
		return nil, serviceutil.WrapServiceError("创建社交账号", err)
	}

	// 记录新用户注册审计日志
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventUserRegister), userID, map[string]interface{}{
		"provider":          provider,
		"provider_user_id":  identity.ProviderUserID,
		"email":             user.Email,
		"social_account_id": account.ID,
	})

	return user, nil
}

// updateSocialAccountIfNeeded 在 provider 端 email 变化时更新 social_account 记录
//
// 注意：仅更新 provider_email / email_verified / metadata 字段，
// 不改变 user_id 关联（防止通过修改 provider 端 email 接管其他用户账号）
//
// 阶段 D 修复（L2）：原实现仅修改内存对象未持久化
// 新增 store.UpdateSocialAccount 接口，UPDATE 语句只更新允许字段
// user_id 关联在 SQL WHERE 中通过 (provider, provider_user_id) 定位，不会被覆盖
func (s *SocialLoginService) updateSocialAccountIfNeeded(
	ctx context.Context,
	account *model.SocialAccount,
	identity *ProviderIdentity,
) {
	// 检测是否需要更新
	needUpdate := false
	if account.ProviderEmail != identity.Email {
		account.ProviderEmail = identity.Email
		needUpdate = true
	}
	if account.EmailVerified != identity.EmailVerified {
		account.EmailVerified = identity.EmailVerified
		needUpdate = true
	}

	if !needUpdate {
		return
	}

	account.UpdatedAt = time.Now()
	// 阶段 D 修复（L2）：实际持久化到 DB
	// store.UpdateSocialAccount 仅更新 provider_email/email_verified/metadata/updated_at
	// 不修改 user_id 关联，防止通过修改 provider 端 email 接管其他用户账号
	if err := s.store.UpdateSocialAccount(ctx, account); err != nil {
		// 更新失败不影响登录主流程（已通过身份校验），但记录审计便于追溯
		auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventSocialLoginRejected), account.UserID, map[string]interface{}{
			"provider":        account.Provider,
			"provider_user_id": account.ProviderUserID,
			"reason":          "social_account_update_failed",
		})
	}
}
