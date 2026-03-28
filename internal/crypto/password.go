// Package crypto 加密工具包
// 提供密码哈希、JWT签名等加密相关功能
package crypto

import (
	"golang.org/x/crypto/bcrypt"

	apperrors "github.com/your-org/sso/internal/errors"
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

// NewPasswordService 创建密码服务
// cost: bcrypt成本因子，推荐值12-14
// 较高的cost值更安全但验证更慢
// 生产环境必须 >= 12
func NewPasswordService(cost int) *PasswordService {
	// 确保cost在合理范围内（生产环境推荐12-14）
	if cost < 12 {
		cost = 12
	}
	if cost > 14 {
		cost = 14
	}
	return &PasswordService{cost: cost}
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
