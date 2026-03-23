-- ============================================================================
-- 性能优化索引
-- 添加缺失的索引以提升查询性能
-- ============================================================================

-- 为tokens表添加复合索引，用于查询用户的有效Token
CREATE INDEX IF NOT EXISTS idx_tokens_user_active ON tokens(user_id, revoked_at) 
    WHERE revoked_at IS NULL;

-- 为tokens表添加复合索引，用于查询过期且未撤销的Token（用于清理）
CREATE INDEX IF NOT EXISTS idx_tokens_expired ON tokens(expires_at) 
    WHERE revoked_at IS NULL;

-- 为authorization_codes表添加复合索引，用于查询未使用的授权码
CREATE INDEX IF NOT EXISTS idx_authorization_codes_unused ON authorization_codes(expires_at) 
    WHERE used_at IS NULL;

-- 为audit_logs表添加复合索引，用于查询用户特定事件类型的时间范围日志
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_event_timestamp ON audit_logs(user_id, event_type, timestamp);

-- 为users表添加created_at索引，用于分页查询
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at DESC);

-- ============================================================================
-- 索引注释
-- ============================================================================

COMMENT ON INDEX idx_tokens_user_active IS '用户有效Token索引';
COMMENT ON INDEX idx_tokens_expired IS '过期Token索引（用于清理）';
COMMENT ON INDEX idx_authorization_codes_unused IS '未使用授权码索引';
COMMENT ON INDEX idx_audit_logs_user_event_timestamp IS '用户事件时间范围索引';
COMMENT ON INDEX idx_users_created_at IS '用户创建时间索引（分页）';
