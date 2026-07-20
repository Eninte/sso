-- ============================================================================
-- 阶段 3.2：Token 哈希存储
--
-- 安全设计：
--   - access_token / refresh_token 明文存储面临数据库泄露风险
--   - 新增 access_token_hash / refresh_token_hash 存储 SHA-256 hex
--   - 查询优先使用 hash，避免明文出现在 WHERE 条件中
--   - 兼容旧数据：迁移脚本回填现有 token 的 hash
--
-- 哈希选择 SHA-256（不加 salt）：
--   - token 本身是 32 字节高熵随机串（base64 后 43 字符），熵足够高
--   - 不需要抵抗彩虹表攻击（每个 token 都是随机的）
--   - 加盐会增加查询复杂度，需要额外存储 salt 字段
-- ============================================================================

-- 启用 pgcrypto 扩展以使用 digest() 函数计算 SHA-256 哈希
-- CI/CD 修复：digest() 来自 pgcrypto 扩展，必须先启用才能使用
-- IF NOT EXISTS 确保多次执行幂等
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 新增 hash 字段（允许 NULL，兼容旧数据）
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS access_token_hash CHAR(64);
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS refresh_token_hash CHAR(64);

-- 回填现有数据的 hash（使用 pgcrypto 的 digest 函数）
-- encode(digest(token, 'sha256'), 'hex') 输出 64 字符 hex 字符串
UPDATE tokens SET access_token_hash = encode(digest(access_token, 'sha256'), 'hex')
    WHERE access_token IS NOT NULL AND access_token_hash IS NULL;

UPDATE tokens SET refresh_token_hash = encode(digest(refresh_token, 'sha256'), 'hex')
    WHERE refresh_token IS NOT NULL AND refresh_token_hash IS NULL;

-- 在 hash 字段上创建唯一索引（与原明文索引对齐）
-- 部分索引：仅对非 NULL 的 hash 建立唯一约束
-- 阶段 D 审查修复（H7）：原迁移同时创建唯一索引 + 普通索引，写入需维护双索引
-- 唯一索引已自动承担查询功能，普通索引冗余，删除以减少写入开销
CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_access_token_hash_unique
    ON tokens(access_token_hash)
    WHERE access_token_hash IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_refresh_token_hash_unique
    ON tokens(refresh_token_hash)
    WHERE refresh_token_hash IS NOT NULL;

COMMENT ON COLUMN tokens.access_token_hash IS '访问令牌的 SHA-256 哈希值（阶段 3.2 安全增强）';
COMMENT ON COLUMN tokens.refresh_token_hash IS '刷新令牌的 SHA-256 哈希值（阶段 3.2 安全增强）';
