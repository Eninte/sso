// Package validator 输入验证工具
// 提供请求参数验证功能
package validator

import (
	"net/mail"
	"strings"
	"unicode"

	apperrors "github.com/your-org/sso/internal/errors"
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
)

// ============================================================================
// 验证函数
// ============================================================================

// ValidateEmail 验证邮箱地址格式
func ValidateEmail(email string) error {
	email = strings.TrimSpace(email)

	if email == "" {
		return ErrEmailRequired
	}

	_, err := mail.ParseAddress(email)
	if err != nil {
		return ErrEmailInvalid
	}

	return nil
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
