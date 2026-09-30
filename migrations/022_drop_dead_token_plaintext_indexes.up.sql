-- ============================================================================
-- 清理 tokens 表明文列上的 4 个死索引
--
-- 背景：
--   - 迁移 019 之后 access_token / refresh_token 两列恒为 NULL（明文不落库）
--   - 建在这两列上的索引 100% 死索引：纯写放大，对任何查询都没有贡献
--     （PostgreSQL 不会用恒 NULL 列上的索引）
--   - 015 的 down 迁移甚至会把列类型缩回 VARCHAR(500)，进一步放大维护成本
--
-- 本迁移删除以下 4 个索引：
--   - tokens_access_token_key  （UNIQUE 约束所属索引，来自 003 列级 UNIQUE）
--   - tokens_refresh_token_key （UNIQUE 约束所属索引，来自 003 列级 UNIQUE）
--   - idx_tokens_access_token  （普通索引，来自 015 REINDEX 保留）
--   - idx_tokens_refresh_token （普通索引，来自 015 REINDEX 保留）
--
-- 实现说明（关键）：
--   - tokens_access_token_key / tokens_refresh_token_key 是 UNIQUE 约束的
--     所属索引，PostgreSQL 拒绝对其执行 DROP INDEX
--     （ERROR: cannot drop index ... because constraint ... requires it），
--     必须用 ALTER TABLE ... DROP CONSTRAINT 删除
--   - 唯一性语义不受影响：明文列已废弃，唯一性由
--     idx_tokens_refresh_token_hash_unique（迁移 018）承担
--   - 幂等：IF EXISTS / IF NOT EXISTS 保证 up 可重复执行
-- ============================================================================

ALTER TABLE tokens DROP CONSTRAINT IF EXISTS tokens_access_token_key;
ALTER TABLE tokens DROP CONSTRAINT IF EXISTS tokens_refresh_token_key;

DROP INDEX IF EXISTS idx_tokens_access_token;
DROP INDEX IF EXISTS idx_tokens_refresh_token;
