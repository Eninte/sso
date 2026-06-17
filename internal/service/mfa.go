// Package service MFA服务
// 处理多因素认证相关的业务逻辑
package service

import (
	// 标准库
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"   // #nosec G505 -- SHA1用于HOTP算法（RFC 4226），不用于安全哈希
	"crypto/sha256" // 用于HMAC-SHA256哈希恢复码
	"crypto/subtle" // 用于恒定时间比较，防止时序攻击
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	// 项目包
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
	"github.com/your-org/sso/internal/util/auditutil"
	"github.com/your-org/sso/internal/util/serviceutil"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

var (
	ErrMFAAlreadyEnabled       = apperrors.ErrMFAAlreadyEnabled
	ErrMFANotEnabled           = apperrors.ErrMFANotEnabled
	ErrInvalidTOTPCode         = apperrors.ErrInvalidTOTPCode
	ErrTOTPCodeExpired         = apperrors.ErrTOTPCodeExpired
	ErrInvalidMFASecret        = apperrors.ErrInvalidMFASecret
	ErrRecoveryCodeInvalid     = apperrors.ErrRecoveryCodeInvalid
	ErrRecoveryCodeUsed        = apperrors.ErrRecoveryCodeUsed
	ErrRecoveryCodeGenerate    = apperrors.ErrRecoveryCodeGenerate
	ErrTooManyRecoveryAttempts = apperrors.ErrTooManyRecoveryAttempts
)

// 恢复码限流配置
const (
	maxRecoveryAttempts     = 5                // 最大失败次数
	recoveryLockoutDuration = 15 * time.Minute // 锁定时间
	recoveryAttemptWindow   = 30 * time.Minute // 尝试窗口时间
)

// ============================================================================
// MFAService MFA服务
// ============================================================================

type MFAService struct {
	store    store.Store
	auditSvc *AuditService
	hmacKey  []byte // HMAC密钥，用于哈希恢复码

	// 恢复码验证限流
	recoveryMu       sync.Mutex
	recoveryAttempts map[string]*recoveryAttempt // userID -> 尝试记录

	// TOTP使用记录（防止重放攻击）
	totpMu    sync.Mutex
	totpUsage map[string]*totpUsageRecord // userID -> 使用记录
	stopChan  chan struct{}                // 停止清理goroutine
}

// recoveryAttempt 恢复码验证尝试记录
type recoveryAttempt struct {
	count     int       // 失败次数
	lastFail  time.Time // 最后失败时间
	lockUntil time.Time // 锁定直到
}

// totpUsageRecord TOTP使用记录（防止重放攻击）
type totpUsageRecord struct {
	code      string    // TOTP代码
	timeStep  uint64    // 时间步
	usedAt    time.Time // 使用时间
}

func NewMFAService(store store.Store) *MFAService {
	svc := &MFAService{
		store:            store,
		auditSvc:         NewAuditService(store),
		recoveryAttempts: make(map[string]*recoveryAttempt),
		totpUsage:        make(map[string]*totpUsageRecord),
		stopChan:         make(chan struct{}),
	}
	go svc.runCleanup()
	return svc
}

func NewMFAServiceWithAudit(store store.Store, auditSvc *AuditService) *MFAService {
	svc := &MFAService{
		store:            store,
		auditSvc:         auditSvc,
		recoveryAttempts: make(map[string]*recoveryAttempt),
		totpUsage:        make(map[string]*totpUsageRecord),
		stopChan:         make(chan struct{}),
	}
	go svc.runCleanup()
	return svc
}

// runCleanup 定期清理过期的TOTP使用记录和恢复码尝试记录
func (s *MFAService) runCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.cleanupTOTPUsage()
			s.cleanupRecoveryAttempts()
		}
	}
}

// Close 停止后台清理goroutine
func (s *MFAService) Close() {
	close(s.stopChan)
}

// SetHMACKey 设置HMAC密钥（用于恢复码哈希）
// 必须在使用恢复码功能前调用
func (s *MFAService) SetHMACKey(key []byte) {
	s.hmacKey = key
}

// checkRecoveryRateLimit 检查恢复码验证限流
// 返回 true 表示被限流
func (s *MFAService) checkRecoveryRateLimit(userID string) bool {
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()

	attempt, exists := s.recoveryAttempts[userID]
	if !exists {
		return false
	}

	now := time.Now()

	// 检查是否在锁定期内
	if !attempt.lockUntil.IsZero() && now.Before(attempt.lockUntil) {
		return true
	}

	// 检查是否超过窗口时间，重置计数
	if now.Sub(attempt.lastFail) > recoveryAttemptWindow {
		delete(s.recoveryAttempts, userID)
		return false
	}

	return false
}

