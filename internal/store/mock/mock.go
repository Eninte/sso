// Package mock Store接口的Mock实现
// 用于单元测试，无需真实数据库连接
package mock

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/example/sso/internal/common"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
)

// hashTokenMock 计算 token 的 SHA-256 哈希（hex 编码，64 字符）
// 阶段 3.2：与 postgres 实现使用同一算法
func hashTokenMock(token string) string {
	return common.HashToken(token)
}

// ============================================================================
// Store Mock存储实现
// ============================================================================

// Store Mock存储实现
// 使用内存map存储数据，支持并发安全
type Store struct {
	mu sync.RWMutex

	users map[string]*model.User

	clients map[string]*model.Client

	tokens             map[string]*model.Token
	authorizationCodes map[string]*model.AuthorizationCode

	verificationTokens map[string]*store.VerificationToken
	resetTokens        map[string]*store.ResetToken

	auditLogs []*model.AuditLog

	keys map[string]*model.KeyVersion

	mfaRecoveryCodes map[string][]string // 每实例独立，避免全局状态污染
	hmacKey          []byte              // Mock HMAC 密钥

	CreateUserErr              error
	GetUserByIDErr             error
	GetUserByEmailErr          error
	UpdateUserErr              error
	UpdateLoginAttemptsErr     error
	DeleteUserErr              error
	ListUsersErr               error
	ExistsUserByRoleErr        error
	CountActiveAdminsErr       error
	CreateAdminAtomicErr       error
	GetClientByClientIDErr     error
	CreateClientErr            error
	StoreAuthorizationCodeErr  error
	GetAuthorizationCodeErr    error
	StoreTokenErr              error
	GetTokenByRefreshTokenErr  error
	GetTokenByAccessTokenErr   error
	RevokeTokenErr             error
	RevokeAllUserTokensErr     error
	RotateRefreshTokenErr      error
	CleanupExpiredErr          error
	StoreVerificationTokenErr  error
	GetVerificationTokenErr    error
	DeleteVerificationTokenErr error
	StoreResetTokenErr         error
	GetResetTokenErr           error
	DeleteResetTokenErr        error
	StoreAuditLogErr           error
	StoreKeyErr                error
	GetActiveKeyErr            error
	GetKeyByIDErr              error
	DeprecateKeyErr            error
	RevokeKeyErr               error
	DeleteKeyErr               error
	CloseErr                   error
	PingErr                    error

	// 阶段 2.3：社交账号相关错误
	CreateSocialAccountErr        error
	GetSocialAccountErr           error
	ListSocialAccountsByUserIDErr error
	DeleteSocialAccountErr        error
	CreateSocialAccountAtomicErr  error
	UpdateSocialAccountErr        error // 阶段 D 修复（L2）

	// 社交账号数据存储
	// key 格式: provider + ":" + provider_user_id
	socialAccounts map[string]*model.SocialAccount
	// userSocialAccounts: user_id -> []*SocialAccount（用于 ListSocialAccountsByUserID）
	userSocialAccounts map[string][]*model.SocialAccount
}

// New 创建Store实例
func New() *Store {
	return &Store{
		users:              make(map[string]*model.User),
		clients:            make(map[string]*model.Client),
		tokens:             make(map[string]*model.Token),
		authorizationCodes: make(map[string]*model.AuthorizationCode),
		verificationTokens: make(map[string]*store.VerificationToken),
		resetTokens:        make(map[string]*store.ResetToken),
		auditLogs:          make([]*model.AuditLog, 0),
		keys:               make(map[string]*model.KeyVersion),
		mfaRecoveryCodes:   make(map[string][]string),
		hmacKey:            []byte("test-hmac-key-32-bytes-long!!!!!"),
		socialAccounts:     make(map[string]*model.SocialAccount),
		userSocialAccounts: make(map[string][]*model.SocialAccount),
	}
}

// ============================================================================
// 用户存储实现
// ============================================================================

