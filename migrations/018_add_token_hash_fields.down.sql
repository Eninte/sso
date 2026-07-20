-- 阶段 3.2 回滚：移除 token hash 字段
DROP INDEX IF EXISTS idx_tokens_access_token_hash;
DROP INDEX IF EXISTS idx_tokens_refresh_token_hash;
DROP INDEX IF EXISTS idx_tokens_access_token_hash_unique;
DROP INDEX IF EXISTS idx_tokens_refresh_token_hash_unique;

ALTER TABLE tokens DROP COLUMN IF EXISTS access_token_hash;
ALTER TABLE tokens DROP COLUMN IF EXISTS refresh_token_hash;
