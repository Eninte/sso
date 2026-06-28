-- 迁移MFA恢复码从bcrypt哈希到HMAC-SHA256哈希
-- 注意：此迁移会使所有现有的恢复码失效，用户需要重新生成
-- 这是安全的，因为旧格式（bcrypt）与新格式（HMAC）不兼容

-- 方案：清空所有恢复码，用户下次启用MFA时会自动生成新的HMAC格式恢复码
-- 这比尝试转换bcrypt哈希到HMAC更安全（因为bcrypt是单向的，无法转换）

-- 记录迁移开始
DO $$
BEGIN
    RAISE NOTICE '开始迁移MFA恢复码：清空所有现有恢复码（bcrypt→HMAC-SHA256）';
    RAISE NOTICE '注意：所有现有恢复码将失效，用户需要重新生成';
END $$;

-- 清空所有恢复码（新代码使用HMAC-SHA256，与旧bcrypt格式不兼容）
DELETE FROM mfa_recovery_codes;

-- 记录迁移完成
DO $$
BEGIN
    RAISE NOTICE 'MFA恢复码迁移完成：所有恢复码已清空';
    RAISE NOTICE '用户下次启用MFA时将自动生成HMAC-SHA256格式的恢复码';
END $$;
