# SSO 安全修复执行计划（AI 可执行版）

> 来源：`docs/reports/SECURITY_REVIEW_2026-07-21.md`（审查基线 commit `c39a54b`）
> 创建日期：2026-07-21
> 使用方式：每个任务（T 编号）自包含，可按顺序独立领取执行。执行任务前必读第 1 节「全局执行规则」；完成后在第 2 节总览表中勾选状态。

---

## 1. 全局执行规则

### 1.1 环境约定（Windows 本机）

```bash
# 所有命令在 Git Bash 中执行，工作目录 E:\DEV\SSO
export PATH="/c/Program Files/Go/bin:$HOME/go/bin:$PATH"

# 本机无 make / gcc，用等效命令替代：
#   make test          → go test -count=1 ./...
#   make lint          → golangci-lint run --timeout 5m ./...
#   无法使用 -race（无 gcc/CGO），竞态验证由 CI 完成

# 集成测试（需要真实 DB/Redis，连接信息在 .env.test，禁止读取/展示/提交该文件内容）：
set -a && source .env.test && set +a
```

### 1.2 标准验证命令（后文以 V1-V6 引用）

| 编号 | 命令 | 用途 |
|------|------|------|
| V1 | `go build ./... && go vet ./...` | 编译与静态检查 |
| V2 | `go test -count=1 ./internal/...` | 单元测试全量 |
| V3 | `set -a && source .env.test && set +a && go test -count=1 -tags=integration ./internal/store/...` | 集成测试 |
| V4 | `gofmt -l . \| grep -v '^$' ; test -z "$(gofmt -l .)"` | 格式检查（应无输出） |
| V5 | `golangci-lint run --timeout 5m ./...` | lint 全量（须 0 issue） |
| V6 | `gosec -exclude G118,G201,G202,G706,G710 ./... && govulncheck ./...` | 安全扫描 |

### 1.3 Git 与提交规范

- 每个任务一个分支：`fix/sec-T<编号>-<简述>`，从最新 `main` 切出
- 提交信息遵循仓库 conventional commit 风格（中文描述），如 `fix(security): T1 tokens 表去除明文存储`
- 推送后必须确认 CI 全绿（六个任务）方可关闭任务：

```bash
curl -s "https://api.github.com/repos/Eninte/sso/actions/runs?per_page=1" | \
  python -c "import json,sys; r=json.load(sys.stdin)['workflow_runs'][0]; print(r['status'], r['conclusion'])"
```

### 1.4 每个任务的 Definition of Done（DoD）

1. V1、V2、V4、V5 全部通过；涉及 `internal/store/` 的任务还需 V3 通过
2. 新增/修改代码有对应测试，整体覆盖率不跌破 80%（`go test -coverprofile=coverage.out $(go list ./internal/... | grep -v '/store/mock' | grep -v '/internal/app$' | grep -v '/internal/testing/') && go tool cover -func=coverage.out | tail -1`，注意此为单元口径，postgres 包需 -tags=integration 才有覆盖）
3. 遵守 AGENTS.md：Service 层用 `serviceutil`/`auditutil`，Handler 层用 `handlerutil`，禁止自创错误类型，测试不共享全局状态
4. 文档同步：涉及配置项更新 `.env.example` + AGENTS.md §4；涉及路由更新 AGENTS.md §10；`docs/CHANGELOG.md` [Unreleased] 对应小节追加条目
5. CI 六任务全绿

### 1.5 红线

- 禁止读取、展示、提交 `.env.test`；禁止在代码/日志/文档中硬编码任何凭据
- 禁止修改既有迁移文件（migrations/00X_*）；Schema 变更一律新建迁移
- 禁止为通过测试而删除断言或使用 `t.Skip()`

---

## 2. 任务总览