// recordRecoveryFailure 记录恢复码验证失败
func (s *MFAService) recordRecoveryFailure(userID string) {
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()

	now := time.Now()
	attempt, exists := s.recoveryAttempts[userID]
	if !exists {
		attempt = &recoveryAttempt{}
		s.recoveryAttempts[userID] = attempt
	}

	attempt.count++
	attempt.lastFail = now

	// 如果失败次数超过限制，锁定账户
	if attempt.count >= maxRecoveryAttempts {
		attempt.lockUntil = now.Add(recoveryLockoutDuration)
	}
}

// clearRecoveryAttempts 清除用户的恢复码尝试记录（成功验证后调用）
func (s *MFAService) clearRecoveryAttempts(userID string) {
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	delete(s.recoveryAttempts, userID)
}

// isTOTPUsed 检查TOTP代码是否已被使用（防止重放攻击）
func (s *MFAService) isTOTPUsed(userID, code string, timeStep uint64) bool {
	s.totpMu.Lock()
	defer s.totpMu.Unlock()

	record, exists := s.totpUsage[userID]
	if !exists {
		return false
	}

	// 检查是否是同一个代码和时间步
	if record.code == code && record.timeStep == timeStep {
		// 检查是否在有效期内（90秒窗口）
		if time.Since(record.usedAt) < 90*time.Second {
			return true
		}
	}

	return false
}

// recordTOTPUsage 记录TOTP使用（防止重放攻击）
func (s *MFAService) recordTOTPUsage(userID, code string, timeStep uint64) {
	s.totpMu.Lock()
	defer s.totpMu.Unlock()

	s.totpUsage[userID] = &totpUsageRecord{
		code:     code,
		timeStep: timeStep,
		usedAt:   time.Now(),
	}
}

// cleanupTOTPUsage 清理过期的TOTP使用记录（定期调用）
func (s *MFAService) cleanupTOTPUsage() {
	s.totpMu.Lock()
	defer s.totpMu.Unlock()

	now := time.Now()
	for userID, record := range s.totpUsage {
		// 清理超过90秒的记录
		if now.Sub(record.usedAt) > 90*time.Second {
			delete(s.totpUsage, userID)
		}
	}
}

// cleanupRecoveryAttempts 清理过期的恢复码验证尝试记录
func (s *MFAService) cleanupRecoveryAttempts() {
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()

	now := time.Now()
	for userID, attempt := range s.recoveryAttempts {
		if now.Sub(attempt.lastFail) > 30*time.Minute {
			delete(s.recoveryAttempts, userID)
		}
	}
}

// ClearTOTPUsageForTesting 清除TOTP使用记录（仅用于测试）
func (s *MFAService) ClearTOTPUsageForTesting(userID string) {
	s.totpMu.Lock()
	defer s.totpMu.Unlock()
	delete(s.totpUsage, userID)
}

// ============================================================================
// MFA操作
// ============================================================================

func (s *MFAService) SetupMFAWithAudit(ctx context.Context, userID string, ipAddress string) (*model.MFASetupResponse, error) {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return nil, serviceutil.WrapServiceError("查询用户", err)
	}

	if user.MFAEnabled {
		return nil, ErrMFAAlreadyEnabled
	}

	secret, err := generateTOTPSecret()
	if err != nil {
		return nil, serviceutil.WrapServiceError("生成MFA密钥", err)
	}

	user.MFASecret = secret
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return nil, serviceutil.WrapServiceError("更新用户", err)
	}

	qrCodeURL := generateTOTPURL(secret, user.Email)

	// 使用统一的审计日志工具记录MFA设置事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventMFASetup), userID, map[string]interface{}{
		"ip_address": ipAddress,
	})

	return &model.MFASetupResponse{
		Secret:    secret,
		QRCodeURL: qrCodeURL,
	}, nil
}

func (s *MFAService) SetupMFA(ctx context.Context, userID string) (*model.MFASetupResponse, error) {
	return s.SetupMFAWithAudit(ctx, userID, "")
}

func (s *MFAService) VerifyAndEnableMFAWithAudit(ctx context.Context, userID, code string, ipAddress string) error {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return serviceutil.WrapServiceError("查询用户", err)
	}

	if user.MFAEnabled {
		return ErrMFAAlreadyEnabled
	}

	if user.MFASecret == "" {
		return ErrInvalidMFASecret
	}

	if !s.validateTOTPWithReplayProtection(userID, user.MFASecret, code) {
		return ErrInvalidTOTPCode
	}

	user.MFAEnabled = true
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return serviceutil.WrapServiceError("更新用户", err)
	}

	// 使用统一的审计日志工具记录MFA启用事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventMFAEnabled), userID, map[string]interface{}{
		"ip_address": ipAddress,
	})

	return nil
}

func (s *MFAService) VerifyAndEnableMFA(ctx context.Context, userID, code string) error {
	return s.VerifyAndEnableMFAWithAudit(ctx, userID, code, "")
}

