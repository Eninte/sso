# SSO 项目代码质量改进计划

**生成时间**: 2026-04-03  
**基于**: CODE_ANALYSIS_REPORT.md 核实结果  
**状态**: 待执行

---

## 一、问题核实结果

### 1.1 已核实的真实问题 ✅

| 优先级 | 问题 | 位置 | 核实状态 |
|--------|------|------|---------|
| **高** | `postgres.go` 1126 行单文件过大 | `internal/store/postgres/postgres.go` | ✅ 确认 |
| **高** | 审计日志 `Close()` 不等待 worker | `internal/service/audit.go:52-56` | ✅ 确认 |
| **高** | Redis `DeletePattern()` 使用 `Keys()` 命令 | `internal/cache/redis.go:217-227` | ✅ 确认 |
| **中** | 邮件模板内联字符串 | `internal/service/email.go:95-145, 155-205` | ✅ 确认 |
| **中** | TOTP 无恢复码 | `internal/service/mfa.go` | ✅ 确认 |
| **中** | 社交登录提供商硬编码 | `internal/service/social.go:68-96` | ✅ 确认 |
| **低** | 内存缓存无大小限制 | `internal/cache/redis.go:73-85` | ✅ 确认 |
| **低** | 分布式限流缺失 | `internal/middleware/ratelimit.go` | ✅ 确认 |

### 1.2 需要进一步评估的问题 ⚠️

| 问题 | 说明 | 建议 |
|------|------|------|
| 自定义 Metrics 非标准 | 需要检查 `internal/metrics/` 实现 | 评估迁移成本 |
| JWT 密钥轮换期间的问题 | 代码已有 `keys` map 保护，需验证边界情况 | 增加集成测试 |
| 两套错误码体系 | Handler 和 Service 层错误码需统一 | 重构错误处理 |

### 1.3 误报或已修复的问题 ❌

| 问题 | 实际情况 |
|------|---------|
| 缓存降级后不会自动切换回 Redis | 这是设计决策，需重启服务，符合预期 |
| 邮件发送无重试机制 | 业务层面决策，SMTP 失败应该快速失败 |

---

## 二、改进计划

### 2.1 高优先级改进（P0 - 必须完成）

#### 问题 1: `postgres.go` 文件过大（1126 行）

**影响**: 代码可维护性差，认知负荷高

**改进方案**:
```
internal/store/postgres/
├── postgres.go          # 核心结构和工厂函数 (~150 行)
├── user.go              # 用户相关操作 (~300 行)
├── client.go            # OAuth 客户端操作 (~100 行)
├── token.go             # Token 和授权码操作 (~250 行)
├── verification.go      # 验证令牌操作 (~100 行)
├── audit.go             # 审计日志操作 (~100 行)
├── key.go               # 密钥版本操作 (~150 行)
└── helpers.go           # 通用辅助函数 (~100 行)
```

**实施步骤**:
1. 创建新文件并移动相关函数
2. 确保所有函数保持 `Store` 接收者
3. 运行完整测试套件验证
4. 更新文档

**预计工作量**: 4-6 小时

---

#### 问题 2: 审计日志 `Close()` 不等待 worker

**当前代码** (`internal/service/audit.go:52-56`):
```go
func (s *AuditService) Close() {
	close(s.stopChan)
}
```

**问题**: 服务关闭时可能丢失 channel 中未处理的审计日志

**改进方案**:
```go
type AuditService struct {
	store    store.Store
	logger   *slog.Logger
	logChan  chan *model.AuditLog
	stopChan chan struct{}
	wg       sync.WaitGroup  // 新增
}

func (s *AuditService) startWorkers() {
	for i := 0; i < auditWorkerCount; i++ {
		s.wg.Add(1)  // 新增
		go s.worker(i)
	}
}

func (s *AuditService) worker(id int) {
	defer s.wg.Done()  // 新增
	slogger := s.logger.With("worker_id", id)
	slogger.Debug("审计日志worker启动")

	for {
		select {
		case <-s.stopChan:
			// 处理剩余日志
			for {
				select {
				case log := <-s.logChan:
					if err := s.store.StoreAuditLog(context.Background(), log); err != nil {
						slogger.Error("存储审计日志失败", "error", err, "log_id", log.ID)
					}
				default:
					slogger.Debug("审计日志worker停止")
					return
				}
			}
		case log := <-s.logChan:
			if err := s.store.StoreAuditLog(context.Background(), log); err != nil {
				slogger.Error("存储审计日志失败", "error", err, "log_id", log.ID)
			}
		}
	}
}

func (s *AuditService) Close() {
	close(s.stopChan)
	s.wg.Wait()  // 等待所有 worker 完成
	close(s.logChan)  // 关闭 channel
}
```