| ID | 级别 | 任务 | 阶段 | 依赖 | 状态 |
|----|------|------|------|------|------|
| T1 | High | tokens 表去除明文存储（H1） | 一 | 无 | ☐ |
| T2 | High | reset/verification 令牌哈希存储（H2，含 L14 顺手修） | 一 | 无 | ☐ |
| T3 | Medium | setup.go 错误脱敏 + 邮件日志邮箱脱敏（M5+L8） | 一 | 无 | ☐ |
| T4 | Low | MFA_RECOVERY_HMAC_KEY 强度校验（L2） | 一 | 无 | ☐ |
| T5 | Low | SERVER_ENV 白名单拒绝未知值（L13） | 一 | 无 | ☐ |
| T6 | Low | 升级 golang.org/x/crypto 至 v0.52.0（L15） | 一 | 无 | ☐ |
| T7 | High | JWT 轮换私钥信封加密（H3） | 二 | 无 | ☐ |
| T8 | Medium | CORS credentials 策略收紧（M1） | 二 | 无 | ☐ |
| T9 | Medium | MFA 限流与 TOTP 重放记录 Redis 化（M3+L1） | 二 | 无 | ☐ |
| T10 | Medium | 敏感端点限流 fail-closed（M4） | 二 | 无 | ☐ |
| T11 | Medium | 社交登录 state 会话绑定（M2） | 二 | 无 | ☐ |
| T12 | Medium | CI 供应链固定（M6） | 三 | 无 | ☐ |
| T13 | Low | /api/v1/token 纳入敏感限流（L4） | 三 | T10 | ☐ |
| T14 | Low | 管理员自锁/末位防护 + 角色变更失效（L5+L6） | 三 | 无 | ☐ |
| T15 | Low | 验证码账号维度计数（L7） | 三 | 无 | ☐ |
| T16 | Low | kid 从密钥内容派生（L3，RFC 7638） | 三 | 无 | ☐ |
| T17 | Low | Docker/Compose 凭据卫生（L9+L10） | 三 | 无 | ☐ |
| T18 | Low | 注册邮箱限制为纯 addr-spec（L11） | 三 | 无 | ☐ |
| T19 | Info | OIDC Discovery 与实际路由对齐（I1） | 三 | 无 | ☐ |

可选加固（另立项评估，不在本计划内）：TOTP secret 落库加密（L12，可在 T7 引入 KEK 后低成本实现）、JWT audience（I3）、consent_token kid 统一（I2）。

---

## 3. 阶段一：高性价比修复

### T1 tokens 表去除明文存储（H1）

**目标**：`tokens` 表停止写入并不再读取 `access_token`/`refresh_token` 明文列，仅保留 hash 存储与 hash 查询。

**前置确认**（执行时先跑，行号以现状为准）：

```bash
grep -n 'getTokenByField\|"refresh_token"\|"access_token"' internal/store/postgres/token.go
grep -rn 'fallback\|回退' internal/store/postgres/token.go internal/store/postgres/token_test.go
grep -rn 'func.*[Hh]ashToken' internal/store/postgres/ internal/crypto/   # 定位现有 hash 帮助函数
```

**改动清单**：

1. `internal/store/postgres/token.go`
   - `StoreToken`（约 :102-134）：INSERT 中 `access_token`/`refresh_token` 两列改写 `NULL`（或从列清单移除，取决于下方迁移决策）；删除"明文保留用于调试和审计"注释，替换为"仅存 hash，明文不落库"
   - `GetTokenByRefreshToken`（约 :140-150）：删除明文回退查询，hash 未命中直接返回 `store.ErrNotFound`
   - `GetTokenByAccessToken`（约 :156-166）：同上
   - `RotateRefreshToken`（约 :260-280）：删除 `WHERE refresh_token = $1` 明文 fallback 分支，hash 未命中即视为已轮换/不存在
   - `RevokeToken` 类方法如存在明文 WHERE 回退，一并删除（前置确认的 grep 会列出）
2. 新建迁移（**禁止改动旧迁移**）：

```bash
# 本机无 make，直接手写两个文件（下一个编号以 ls migrations/ 实际为准，预期 019）：
# migrations/019_clear_token_plaintext.up.sql
# migrations/019_clear_token_plaintext.down.sql
```

