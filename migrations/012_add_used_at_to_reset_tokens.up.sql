-- 添加 used_at 字段到 reset_tokens 表
-- 用于防止密码重置令牌被重复使用

ALTER TABLE reset_tokens
ADD COLUMN used_at TIMESTAMP NULL;

-- 添加索引以快速查询未使用的令牌
CREATE INDEX idx_reset_tokens_unused ON reset_tokens (user_id) WHERE used_at IS NULL;

-- 添加注释
COMMENT ON COLUMN reset_tokens.used_at IS '令牌使用时间；NULL表示未使用';
