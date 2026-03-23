-- 为验证令牌和重置令牌添加唯一索引，防止令牌重复
CREATE UNIQUE INDEX IF NOT EXISTS idx_verification_token ON verification_tokens(token);
CREATE UNIQUE INDEX IF NOT EXISTS idx_reset_token ON reset_tokens(token);