```sql
-- 019_clear_token_plaintext.up.sql
-- 清除 tokens 表明文列数据。前提：代码已切换为仅存/只查 hash。
-- 注意：迁移 018 之前签发的 refresh token（hash 列为 NULL）将立即失效，用户需重新登录。
UPDATE tokens SET access_token = NULL, refresh_token = NULL;
```

```sql
-- 019_clear_token_plaintext.down.sql
-- 不可逆：明文已被清除，无法恢复。保留空迁移占位。
SELECT 1;
```

   - 若 `access_token`/`refresh_token` 列定义为 `NOT NULL`（先 `grep -A20 'CREATE TABLE.*tokens' migrations/00*.up.sql | head -30` 确认），迁移须先 `ALTER TABLE tokens ALTER COLUMN access_token DROP NOT NULL, ALTER COLUMN refresh_token DROP NOT NULL;` 再 UPDATE。
3. `internal/store/mock/mock.go`：mock 为进程内存，无泄露面，**保持现状不改**；但若 mock 断言依赖明文查询行为，同步适配。
4. `internal/model/`：`model.Token` 的 `AccessToken`/`RefreshToken` 字段**保留**——签发后须向客户端返回明文，只是不落库。

**测试改动**：

- 删除/改写为 NULL hash 回退路径写的测试（commit `bf8ab5e` 引入，前置 grep 会定位到 `token_test.go` 中相关用例）
- 新增集成测试（`token_test.go`，`//go:build integration`）：`StoreToken` 后直连 DB 查询该行，断言 `access_token IS NULL AND refresh_token IS NULL` 且 hash 列非空、hash 查询可用
- 新增/保留：`GetTokenByRefreshToken` 对不存在 hash 返回 `ErrNotFound`

**验证**：V1、V2、V3、V4、V5

**风险与回滚**：迁移执行后，018 之前签发的长寿 refresh token 全部失效（强制重新登录，可接受）；回滚 = 代码回滚（数据无法恢复，down 迁移为空）。CHANGELOG 必须在 Fixed 中注明该行为变更。

**提交**：`fix(security): T1 tokens 表去除明文存储，仅保留 hash`

---

### T2 reset/verification 令牌哈希存储（H2，含 L14）

**目标**：`reset_tokens.token` 与 `verification_tokens.token` 改存 SHA-256 hex，校验时 hash 后比对；顺带修复 `storeToken` 忽略 DELETE 错误（L14）。

**前置确认**：

```bash
grep -n 'storeToken\|GetResetToken\|GetVerificationToken' internal/store/postgres/verification.go
grep -rn 'GetResetToken\|VerifyResetToken\|GetVerificationToken' internal/service/*.go | grep -v _test
```

**改动清单**：

1. 复用 T1 定位到的 hash 帮助函数（SHA-256 hex）。若该函数为 postgres 包私有，则在 `internal/crypto/` 新增导出函数（如 `HashTokenSHA256(token string) string`）并让 postgres/token.go 与 verification.go 共用，避免两份实现。
2. `internal/store/postgres/verification.go`
   - `storeToken`（约 :36-49）：入库前对 token 计算 hash 后存储；修复 `_, _ = db.Exec(DELETE ...)`（L14）为检查并包装返回错误
   - `GetVerificationToken`/`GetResetToken` 返回的 `Token` 字段此时为 hash 值——**在函数文档注释中明确说明该字段为哈希值**
3. `internal/service/user.go`（及 email 验证调用方）
   - 生成令牌后：明文只用于拼邮件链接；调用 `StoreResetToken/StoreVerificationToken` 前不做改动（store 内部哈希）
   - 校验点（`ResetPasswordWithAudit` 约 :274、`VerifyEmail` 调用方）：比对前对输入 token 计算 hash，与 store 返回的 hash 比对；保持现有 ConstantTimeCompare（如有）
4. 新建迁移 `020_clear_verification_reset_tokens.up.sql`：

```sql
-- 在飞令牌全部失效，用户重新发起验证/重置即可，影响可接受
DELETE FROM verification_tokens;
DELETE FROM reset_tokens;
```

（down 同样为空占位并注释不可逆。）

**测试改动**：

