-- ============================================================================
-- 扩大 tokens 表的 token 列长度
-- RS256 JWT（2048-bit RSA 密钥）生成的 access_token 约 600-800 字符，
-- 超出 VARCHAR(500) 限制导致登录返回 500。
-- 改为 TEXT 类型以支持任意长度的 token。
-- ============================================================================

ALTER TABLE tokens ALTER COLUMN access_token TYPE TEXT;
ALTER TABLE tokens ALTER COLUMN refresh_token TYPE TEXT;

-- 刷新现有索引（PostgreSQL ALTER TYPE 会自动重建依赖索引，
-- 但显式 REINDEX 确保统计信息更新）
REINDEX INDEX idx_tokens_access_token;
REINDEX INDEX idx_tokens_refresh_token;
REINDEX INDEX tokens_access_token_key;
REINDEX INDEX tokens_refresh_token_key;
