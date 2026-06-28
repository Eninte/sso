// Package service MFA服务
// 处理多因素认证相关的业务逻辑
package service

import (
	// 标准库
	"sync"
	"time"

	// 项目包
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/store"
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
	ErrMFAHMACKeyNotSet        = apperrors.ErrMFAHMACKeyNotSet
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
	stopChan  chan struct{}               // 停止清理goroutine
}

// recoveryAttempt 恢复码验证尝试记录
type recoveryAttempt struct {
	count     int       // 失败次数
	lastFail  time.Time // 最后失败时间
	lockUntil time.Time // 锁定直到
}

// totpUsageRecord TOTP使用记录（防止重放攻击）
type totpUsageRecord struct {
	code     string    // TOTP代码
	timeStep uint64    // 时间步
	usedAt   time.Time // 使用时间
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
		if now.Sub(attempt.lastFail) > recoveryAttemptWindow {
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
