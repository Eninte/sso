-- ============================================================================
-- E2E测试：自动验证测试用户触发器
-- 用途：在测试环境中，自动验证 @example.com 域名的测试用户
-- 警告：仅用于测试环境，生产环境禁止使用！
-- ============================================================================

-- 创建触发器函数：自动验证测试用户邮箱
CREATE OR REPLACE FUNCTION auto_verify_test_users()
RETURNS TRIGGER AS $$
BEGIN
    -- 如果是 @example.com 域名的邮箱，自动设置为已验证
    IF NEW.email LIKE '%@example.com' THEN
        NEW.email_verified := true;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 创建触发器：在插入用户时自动验证测试用户
DROP TRIGGER IF EXISTS trigger_auto_verify_test_users ON users;
CREATE TRIGGER trigger_auto_verify_test_users
    BEFORE INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION auto_verify_test_users();

-- 验证现有的测试用户
UPDATE users SET email_verified = true WHERE email LIKE '%@example.com' AND email_verified = false;

SELECT '✓ 测试用户自动验证触发器已启用' AS status;