**测试验证**:
```go
func TestAuditServiceGracefulShutdown(t *testing.T) {
	// 1. 创建服务
	// 2. 发送 1000 条日志
	// 3. 立即调用 Close()
	// 4. 验证所有日志都已存储
}
```

**预计工作量**: 2-3 小时

---

#### 问题 3: Redis `DeletePattern()` 使用 `Keys()` 命令

**当前代码** (`internal/cache/redis.go:217-227`):
```go
func (c *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}
```

**问题**: `Keys()` 命令在大数据集上会阻塞 Redis

**改进方案**:
```go
func (c *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	var cursor uint64
	var deletedCount int

	for {
		// 使用 SCAN 命令，每次扫描 100 个键
		keys, nextCursor, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("scan keys failed: %w", err)
		}

		// 批量删除扫描到的键
		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("delete keys failed: %w", err)
			}
			deletedCount += len(keys)
		}

		// 检查是否扫描完成
		cursor = nextCursor
		if cursor == 0 {
			break
		}

		// 避免过度占用 Redis 资源
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}

	return nil
}
```

**性能对比测试**:
```go
func BenchmarkDeletePattern_Keys(b *testing.B)  // 旧实现
func BenchmarkDeletePattern_Scan(b *testing.B)  // 新实现
```

**预计工作量**: 2 小时

---

### 2.2 中优先级改进（P1 - 应该完成）

#### 问题 4: 邮件模板内联字符串

**当前问题**: 
- 模板硬编码在代码中，维护困难
- 无法支持多语言
- 修改模板需要重新编译

**改进方案**:

**目录结构**:
```
internal/service/email/
├── service.go           # 邮件服务核心
├── templates/
│   ├── verification_zh.html
│   ├── verification_en.html
│   ├── reset_zh.html
│   └── reset_en.html
└── templates.go         # 模板加载器
```

**实现**:
```go
// templates.go
package email

import (
	"embed"
	"html/template"
	"sync"
)

//go:embed templates/*.html
var templateFS embed.FS

type TemplateManager struct {
	mu        sync.RWMutex
	templates map[string]*template.Template
}

func NewTemplateManager() (*TemplateManager, error) {
	tm := &TemplateManager{
		templates: make(map[string]*template.Template),
	}
	
	// 加载所有模板
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		name := entry.Name()
		tmpl, err := template.ParseFS(templateFS, "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
		
		tm.templates[name] = tmpl
	}
	
	return tm, nil
}

func (tm *TemplateManager) Render(name string, data interface{}) (string, error) {
	tm.mu.RLock()
	tmpl, ok := tm.templates[name]
	tm.mu.RUnlock()
	
	if !ok {
		return "", fmt.Errorf("template not found: %s", name)
	}
	
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	
	return buf.String(), nil
}
```

**使用示例**:
```go
func (s *EmailService) SendVerificationEmail(ctx context.Context, to, username, verifyLink, lang string) error {
	templateName := fmt.Sprintf("verification_%s.html", lang)
	body, err := s.templateMgr.Render(templateName, map[string]string{
		"Username":   username,
		"VerifyLink": verifyLink,
	})
	if err != nil {
		return err
	}
	
	return s.SendEmail(ctx, to, "验证您的邮箱 - SSO服务", body)
}
```

**预计工作量**: 4 小时

---

#### 问题 5: TOTP 无恢复码

**当前问题**: 用户丢失 TOTP 设备后无法恢复账户

**改进方案**:

**数据库迁移**:
```sql
-- 新增恢复码表
CREATE TABLE mfa_recovery_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash VARCHAR(255) NOT NULL,
    used_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, code_hash)
);

CREATE INDEX idx_mfa_recovery_codes_user_id ON mfa_recovery_codes(user_id);
CREATE INDEX idx_mfa_recovery_codes_unused ON mfa_recovery_codes(user_id) WHERE used_at IS NULL;
```

