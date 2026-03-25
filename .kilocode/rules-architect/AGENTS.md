# 项目架构规则（仅非显眼内容）

## 分层架构
- **Handler层**：HTTP请求处理，输入验证，错误响应
- **Service层**：业务逻辑，事务管理，外部服务调用
- **Store层**：数据访问抽象，数据库操作
- **Model层**：数据结构定义，JSON序列化

## 依赖注入
- 使用接口定义依赖关系（如 `store.Store`、`service.AuthServiceInterface`）
- 测试时使用 `mock.MockStore` 替代真实数据库
- 配置通过 `config.Load()` 函数加载

## 错误处理流程
- Store层返回统一错误（`store.ErrNotFound`、`store.ErrDuplicateEmail`）
- Service层包装错误并添加业务上下文
- Handler层转换为HTTP状态码和错误消息

## 认证流程
1. 用户登录 → 生成Access Token（RS256签名）和Refresh Token（随机字符串）
2. Token验证 → 验证签名算法、过期时间、撤销状态
3. Token刷新 → 使用Refresh Token获取新的Token对
4. Token撤销 → 设置 `RevokedAt` 字段标记撤销

## 数据库设计
- 用户表：`users`（包含登录尝试次数、锁定时间）
- 客户端表：`clients`（OAuth客户端配置）
- Token表：`tokens`（访问令牌和刷新令牌）
- 授权码表：`authorization_codes`（OAuth授权码）
- 审计日志表：`audit_logs`（操作记录）

## 安全机制
- 密码使用bcrypt哈希（生产环境cost >= 12）
- JWT使用RS256算法签名
- 登录失败锁定机制（默认5次失败锁定30分钟）
- 限流保护（默认100请求/分钟）
- CORS配置（生产环境必须设置允许的源）
