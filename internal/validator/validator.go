// Package validator 输入验证工具
// 提供请求参数验证功能
package validator

import (
	"net/mail"
	"strings"
	"unicode"

	apperrors "github.com/example/sso/internal/errors"
)

// ============================================================================
// 使用统一的错误定义
// ============================================================================

var (
	ErrEmailRequired       = apperrors.ErrEmailRequired
	ErrEmailInvalid        = apperrors.ErrEmailInvalid
	ErrPasswordRequired    = apperrors.ErrPasswordRequired
	ErrPasswordTooShort    = apperrors.ErrPasswordTooShort
	ErrPasswordTooLong     = apperrors.ErrPasswordTooLong
	ErrPasswordNoUppercase = apperrors.ErrPasswordNoUppercase
	ErrPasswordNoLowercase = apperrors.ErrPasswordNoLowercase
	ErrPasswordNoDigit     = apperrors.ErrPasswordNoDigit
	ErrPasswordNoSpecial   = apperrors.ErrPasswordNoSpecial
	ErrPasswordTooWeak     = apperrors.ErrPasswordTooWeak
)

// ============================================================================
// 验证函数
// ============================================================================

// ValidateEmail 验证邮箱地址格式
// T18（安全修复，审计 L11）：仅接受纯 addr-spec —— mail.ParseAddress 通过后
// 追加校验"无 display-name 且解析结果与原输入完全一致"，拒绝以下变体：
//   - display-name 形式："Name" <a@b.c>、Name <a@b.c>
//   - 注释形式：a@b.c (comment)
//   - 尖括号形式：<a@b.c>
//   - 前后多余空白：" a@b.c "
//
// 防止同一人以多种写法绕过邮箱唯一性约束或干扰审计/计费的账号口径
func ValidateEmail(email string) error {
	if strings.TrimSpace(email) == "" {
		return ErrEmailRequired
	}

	addr, err := mail.ParseAddress(email)
	if err != nil {
		return ErrEmailInvalid
	}

	// 纯 addr-spec 校验：解析结果必须与原输入逐字符一致
	if addr.Name != "" || addr.Address != email {
		return ErrEmailInvalid
	}

	return nil
}

// weakPasswords 常见弱密码黑名单（小写匹配）
var weakPasswords = map[string]bool{
	"password1!":  true,
	"qwerty123!":  true,
	"abc123456!":  true,
	"admin123!":   true,
	"letmein123!": true,
	"welcome1!":   true,
	"monkey123!":  true,
	"dragon123!":  true,
	"master123!":  true,
	"login123!":   true,
	"princess1!":  true,
	"solo1234!":   true,
	"passw0rd!":   true,
	"trustno1!":   true,
	"hello123!":   true,
	"charlie1!":   true,
	"12345678!":   true,
	"123456789!":  true,
	"12345678a!":  true,
	"aaaaaaa1!":   true,
}

// ValidatePassword 验证密码强度
func ValidatePassword(password string) error {
	if password == "" {
		return ErrPasswordRequired
	}

	if len(password) < 8 {
		return ErrPasswordTooShort
	}

	if len(password) > 72 {
		return ErrPasswordTooLong
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return ErrPasswordNoUppercase
	}
	if !hasLower {
		return ErrPasswordNoLowercase
	}
	if !hasDigit {
		return ErrPasswordNoDigit
	}
	if !hasSpecial {
		return ErrPasswordNoSpecial
	}

	// 检查弱密码黑名单
	if weakPasswords[strings.ToLower(password)] {
		return ErrPasswordTooWeak
	}

	return nil
}

// ValidatePasswordSimple 简单密码验证
func ValidatePasswordSimple(password string) error {
	if password == "" {
		return ErrPasswordRequired
	}

	if len(password) < 8 {
		return ErrPasswordTooShort
	}

	if len(password) > 72 {
		return ErrPasswordTooLong
	}

	return nil
}

// ValidateRegisterRequest 验证注册请求
func ValidateRegisterRequest(email, password string) error {
	if err := ValidateEmail(email); err != nil {
		return err
	}

	if err := ValidatePassword(password); err != nil {
		return err
	}

	return nil
}

// ValidateLoginRequest 验证登录请求
func ValidateLoginRequest(email, password string) error {
	if err := ValidateEmail(email); err != nil {
		return err
	}

	if password == "" {
		return ErrPasswordRequired
	}

	return nil
}
