-- ============================================================================
-- OAuth2客户端表
-- 存储注册的OAuth2应用程序信息
-- 每个需要接入SSO的应用都需要在此表注册
-- ============================================================================

CREATE TABLE IF NOT EXISTS oauth_clients (
    -- 主键
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- 客户端ID (公开标识，用于OAuth2流程)
    client_id VARCHAR(255) UNIQUE NOT NULL,

    -- 客户端密钥 (哈希存储，仅机密客户端使用)
    -- 公共客户端(如SPA、移动应用)不使用此字段
    client_secret VARCHAR(255),

    -- 应用显示名称
    name VARCHAR(255) NOT NULL,

    -- 允许的重定向URI列表
    -- OAuth2授权后将用户重定向到这些URI之一
    redirect_uris TEXT[] NOT NULL DEFAULT '{}',

    -- 允许的授权类型
    -- authorization_code: 授权码模式 (最安全)
    -- refresh_token: 刷新令牌
    -- client_credentials: 客户端凭证模式 (服务间通信)
    grant_types TEXT[] NOT NULL DEFAULT '{authorization_code}',

    -- 允许的权限范围
    -- openid: OpenID Connect必需
    -- profile: 用户基本信息
    -- email: 用户邮箱
    scopes TEXT[] NOT NULL DEFAULT '{openid,profile,email}',

    -- 是否为公共客户端
    -- true: 移动应用、SPA等无法安全存储密钥的客户端
    -- false: 服务器端应用，可以安全存储密钥
    public_client BOOLEAN DEFAULT FALSE,

    -- 记录创建时间
    created_at TIMESTAMP DEFAULT NOW()
);

-- ============================================================================
-- 索引
-- ============================================================================

-- 客户端ID索引，加速OAuth2流程查询
CREATE INDEX idx_oauth_clients_client_id ON oauth_clients(client_id);

-- ============================================================================
-- 表注释
-- ============================================================================

COMMENT ON TABLE oauth_clients IS 'OAuth2客户端表 - 存储接入SSO的应用信息';
COMMENT ON COLUMN oauth_clients.client_id IS '客户端ID (公开)';
COMMENT ON COLUMN oauth_clients.client_secret IS '客户端密钥 (哈希)';
COMMENT ON COLUMN oauth_clients.redirect_uris IS '允许的重定向URI';
COMMENT ON COLUMN oauth_clients.grant_types IS '允许的授权类型';
COMMENT ON COLUMN oauth_clients.public_client IS '是否为公共客户端';