**服务实现**:
```go
// internal/service/mfa.go

type RecoveryCode struct {
	Code     string    // 明文（仅在生成时返回）
	CodeHash string    // bcrypt 哈希
	UsedAt   *time.Time
}

func (s *MFAService) GenerateRecoveryCodes(ctx context.Context, userID string, count int) ([]string, error) {
	// 1. 生成 8 个恢复码（每个 16 字符）
	// 2. 使用 bcrypt 哈希存储
	// 3. 返回明文恢复码（用户需保存）
	// 4. 删除旧的未使用恢复码
}

func (s *MFAService) VerifyRecoveryCode(ctx context.Context, userID, code string) error {
	// 1. 查询用户的未使用恢复码
	// 2. 使用 bcrypt.CompareHashAndPassword 验证
	// 3. 标记为已使用
	// 4. 记录审计日志
}

func (s *MFAService) GetRecoveryCodeStatus(ctx context.Context, userID string) (*RecoveryCodeStatus, error) {
	// 返回剩余恢复码数量
}
```

**API 端点**:
```
POST   /api/v1/mfa/recovery-codes/generate  # 生成恢复码
POST   /api/v1/mfa/recovery-codes/verify    # 验证恢复码
GET    /api/v1/mfa/recovery-codes/status    # 查询状态
```

**预计工作量**: 6-8 小时

---

#### 问题 6: 社交登录提供商硬编码

**当前问题**: 
- 新增提供商需修改核心代码
- 违反开闭原则
- 测试困难

**改进方案**:

**插件式架构**:
```go
// internal/service/social/provider.go

type Provider interface {
	Name() string
	GetAuthURL(redirectURI, state string) string
	ExchangeToken(code, redirectURI string) (string, error)
	GetUserInfo(accessToken string) (*UserInfo, error)
}

type UserInfo struct {
	Email         string
	Name          string
	AvatarURL     string
	EmailVerified bool
}

// internal/service/social/google.go
type GoogleProvider struct {
	clientID     string
	clientSecret string
	httpClient   HTTPClient
}

func NewGoogleProvider(clientID, clientSecret string, httpClient HTTPClient) *GoogleProvider {
	return &GoogleProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   httpClient,
	}
}

func (p *GoogleProvider) Name() string {
	return "google"
}

func (p *GoogleProvider) GetAuthURL(redirectURI, state string) string {
	// 实现 Google OAuth 授权 URL 生成
}

// ... 其他方法实现

// internal/service/social/github.go
type GitHubProvider struct {
	// 类似实现
}

// internal/service/social/service.go
type SocialLoginService struct {
	store     store.Store
	jwtSvc    *crypto.JWTService
	tokenSvc  *TokenService
	providers map[string]Provider  // 改为接口
	stateCache sync.Map
	stopChan  chan struct{}
}

func (s *SocialLoginService) RegisterProvider(provider Provider) {
	s.providers[provider.Name()] = provider
}
```

**使用示例**:
```go
// cmd/server/main.go
socialSvc := social.NewSocialLoginService(store, jwtSvc)

if googleClientID != "" {
	googleProvider := social.NewGoogleProvider(googleClientID, googleClientSecret, http.DefaultClient)
	socialSvc.RegisterProvider(googleProvider)
}

if githubClientID != "" {
	githubProvider := social.NewGitHubProvider(githubClientID, githubClientSecret, http.DefaultClient)
	socialSvc.RegisterProvider(githubProvider)
}
```

**预计工作量**: 6 小时

---

### 2.3 低优先级改进（P2 - 可选完成）

#### 问题 7: 内存缓存无大小限制

**改进方案**: 实现 LRU 淘汰策略

```go
// internal/cache/lru.go

type LRUCache struct {
	mu       sync.RWMutex
	capacity int
	cache    map[string]*lruNode
	head     *lruNode
	tail     *lruNode
}

type lruNode struct {
	key       string
	value     []byte
	expiresAt time.Time
	prev      *lruNode
	next      *lruNode
}

func NewLRUCache(capacity int) *LRUCache {
	// 实现 LRU 缓存
}
```

**预计工作量**: 4 小时

---

#### 问题 8: 分布式限流缺失

**改进方案**: 使用 Redis 实现分布式限流