- service 层既有重置/验证流程测试适配（mock 同步：mock 的 storeToken 路径是否哈希与真实实现保持一致——建议在 mock 中也走同一 hash 函数，保证 service 测试真实）
- 新增集成测试：DB 中断言两表 token 列不含明文（值为 64 位 hex）

**验证**：V1、V2、V3、V4、V5

**提交**：`fix(security): T2 重置/验证令牌改为 SHA-256 哈希存储`

---

### T3 setup.go 错误脱敏 + 邮件日志邮箱脱敏（M5+L8）

**目标**：消除全项目仅剩的两处脱敏遗漏。

**改动清单**：

1. `internal/handler/setup.go:383-386`
   - `slog.Error("setup test-db 数据库连接失败", "error", err, ...)` → 错误值先经 `logging.SanitizeDBURL(err.Error())` 或直接只记录脱敏后的 DSN；参考同文件/项目内其他 `SanitizeDBURL` 用法（`grep -rn 'SanitizeDBURL' internal/ | head -5`）
   - `apperrors.ErrInternal.WithDetails(err.Error())` → 移除 details，返回通用消息（对照 AGENTS.md §8.4：禁止响应暴露内部错误详情）
2. `internal/service/email.go:138,142`：日志字段 `"to", to` → `"to", logging.SanitizeEmail(to)`

**测试改动**：

- `grep -rn 'test-db\|WithDetails' internal/handler/setup_*_test.go` 定位断言错误详情的用例并改为断言通用消息
- 新增用例：连接失败响应体不包含原始 DSN/驱动错误串

**验证**：V1、V2、V4、V5

**提交**：`fix(security): T3 setup 错误与邮件日志脱敏补齐`

---

### T4 MFA_RECOVERY_HMAC_KEY 强度校验（L2）

**改动清单**：

1. `internal/config/config.go`（现有生产校验 :489-501 附近）：`SERVER_ENV=production` 时增加 `len(MFA_RECOVERY_HMAC_KEY) < 32` 拒绝启动；非生产环境不足 32 字节时 `slog.Warn` 提示
2. `.env.example`：该项注释补充生成方式 `openssl rand -hex 32`
3. AGENTS.md §4 配置表该行的生产要求同步更新

**测试**：`internal/config/` 新增用例：生产 + 弱密钥（如 `123456`）→ `Load()` 返回错误；生产 + 32 字节 → 通过

**验证**：V1、V2、V4、V5

**提交**：`fix(security): T4 生产环境强制 MFA_RECOVERY_HMAC_KEY 最小长度`

---

### T5 SERVER_ENV 白名单（L13）

**改动清单**：`internal/config/config.go:574-576` 附近，未知 `SERVER_ENV` 由警告改为返回错误拒绝启动；合法值 `development|test|setup|production`（先 grep 确认现有取值集合）。新增大小写混用用例（`Production`）断言报错。

**验证**：V1、V2、V4、V5

**提交**：`fix(security): T5 SERVER_ENV 非白名单值拒绝启动`

---

### T6 升级 golang.org/x/crypto（L15）

```bash
go get golang.org/x/crypto@v0.52.0 && go mod tidy
go build ./... && go test -count=1 ./...
govulncheck ./...   # 确认 x/crypto 相关 15 条不再出现
```

注意 `go.mod` 的 Go 版本兼容性（v0.52.0 若要求更高 Go 版本，执行 `go get` 时会有提示；本机 go1.26.5 预期无问题）。CI 的 security job 会复验。

**提交**：`chore(deps): T6 升级 golang.org/x/crypto 至 v0.52.0`

---

## 4. 阶段二：需要设计决策的修复

### T7 JWT 轮换私钥信封加密（H3）

**目标**：`key_versions.private_key` 落库前用 KEK 做 AES-256-GCM 加密，读取时解密；兼容存量明文行。

**设计决策（执行前与项目所有者确认）**：

