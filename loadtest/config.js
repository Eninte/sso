// k6 全局配置
// 通过环境变量覆盖: k6 run --env BASE_URL=http://sso:9090 config.js

// 从环境变量读取配置（无硬编码敏感默认值 #6）
export const BASE_URL = __ENV.BASE_URL || 'http://localhost:9090';
export const ADMIN_EMAIL = __ENV.E2E_ADMIN_EMAIL || '';
export const ADMIN_PASSWORD = __ENV.E2E_ADMIN_PASSWORD || '';
export const CLIENT_SECRET = __ENV.OAUTH_CLIENT_SECRET || '';
export const PUBLIC_CLIENT_ID = __ENV.OAUTH_PUBLIC_CLIENT_ID || 'public-test-client';
export const CONFIDENTIAL_CLIENT_ID = __ENV.OAUTH_CONFIDENTIAL_CLIENT_ID || 'confidential-test-client';
export const REDIRECT_URI = __ENV.OAUTH_REDIRECT_URI || 'http://localhost:3000/callback';
export const TEST_PASSWORD = __ENV.TEST_PASSWORD || '';

// 数据池规模（用于数据生成）
export const USER_POOL_SIZE = parseInt(__ENV.USER_POOL_SIZE || '1000', 10);
export const TOKEN_POOL_SIZE = parseInt(__ENV.TOKEN_POOL_SIZE || '2000', 10);
export const ADMIN_TOKEN_POOL_SIZE = parseInt(__ENV.ADMIN_TOKEN_POOL_SIZE || '50', 10);
export const EMAIL_POOL_SIZE = parseInt(__ENV.EMAIL_POOL_SIZE || '10000', 10);
export const MALICIOUS_POOL_SIZE = parseInt(__ENV.MALICIOUS_POOL_SIZE || '100', 10);

// 验证必填环境变量
if (!ADMIN_EMAIL || !ADMIN_PASSWORD) {
  console.warn('警告: 未设置 E2E_ADMIN_EMAIL 或 E2E_ADMIN_PASSWORD，管理员场景将不可用');
}
if (!TEST_PASSWORD) {
  console.warn('警告: 未设置 TEST_PASSWORD，注册场景将使用空密码');
}
