// Package common 公共工具包
// 提供项目级别的通用函数
package common

import (
	"crypto/rand"
	"encoding/base64"
)

// GenerateRandomString 生成指定长度的随机字符串
// 使用 crypto/rand 生成安全的随机字节，然后进行 Base64 编码
func GenerateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// GenerateToken 生成32字节的随机令牌
// 返回 URL 安全的 Base64 编码字符串
func GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
