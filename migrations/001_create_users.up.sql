-- ============================================================================
-- 用户表
-- 存储所有注册用户的基本信息和认证数据
-- ============================================================================

CREATE TABLE IF NOT EXISTS users (
    -- 主键，使用UUID作为唯一标识
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- 邮箱地址，用于登录和通信，必须唯一
    email VARCHAR(255) UNIQUE NOT NULL,

    -- 密码哈希值 (使用bcrypt算法)
    -- 注意: 永远不要存储明文密码
    password_hash VARCHAR(255),

    -- 邮箱是否已通过验证
    email_verified BOOLEAN DEFAULT FALSE,

    -- 多因素认证(MFA)是否已启用
    mfa_enabled BOOLEAN DEFAULT FALSE,

    -- MFA密钥 (用于TOTP验证器应用)
    mfa_secret VARCHAR(255),

    -- 账户状态
    -- 可选值: active(正常), locked(锁定), disabled(禁用)
    status VARCHAR(20) DEFAULT 'active',

    -- 登录失败计数器
    -- 达到阈值后账户将被锁定
    login_attempts INTEGER DEFAULT 0,

    -- 账户锁定截止时间
    -- 锁定期间用户无法登录
    locked_until TIMESTAMP,

    -- 记录创建时间
    created_at TIMESTAMP DEFAULT NOW(),

    -- 记录最后更新时间
    updated_at TIMESTAMP DEFAULT NOW()
);

-- ============================================================================
-- 索引
-- ============================================================================

-- 邮箱索引，加速登录查询
CREATE INDEX idx_users_email ON users(email);

-- 状态索引，用于过滤活跃用户
CREATE INDEX idx_users_status ON users(status);

-- ============================================================================
-- 表注释
-- ============================================================================

COMMENT ON TABLE users IS '用户表 - 存储用户基本信息和认证数据';
COMMENT ON COLUMN users.id IS '用户唯一标识 (UUID)';
COMMENT ON COLUMN users.email IS '邮箱地址 (唯一)';
COMMENT ON COLUMN users.password_hash IS '密码哈希 (bcrypt)';
COMMENT ON COLUMN users.email_verified IS '邮箱验证状态';
COMMENT ON COLUMN users.mfa_enabled IS 'MFA启用状态';
COMMENT ON COLUMN users.mfa_secret IS 'MFA密钥 (TOTP)';
COMMENT ON COLUMN users.status IS '账户状态: active/locked/disabled';
COMMENT ON COLUMN users.login_attempts IS '登录失败次数';
COMMENT ON COLUMN users.locked_until IS '锁定截止时间';
