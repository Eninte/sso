# SSO 服务代码质量分析 - 执行摘要

> **分析日期**: 2026-03-31 22:43:17  
> **分析耗时**: 231秒  
> **分析工具**: golangci-lint, gocyclo, dupl, gosec, govulncheck, go test

---

## 📊 关键指标

### 测试覆盖率
```
total:									(statements)				55.7%
```

### Lint问题统计
```
0
0 个问题
```

### 安全漏洞
```
18 个潜在问题
```

### 复杂度热点
```
21 service (*AuthService).LoginWithAudit internal/service/auth.go:224:1
21 config (*Config).validate internal/config/config.go:191:1
14 validator ValidatePassword internal/validator/validator.go:50:1
12 service (*UserService).ResetPasswordWithAudit internal/service/user.go:190:1
12 service sendEmailSSL internal/service/email.go:119:1
12 service (*OAuthService).CreateAuthorizationCode internal/service/oauth.go:111:1
12 service (*KeyRotationService).RotateKey internal/service/keyrotation.go:36:1
10 service (*SocialLoginService).HandleCallback internal/service/social.go:228:1
10 handler (*TokenHandler).handleAuthorizationCode internal/handler/token.go:83:1
10 handler (*AdminHandler).HandleListUsers internal/handler/admin.go:50:1
```

---

## 📁 详细报告

- [静态分析](static/) - 代码规范、复杂度、重复度
- [安全审计](security/) - 漏洞扫描、安全检查
- [测试质量](testing/) - 覆盖率、竞态检测
- [性能剖析](performance/) - 基准测试、CPU/内存分析
- [架构分析](architecture/) - 分层验证、依赖检查
- [文档分析](documentation/) - 文档完整性

---

## 🎯 下一步行动

1. 查看各子报告了解详细问题
2. 按优先级修复发现的问题
3. 定期重新运行分析验证改进

---

**生成命令**: `bash scripts/run-full-analysis.sh`
