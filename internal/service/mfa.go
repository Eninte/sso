// Package service MFA服务
// 处理多因素认证相关的业务逻辑
package service

import (
	// 标准库
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	// 项目包
	"github.com/example/sso/internal/cache"
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

// T9（M3+L1）：Redis 键设计与 TTL
// 恢复码限流：mfa:recovery:attempts:{userID}，INCR 计数，首次计数 EXPIRE 30m（尝试窗口），
// 达上限时 EXPIRE 15m（锁定期）；TOTP 重放：mfa:totp:used:{userID}:{timeStep}，
// INCR 原子占用（>1 即重放），EXPIRE 90s。每 timeStep 独立键，旧码不可二次使用
const (
	mfaRecoveryAttemptsKeyPrefix = "mfa:recovery:attempts:"
	mfaTOTPUsedKeyPrefix         = "mfa:totp:used:"
	totpReplayWindow             = 90 * time.Second // TOTP 重放保护窗口（±1 时间步）
)

// ============================================================================
// MFAService MFA服务
// ============================================================================

type MFAService struct {
	store    store.Store
	auditSvc *AuditService
	hmacKey  []byte      // HMAC密钥，用于哈希恢复码
	cache    cache.Cache // T9：Redis 限流/重放记录；nil 时降级为进程内存

	// 恢复码验证限流（内存降级路径）
	recoveryMu       sync.Mutex
	recoveryAttempts map[string]*recoveryAttempt // userID -> 尝试记录

	// TOTP使用记录（防止重放攻击，内存降级路径）
	// T9（L1）：键为 userID:timeStep 复合键，支持多 timeStep 记录，
	// 旧 timeStep 的码在窗口内不可二次使用
	totpMu    sync.Mutex
	totpUsage map[string]*totpUsageRecord // userID:timeStep -> 使用记录
	stopChan  chan struct{}               // 停止清理goroutine
	stopOnce  sync.Once                   // 确保 Close 只执行一次
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

// SetCache 注入缓存（T9：恢复码限流与 TOTP 重放记录 Redis 化）
// 注入后多副本部署下共享限流状态；cache 为 nil 或操作失败时降级为进程内存
func (s *MFAService) SetCache(c cache.Cache) {
	s.cache = c
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
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})
}

// SetHMACKey 设置HMAC密钥（用于恢复码哈希）
// 必须在使用恢复码功能前调用
func (s *MFAService) SetHMACKey(key []byte) {
	s.hmacKey = key
}

// ============================================================================
// 恢复码限流（T9：Redis 优先，内存降级）
// ============================================================================

// checkRecoveryRateLimit 检查恢复码验证限流
// 返回 true 表示被限流
func (s *MFAService) checkRecoveryRateLimit(ctx context.Context, userID string) bool {
	if s.cache != nil {
		limited, err := s.checkRecoveryRateLimitRedis(ctx, userID)
		if err == nil {
			return limited
		}
		slog.Error("MFA恢复码限流Redis读取失败，降级为内存限流", "user_id", userID, "error", err)
	}
	return s.checkRecoveryRateLimitMemory(userID)
}

// checkRecoveryRateLimitRedis 基于 Redis 计数判断是否锁定
func (s *MFAService) checkRecoveryRateLimitRedis(ctx context.Context, userID string) (bool, error) {
	var count int
	err := s.cache.Get(ctx, mfaRecoveryAttemptsKeyPrefix+userID, &count)
	if errors.Is(err, cache.ErrCacheMiss) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return count >= maxRecoveryAttempts, nil
}

