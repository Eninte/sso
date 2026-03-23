-- ============================================================================
-- 审计日志表
-- 存储所有认证和授权事件的日志记录
-- ============================================================================

CREATE TABLE IF NOT EXISTS audit_logs (
    -- 主键
    id VARCHAR(64) PRIMARY KEY,

    -- 事件类型
    event_type VARCHAR(50) NOT NULL,

    -- 用户ID (可为空，如限流事件)
    user_id VARCHAR(36),

    -- 客户端ID
    client_id VARCHAR(255),

    -- IP地址
    ip_address VARCHAR(45),

    -- 用户代理
    user_agent TEXT,

    -- 事件详情 (JSON格式)
    details JSONB,

    -- 是否成功
    success BOOLEAN DEFAULT TRUE,

    -- 时间戳
    timestamp TIMESTAMP DEFAULT NOW()
);

-- ============================================================================
-- 索引
-- ============================================================================

-- 事件类型索引，用于按类型查询
CREATE INDEX idx_audit_logs_event_type ON audit_logs(event_type);

-- 用户ID索引，用于查询用户相关日志
CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id);

-- 时间戳索引，用于按时间范围查询
CREATE INDEX idx_audit_logs_timestamp ON audit_logs(timestamp);

-- 复合索引，用于查询用户的时间范围日志
CREATE INDEX idx_audit_logs_user_timestamp ON audit_logs(user_id, timestamp);

-- ============================================================================
-- 表注释
-- ============================================================================

COMMENT ON TABLE audit_logs IS '审计日志表 - 存储所有认证和授权事件';
COMMENT ON COLUMN audit_logs.id IS '日志唯一标识';
COMMENT ON COLUMN audit_logs.event_type IS '事件类型';
COMMENT ON COLUMN audit_logs.user_id IS '用户ID';
COMMENT ON COLUMN audit_logs.client_id IS '客户端ID';
COMMENT ON COLUMN audit_logs.ip_address IS 'IP地址';
COMMENT ON COLUMN audit_logs.user_agent IS '用户代理';
COMMENT ON COLUMN audit_logs.details IS '事件详情 (JSON)';
COMMENT ON COLUMN audit_logs.success IS '是否成功';
COMMENT ON COLUMN audit_logs.timestamp IS '时间戳';
