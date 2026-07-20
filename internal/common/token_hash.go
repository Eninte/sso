// Package common 公共工具包
// token_hash.go - Token 哈希工具
//
// 阶段 3.2 安全增强：Token 哈希存储
//
// 设计：
//   - 使用 SHA-256 计算 token 哈希
//   - 输出 hex 编码（64 字符固定长度）
//   - 不加盐：token 本身是 32 字节高熵随机串，不需要抵抗彩虹表攻击
//   - 用于数据库存储：access_token_hash / refresh_token_hash 字段
//   - 用于安全查询：WHERE access_token_hash = $1（避免明文出现在 SQL）
package common

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashToken 计算 token 的 SHA-256 哈希
//
// 输入: 任意长度的 token 字符串
// 输出: 64 字符 hex 编码字符串
//
// 用法：
//   hash := common.HashToken(accessToken)
//   db.Query("SELECT ... WHERE access_token_hash = $1", hash)
//
// 安全性：
//   - SHA-256 抗碰撞
//   - token 高熵随机串，无需加盐
//   - 相同 token 永远产生相同 hash（便于查询）
func HashToken(token string) string {
	if token == "" {
		return ""
	}
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
