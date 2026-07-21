-- ============================================================================
-- T1 安全修复：tokens 表去除明文存储（H1）
--
-- 背景：
--   - access_token / refresh_token 明文落库面临数据库泄露风险
--   - 代码已切换为仅存/只查 hash（access_token_hash / refresh_token_hash）
--   - 本迁移清除两列明文数据
--
-- 注意：
--   - access_token 列定义为 NOT NULL（迁移 003），须先 DROP NOT NULL
--   - refresh_token 列原本允许 NULL，无需变更约束
--   - 迁移执行后，018 之前签发且 hash 列为 NULL 的 token 立即失效，
--     用户需重新登录（强制轮换，可接受）
--   - access_token 上的 UNIQUE 约束保留：Postgres 中多个 NULL 互不冲突
-- ============================================================================

-- 解除 access_token 的 NOT NULL 约束，允许写入 NULL
ALTER TABLE tokens ALTER COLUMN access_token DROP NOT NULL;

-- 清除全部明文数据
UPDATE tokens SET access_token = NULL, refresh_token = NULL;

COMMENT ON COLUMN tokens.access_token IS '已废弃（T1）：仅存 NULL，明文不落库，保留列仅为兼容';
COMMENT ON COLUMN tokens.refresh_token IS '已废弃（T1）：仅存 NULL，明文不落库，保留列仅为兼容';
