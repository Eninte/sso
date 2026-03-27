-- 回滚：移除角色字段
DROP INDEX IF EXISTS idx_users_role;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_user_role;
ALTER TABLE users DROP COLUMN IF EXISTS role;