```go
// internal/middleware/ratelimit_redis.go

type RedisRateLimiter struct {
	client *redis.Client
	limit  int
	window time.Duration
}

func (rl *RedisRateLimiter) Allow(clientIP string) bool {
	key := "ratelimit:" + clientIP
	
	// 使用 Redis INCR + EXPIRE 实现
	pipe := rl.client.Pipeline()
	incr := pipe.Incr(context.Background(), key)
	pipe.Expire(context.Background(), key, rl.window)
	_, err := pipe.Exec(context.Background())
	
	if err != nil {
		return true  // 降级：Redis 失败时允许请求
	}
	
	return incr.Val() <= int64(rl.limit)
}
```

**预计工作量**: 3 小时

---

## 三、实施时间表

### 第一阶段（Week 1）- 高优先级

| 任务 | 预计工作量 | 负责人 | 状态 |
|------|-----------|--------|------|
| 拆分 `postgres.go` | 4-6h | TBD | 待开始 |
| 修复审计日志 Close | 2-3h | TBD | 待开始 |
| 修复 Redis DeletePattern | 2h | TBD | 待开始 |

**总计**: 8-11 小时

### 第二阶段（Week 2）- 中优先级

| 任务 | 预计工作量 | 负责人 | 状态 |
|------|-----------|--------|------|
| 邮件模板外部化 | 4h | TBD | 待开始 |
| 实现 TOTP 恢复码 | 6-8h | TBD | 待开始 |
| 重构社交登录提供商 | 6h | TBD | 待开始 |

**总计**: 16-18 小时

### 第三阶段（Week 3-4）- 低优先级（可选）

| 任务 | 预计工作量 | 负责人 | 状态 |
|------|-----------|--------|------|
| 实现 LRU 缓存 | 4h | TBD | 待开始 |
| 实现分布式限流 | 3h | TBD | 待开始 |

**总计**: 7 小时

---

## 四、测试策略

### 4.1 单元测试

每个改进必须包含：
- 正常流程测试
- 边界条件测试
- 错误处理测试
- 并发安全测试（如适用）

### 4.2 集成测试

- 审计日志优雅关闭测试
- Redis SCAN 性能测试
- 社交登录提供商集成测试

### 4.3 E2E 测试

- TOTP 恢复码完整流程测试
- 邮件模板多语言测试

### 4.4 性能测试

- Redis DeletePattern 性能对比
- LRU 缓存性能测试
- 分布式限流压力测试

---

## 五、回滚计划

每个改进必须：
1. 使用 Git 分支开发
2. 通过 Code Review
3. 在测试环境验证
4. 准备回滚脚本（如涉及数据库迁移）

---

## 六、成功指标

| 指标 | 当前值 | 目标值 |
|------|--------|--------|
| 最大文件行数 | 1126 | < 400 |
| 审计日志丢失率 | 未知 | 0% |
| Redis 阻塞时间 | 未测量 | < 10ms |
| 代码覆盖率 | 75% | > 80% |
| 用户账户恢复成功率 | 0% | > 95% |

---

## 七、风险评估

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| 拆分 postgres.go 引入 bug | 高 | 中 | 完整测试套件 + Code Review |
| 审计日志性能下降 | 中 | 低 | 性能测试 + 监控 |
| Redis SCAN 性能不如预期 | 低 | 低 | 性能对比测试 |
| 恢复码被暴力破解 | 高 | 低 | bcrypt + 限流 + 审计 |

---

## 八、后续优化建议

1. **统一错误码体系**: 合并 Handler 和 Service 层错误码
2. **引入 OpenTelemetry**: 分布式追踪
3. **迁移到标准 Prometheus Client**: 替换自定义 Metrics
4. **实现 API 版本控制**: 支持 v1, v2 并存
5. **添加 GraphQL 支持**: 提供更灵活的 API

---

## 附录

### A. 相关文档

- [CODE_ANALYSIS_REPORT.md](./CODE_ANALYSIS_REPORT.md)
- [AGENTS.md](../AGENTS.md)
- [TESTING.md](../TESTING.md)

### B. 参考资料

- [Redis SCAN 命令文档](https://redis.io/commands/scan/)
- [RFC 6238 - TOTP](https://tools.ietf.org/html/rfc6238)
- [OWASP 认证备忘单](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)

---

**文档版本**: 1.0  
**最后更新**: 2026-04-03  
**审核状态**: 待审核