func (s *MFAService) DisableMFAWithAudit(ctx context.Context, userID, code string, ipAddress string) error {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return serviceutil.WrapServiceError("查询用户", err)
	}

	if !user.MFAEnabled {
		return ErrMFANotEnabled
	}

	if !s.validateTOTPWithReplayProtection(userID, user.MFASecret, code) {
		return ErrInvalidTOTPCode
	}

	user.MFAEnabled = false
	user.MFASecret = ""
	user.UpdatedAt = time.Now()

	if err := s.store.Update(ctx, user); err != nil {
		return serviceutil.WrapServiceError("更新用户", err)
	}

	// 使用统一的审计日志工具记录MFA禁用事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventMFADisabled), userID, map[string]interface{}{
		"ip_address": ipAddress,
	})

	return nil
}

func (s *MFAService) DisableMFA(ctx context.Context, userID, code string) error {
	return s.DisableMFAWithAudit(ctx, userID, code, "")
}

func (s *MFAService) GetMFAStatus(ctx context.Context, userID string) (*model.MFAStatusResponse, error) {
	user, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &model.MFAStatusResponse{
		Enabled: user.MFAEnabled,
	}, nil
}

// ============================================================================
// TOTP实现
// ============================================================================

func generateTOTPSecret() (string, error) {
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

func generateTOTPURL(secret, email string) string {
	return fmt.Sprintf("otpauth://totp/SSO:%s?secret=%s&issuer=SSO", email, secret)
}

// validateTOTPWithReplayProtection 验证TOTP并防止重放攻击
// 允许±1时间步（90秒窗口）但记录使用防止重放
func (s *MFAService) validateTOTPWithReplayProtection(userID, secret, code string) bool {
	secret = strings.ToUpper(strings.TrimSpace(secret))
	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return false
	}

	now := time.Now()
	baseTimeStep := now.Unix() / 30

	// 检查时间窗口: -1, 0, +1 (90秒窗口)
	for i := -1; i <= 1; i++ {
		var timeStep uint64

		if i < 0 {
			// 安全处理负偏移，防止整数下溢
			offset := uint64(-i)
			if baseTimeStep < int64(offset) { // #nosec G115 -- 安全的比较，baseTimeStep已验证为非负
				// 会发生下溢，跳过该时间窗口
				continue
			}
			timeStep = uint64(baseTimeStep) - offset // #nosec G115 -- 安全的减法，已验证baseTimeStep >= offset
		} else {
			// 正偏移总是安全的
			timeStep = uint64(baseTimeStep) + uint64(i) // #nosec G115 -- 安全的加法，i总是非负的
		}

		expectedCode := generateHOTP(secretBytes, timeStep)
		if expectedCode == code {
			// 检查是否已使用（防止重放攻击）
			if s.isTOTPUsed(userID, code, timeStep) {
				return false
			}

			// 记录使用
			s.recordTOTPUsage(userID, code, timeStep)
			return true
		}
	}

	return false
}

// validateTOTP 验证TOTP（向后兼容，不带重放保护）
// 新代码应使用validateTOTPWithReplayProtection
func validateTOTP(secret, code string) bool {
	secret = strings.ToUpper(strings.TrimSpace(secret))
	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return false
	}

	now := time.Now()
	baseTimeStep := now.Unix() / 30

	// 检查时间窗口: -1, 0, +1 (90秒窗口)
	for i := -1; i <= 1; i++ {
		var timeStep uint64

		if i < 0 {
			// 安全处理负偏移，防止整数下溢
			offset := uint64(-i)
			if baseTimeStep < int64(offset) { // #nosec G115 -- 安全的比较，baseTimeStep已验证为非负
				// 会发生下溢，跳过该时间窗口
				continue
			}
			timeStep = uint64(baseTimeStep) - offset // #nosec G115 -- 安全的减法，已验证baseTimeStep >= offset
		} else {
			// 正偏移总是安全的
			timeStep = uint64(baseTimeStep) + uint64(i) // #nosec G115 -- 安全的加法，i总是非负的
		}

		expectedCode := generateHOTP(secretBytes, timeStep)
		if expectedCode == code {
			return true
		}
	}

	return false
}

func generateHOTP(secret []byte, counter uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	// RFC 6238 (TOTP) 和 RFC 4226 (HOTP) 标准规定使用SHA1哈希算法
	// 这是业界标准实现，被Google Authenticator、Authy等广泛应用
	// 参考: https://tools.ietf.org/html/rfc6238
	// 注意: 这里的sha1用于HMAC-SHA1，不是直接哈希，安全性有保障
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	return fmt.Sprintf("%06d", code%1000000)
}

// ============================================================================
// MFA恢复码功能
// ============================================================================

