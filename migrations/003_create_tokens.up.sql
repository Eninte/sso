-- ============================================================================
-- Token记录表
-- 用于Token追踪、撤销和审计
-- ============================================================================

CREATE TABLE IF NOT EXISTS tokens (
    -- 主键
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- 访问令牌 (唯一)
    access_token VARCHAR(500) UNIQUE NOT NULL,

    -- 刷新令牌 (唯一，可选)
    refresh_token VARCHAR(500) UNIQUE,

    -- 关联的用户ID
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,

    -- 关联的客户端ID (可选)
    client_id VARCHAR(255) REFERENCES oauth_clients(client_id),

    -- 授予的权限范围
    scopes TEXT[] DEFAULT '{}',

    -- Token过期时间
    expires_at TIMESTAMP NOT NULL,

    -- 记录创建时间
    created_at TIMESTAMP DEFAULT NOW(),

    -- Token撤销时间 (NULL表示未撤销)
    revoked_at TIMESTAMP
);

-- ============================================================================
-- 授权码表
-- 存储OAuth2授权码，用于授权码交换流程
-- ============================================================================

CREATE TABLE IF NOT EXISTS authorization_codes (
    -- 授权码 (主键)
    code VARCHAR(255) PRIMARY KEY,

    -- 关联的客户端ID
    client_id VARCHAR(255) REFERENCES oauth_clients(client_id),

    -- 关联的用户ID
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,

    -- 注册的重定向URI
    redirect_uri VARCHAR(500),

    -- 授予的权限范围
    scopes TEXT[] DEFAULT '{}',

    -- PKCE挑战码 (用于公共客户端安全增强)
    code_challenge VARCHAR(255),

    -- PKCE挑战方法 (S256 或 plain)
    code_challenge_method VARCHAR(10),

    -- 授权码过期时间 (通常很短，如10分钟)
    expires_at TIMESTAMP NOT NULL,

    -- 记录创建时间
    created_at TIMESTAMP DEFAULT NOW(),

    -- 授权码使用时间 (NULL表示未使用)
    -- 授权码只能使用一次
    used_at TIMESTAMP
);

-- ============================================================================
-- 索引
-- ============================================================================

-- Token表索引
CREATE INDEX idx_tokens_user_id ON tokens(user_id);
CREATE INDEX idx_tokens_access_token ON tokens(access_token);
CREATE INDEX idx_tokens_refresh_token ON tokens(refresh_token);
CREATE INDEX idx_tokens_expires_at ON tokens(expires_at);

-- 授权码表索引
CREATE INDEX idx_authorization_codes_client_id ON authorization_codes(client_id);
CREATE INDEX idx_authorization_codes_expires_at ON authorization_codes(expires_at);

-- ============================================================================
-- 表注释
-- ============================================================================

COMMENT ON TABLE tokens IS 'Token记录表 - 用于Token追踪和撤销';
COMMENT ON COLUMN tokens.access_token IS '访问令牌';
COMMENT ON COLUMN tokens.refresh_token IS '刷新令牌';
COMMENT ON COLUMN tokens.revoked_at IS '撤销时间 (NULL=未撤销)';

COMMENT ON TABLE authorization_codes IS '授权码表 - 存储OAuth2授权码';
COMMENT ON COLUMN authorization_codes.code_challenge IS 'PKCE挑战码';
COMMENT ON COLUMN authorization_codes.used_at IS '使用时间 (NULL=未使用)';