- KEK 来源：新增环境变量 `JWT_KEY_ENCRYPTION_KEY`（64 位 hex = 32 字节）。生产启用 DB 密钥存储（keyrotation）时必填，否则拒绝启动
- 密文格式：`v1:gcm:<base64(nonce|ciphertext)>`；读取时按前缀分派，无前缀按明文 PEM 处理（过渡兼容）
- 旧明文行迁移策略（二选一，推荐 B）：
  - A. 迁移脚本一次性重写为密文（需要 KEK 注入迁移环境，复杂）
  - B. 运行时懒加密：读取到明文行成功加载后回写密文（代码内完成，无需迁移）

**改动清单**：

1. `internal/crypto/` 新增 `keycipher.go`：AES-256-GCM 加/解密、KEK 解析与强度校验（必须 32 字节）、格式前缀分派；错误消息英文
2. `internal/crypto/jwt.go` `CreateKeyVersion`（:445-458）与 `LoadKeysFromStore`（:119-156）：写入前加密、读取时解密（含明文回退 + 懒加密回写钩子）
3. `internal/config/config.go`：新配置项 + 生产校验；`.env.example` + AGENTS.md §4 同步
4. `internal/service/keyrotation.go`：StoreKey 路径接入加密（视分层决定加密在 crypto 还是 service，建议 crypto 层，store 无感知）

**测试**：加解密往返表驱动用例；错误 KEK 长度拒绝；明文行兼容读取；篡改密文解密失败；`keyrotation_test.go` 集成路径适配

**验证**：V1、V2、V3、V4、V5、V6

**提交**：`fix(security): T7 JWT 轮换私钥 AES-GCM 信封加密存储`

---

### T8 CORS credentials 策略收紧（M1）

**改动清单**：

1. `internal/middleware/cors.go`
   - 仅当 Origin **精确匹配**允许列表中的具体 origin 时，才发送 `Access-Control-Allow-Credentials: true`
   - 命中通配（`*` 或 `*.suffix` 后缀匹配）时：回显允许但不发送 credentials 头
   - 响应增加 `Vary: Origin`
   - `Validate()`：生产环境下对「通配形式 + 需要 credentials」的组合告警（或拒绝，与所有者确认）
2. `internal/middleware/cors_security_test.go` 扩充：精确匹配有 credentials、通配无 credentials、Vary 头存在、生产校验新规则

**验证**：V1、V2、V4、V5

**提交**：`fix(security): T8 CORS 通配场景不再发送 Allow-Credentials`

---

### T9 MFA 限流与 TOTP 重放记录 Redis 化（M3+L1）

**目标**：恢复码失败限流与 TOTP 已用码记录迁入 Redis，多副本一致；顺带解决 T1 报告 L1（每 timeStep 独立键，旧码不可二次使用）。

**改动清单**：

1. 参考 `internal/service/login_ratelimit.go` 的 Redis/内存双模结构：
   - 恢复码限流：键 `mfa:recovery:attempts:{userID}`，`INCR` + 达到 5 次后 `EXPIRE 15m`；读取失败次数决定是否拒绝
   - TOTP 重放：键 `mfa:totp:used:{userID}:{timeStep}`，`SET NX EX 90`，设置失败即视为重放（替代 `mfa.go:181-211` 的单条 map）
2. Redis 不可用时的行为与 T10 对齐（本任务先保持内存降级，T10 统一决策 fail-open/closed）
3. `internal/service/mfa.go:127-171`、`181-211` 的内存 map 删除或降级为无 Redis 时的回退实现

**测试**：miniredis 用例（参考 `internal/cache/redis_miniredis_test.go` 模式）：限流计数、锁定期拒绝、TTL 过期恢复、TOTP 同 timeStep 重放拒绝、不同 timeStep 放行

**验证**：V1、V2、V3、V4、V5

**提交**：`fix(security): T9 MFA 限流与 TOTP 重放记录迁移至 Redis`

---

### T10 敏感端点限流 fail-closed（M4）

**设计决策（执行前与所有者确认其一）**：

- 方案 A：敏感端点 Redis 故障时返回 503（fail-closed，最安全，Redis 故障期登录不可用）
- 方案 B：降级为进程内内存限流（可用性与安全折中，多副本下限额放宽但非无限）
- 推荐 **B**，并在两条路径都加 Error 级日志 + metrics（现行静默放行必须消除）