// GenerateRecoveryCodes 生成恢复码
// 返回明文恢复码（仅在生成时返回给用户）
// 使用HMAC-SHA256哈希存储，性能优于bcrypt且安全性足够（恢复码为高熵随机值）
func (s *MFAService) GenerateRecoveryCodes(ctx context.Context, userID string, count int) ([]string, error) {
	if count <= 0 || count > 20 {
		count = 8 // 默认生成8个恢复码
	}

	// 验证HMAC密钥已设置
	if len(s.hmacKey) == 0 {
		return nil, fmt.Errorf("HMAC key not set, call SetHMACKey first")
	}

	// 生成随机恢复码（高熵：16个十六进制字符 = 64位熵）
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		code, err := generateRecoveryCode()
		if err != nil {
			return nil, ErrRecoveryCodeGenerate
		}
		codes[i] = code
	}

	// 使用HMAC-SHA256哈希后存储
	codeHashes := make([]string, count)
	for i, code := range codes {
		hash := s.hashRecoveryCodeHMAC(code)
		codeHashes[i] = hash
	}

	// 存储到数据库
	if err := s.store.StoreMFARecoveryCodes(ctx, userID, codeHashes); err != nil {
		return nil, ErrRecoveryCodeGenerate
	}

	// 返回明文恢复码（仅在生成时）
	return codes, nil
}

// VerifyRecoveryCode 验证恢复码
// 如果验证成功，标记为已使用
// 使用HMAC-SHA256验证，性能优于bcrypt（~0.001ms vs ~250ms）
// 使用恒定时间比较防止时序攻击
func (s *MFAService) VerifyRecoveryCode(ctx context.Context, userID, code string) (bool, error) {
	// 检查限流
	if s.checkRecoveryRateLimit(userID) {
		return false, ErrTooManyRecoveryAttempts
	}

	// 验证HMAC密钥已设置
	if len(s.hmacKey) == 0 {
		s.recordRecoveryFailure(userID)
		return false, fmt.Errorf("HMAC key not set")
	}

	// 计算输入恢复码的HMAC哈希
	inputHash := s.hashRecoveryCodeHMAC(code)

	// 获取所有未使用的恢复码哈希
	storedHashes, err := s.store.GetUnusedMFARecoveryCodes(ctx, userID)
	if err != nil {
		s.recordRecoveryFailure(userID)
		return false, ErrRecoveryCodeInvalid
	}

	// 使用恒定时间比较防止时序攻击
	// 遍历所有哈希，即使找到匹配也继续遍历（恒定时间）
	var matched bool
	for _, storedHash := range storedHashes {
		if subtle.ConstantTimeCompare([]byte(inputHash), []byte(storedHash)) == 1 {
			matched = true
			// 不要break，继续遍历所有哈希（恒定时间）
		}
	}

	if !matched {
		s.recordRecoveryFailure(userID)
		return false, ErrRecoveryCodeInvalid
	}

	// 标记为已使用
	// 注意：这里直接传入原始code，store层会重新哈希
	used, err := s.store.VerifyAndUseMFARecoveryCode(ctx, userID, code)
	if err != nil || !used {
		s.recordRecoveryFailure(userID)
		return false, ErrRecoveryCodeInvalid
	}

	// 验证成功，清除尝试记录
	s.clearRecoveryAttempts(userID)

	// 记录审计日志
	auditutil.SafeAuditLog(ctx, s.auditSvc, "mfa_recovery_code_used", userID, map[string]interface{}{
		"ip_address": "",
	})

	return true, nil
}

// GetRecoveryCodeStatus 获取恢复码状态
// 返回剩余未使用的恢复码数量
func (s *MFAService) GetRecoveryCodeStatus(ctx context.Context, userID string) (int, error) {
	codes, err := s.store.GetUnusedMFARecoveryCodes(ctx, userID)
	if err != nil {
		return 0, ErrRecoveryCodeInvalid
	}
	return len(codes), nil
}

// ============================================================================
// 辅助函数
// ============================================================================

// generateRecoveryCode 生成单个恢复码（16字符，包含连字符）
// 格式：XXXX-XXXX-XXXX-XXXX（16个十六进制字符 = 64位熵）
func generateRecoveryCode() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", ErrRecoveryCodeGenerate
	}
	// 格式化为 XXXX-XXXX-XXXX-XXXX 形式
	return fmt.Sprintf("%04X-%04X-%04X-%04X",
		bytes[0:2], bytes[2:4], bytes[4:6], bytes[6:8]), nil
}

// hashRecoveryCodeHMAC 使用HMAC-SHA256哈希恢复码
// 返回十六进制编码的哈希值
// 性能：~0.001ms（比bcrypt快250,000倍）
// 安全性：恢复码为高熵随机值（64位），HMAC-SHA256足够安全
func (s *MFAService) hashRecoveryCodeHMAC(code string) string {
	mac := hmac.New(sha256.New, s.hmacKey)
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}
