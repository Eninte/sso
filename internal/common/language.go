// Package common 公共工具包
// 提供项目级别的通用函数
package common

import (
	"strings"
)

// NormalizeLanguage 规范化语言代码
// 处理 Accept-Language 格式，返回标准化的语言代码
func NormalizeLanguage(lang string) string {
	lang = strings.ToLower(lang)

	// 处理 Accept-Language 格式: "en-US,en;q=0.9,zh-CN;q=0.8"
	if idx := strings.Index(lang, ","); idx != -1 {
		lang = lang[:idx]
	}
	if idx := strings.Index(lang, ";"); idx != -1 {
		lang = lang[:idx]
	}

	// 去除空格
	lang = strings.TrimSpace(lang)

	// 映射简化语言代码
	switch {
	case strings.HasPrefix(lang, "zh"):
		return "zh-CN"
	case strings.HasPrefix(lang, "en"):
		return "en-US"
	default:
		return lang
	}
}