**改动清单**：

1. `internal/middleware/ratelimit_distributed.go`：新增 `WithFailMode(mode)` 选项；`internal/app/router.go` 敏感子路由（登录/注册/忘记密码/重置密码/MFA）使用降级模式
2. `internal/service/login_ratelimit.go:62-65`、`email_ratelimit.go:56-59`：fail-open 分支补 Error 日志 + 指标
3. 测试：mock/cache 错误注入，断言降级行为与日志

**验证**：V1、V2、V4、V5

**提交**：`fix(security): T10 敏感端点限流 Redis 故障降级策略`

---

### T11 社交登录 state 会话绑定（M2）

**设计决策**：纯 API 客户端（无 cookie）如何集成需确认。推荐实现：

1. `internal/handler/social.go:43` 起：服务端始终生成 state（忽略或混合客户端传入值），同时下发 `HttpOnly; SameSite=Lax; Secure` 的 state 指纹 cookie（state 的 HMAC）
2. 回调：校验 cookie 指纹与 state 匹配 + 现有 GETDEL/provider/TTL 校验；无 cookie 的客户端可通过新配置项 `SOCIAL_LOGIN_STATE_COOKIE_BINDING=false` 关闭（默认开启，文档说明）
3. `.env.example` + AGENTS.md §4 同步新配置项

**测试**：绑定开启时无 cookie 回调拒绝、指纹不匹配拒绝、正常流程通过；绑定关闭时兼容旧行为

**验证**：V1、V2、V4、V5

**提交**：`fix(security): T11 社交登录 state 绑定发起方会话`

---

## 5. 阶段三：纵深加固与卫生

### T12 CI 供应链固定（M6）

- `.github/workflows/ci.yml`：`actions/checkout@v5` 等固定到 commit SHA（`grep -n 'uses:' .github/workflows/ci.yml` 逐一替换）；`gotestsum@latest`/`migrate@latest` 固定具体版本
- `docker/Dockerfile:25` 的 migrate 安装同样固定
- 新增 `.github/dependabot.yml`（github-actions + gomod 周更）
- 验证：推送后 CI 全绿即通过（V1-V5 本地不适用，DoD 第 5 条为准）

**提交**：`ci(security): T12 固定 actions SHA 与工具链版本，引入 Dependabot`

### T13 /api/v1/token 纳入敏感限流（L4，依赖 T10）

- `internal/app/router.go:164` 附近：将 `/api/v1/token`（至少 `authorization_code` grant）移入 sensitive 子路由；确认 OAuth 压测场景（loadtest s6/s7）限流配额可配置，避免压测误伤
- 测试：集成或 E2E 层验证超配额返回 429

**提交**：`fix(security): T13 token 端点纳入敏感限流`

### T14 管理员防护（L5+L6）

- `internal/service/admin.go:167-187`：Disable/Delete 前校验 ① 目标不是操作者本人 ② 目标不是最后一个 active admin（COUNT 查询）；违反返回预定义 apperrors 错误
- `internal/handler/admin.go:181-184`：`RevokeAllUserTokens` 失败时返回错误而非仅 Warn（或加入补偿重试，与所有者确认取舍）
- L6（角色降级窗口）：在 `docs/SECURITY.md` 明确声明「角色变更在 access token 剩余有效期（≤15 分钟）后生效」即可，不改代码；若所有者要求即时生效，复用 userinfo 缓存做 admin 路由角色回查

**提交**：`fix(security): T14 管理员自锁/末位防护与禁用撤销失败处理`

### T15 验证码账号维度计数（L7）

- `internal/handler/helpers.go:50-77` `ShouldRequireCaptcha`：增加以 email 为键的失败计数（Redis，与 IP 维度并行，任一触发即要求验证码）；计数在登录失败处递增、成功处清零
- 测试：同一 email 多 IP 触发验证码用例

**提交**：`fix(security): T15 验证码增加账号维度失败计数`

