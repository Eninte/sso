-- ============================================================================
-- 密钥版本表
-- 存储JWT签名密钥的版本信息，支持密钥轮换
-- ============================================================================

CREATE TABLE IF NOT EXISTS key_versions (
    -- 主键 (密钥ID/kid)
    id VARCHAR(64) PRIMARY KEY,

    -- 公钥 (PEM格式)
    public_key TEXT NOT NULL,

    -- 私钥 (PEM格式，仅主密钥存储)
    private_key TEXT,

    -- 密钥状态: active, deprecated, revoked
    status VARCHAR(20) NOT NULL DEFAULT 'active',

    -- 创建时间
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- 过期时间 (用于过渡期)
    expires_at TIMESTAMP
);

-- ============================================================================
-- 索引
-- ============================================================================

-- 状态索引，用于查询活跃密钥
CREATE INDEX idx_key_versions_status ON key_versions(status);

-- 创建时间索引，用于按时间排序
CREATE INDEX idx_key_versions_created_at ON key_versions(created_at);

-- ============================================================================
-- 表注释
-- ============================================================================

COMMENT ON TABLE key_versions IS '密钥版本表 - 存储JWT签名密钥的版本信息';
COMMENT ON COLUMN key_versions.id IS '密钥唯一标识 (kid)';
COMMENT ON COLUMN key_versions.public_key IS '公钥 (PEM格式)';
COMMENT ON COLUMN key_versions.private_key IS '私钥 (PEM格式，仅主密钥存储)';
COMMENT ON COLUMN key_versions.status IS '密钥状态: active(活跃), deprecated(过渡期), revoked(已撤销)';
COMMENT ON COLUMN key_versions.created_at IS '创建时间';
COMMENT ON COLUMN key_versions.expires_at IS '过期时间 (用于过渡期)';
