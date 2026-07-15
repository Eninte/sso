-- 回滚：将 tokens 表的 token 列恢复为 VARCHAR(500)
-- 注意：如果已有 token 超过 500 字符，回滚会失败

ALTER TABLE tokens ALTER COLUMN access_token TYPE VARCHAR(500);
ALTER TABLE tokens ALTER COLUMN refresh_token TYPE VARCHAR(500);
