-- 022: OIDC id_token 支持：授权码携带 nonce（浏览器 PKCE 授权码流程）
-- nonce 在 authorize 时由客户端提供，随授权码存储，token 交换时写入 id_token。
ALTER TABLE authorization_codes ADD COLUMN IF NOT EXISTS nonce TEXT;
