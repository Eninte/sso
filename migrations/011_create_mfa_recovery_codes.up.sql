-- 创建MFA恢复码表
CREATE TABLE IF NOT EXISTS mfa_recovery_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash VARCHAR(255) NOT NULL,
    used_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- 创建索引
CREATE INDEX idx_mfa_recovery_codes_user_id ON mfa_recovery_codes(user_id);
CREATE INDEX idx_mfa_recovery_codes_unused ON mfa_recovery_codes(user_id) WHERE used_at IS NULL;

-- 添加唯一约束（用户不能有重复的恢复码哈希）
CREATE UNIQUE INDEX idx_mfa_recovery_codes_user_code ON mfa_recovery_codes(user_id, code_hash);
