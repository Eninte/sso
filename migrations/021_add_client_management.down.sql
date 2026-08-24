-- ============================================================================
-- 回滚 021（开发环境用；存在历史 token 行时可能因 FK 校验失败，属预期）
-- ============================================================================

ALTER TABLE tokens DROP CONSTRAINT IF EXISTS tokens_client_id_fkey;
ALTER TABLE tokens ADD CONSTRAINT tokens_client_id_fkey
    FOREIGN KEY (client_id) REFERENCES oauth_clients(client_id);

ALTER TABLE authorization_codes DROP CONSTRAINT IF EXISTS authorization_codes_client_id_fkey;
ALTER TABLE authorization_codes ADD CONSTRAINT authorization_codes_client_id_fkey
    FOREIGN KEY (client_id) REFERENCES oauth_clients(client_id);

ALTER TABLE oauth_clients DROP COLUMN IF EXISTS is_active;
ALTER TABLE oauth_clients DROP COLUMN IF EXISTS updated_at;
