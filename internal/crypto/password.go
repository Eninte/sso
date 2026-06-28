// Package crypto 加密工具包
// 提供密码哈希、JWT签名等加密相关功能
package crypto

import (
	"golang.org/x/crypto/bcrypt"

	apperrors "github.com/example/sso/internal/errors"
)

// ============================================================================
// 密码相关错误（使用统一错误定义）
// ============================================================================

var (
	ErrPasswordTooShort = apperrors.ErrPasswordTooShort
	ErrPasswordTooLong  = apperrors.ErrPasswordTooLong
	ErrPasswordMismatch = apperrors.ErrPasswordMismatch
)

// ============================================================================
// PasswordService 密码服务
// ============================================================================

// PasswordService 密码服务
// 提供密码哈希和验证功能
type PasswordService struct {
	cost int // bcrypt成本因子
}

// NormalizeBcryptCost 规范化bcrypt cost值
// 将 cost 限制在 bcrypt 合法范围内 (MinCost-MaxCost)
// 生产环境强制 cost >= 12 由 config.validate() 负责，不在此处硬编码
func NormalizeBcryptCost(cost int) int {
	if cost < bcrypt.MinCost {
		cost = bcrypt.MinCost
	}
	if cost > bcrypt.MaxCost {
		cost = bcrypt.MaxCost
	}
	return cost
}

// NewPasswordService 创建密码服务
// cost: bcrypt成本因子，内部会调用 NormalizeBcryptCost 归一化
// 推荐值: 12-14，越高越安全但性能越低
// 生产环境必须 >= 12（由 config.validate() 强制执行）
func NewPasswordService(cost int) *PasswordService {
	return &PasswordService{cost: NormalizeBcryptCost(cost)}
}

// HashPassword 对密码进行哈希
// 使用bcrypt算法，自动加盐
// 返回的哈希字符串包含算法版本、cost值、盐值和哈希值
func (s *PasswordService) HashPassword(password string) (string, error) {
	// 验证密码长度
	if len(password) < 8 {
		return "", ErrPasswordTooShort
	}
	if len(password) > 72 {
		return "", ErrPasswordTooLong
	}

	// 生成哈希
	// bcrypt.GenerateFromPassword 会自动:
	// 1. 生成随机盐值
	// 2. 使用指定的cost进行哈希
	// 3. 返回格式化的哈希字符串
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

// VerifyPassword 验证密码是否匹配
// 将明文密码与哈希值进行比较
func (s *PasswordService) VerifyPassword(hashedPassword, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		return ErrPasswordMismatch
	}
	return nil
}
