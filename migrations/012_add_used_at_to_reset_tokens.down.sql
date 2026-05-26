-- 回滚：删除 used_at 字段

DROP INDEX IF EXISTS idx_reset_tokens_unused;

ALTER TABLE reset_tokens
DROP COLUMN IF EXISTS used_at;
