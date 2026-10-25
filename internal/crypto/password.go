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

// dummyPasswordHash 预计算的 bcrypt 哈希（cost=12，与生产最低要求一致）
// 唯一用途：用户不存在时执行一次等成本的 bcrypt 比较，
// 消除"用户不存在即时返回 vs 用户存在执行 bcrypt"的时序侧信道（账号枚举）
// 该哈希对应的明文不可知且永远不会被用作真实凭据
//
// 已知残余差异：cost 固定为 12（生产下限）。若部署把 BCRYPT_COST 调到 >12，
// dummy 与真实校验之间仍会有耗时差（量级远小于"不执行 bcrypt"时的 ~200ms），
// 需要完全对齐时应将本常量按实际 cost 重新生成。
//
// #nosec G101 -- 这是用于耗时均衡的 bcrypt 哈希常量，不是凭据
const dummyPasswordHash = "$2a$12$MjZI59ALN.C/5T4GW5CUG.s11mmiVyzsFsziVxrVMaKQJmN2UCQyi"

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

// VerifyDummyPassword 执行一次与真实校验等成本的 bcrypt 比较
//
// 用途：登录路径对"用户不存在"的分支调用本函数后再返回凭据错误。
// 没有它时，不存在用户的请求在 bcrypt 开始前就返回（~0ms），
// 而存在用户的请求要执行完整 bcrypt（cost=12 约 200ms+），
// 响应时间差让远端脚本可以稳定枚举有效账号。
// 本函数恒定返回 ErrPasswordMismatch，调用方无需关心结果。
func (s *PasswordService) VerifyDummyPassword(password string) error {
	// 与 VerifyPassword 保持相同错误返回，调用方统一处理
	return s.VerifyPassword(dummyPasswordHash, password)
}
