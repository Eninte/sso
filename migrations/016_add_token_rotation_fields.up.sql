-- ============================================================================
-- 阶段 2.1：Refresh Token 原子轮换支持字段
-- 1. rotated_at         — refresh token 被轮换使用的时间（NULL 表示未使用）
-- 2. replaced_by_token_id — 轮换后新 token 的 ID，用于 token 家族追踪
-- 3. refresh_expires_at — refresh token 独立过期时间（与 access_token 的 expires_at 解耦）
--
-- 安全设计：
--   - rotated_at + WHERE rotated_at IS NULL 用于原子轮换（防 TOCTOU）
--   - replaced_by_token_id 用于 token 家族追踪与盗用检测
--   - refresh_expires_at 用于强制 refresh token 过期检查
-- ============================================================================

ALTER TABLE tokens ADD COLUMN IF NOT EXISTS rotated_at TIMESTAMP;
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS replaced_by_token_id UUID;
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS refresh_expires_at TIMESTAMP;

-- 回填：未撤销的旧 token 沿用 access_token 过期时间 + 7 天作为 refresh 过期时间
UPDATE tokens
SET refresh_expires_at = expires_at + INTERVAL '7 days'
WHERE refresh_expires_at IS NULL
  AND revoked_at IS NULL;

-- 索引：支持按 replaced_by_token_id 进行家族追踪查询
CREATE INDEX IF NOT EXISTS idx_tokens_replaced_by_token_id
    ON tokens(replaced_by_token_id)
    WHERE replaced_by_token_id IS NOT NULL;

-- 索引：支持按 refresh_expires_at 清理过期 refresh token
CREATE INDEX IF NOT EXISTS idx_tokens_refresh_expires_at
    ON tokens(refresh_expires_at)
    WHERE refresh_expires_at IS NOT NULL;

COMMENT ON COLUMN tokens.rotated_at IS 'Refresh Token 被轮换使用的时间 (NULL=未使用)';
COMMENT ON COLUMN tokens.replaced_by_token_id IS '轮换后新 token 的 ID，用于 token 家族追踪';
COMMENT ON COLUMN tokens.refresh_expires_at IS 'Refresh Token 独立过期时间';
