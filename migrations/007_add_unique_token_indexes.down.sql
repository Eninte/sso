-- 为验证令牌和重置令牌移除唯一索引
DROP INDEX IF EXISTS idx_verification_token;
DROP INDEX IF EXISTS idx_reset_token;
