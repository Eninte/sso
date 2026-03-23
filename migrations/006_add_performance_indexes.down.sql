-- ============================================================================
-- 回滚性能优化索引
-- ============================================================================

DROP INDEX IF EXISTS idx_tokens_user_active;
DROP INDEX IF EXISTS idx_tokens_expired;
DROP INDEX IF EXISTS idx_authorization_codes_unused;
DROP INDEX IF EXISTS idx_audit_logs_user_event_timestamp;
DROP INDEX IF EXISTS idx_users_created_at;
