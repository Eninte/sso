// MFA 恢复码逻辑（从 mfa.go 拆分）
package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256" // 用于HMAC-SHA256哈希恢复码
	"crypto/subtle" // 恒定时间比较，防止时序攻击
	"encoding/hex"
	"fmt"

	"github.com/example/sso/internal/util/auditutil"
)

func (s *MFAService) GenerateRecoveryCodes(ctx context.Context, userID string, count int) ([]string, error) {
	if count <= 0 || count > 20 {
		count = 8 // 默认生成8个恢复码
	}

	// 验证HMAC密钥已设置
	if len(s.hmacKey) == 0 {
		return nil, ErrMFAHMACKeyNotSet
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
// ipAddress 用于审计日志，记录验证来源
func (s *MFAService) VerifyRecoveryCode(ctx context.Context, userID, code, ipAddress string) (bool, error) {
	// 检查限流
	if s.checkRecoveryRateLimit(userID) {
		return false, ErrTooManyRecoveryAttempts
	}

	// 验证HMAC密钥已设置
	if len(s.hmacKey) == 0 {
		s.recordRecoveryFailure(userID)
		return false, ErrMFAHMACKeyNotSet
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
		"ip_address": ipAddress,
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
