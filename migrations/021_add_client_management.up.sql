-- ============================================================================
-- OAuth 客户端管理（021）
-- 1. oauth_clients 增加 is_active / updated_at 字段（存量行默认 active）
-- 2. authorization_codes.client_id 外键改为 ON DELETE CASCADE（授权码为短命数据）
-- 3. tokens.client_id 外键改为 ON DELETE SET NULL（保留历史 token 行，置空归属）
-- ============================================================================

ALTER TABLE oauth_clients ADD COLUMN updated_at TIMESTAMP DEFAULT NOW();
ALTER TABLE oauth_clients ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT TRUE;

-- 注意：003 迁移未命名约束，默认名为 <表>_<列>_fkey。
-- 执行前先在测试库确认：SELECT conname FROM pg_constraint WHERE conrelid = 'authorization_codes'::regclass AND contype='f';
ALTER TABLE authorization_codes DROP CONSTRAINT IF EXISTS authorization_codes_client_id_fkey;
ALTER TABLE authorization_codes ADD CONSTRAINT authorization_codes_client_id_fkey
    FOREIGN KEY (client_id) REFERENCES oauth_clients(client_id) ON DELETE CASCADE;

ALTER TABLE tokens DROP CONSTRAINT IF EXISTS tokens_client_id_fkey;
ALTER TABLE tokens ADD CONSTRAINT tokens_client_id_fkey
    FOREIGN KEY (client_id) REFERENCES oauth_clients(client_id) ON DELETE SET NULL;
