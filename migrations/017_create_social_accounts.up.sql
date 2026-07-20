-- 阶段 2.3：社交登录独立身份表
--
-- 目标：
--   将社交账号身份从 users.email 解耦，避免账号接管攻击
--   原实现：用 email 查找 user → 同一 email 跨 provider 自动合并（接管风险）
--   新实现：用 (provider, provider_user_id) 查找 social_accounts → user_id 关联
--
-- 安全收益：
--   1. 攻击者无法通过控制 Google/GitHub 上一个 email 来接管本系统已有账号
--   2. 一个用户可绑定多个社交账号（多对一关系）
--   3. 社交账号与本地账号的关联可追溯（created_at / updated_at）
--
-- 字段说明：
--   id                    — 主键（UUID）
--   provider              — 登录提供商：google / github（NOT NULL）
--   provider_user_id      — 提供商返回的用户 ID（NOT NULL）
--   user_id               — 关联的本系统 users.id（NOT NULL）
--   provider_email        — 提供商返回的 email（可能为空，GitHub 不公开 email 时）
--   email_verified        — 提供商返回的 email_verified 字段（boolean）
--   provider_metadata    — 提供商返回的其他元信息（JSONB，如头像、昵称等）
--   created_at            — 绑定时间
--   updated_at            — 最后更新时间
--
-- 索引：
--   uniq_provider_provider_user_id  — (provider, provider_user_id) 唯一索引
--                                       确保同一社交账号只能绑定一个本系统用户
--   idx_social_accounts_user_id      — user_id 索引，便于查询用户绑定的所有社交账号
--   uniq_user_provider               — (user_id, provider) 唯一索引
--                                       确保一个用户在同一 provider 下只能绑定一个账号

CREATE TABLE IF NOT EXISTS social_accounts (
    id                  VARCHAR(36) PRIMARY KEY,
    provider            VARCHAR(32)  NOT NULL,
    provider_user_id    VARCHAR(255) NOT NULL,
    user_id             VARCHAR(36)  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_email      VARCHAR(255),
    email_verified      BOOLEAN      NOT NULL DEFAULT FALSE,
    provider_metadata   JSONB,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uniq_provider_provider_user_id UNIQUE (provider, provider_user_id),
    CONSTRAINT uniq_user_provider UNIQUE (user_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_social_accounts_user_id ON social_accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_social_accounts_provider_email ON social_accounts(provider, provider_email);
