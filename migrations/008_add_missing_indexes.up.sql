-- ============================================================================
-- 补充缺失的索引
-- 为外键字段和其他常用查询字段添加索引
-- ============================================================================

-- 为tokens表添加client_id索引（外键未自动创建索引）
CREATE INDEX IF NOT EXISTS idx_tokens_client_id ON tokens(client_id);

-- 为authorization_codes表添加user_id索引
CREATE INDEX IF NOT EXISTS idx_authorization_codes_user_id ON authorization_codes(user_id);

-- 为verification_tokens表添加user_id索引
CREATE INDEX IF NOT EXISTS idx_verification_tokens_user_id ON verification_tokens(user_id);

-- ============================================================================
-- 索引注释
-- ============================================================================

COMMENT ON INDEX idx_tokens_client_id IS 'Token客户端ID索引';
COMMENT ON INDEX idx_authorization_codes_user_id IS '授权码用户ID索引';
COMMENT ON INDEX idx_verification_tokens_user_id IS '验证令牌用户ID索引';
