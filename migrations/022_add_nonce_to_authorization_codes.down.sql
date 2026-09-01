-- 022 回滚
ALTER TABLE authorization_codes DROP COLUMN IF EXISTS nonce;
