-- ============================================================================
-- 禁用E2E测试自动验证触发器
-- 用途：测试完成后，移除自动验证触发器，恢复正常行为
-- ============================================================================

-- 删除触发器
DROP TRIGGER IF EXISTS trigger_auto_verify_test_users ON users;

-- 删除触发器函数
DROP FUNCTION IF EXISTS auto_verify_test_users();

SELECT '✓ 测试用户自动验证触发器已禁用' AS status;
