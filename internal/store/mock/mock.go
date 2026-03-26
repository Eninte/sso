// Package mock Store接口的Mock实现
// 用于单元测试，无需真实数据库连接
package mock

import (
	"context"
	"sync"
	"time"

	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
)

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

	CreateUserErr                error
	GetUserByIDErr               error
	GetUserByEmailErr            error
	UpdateUserErr                error
	UpdateLoginAttemptsErr       error
	DeleteUserErr                error
	GetClientByClientIDErr       error
	CreateClientErr              error
	StoreAuthorizationCodeErr    error
	GetAuthorizationCodeErr      error
	MarkAuthorizationCodeUsedErr error
	StoreTokenErr                error
	GetTokenByRefreshTokenErr    error
	GetTokenByAccessTokenErr     error
	RevokeTokenErr               error
	RevokeAllUserTokensErr       error
	CleanupExpiredErr            error
	StoreVerificationTokenErr    error
	GetVerificationTokenErr      error
	DeleteVerificationTokenErr   error
	StoreResetTokenErr           error
	GetResetTokenErr             error
	DeleteResetTokenErr          error
	StoreAuditLogErr             error
	StoreKeyErr                  error
	GetActiveKeyErr              error
	GetKeyByIDErr                error
	DeprecateKeyErr              error
	RevokeKeyErr                 error
	DeleteKeyErr                 error
	CloseErr                     error
	PingErr                      error
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
func (m *Store) StoreAuthorizationCode(ctx context.Context, code *model.AuthorizationCode) error {
	if m.StoreAuthorizationCodeErr != nil {
		return m.StoreAuthorizationCodeErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.authorizationCodes[code.Code] = code
	return nil
}

// GetAuthorizationCode 获取授权码
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
	return authCode, nil
}

// MarkAuthorizationCodeUsed 标记授权码已使用
func (m *Store) MarkAuthorizationCodeUsed(ctx context.Context, code string) error {
	if m.MarkAuthorizationCodeUsedErr != nil {
		return m.MarkAuthorizationCodeUsedErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	authCode, ok := m.authorizationCodes[code]
	if !ok {
		return store.ErrNotFound
	}

	now := time.Now()
	authCode.UsedAt = &now
	return nil
}

// UpdateAuthorizationCode 更新授权码
func (m *Store) UpdateAuthorizationCode(ctx context.Context, code *model.AuthorizationCode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.authorizationCodes[code.Code] = code
	return nil
}

// StoreToken 存储Token记录
func (m *Store) StoreToken(ctx context.Context, token *model.Token) error {
	if m.StoreTokenErr != nil {
		return m.StoreTokenErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.tokens[token.AccessToken] = token
	return nil
}

// GetTokenByRefreshToken 根据刷新令牌获取Token记录
func (m *Store) GetTokenByRefreshToken(ctx context.Context, refreshToken string) (*model.Token, error) {
	if m.GetTokenByRefreshTokenErr != nil {
		return nil, m.GetTokenByRefreshTokenErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, token := range m.tokens {
		if token.RefreshToken == refreshToken {
			return token, nil
		}
	}
	return nil, store.ErrNotFound
}

// GetTokenByAccessToken 根据访问令牌获取Token记录
func (m *Store) GetTokenByAccessToken(ctx context.Context, accessToken string) (*model.Token, error) {
	if m.GetTokenByAccessTokenErr != nil {
		return nil, m.GetTokenByAccessTokenErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	token, ok := m.tokens[accessToken]
	if !ok {
		return nil, store.ErrNotFound
	}
	return token, nil
}

// RevokeToken 撤销Token
func (m *Store) RevokeToken(ctx context.Context, accessToken string) error {
	if m.RevokeTokenErr != nil {
		return m.RevokeTokenErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[accessToken]
	if !ok {
		return store.ErrNotFound
	}

	now := time.Now()
	token.RevokedAt = &now
	return nil
}

// RevokeAllUserTokens 撤销用户所有Token
func (m *Store) RevokeAllUserTokens(ctx context.Context, userID string) error {
	if m.RevokeAllUserTokensErr != nil {
		return m.RevokeAllUserTokensErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, token := range m.tokens {
		if token.UserID == userID && token.RevokedAt == nil {
			token.RevokedAt = &now
		}
	}
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

	m.verificationTokens[userID] = &store.VerificationToken{
		Token:     token,
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

	m.resetTokens[userID] = &store.ResetToken{
		Token:     token,
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
}

// ============================================================================
// 审计日志存储实现
// ============================================================================

// StoreAuditLog 存储审计日志
func (m *Store) StoreAuditLog(ctx context.Context, log *model.AuditLog) error {
	if m.StoreAuditLogErr != nil {
		return m.StoreAuditLogErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

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