// checkRecoveryRateLimitMemory 内存降级：检查恢复码验证限流
func (s *MFAService) checkRecoveryRateLimitMemory(userID string) bool {
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
func (s *MFAService) recordRecoveryFailure(ctx context.Context, userID string) {
	if s.cache != nil {
		err := s.recordRecoveryFailureRedis(ctx, userID)
		if err == nil {
			return
		}
		slog.Error("MFA恢复码限流Redis写入失败，降级为内存限流", "user_id", userID, "error", err)
	}
	s.recordRecoveryFailureMemory(userID)
}

// recordRecoveryFailureRedis Redis 计数：INCR；首次计数设置尝试窗口 TTL，
// 首次达上限时改为锁定期 TTL（T9 键设计：mfa:recovery:attempts:{userID}）
func (s *MFAService) recordRecoveryFailureRedis(ctx context.Context, userID string) error {
	key := mfaRecoveryAttemptsKeyPrefix + userID
	count, err := s.cache.Increment(ctx, key)
	if err != nil {
		return err
	}

	switch {
	case count == 1:
		// 首次失败：设置尝试窗口过期时间
		if err := s.cache.SetTTL(ctx, key, recoveryAttemptWindow); err != nil {
			return err
		}
	case count == maxRecoveryAttempts:
		// 首次达上限：切换为锁定期过期时间（锁定期内计数持续可读）
		if err := s.cache.SetTTL(ctx, key, recoveryLockoutDuration); err != nil {
			return err
		}
	}
	return nil
}

// recordRecoveryFailureMemory 内存降级：记录恢复码验证失败
func (s *MFAService) recordRecoveryFailureMemory(userID string) {
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
func (s *MFAService) clearRecoveryAttempts(ctx context.Context, userID string) {
	if s.cache != nil {
		if err := s.cache.Delete(ctx, mfaRecoveryAttemptsKeyPrefix+userID); err != nil {
			slog.Error("MFA恢复码限流Redis清除失败", "user_id", userID, "error", err)
		}
	}

	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	delete(s.recoveryAttempts, userID)
}

// ============================================================================
// TOTP 重放保护（T9：Redis 优先，内存降级；L1 修复：每 timeStep 独立记录）
// ============================================================================

// markTOTPUsed 原子标记 TOTP 代码已使用（防重放）
// 返回 true 表示首次使用（放行），false 表示重放（拒绝）
func (s *MFAService) markTOTPUsed(ctx context.Context, userID, code string, timeStep uint64) bool {
	if s.cache != nil {
		marked, err := s.markTOTPUsedRedis(ctx, userID, timeStep)
		if err == nil {
			return marked
		}
		slog.Error("TOTP重放记录Redis写入失败，降级为内存记录", "user_id", userID, "error", err)
	}
	return s.markTOTPUsedMemory(userID, code, timeStep)
}

// markTOTPUsedRedis Redis 原子占用：INCR mfa:totp:used:{userID}:{timeStep}，
// 计数 >1 即重放；首次占用设置 90 秒过期（等价 SET NX EX 90 语义）
func (s *MFAService) markTOTPUsedRedis(ctx context.Context, userID string, timeStep uint64) (bool, error) {
	key := fmt.Sprintf("%s%s:%d", mfaTOTPUsedKeyPrefix, userID, timeStep)
	count, err := s.cache.Increment(ctx, key)
	if err != nil {
		return false, err
	}
	if count > 1 {
		return false, nil // 重放
	}
	if err := s.cache.SetTTL(ctx, key, totpReplayWindow); err != nil {
		return false, err
	}
	return true, nil
}

// totpUsageKey 生成内存记录的复合键（L1：每 timeStep 独立记录）
func totpUsageKey(userID string, timeStep uint64) string {
	return fmt.Sprintf("%s:%d", userID, timeStep)
}

// markTOTPUsedMemory 内存降级：原子检查并记录 TOTP 使用
func (s *MFAService) markTOTPUsedMemory(userID, code string, timeStep uint64) bool {
	s.totpMu.Lock()
	defer s.totpMu.Unlock()

	key := totpUsageKey(userID, timeStep)
	if record, exists := s.totpUsage[key]; exists {
		if record.code == code && time.Since(record.usedAt) < totpReplayWindow {
			return false // 重放
		}
	}

	s.totpUsage[key] = &totpUsageRecord{
		code:     code,
		timeStep: timeStep,
		usedAt:   time.Now(),
	}
	return true
}

// isTOTPUsed 检查TOTP代码是否已被使用（防止重放攻击）
func (s *MFAService) isTOTPUsed(userID, code string, timeStep uint64) bool {
	s.totpMu.Lock()
	defer s.totpMu.Unlock()

	record, exists := s.totpUsage[totpUsageKey(userID, timeStep)]
	if !exists {
		return false
	}

	// 检查是否是同一个代码，且在有效期内（90秒窗口）
	if record.code == code && time.Since(record.usedAt) < totpReplayWindow {
		return true
	}

	return false
}

// recordTOTPUsage 记录TOTP使用（防止重放攻击）
func (s *MFAService) recordTOTPUsage(userID, code string, timeStep uint64) {
	s.totpMu.Lock()
	defer s.totpMu.Unlock()

	s.totpUsage[totpUsageKey(userID, timeStep)] = &totpUsageRecord{
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
	for key, record := range s.totpUsage {
		// 清理超过90秒的记录
		if now.Sub(record.usedAt) > totpReplayWindow {
			delete(s.totpUsage, key)
		}
	}
}

// cleanupRecoveryAttempts 清理过期的恢复码验证尝试记录（定期调用）
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
	if s.cache != nil {
		// Redis 模式：按用户前缀清理全部 timeStep 记录
		if err := s.cache.DeletePattern(context.Background(), mfaTOTPUsedKeyPrefix+userID+":*"); err != nil {
			slog.Error("TOTP重放记录Redis清除失败", "user_id", userID, "error", err)
		}
	}

	s.totpMu.Lock()
	defer s.totpMu.Unlock()
	// 内存记录键为 userID:timeStep 复合键，按前缀清理
	prefix := userID + ":"
	for key := range s.totpUsage {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(s.totpUsage, key)
		}
	}
}