### T16 kid 从密钥内容派生（L3）

- `internal/crypto/keyloader.go:269-299`：`GenerateKeyID()` 改为 RFC 7638 JWK thumbprint（对公钥模数/指数规范序列化后 SHA-256，base64url 取前 16 字符）；保证跨重启稳定
- 测试：同一密钥两次加载 kid 一致；不同密钥 kid 不同

**提交**：`fix(security): T16 kid 改为 JWK thumbprint 派生，重启不再强制全员登出`

### T17 Docker/Compose 凭据卫生（L9+L10）

- `docker/entrypoint.sh:36,42`：migrate 改读配置文件（`migrate -database` 改为从 `MIGRATE_CONFIG` 文件读取或 stdin 构造），避免密码出现在进程参数
- `docker-compose.yml:30`：`${DB_PASSWORD}` → `${DB_PASSWORD:?DB_PASSWORD is required}`；redis healthcheck 密码改走 `redis-cli --no-auth-warning -a "$REDIS_PASSWORD"` 的环境变量传递（现状已可见，至少加注释说明并评估 socket 方案）

**提交**：`fix(security): T17 消除容器内进程参数/环境中的凭据暴露`

### T18 注册邮箱限制纯 addr-spec（L11）

- `internal/validator/validator.go:42`：`mail.ParseAddress` 通过后追加校验 `addr.Name == ""` 且 `addr.Address == 原输入`（拒绝 display-name 形式与多余空白）
- 测试：`"Name" <a@b.c>`、前后空格、`a@b.c` 正常值三类用例

**提交**：`fix(security): T18 注册邮箱仅接受纯 addr-spec`

### T19 OIDC Discovery 对齐（I1）

- `internal/handler/wellknown.go:50`：`authorization_endpoint` 改为 `{base}/api/v1/authorize`；核对 `token_endpoint_auth_methods_supported` 声明（当前 token 端点为 JSON body，非 RFC 6749 惯例的 form-urlencoded）
- **设计决策**：是否在 token 端点追加 form-urlencoded 支持以兼容标准 OIDC 客户端（改动较大，可拆分为独立任务并评估 sdks/ 六语言 SDK 的兼容性，见 AGENTS.md §15）
- 测试：`wellknown_test.go` 断言更新

**提交**：`fix(security): T19 OIDC Discovery 端点声明与实际路由对齐`

---

## 6. 风险登记册

| 风险 | 涉及任务 | 影响 | 缓解 |
|------|---------|------|------|
| 迁移执行后在飞 refresh token 失效 | T1 | 用户强制重新登录一次 | CHANGELOG 醒目标注；选低峰执行迁移 |
| 在飞重置/验证令牌失效 | T2 | 用户重新发起流程 | 同上 |
| 私钥加密后 KEK 丢失 | T7 | DB 中私钥永久不可用 | 文档强制 KEK 备份；支持明文行回退读取（过渡期内） |
| 限流 fail-closed 影响可用性 | T10 | Redis 故障时登录不可用 | 采用降级方案 B |
| state cookie 绑定破坏既有前端集成 | T11 | 社交登录流程报错 | 配置开关 + 灰度 |
| token 端点敏感限流影响压测/合法客户端 | T13 | 429 增多 | 配额可配置；压测前调整 RATE_LIMIT 配置 |
| 邮箱校验收紧拒绝存量合法输入 | T18 | 个别注册失败 | 仅影响新注册；错误消息明确 |

## 7. 全部完成后的收尾

1. `docs/CHANGELOG.md` [Unreleased] 汇总所有条目（各任务已分别追加，收尾时校对）
2. 更新 `docs/reports/SECURITY_REVIEW_2026-07-21.md`：已修复项标注「✅ 已修复（commit xxx）」
3. 全量复扫：V6 + `go test -count=1 ./...` + 集成测试 + E2E 流程（prepare → test → cleanup）
4. `docs/SECURITY.md` 补充新机制说明（信封加密、限流降级策略等）
5. 视改动面评估是否发布新版本（docs/CHANGELOG.md 从 [Unreleased] 落版）
