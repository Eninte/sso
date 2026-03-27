-- ============================================================================
-- 用户角色字段
-- 添加基于角色的访问控制 (RBAC)
-- ============================================================================

-- 添加角色字段，默认为 'user'
ALTER TABLE users ADD COLUMN IF NOT EXISTS role VARCHAR(20) DEFAULT 'user';

-- 角色索引，加速管理员查询
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);

-- 添加约束确保角色值有效
ALTER TABLE users ADD CONSTRAINT chk_user_role 
    CHECK (role IN ('user', 'admin'));

-- ============================================================================
-- 表注释
-- ============================================================================

COMMENT ON COLUMN users.role IS '用户角色: user(普通用户), admin(管理员)';