// Create 创建新用户
func (m *Store) Create(ctx context.Context, user *model.User) error {
	if m.CreateUserErr != nil {
		return m.CreateUserErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查邮箱是否已存在
	for _, u := range m.users {
		if u.Email == user.Email {
			return store.ErrDuplicateEmail
		}
	}

	m.users[user.ID] = user
	return nil
}

// GetByID 根据ID获取用户
func (m *Store) GetByID(ctx context.Context, id string) (*model.User, error) {
	if m.GetUserByIDErr != nil {
		return nil, m.GetUserByIDErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	user, ok := m.users[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return user, nil
}

// GetByEmail 根据邮箱获取用户
func (m *Store) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	if m.GetUserByEmailErr != nil {
		return nil, m.GetUserByEmailErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, user := range m.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, store.ErrNotFound
}

// Update 更新用户信息
func (m *Store) Update(ctx context.Context, user *model.User) error {
	if m.UpdateUserErr != nil {
		return m.UpdateUserErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.users[user.ID]; !ok {
		return store.ErrNotFound
	}

	m.users[user.ID] = user
	return nil
}

// UpdateLoginAttempts 更新登录尝试次数
func (m *Store) UpdateLoginAttempts(ctx context.Context, userID string, attempts int, lockedUntil *time.Time) error {
	if m.UpdateLoginAttemptsErr != nil {
		return m.UpdateLoginAttemptsErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[userID]
	if !ok {
		return store.ErrNotFound
	}

	user.LoginAttempts = attempts
	user.LockedUntil = lockedUntil
	return nil
}

// IncrementLoginAttempts 原子递增登录尝试次数
func (m *Store) IncrementLoginAttempts(ctx context.Context, userID string, maxAttempts int, lockoutDuration time.Duration) (attempts int, locked bool, lockedUntil *time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[userID]
	if !ok {
		return 0, false, nil, store.ErrNotFound
	}

	user.LoginAttempts++
	attempts = user.LoginAttempts

	if attempts >= maxAttempts {
		t := time.Now().Add(lockoutDuration)
		user.LockedUntil = &t
		user.Status = model.UserStatusLocked
		locked = true
		lockedUntil = &t
	}

	return attempts, locked, lockedUntil, nil
}

// ResetLoginAttempts 重置登录尝试次数
func (m *Store) ResetLoginAttempts(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[userID]
	if !ok {
		return store.ErrNotFound
	}

	user.LoginAttempts = 0
	user.LockedUntil = nil
	if user.Status == model.UserStatusLocked {
		user.Status = model.UserStatusActive
	}
	return nil
}

// UnlockExpiredAccount 解锁已过期的锁定账户
func (m *Store) UnlockExpiredAccount(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[userID]
	if !ok {
		return store.ErrNotFound
	}

	if user.Status != model.UserStatusLocked {
		return store.ErrNotFound
	}

	if user.LockedUntil == nil || user.LockedUntil.After(time.Now()) {
		return store.ErrNotFound
	}

	user.LoginAttempts = 0
	user.Status = model.UserStatusActive
	return nil
}

// Delete 删除用户
func (m *Store) Delete(ctx context.Context, id string) error {
	if m.DeleteUserErr != nil {
		return m.DeleteUserErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.users[id]; !ok {
		return store.ErrNotFound
	}

	delete(m.users, id)
	return nil
}

// ListUsers 列出用户（支持分页）
func (m *Store) ListUsers(ctx context.Context, offset, limit int) ([]*model.User, int, error) {
	if m.ListUsersErr != nil {
		return nil, 0, m.ListUsersErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// 将所有用户转换为切片
	allUsers := make([]*model.User, 0, len(m.users))
	for _, user := range m.users {
		allUsers = append(allUsers, user)
	}

	total := len(allUsers)

	// 处理分页
	if offset >= total {
		return []*model.User{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return allUsers[offset:end], total, nil
}

// ExistsUserByRole 检查是否存在指定角色的用户
func (m *Store) ExistsUserByRole(ctx context.Context, role string) (bool, error) {
	if m.ExistsUserByRoleErr != nil {
		return false, m.ExistsUserByRoleErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, user := range m.users {
		if user.Role == role {
			return true, nil
		}
	}
	return false, nil
}

// CountActiveAdmins 统计活跃状态的管理员数量
func (m *Store) CountActiveAdmins(ctx context.Context) (int, error) {
	if m.CountActiveAdminsErr != nil {
		return 0, m.CountActiveAdminsErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, user := range m.users {
		if user.Role == model.UserRoleAdmin && user.Status == model.UserStatusActive {
			count++
		}
	}
	return count, nil
}

// CreateAdminAtomic 模拟原子创建初始管理员
// 在 mock 层通过单个互斥锁同时完成"检查管理员是否存在"和"插入用户"，
// 等效模拟 PostgreSQL 的 advisory_xact_lock + EXISTS 检查 + INSERT 事务
//
// 行为：
//   - 若已存在 role=admin 的用户：返回 store.ErrForbidden
//   - 若邮箱已存在：返回 store.ErrDuplicateEmail
//   - 若 CreateAdminAtomicErr 注入了错误：返回注入的错误
//   - 否则插入用户并返回 nil
//
// 并发安全：使用 m.mu.Lock() 确保并发调用串行执行，
// 第二个并发调用会看到第一个调用插入的管理员
func (m *Store) CreateAdminAtomic(ctx context.Context, user *model.User) error {
	if m.CreateAdminAtomicErr != nil {
		return m.CreateAdminAtomicErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已存在管理员
	for _, u := range m.users {
		if u.Role == model.UserRoleAdmin {
			return store.ErrForbidden
		}
	}

	// 检查邮箱是否已存在
	for _, u := range m.users {
		if u.Email == user.Email {
			return store.ErrDuplicateEmail
		}
	}

	m.users[user.ID] = user
	return nil
}

// ============================================================================
// 客户端存储实现
// ============================================================================

// GetByClientID 根据客户端ID获取客户端
func (m *Store) GetByClientID(ctx context.Context, clientID string) (*model.Client, error) {
	if m.GetClientByClientIDErr != nil {
		return nil, m.GetClientByClientIDErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	client, ok := m.clients[clientID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return client, nil
}

// CreateClient 创建新客户端
func (m *Store) CreateClient(ctx context.Context, client *model.Client) error {
	if m.CreateClientErr != nil {
		return m.CreateClientErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.clients[client.ClientID] = client
	return nil
}

// ValidateRedirectURI 验证重定向URI是否在允许列表中
func (m *Store) ValidateRedirectURI(ctx context.Context, clientID string, redirectURI string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, ok := m.clients[clientID]
	if !ok {
		return false
	}

	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			return true
		}
	}
	return false
}

// ============================================================================
// Token存储实现
// ============================================================================

// StoreAuthorizationCode 存储授权码
// 存储深拷贝以模拟真实数据库行为，避免调用方修改指针影响存储状态
func (m *Store) StoreAuthorizationCode(ctx context.Context, code *model.AuthorizationCode) error {
	if m.StoreAuthorizationCodeErr != nil {
		return m.StoreAuthorizationCodeErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.authorizationCodes[code.Code] = copyAuthorizationCode(code)
	return nil
}

// GetAuthorizationCode 获取授权码
// 返回深拷贝以模拟真实数据库行为，避免调用方修改影响存储状态
func (m *Store) GetAuthorizationCode(ctx context.Context, code string) (*model.AuthorizationCode, error) {
	if m.GetAuthorizationCodeErr != nil {
		return nil, m.GetAuthorizationCodeErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	authCode, ok := m.authorizationCodes[code]
	if !ok {
		return nil, store.ErrNotFound
	}
	return copyAuthorizationCode(authCode), nil
}

// copyAuthorizationCode 深拷贝 AuthorizationCode
// 防止调用方通过指针修改影响 mock 存储状态，模拟真实数据库的值语义
func copyAuthorizationCode(src *model.AuthorizationCode) *model.AuthorizationCode {
	if src == nil {
		return nil
	}
	dst := *src
	if src.Scopes != nil {
		dst.Scopes = make([]string, len(src.Scopes))
		copy(dst.Scopes, src.Scopes)
	}
	return &dst
}

// UpdateAuthorizationCode 原子地标记授权码为已使用
// 与 Postgres 实现保持一致：如果授权码已被使用，返回 ErrAuthorizationCodeUsed
func (m *Store) UpdateAuthorizationCode(ctx context.Context, code *model.AuthorizationCode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.authorizationCodes[code.Code]
	if !ok {
		return store.ErrNotFound
	}

	// 模拟数据库的原子性条件：used_at IS NULL
	if existing.UsedAt != nil {
		return store.ErrAuthorizationCodeUsed
	}

	m.authorizationCodes[code.Code] = code
	return nil
}

// StoreToken 存储Token记录
//
// 阶段 3.2：同时计算并存储 hash，与 Postgres 实现对齐
func (m *Store) StoreToken(ctx context.Context, token *model.Token) error {
	if m.StoreTokenErr != nil {
		return m.StoreTokenErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 计算 hash（与 postgres StoreToken 行为一致）
	if token.AccessTokenHash == "" && token.AccessToken != "" {
		token.AccessTokenHash = hashTokenMock(token.AccessToken)
	}
	if token.RefreshTokenHash == "" && token.RefreshToken != "" {
		token.RefreshTokenHash = hashTokenMock(token.RefreshToken)
	}

	m.tokens[token.AccessToken] = token
	return nil
}

// GetTokenByRefreshToken 根据刷新令牌获取Token记录
//
// 阶段 3.2：优先使用 hash 查询，回退到明文（与 Postgres 实现对齐）
func (m *Store) GetTokenByRefreshToken(ctx context.Context, refreshToken string) (*model.Token, error) {
	if m.GetTokenByRefreshTokenErr != nil {
		return nil, m.GetTokenByRefreshTokenErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	hash := hashTokenMock(refreshToken)
	for _, token := range m.tokens {
		// 优先 hash 匹配
		if token.RefreshTokenHash == hash && token.RefreshTokenHash != "" {
			return token, nil
		}
	}
	// 回退到明文（兼容旧数据）
	for _, token := range m.tokens {
		if token.RefreshToken == refreshToken {
			return token, nil
		}
	}
	return nil, store.ErrNotFound
}

// GetTokenByAccessToken 根据访问令牌获取Token记录
//
// 阶段 3.2：优先使用 hash 查询，回退到明文（与 Postgres 实现对齐）
func (m *Store) GetTokenByAccessToken(ctx context.Context, accessToken string) (*model.Token, error) {
	if m.GetTokenByAccessTokenErr != nil {
		return nil, m.GetTokenByAccessTokenErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	hash := hashTokenMock(accessToken)
	for _, token := range m.tokens {
		// 优先 hash 匹配
		if token.AccessTokenHash == hash && token.AccessTokenHash != "" {
			return token, nil
		}
	}
	// 回退到明文（兼容旧数据）
	token, ok := m.tokens[accessToken]
	if !ok {
		return nil, store.ErrNotFound
	}
	return token, nil
}

// RevokeToken 撤销Token
//
// 阶段 2.4：与 Postgres 行为对齐
//   - token 不存在时不报错（Postgres UPDATE 0 行也返回 nil）
//   - 已撤销时不覆盖原撤销时间（与 SQL WHERE revoked_at IS NULL 一致）
//   - 仅当 RevokeTokenErr 注入时返回注入错误
//
// 阶段 3.2：优先使用 hash 查询，回退到明文
//
// 阶段 D 修复（竞态）：采用拷贝-替换，不原地修改已存入 map 的对象。
// mock 的 map 存的是共享指针，getter 释放 RLock 后调用方仍持有同一指针并读字段；
// 若原地改 RevokedAt 会与该锁外读产生数据竞争（CI -race 会检出）。
// 拷贝替换后旧指针成为不可变快照，与真实 DB 行更新语义一致。
func (m *Store) RevokeToken(ctx context.Context, accessToken string) error {
	if m.RevokeTokenErr != nil {
		return m.RevokeTokenErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	hash := hashTokenMock(accessToken)
	// 优先 hash 匹配
	for key, token := range m.tokens {
		if token.AccessTokenHash == hash && token.AccessTokenHash != "" {
			// 已撤销则保持原撤销时间（幂等），不覆盖首次撤销时间戳
			if token.RevokedAt != nil {
				return nil
			}
			// 拷贝-替换：修改副本再写回 map，旧指针保持不可变
			now := time.Now()
			updated := *token
			updated.RevokedAt = &now
			m.tokens[key] = &updated
			return nil
		}
	}
	// 回退到明文
	token, ok := m.tokens[accessToken]
	if !ok {
		// 与 Postgres 行为对齐：token 不存在时不报错
		return nil
	}

	// 仅在未撤销时设置撤销时间，避免覆盖首次撤销时间戳
	if token.RevokedAt != nil {
		return nil
	}

	now := time.Now()
	updated := *token
	updated.RevokedAt = &now
	m.tokens[accessToken] = &updated
	return nil
}

// RevokeAllUserTokens 撤销用户所有Token
//
// 阶段 D 修复（竞态）：与 RevokeToken 同样采用拷贝-替换，不原地修改已存入 map 的对象。
// 详见 RevokeToken 注释。
func (m *Store) RevokeAllUserTokens(ctx context.Context, userID string) error {
	if m.RevokeAllUserTokensErr != nil {
		return m.RevokeAllUserTokensErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, token := range m.tokens {
		if token.UserID == userID && token.RevokedAt == nil {
			// 拷贝-替换：修改副本再写回 map，旧指针保持不可变
			updated := *token
			updated.RevokedAt = &now
			m.tokens[key] = &updated
		}
	}
	return nil
}

// RotateRefreshToken 原子地轮换 refresh token
//
// 在单个事务（mock 用互斥锁模拟）内：
//  1. 查找旧 token；不存在 → 返回 store.ErrTokenRotated（与 postgresql RowsAffected==0 语义一致）
//  2. 检查旧 token 是否已被轮换或撤销（rotated_at != nil 或 revoked_at != nil）
//     若是，说明发生重放攻击 → 返回 store.ErrTokenRotated
//  3. 标记旧 token：revoked_at = NOW()、rotated_at = NOW()、replaced_by_token_id = newToken.ID
//  4. 插入新 token（深拷贝，避免外部修改）
//
// 安全设计：与 postgres 实现保持一致，通过 rotated_at==nil 保证一次性
// 阶段 3.2：优先使用 hash 查找旧 token，回退到明文
func (m *Store) RotateRefreshToken(ctx context.Context, oldRefreshToken string, newToken *model.Token) error {
	if m.RotateRefreshTokenErr != nil {
		return m.RotateRefreshTokenErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. 查找旧 token（优先 hash）
	oldHash := hashTokenMock(oldRefreshToken)
	var oldToken *model.Token
	for _, token := range m.tokens {
		if token.RefreshTokenHash == oldHash && token.RefreshTokenHash != "" {
			oldToken = token
			break
		}
	}
	if oldToken == nil {
		// 回退到明文（兼容旧数据）
		for _, token := range m.tokens {
			if token.RefreshToken == oldRefreshToken {
				oldToken = token
				break
			}
		}
	}
	if oldToken == nil {
		return store.ErrTokenRotated
	}

	// 2. 重放检测：已轮换或已撤销的 token 不能再次轮换
	if oldToken.RotatedAt != nil || oldToken.RevokedAt != nil {
		return store.ErrTokenRotated
	}

	// 3. 标记旧 token（拷贝-替换，不原地修改）：
	//    mock 的 map 存的是共享指针，getter 返回同一指针，调用方（service 层）
	//    可能在锁外仍持有 oldToken 并读取其字段；原地写会与该读产生数据竞争。
	//    拷贝替换后旧指针对象不再变化，与真实 DB 行更新语义一致。
	now := time.Now()
	newTokenID := newToken.ID
	updated := *oldToken // 浅拷贝足够：此处仅替换三个指针字段
	updated.RevokedAt = &now
	updated.RotatedAt = &now
	updated.ReplacedByTokenID = &newTokenID
	for k, v := range m.tokens {
		if v == oldToken {
			m.tokens[k] = &updated
		}
	}

	// 4. 计算新 token 的 hash（与 postgres 实现一致）
	if newToken.AccessTokenHash == "" && newToken.AccessToken != "" {
		newToken.AccessTokenHash = hashTokenMock(newToken.AccessToken)
	}
	if newToken.RefreshTokenHash == "" && newToken.RefreshToken != "" {
		newToken.RefreshTokenHash = hashTokenMock(newToken.RefreshToken)
	}

	// 5. 插入新 token（深拷贝 scopes，避免外部修改）
	newScopes := make([]string, len(newToken.Scopes))
	copy(newScopes, newToken.Scopes)
	storedToken := &model.Token{
		ID:                newToken.ID,
		AccessToken:       newToken.AccessToken,
		RefreshToken:      newToken.RefreshToken,
		AccessTokenHash:   newToken.AccessTokenHash,
		RefreshTokenHash:  newToken.RefreshTokenHash,
		UserID:            newToken.UserID,
		Scopes:            newScopes,
		ClientID:          newToken.ClientID,
		ExpiresAt:         newToken.ExpiresAt,
		CreatedAt:         newToken.CreatedAt,
		RevokedAt:         newToken.RevokedAt,
		RotatedAt:         newToken.RotatedAt,
		ReplacedByTokenID: newToken.ReplacedByTokenID,
		RefreshExpiresAt:  newToken.RefreshExpiresAt,
	}
	m.tokens[storedToken.AccessToken] = storedToken

	return nil
}

// CleanupExpired 清理过期的Token和授权码
func (m *Store) CleanupExpired(ctx context.Context) error {
	if m.CleanupExpiredErr != nil {
		return m.CleanupExpiredErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// 清理过期的Token
	for key, token := range m.tokens {
		if token.ExpiresAt.Before(now) {
			delete(m.tokens, key)
		}
	}

	// 清理过期的授权码
	for key, code := range m.authorizationCodes {
		if code.ExpiresAt.Before(now) {
			delete(m.authorizationCodes, key)
		}
	}

	return nil
}

// ============================================================================
// 验证令牌存储实现
// ============================================================================

// StoreVerificationToken 存储验证令牌
func (m *Store) StoreVerificationToken(ctx context.Context, userID string, token string, expiresAt time.Time) error {
	if m.StoreVerificationTokenErr != nil {
		return m.StoreVerificationTokenErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// T2：与 postgres 实现对齐，仅存 hash，不存明文
	m.verificationTokens[userID] = &store.VerificationToken{
		Token:     hashTokenMock(token),
		ExpiresAt: expiresAt,
	}
	return nil
}

// GetVerificationToken 获取验证令牌
func (m *Store) GetVerificationToken(ctx context.Context, userID string) (*store.VerificationToken, error) {
	if m.GetVerificationTokenErr != nil {
		return nil, m.GetVerificationTokenErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	token, ok := m.verificationTokens[userID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return token, nil
}

// DeleteVerificationToken 删除验证令牌
func (m *Store) DeleteVerificationToken(ctx context.Context, userID string) error {
	if m.DeleteVerificationTokenErr != nil {
		return m.DeleteVerificationTokenErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.verificationTokens, userID)
	return nil
}

// StoreResetToken 存储重置令牌
func (m *Store) StoreResetToken(ctx context.Context, userID string, token string, expiresAt time.Time) error {
	if m.StoreResetTokenErr != nil {
		return m.StoreResetTokenErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// T2：与 postgres 实现对齐，仅存 hash，不存明文
	m.resetTokens[userID] = &store.ResetToken{
		Token:     hashTokenMock(token),
		ExpiresAt: expiresAt,
	}
	return nil
}

// GetResetToken 获取重置令牌
func (m *Store) GetResetToken(ctx context.Context, userID string) (*store.ResetToken, error) {
	if m.GetResetTokenErr != nil {
		return nil, m.GetResetTokenErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	token, ok := m.resetTokens[userID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return token, nil
}

// MarkResetTokenUsed 标记重置令牌为已使用
func (m *Store) MarkResetTokenUsed(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.resetTokens[userID]
	if !ok {
		return store.ErrNotFound
	}

	// 如果已经被使用，返回错误
	if token.UsedAt != nil {
		return store.ErrNotFound
	}

	now := time.Now()
	token.UsedAt = &now
	return nil
}

// DeleteResetToken 删除重置令牌
func (m *Store) DeleteResetToken(ctx context.Context, userID string) error {
	if m.DeleteResetTokenErr != nil {
		return m.DeleteResetTokenErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.resetTokens, userID)
	return nil
}

// ============================================================================
// 连接管理
// ============================================================================

// Close 关闭数据库连接
func (m *Store) Close() error {
	return m.CloseErr
}

// Ping 检查数据库连接
func (m *Store) Ping(ctx context.Context) error {
	return m.PingErr
}

// ============================================================================
// 社交账号存储实现（阶段 2.3）
// ============================================================================

// socialAccountKey 生成社交账号存储键
func socialAccountKey(provider, providerUserID string) string {
	return provider + ":" + providerUserID
}

// CreateSocialAccount 创建社交账号绑定记录
func (m *Store) CreateSocialAccount(ctx context.Context, account *model.SocialAccount) error {
	if m.CreateSocialAccountErr != nil {
		return m.CreateSocialAccountErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	key := socialAccountKey(account.Provider, account.ProviderUserID)
	if _, exists := m.socialAccounts[key]; exists {
		return store.ErrSocialAccountConflict
	}
	// 检查 (user_id, provider) 唯一约束
	for _, existing := range m.socialAccounts {
		if existing.UserID == account.UserID && existing.Provider == account.Provider {
			return store.ErrSocialAccountConflict
		}
	}

	// 深拷贝避免外部修改
	copied := *account
	m.socialAccounts[key] = &copied
	m.userSocialAccounts[account.UserID] = append(m.userSocialAccounts[account.UserID], &copied)
	return nil
}

// GetSocialAccount 通过 (provider, provider_user_id) 查找社交账号
func (m *Store) GetSocialAccount(ctx context.Context, provider, providerUserID string) (*model.SocialAccount, error) {
	if m.GetSocialAccountErr != nil {
		return nil, m.GetSocialAccountErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := socialAccountKey(provider, providerUserID)
	account, exists := m.socialAccounts[key]
	if !exists {
		return nil, store.ErrNotFound
	}
	copied := *account
	return &copied, nil
}

// ListSocialAccountsByUserID 列出用户绑定的所有社交账号
func (m *Store) ListSocialAccountsByUserID(ctx context.Context, userID string) ([]*model.SocialAccount, error) {
	if m.ListSocialAccountsByUserIDErr != nil {
		return nil, m.ListSocialAccountsByUserIDErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	accounts := m.userSocialAccounts[userID]
	result := make([]*model.SocialAccount, 0, len(accounts))
	for _, a := range accounts {
		copied := *a
		result = append(result, &copied)
	}
	return result, nil
}

// DeleteSocialAccount 解绑社交账号
func (m *Store) DeleteSocialAccount(ctx context.Context, provider, providerUserID string) error {
	if m.DeleteSocialAccountErr != nil {
		return m.DeleteSocialAccountErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	key := socialAccountKey(provider, providerUserID)
	account, exists := m.socialAccounts[key]
	if !exists {
		return store.ErrNotFound
	}
	delete(m.socialAccounts, key)

	// 从 userSocialAccounts 索引中移除
	userAccounts := m.userSocialAccounts[account.UserID]
	for i, a := range userAccounts {
		if a.Provider == provider && a.ProviderUserID == providerUserID {
			m.userSocialAccounts[account.UserID] = append(userAccounts[:i], userAccounts[i+1:]...)
			break
		}
	}
	if len(m.userSocialAccounts[account.UserID]) == 0 {
		delete(m.userSocialAccounts, account.UserID)
	}
	return nil
}

// UpdateSocialAccount 更新社交账号绑定信息
// 阶段 D 修复（L2）：原 updateSocialAccountIfNeeded 仅修改内存对象未持久化
// 仅更新 provider_email / email_verified / provider_metadata / updated_at 字段
// 不修改 user_id 关联（防止通过修改 provider 端 email 接管其他用户账号）
func (m *Store) UpdateSocialAccount(ctx context.Context, account *model.SocialAccount) error {
	if m.UpdateSocialAccountErr != nil {
		return m.UpdateSocialAccountErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	key := socialAccountKey(account.Provider, account.ProviderUserID)
	existing, exists := m.socialAccounts[key]
	if !exists {
		return store.ErrNotFound
	}

	// 仅更新允许的字段，保留 user_id 不变
	existing.ProviderEmail = account.ProviderEmail
	existing.EmailVerified = account.EmailVerified
	existing.ProviderMetadata = account.ProviderMetadata
	existing.UpdatedAt = account.UpdatedAt

	// 同步更新 userSocialAccounts 索引中的指针指向的内容（同一指针，无需额外操作）
	return nil
}

// CreateSocialAccountAtomic 原子地创建用户 + 社交账号绑定
func (m *Store) CreateSocialAccountAtomic(ctx context.Context, user *model.User, account *model.SocialAccount) error {
	if m.CreateSocialAccountAtomicErr != nil {
		return m.CreateSocialAccountAtomicErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. 检查 email 唯一
	for _, u := range m.users {
		if u.Email == user.Email {
			return store.ErrDuplicateEmail
		}
	}
	// 2. 检查 (provider, provider_user_id) 唯一
	key := socialAccountKey(account.Provider, account.ProviderUserID)
	if _, exists := m.socialAccounts[key]; exists {
		return store.ErrSocialAccountConflict
	}
	// 3. 检查 (user_id, provider) 唯一
	for _, existing := range m.socialAccounts {
		if existing.UserID == account.UserID && existing.Provider == account.Provider {
			return store.ErrSocialAccountConflict
		}
	}

	// 4. 插入用户
	userCopy := *user
	m.users[user.ID] = &userCopy

	// 5. 插入社交账号
	accountCopy := *account
	m.socialAccounts[key] = &accountCopy
	m.userSocialAccounts[account.UserID] = append(m.userSocialAccounts[account.UserID], &accountCopy)

	return nil
}

// AddSocialAccount 测试辅助方法：直接添加社交账号（不校验）
func (m *Store) AddSocialAccount(account *model.SocialAccount) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := socialAccountKey(account.Provider, account.ProviderUserID)
	copied := *account
	m.socialAccounts[key] = &copied
	m.userSocialAccounts[account.UserID] = append(m.userSocialAccounts[account.UserID], &copied)
}

// ============================================================================
// 测试辅助方法
// ============================================================================

// AddUser 添加用户（测试辅助）
func (m *Store) AddUser(user *model.User) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[user.ID] = user
}

// AddClient 添加客户端（测试辅助）
func (m *Store) AddClient(client *model.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[client.ClientID] = client
}

// AddToken 添加Token（测试辅助）
func (m *Store) AddToken(token *model.Token) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[token.AccessToken] = token
}

// Reset 重置所有数据（测试辅助）
func (m *Store) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.users = make(map[string]*model.User)
	m.clients = make(map[string]*model.Client)
	m.tokens = make(map[string]*model.Token)
	m.authorizationCodes = make(map[string]*model.AuthorizationCode)
	m.verificationTokens = make(map[string]*store.VerificationToken)
	m.resetTokens = make(map[string]*store.ResetToken)
	m.auditLogs = make([]*model.AuditLog, 0)
	m.keys = make(map[string]*model.KeyVersion)
	m.socialAccounts = make(map[string]*model.SocialAccount)
	m.userSocialAccounts = make(map[string][]*model.SocialAccount)

	// 重置所有注入错误
	m.CreateUserErr = nil
	m.GetUserByIDErr = nil
	m.GetUserByEmailErr = nil
	m.UpdateUserErr = nil
	m.UpdateLoginAttemptsErr = nil
	m.DeleteUserErr = nil
	m.ListUsersErr = nil
	m.ExistsUserByRoleErr = nil
	m.CountActiveAdminsErr = nil
	m.CreateAdminAtomicErr = nil
	m.GetClientByClientIDErr = nil
	m.CreateClientErr = nil
	m.StoreAuthorizationCodeErr = nil
	m.GetAuthorizationCodeErr = nil
	m.StoreTokenErr = nil
	m.GetTokenByRefreshTokenErr = nil
	m.GetTokenByAccessTokenErr = nil
	m.RevokeTokenErr = nil
	m.RevokeAllUserTokensErr = nil
	m.RotateRefreshTokenErr = nil
	m.CleanupExpiredErr = nil
	m.StoreVerificationTokenErr = nil
	m.GetVerificationTokenErr = nil
	m.DeleteVerificationTokenErr = nil
	m.StoreResetTokenErr = nil
	m.GetResetTokenErr = nil
	m.DeleteResetTokenErr = nil
	m.StoreAuditLogErr = nil
	m.StoreKeyErr = nil
	m.GetActiveKeyErr = nil
	m.GetKeyByIDErr = nil
	m.DeprecateKeyErr = nil
	m.RevokeKeyErr = nil
	m.DeleteKeyErr = nil
	m.CloseErr = nil
	m.PingErr = nil
	m.CreateSocialAccountErr = nil
	m.GetSocialAccountErr = nil
	m.ListSocialAccountsByUserIDErr = nil
	m.DeleteSocialAccountErr = nil
	m.CreateSocialAccountAtomicErr = nil
	m.UpdateSocialAccountErr = nil
}

// ============================================================================
// 审计日志存储实现
// ============================================================================

// StoreAuditLog 存储审计日志
func (m *Store) StoreAuditLog(ctx context.Context, log *model.AuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.StoreAuditLogErr != nil {
		return m.StoreAuditLogErr
	}

	m.auditLogs = append(m.auditLogs, log)
	return nil
}

// ListAuditLogs 列出审计日志（支持分页和过滤）
func (m *Store) ListAuditLogs(ctx context.Context, userID string, eventType string, offset, limit int) ([]*model.AuditLog, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 过滤日志
	filtered := make([]*model.AuditLog, 0)
	for _, log := range m.auditLogs {
		if userID != "" && log.UserID != userID {
			continue
		}
		if eventType != "" && log.EventType != eventType {
			continue
		}
		filtered = append(filtered, log)
	}

	total := len(filtered)

	// 处理分页
	if offset >= total {
		return []*model.AuditLog{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return filtered[offset:end], total, nil
}

func (m *Store) StoreKey(ctx context.Context, key *model.KeyVersion) error {
	if m.StoreKeyErr != nil {
		return m.StoreKeyErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.keys[key.ID] = key
	return nil
}

func (m *Store) GetActiveKey(ctx context.Context) (*model.KeyVersion, error) {
	if m.GetActiveKeyErr != nil {
		return nil, m.GetActiveKeyErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, key := range m.keys {
		if key.Status == model.KeyStatusActive {
			return key, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *Store) GetKeyByID(ctx context.Context, keyID string) (*model.KeyVersion, error) {
	if m.GetKeyByIDErr != nil {
		return nil, m.GetKeyByIDErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	key, ok := m.keys[keyID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return key, nil
}

func (m *Store) ListActiveKeys(ctx context.Context) ([]*model.KeyVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*model.KeyVersion, 0)
	for _, key := range m.keys {
		if key.Status == model.KeyStatusActive || key.Status == model.KeyStatusDeprecated {
			result = append(result, key)
		}
	}
	return result, nil
}

func (m *Store) ListAllKeys(ctx context.Context) ([]*model.KeyVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*model.KeyVersion, 0, len(m.keys))
	for _, key := range m.keys {
		result = append(result, key)
	}
	return result, nil
}

func (m *Store) DeprecateKey(ctx context.Context, keyID string, expiresAt time.Time) error {
	if m.DeprecateKeyErr != nil {
		return m.DeprecateKeyErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key, ok := m.keys[keyID]
	if !ok {
		return store.ErrNotFound
	}

	key.Status = model.KeyStatusDeprecated
	key.ExpiresAt = &expiresAt
	return nil
}

func (m *Store) RevokeKey(ctx context.Context, keyID string) error {
	if m.RevokeKeyErr != nil {
		return m.RevokeKeyErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key, ok := m.keys[keyID]
	if !ok {
		return store.ErrNotFound
	}

	key.Status = model.KeyStatusRevoked
	return nil
}

func (m *Store) DeleteKey(ctx context.Context, keyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.keys, keyID)
	return nil
}

// UpdateKeyPrivateKey 更新密钥的私钥字段（T7：懒加密回写）
func (m *Store) UpdateKeyPrivateKey(ctx context.Context, keyID string, privateKey []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, ok := m.keys[keyID]
	if !ok {
		return store.ErrNotFound
	}
	key.PrivateKey = privateKey
	return nil
}

// ============================================================================
// MFA恢复码 Mock实现
// ============================================================================

// mfaRecoveryCodes MFA恢复码存储已移至 Store 结构体字段，避免全局状态污染

// StoreMFARecoveryCodes 存储MFA恢复码
// Mock实现：直接存储哈希值（与真实实现一致）
func (m *Store) StoreMFARecoveryCodes(ctx context.Context, userID string, codeHashes []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 复制切片
	codes := make([]string, len(codeHashes))
	copy(codes, codeHashes)
	m.mfaRecoveryCodes[userID] = codes
	return nil
}

// GetUnusedMFARecoveryCodes 获取未使用的恢复码
func (m *Store) GetUnusedMFARecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	codes, ok := m.mfaRecoveryCodes[userID]
	if !ok {
		return nil, nil
	}
	// 返回副本
	result := make([]string, len(codes))
	copy(result, codes)
	return result, nil
}

// VerifyAndUseMFARecoveryCode 验证并使用恢复码
// Mock实现：与真实实现一致，接收原始code并进行HMAC哈希
func (m *Store) VerifyAndUseMFARecoveryCode(ctx context.Context, userID, code string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	codes, ok := m.mfaRecoveryCodes[userID]
	if !ok || len(codes) == 0 {
		return false, nil
	}

	// 对输入的code进行HMAC哈希（与真实实现一致）
	codeHash, err := m.hashRecoveryCode(code)
	if err != nil {
		return false, err
	}

	// 比较哈希值
	for i, hash := range codes {
		if hash == codeHash {
			// 标记为已使用（从列表中移除）
			m.mfaRecoveryCodes[userID] = append(codes[:i], codes[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

// hashRecoveryCode Mock实现的恢复码哈希函数
// 使用与真实实现相同的HMAC-SHA256算法
// 注意：密钥存储在 Store.hmacKey 字段，测试时通过 SetMockHMACKey 修改
func (m *Store) hashRecoveryCode(code string) (string, error) {
	mac := hmac.New(sha256.New, m.hmacKey)
	mac.Write([]byte(code))
	return fmt.Sprintf("%x", mac.Sum(nil)), nil
}

// SetMockHMACKey 设置Mock的HMAC密钥（用于测试）
func (m *Store) SetMockHMACKey(key []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hmacKey = key
}

// DeleteUsedMFARecoveryCodes 删除已使用的恢复码
// Mock实现不区分已使用/未使用，直接删除该用户的所有恢复码
func (m *Store) DeleteUsedMFARecoveryCodes(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.mfaRecoveryCodes, userID)
	return nil
}

// DeleteAllMFARecoveryCodes 删除用户的所有恢复码
func (m *Store) DeleteAllMFARecoveryCodes(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.mfaRecoveryCodes, userID)
	return nil
}

// DisableMFAAndClearRecoveryCodes 原子地禁用MFA并清除所有恢复码
// mock 实现不使用真实事务，顺序执行 Update + DeleteAllMFARecoveryCodes
func (m *Store) DisableMFAAndClearRecoveryCodes(ctx context.Context, user *model.User) error {
	if err := m.Update(ctx, user); err != nil {
		return err
	}
	return m.DeleteAllMFARecoveryCodes(ctx, user.ID)
}
