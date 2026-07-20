-- 回滚阶段 2.1：Refresh Token 原子轮换支持字段

DROP INDEX IF EXISTS idx_tokens_refresh_expires_at;
DROP INDEX IF EXISTS idx_tokens_replaced_by_token_id;

ALTER TABLE tokens DROP COLUMN IF EXISTS refresh_expires_at;
ALTER TABLE tokens DROP COLUMN IF EXISTS replaced_by_token_id;
ALTER TABLE tokens DROP COLUMN IF EXISTS rotated_at;
