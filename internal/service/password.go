package service

import (
	"errors"
	"unicode/utf8"
)

var (
	errPasswordTooShort  = errors.New("密码至少 12 位")
	errPasswordNoUpper   = errors.New("密码需包含大写字母")
	errPasswordNoLower   = errors.New("密码需包含小写字母")
	errPasswordNoDigit   = errors.New("密码需包含数字")
	errPasswordNoSpecial = errors.New("密码需包含特殊字符")
)

// validatePassword 密码强度规则：至少 12 位，且同时包含大写字母、小写字母、数字、特殊字符。
// 用于创建用户、管理员重置密码、个人修改密码三处，保证全局一致。
func validatePassword(pw string) error {
	if utf8.RuneCountInString(pw) < 12 {
		return errPasswordTooShort
	}
	hasUpper, hasLower, hasDigit, hasSpecial := false, false, false, false
	for _, r := range pw {
		switch {
		case 'A' <= r && r <= 'Z':
			hasUpper = true
		case 'a' <= r && r <= 'z':
			hasLower = true
		case '0' <= r && r <= '9':
			hasDigit = true
		default:
			hasSpecial = true
		}
	}
	if !hasUpper {
		return errPasswordNoUpper
	}
	if !hasLower {
		return errPasswordNoLower
	}
	if !hasDigit {
		return errPasswordNoDigit
	}
	if !hasSpecial {
		return errPasswordNoSpecial
	}
	return nil
}
