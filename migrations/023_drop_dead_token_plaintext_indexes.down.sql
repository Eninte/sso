-- ============================================================================
-- 清理 tokens 表明文列上的 4 个死索引 — 回滚
--
-- 还原迁移前的 schema 形态：
--   - 003 的两个 UNIQUE 约束用 ADD CONSTRAINT 重建
--     （PG 不支持 ADD CONSTRAINT IF NOT EXISTS，本回滚不可重复执行，
--      与既有迁移 015 的 down 保持同等幂等性口径）
--   - 015 保留的两个普通索引用 CREATE INDEX IF NOT EXISTS 重建
--
-- 注意：回滚后这些索引仍是无用的死索引（列恒为 NULL），仅用于完整还原
-- 迁移前的 schema 形态。列上多行 NULL 互不冲突，UNIQUE 约束可以正常重建。
-- ============================================================================

ALTER TABLE tokens ADD CONSTRAINT tokens_access_token_key UNIQUE (access_token);
ALTER TABLE tokens ADD CONSTRAINT tokens_refresh_token_key UNIQUE (refresh_token);

CREATE INDEX IF NOT EXISTS idx_tokens_access_token ON tokens (access_token);
CREATE INDEX IF NOT EXISTS idx_tokens_refresh_token ON tokens (refresh_token);
