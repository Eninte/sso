#!/bin/bash
# 生成详细的代码质量分析报告

set -e

REPORTS_DIR="./reports"
OUTPUT_FILE="$REPORTS_DIR/DETAILED_ANALYSIS_REPORT.md"

# 确保报告目录存在
mkdir -p "$REPORTS_DIR"

# 获取当前日期
REPORT_DATE=$(date '+%Y-%m-%d %H:%M:%S')

# 开始生成报告
cat > "$OUTPUT_FILE" << 'EOF'
# SSO 服务 - 详细代码质量分析报告

> **生成时间**: REPORT_DATE_PLACEHOLDER  
> **项目**: SSO 单点登录服务  
> **Go版本**: Go 1.26+

---

## 📋 目录

1. [执行摘要](#执行摘要)
2. [静态代码分析](#静态代码分析)
3. [安全审计](#安全审计)
4. [测试质量分析](#测试质量分析)
5. [性能分析](#性能分析)
6. [架构分析](#架构分析)
7. [改进建议](#改进建议)
8. [附录](#附录)

---

## 📊 执行摘要

### 总体评分

| 维度 | 评分 | 状态 | 说明 |
|------|------|------|------|
| 代码质量 | -/10 | ⏳ | 待评估 |
| 安全性 | -/10 | ⏳ | 待评估 |
| 测试覆盖 | -/10 | ⏳ | 待评估 |
| 性能 | -/10 | ⏳ | 待评估 |
| 架构设计 | -/10 | ⏳ | 待评估 |
| 文档完整性 | -/10 | ⏳ | 待评估 |

### 关键发现

#### 🔴 高优先级问题
- 待分析...

#### 🟡 中优先级问题
- 待分析...

#### 🟢 低优先级问题
- 待分析...

---

## 🔍 静态代码分析

### 1.1 Lint检查结果

EOF

# 添加lint结果
if [ -f "$REPORTS_DIR/static/lint-full.txt" ]; then
    echo "#### 问题统计" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    grep -c "^internal/" "$REPORTS_DIR/static/lint-full.txt" 2>/dev/null | awk '{print "总问题数: " $1}' >> "$OUTPUT_FILE" || echo "总问题数: 0" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    echo "" >> "$OUTPUT_FILE"
    
    echo "#### 主要问题类型" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    grep -oP '^\w+:' "$REPORTS_DIR/static/lint-full.txt" 2>/dev/null | sort | uniq -c | sort -rn | head -10 >> "$OUTPUT_FILE" || echo "无问题" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
else
    echo "Lint报告未生成" >> "$OUTPUT_FILE"
fi

cat >> "$OUTPUT_FILE" << 'EOF'

### 1.2 复杂度分析

EOF

# 添加复杂度结果
if [ -f "$REPORTS_DIR/static/complexity-hotspots.txt" ]; then
    echo "#### 复杂度热点（Top 10）" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    head -10 "$REPORTS_DIR/static/complexity-hotspots.txt" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    echo "" >> "$OUTPUT_FILE"
    
    echo "#### 高复杂度函数（>15）" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    awk '$1 > 15' "$REPORTS_DIR/static/complexity-hotspots.txt" | wc -l | awk '{print "高复杂度函数数量: " $1}' >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
else
    echo "复杂度报告未生成" >> "$OUTPUT_FILE"
fi

cat >> "$OUTPUT_FILE" << 'EOF'

### 1.3 代码重复分析

EOF

# 添加重复代码结果
if [ -f "$REPORTS_DIR/static/duplication.txt" ]; then
    echo "#### 重复代码统计" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    grep -c "found" "$REPORTS_DIR/static/duplication.txt" 2>/dev/null | awk '{print "重复代码块数: " $1}' >> "$OUTPUT_FILE" || echo "重复代码块数: 0" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    echo "" >> "$OUTPUT_FILE"
    echo "详细信息请查看: [duplication.html](static/duplication.html)" >> "$OUTPUT_FILE"
else
    echo "重复代码报告未生成" >> "$OUTPUT_FILE"
fi

cat >> "$OUTPUT_FILE" << 'EOF'

---

## 🔒 安全审计

### 2.1 自动化安全扫描

EOF

# 添加gosec结果
if [ -f "$REPORTS_DIR/security/gosec.txt" ]; then
    echo "#### gosec扫描结果" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    grep "Severity:" "$REPORTS_DIR/security/gosec.txt" 2>/dev/null | wc -l | awk '{print "发现问题数: " $1}' >> "$OUTPUT_FILE" || echo "发现问题数: 0" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    echo "" >> "$OUTPUT_FILE"
    
    if grep -q "Severity: HIGH" "$REPORTS_DIR/security/gosec.txt" 2>/dev/null; then
        echo "⚠️ 发现高危问题！" >> "$OUTPUT_FILE"
        echo '```' >> "$OUTPUT_FILE"
        grep -A5 "Severity: HIGH" "$REPORTS_DIR/security/gosec.txt" | head -20 >> "$OUTPUT_FILE"
        echo '```' >> "$OUTPUT_FILE"
    fi
else
    echo "gosec报告未生成" >> "$OUTPUT_FILE"
fi

echo "" >> "$OUTPUT_FILE"

# 添加govulncheck结果
if [ -f "$REPORTS_DIR/security/vulncheck.txt" ]; then
    echo "#### 漏洞检查结果" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    if grep -q "No vulnerabilities found" "$REPORTS_DIR/security/vulncheck.txt" 2>/dev/null; then
        echo "✅ 未发现已知漏洞" >> "$OUTPUT_FILE"
    else
        grep -c "Vulnerability" "$REPORTS_DIR/security/vulncheck.txt" 2>/dev/null | awk '{print "发现漏洞数: " $1}' >> "$OUTPUT_FILE" || echo "状态: 待检查" >> "$OUTPUT_FILE"
    fi
    echo '```' >> "$OUTPUT_FILE"
else
    echo "漏洞检查报告未生成" >> "$OUTPUT_FILE"
fi

cat >> "$OUTPUT_FILE" << 'EOF'

### 2.2 关键安全检查

#### JWT安全
- [ ] 签名算法验证（RS256）
- [ ] Token过期时间配置
- [ ] Refresh Token轮换
- [ ] 并发安全

#### 密码安全
- [ ] bcrypt cost配置（生产>=12）
- [ ] 密码复杂度验证
- [ ] 登录失败锁定
- [ ] 时序攻击防护

#### 注入防护
- [ ] SQL参数化查询
- [ ] 输入验证
- [ ] 输出编码

---

## 🧪 测试质量分析

### 3.1 测试覆盖率

EOF

# 添加覆盖率结果
if [ -f "$REPORTS_DIR/testing/coverage-func.txt" ]; then
    echo "#### 整体覆盖率" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    grep "total:" "$REPORTS_DIR/testing/coverage-func.txt" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    echo "" >> "$OUTPUT_FILE"
    
    echo "#### 各包覆盖率" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    grep "internal/" "$REPORTS_DIR/testing/coverage-func.txt" | awk '{print $1 " " $3}' | sort -t'/' -k3 >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    echo "" >> "$OUTPUT_FILE"
    echo "详细报告: [coverage.html](testing/coverage.html)" >> "$OUTPUT_FILE"
else
    echo "覆盖率报告未生成" >> "$OUTPUT_FILE"
fi

cat >> "$OUTPUT_FILE" << 'EOF'

### 3.2 竞态条件检测

EOF

# 添加竞态检测结果
if [ -f "$REPORTS_DIR/testing/race-detection.txt" ]; then
    echo '```' >> "$OUTPUT_FILE"
    if grep -q "WARNING: DATA RACE" "$REPORTS_DIR/testing/race-detection.txt" 2>/dev/null; then
        echo "⚠️ 发现竞态条件！" >> "$OUTPUT_FILE"
        grep -c "WARNING: DATA RACE" "$REPORTS_DIR/testing/race-detection.txt" | awk '{print "竞态问题数: " $1}' >> "$OUTPUT_FILE"
    else
        echo "✅ 未发现竞态条件" >> "$OUTPUT_FILE"
    fi
    echo '```' >> "$OUTPUT_FILE"
else
    echo "竞态检测报告未生成" >> "$OUTPUT_FILE"
fi

cat >> "$OUTPUT_FILE" << 'EOF'

### 3.3 测试统计

EOF

# 添加测试统计
if [ -f "$REPORTS_DIR/testing/test-statistics.txt" ]; then
    echo '```' >> "$OUTPUT_FILE"
    cat "$REPORTS_DIR/testing/test-statistics.txt" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
else
    echo "测试统计未生成" >> "$OUTPUT_FILE"
fi

cat >> "$OUTPUT_FILE" << 'EOF'

---

## ⚡ 性能分析

### 4.1 基准测试结果

EOF

# 添加基准测试结果
if [ -f "$REPORTS_DIR/performance/benchmark.txt" ]; then
    echo "#### 关键路径性能" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
    grep "Benchmark" "$REPORTS_DIR/performance/benchmark.txt" | head -20 >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
else
    echo "基准测试报告未生成" >> "$OUTPUT_FILE"
fi

cat >> "$OUTPUT_FILE" << 'EOF'

### 4.2 CPU性能热点

EOF

# 添加CPU剖析结果
if [ -f "$REPORTS_DIR/performance/cpu-top.txt" ]; then
    echo '```' >> "$OUTPUT_FILE"
    head -20 "$REPORTS_DIR/performance/cpu-top.txt" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
else
    echo "CPU剖析未生成" >> "$OUTPUT_FILE"
fi

cat >> "$OUTPUT_FILE" << 'EOF'

### 4.3 内存分配分析

EOF

# 添加内存剖析结果
if [ -f "$REPORTS_DIR/performance/mem-top.txt" ]; then
    echo '```' >> "$OUTPUT_FILE"
    head -20 "$REPORTS_DIR/performance/mem-top.txt" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
else
    echo "内存剖析未生成" >> "$OUTPUT_FILE"
fi

cat >> "$OUTPUT_FILE" << 'EOF'

---

## 🏗️ 架构分析

### 5.1 分层架构验证

EOF

# 添加架构分析结果
if [ -f "$REPORTS_DIR/architecture/layering-analysis.txt" ]; then
    echo '```' >> "$OUTPUT_FILE"
    cat "$REPORTS_DIR/architecture/layering-analysis.txt" >> "$OUTPUT_FILE"
    echo '```' >> "$OUTPUT_FILE"
else
    echo "架构分析未生成" >> "$OUTPUT_FILE"
fi

cat >> "$OUTPUT_FILE" << 'EOF'

### 5.2 包大小分布

- 检查是否有过大的包（>1000行）
- 验证职责划分是否合理

---

## 💡 改进建议

### 高优先级（立即修复）

1. **安全问题**
   - 修复所有高危安全漏洞
   - 确保JWT使用RS256
   - 验证bcrypt cost配置

2. **代码质量**
   - 重构高复杂度函数（>15）
   - 修复lint错误

### 中优先级（近期改进）

1. **测试覆盖**
   - 提升覆盖率至80%+
   - 修复竞态条件

2. **性能优化**
   - 优化性能热点
   - 减少内存分配

### 低优先级（长期优化）

1. **代码重复**
   - 提取公共函数
   - 重构重复代码

2. **文档完善**
   - 补充API文档
   - 添加函数注释

---

## 📎 附录

### A. 分析工具版本

- golangci-lint: latest
- gocyclo: latest
- dupl: latest
- gosec: latest
- govulncheck: latest

### B. 报告文件清单

```
reports/
├── EXECUTIVE_SUMMARY.md
├── DETAILED_ANALYSIS_REPORT.md (本文件)
├── static/
│   ├── lint-full.txt
│   ├── complexity.txt
│   ├── duplication.html
│   └── dependencies.txt
├── security/
│   ├── gosec.txt
│   └── vulncheck.txt
├── testing/
│   ├── coverage.html
│   ├── coverage-func.txt
│   └── race-detection.txt
├── performance/
│   ├── benchmark.txt
│   ├── cpu-top.txt
│   └── mem-top.txt
└── architecture/
    └── layering-analysis.txt
```

### C. 相关文档

- [项目规范](../../AGENTS.md)
- [测试指南](../../docs/TESTING.md)
- [架构文档](../../docs/ARCHITECTURE.md)
- [现有分析报告](../../docs/reports/code-analysis/)

---

**报告生成**: `bash scripts/generate-detailed-report.sh`  
**完整分析**: `bash scripts/run-full-analysis.sh`
EOF

# 替换日期占位符
sed -i.bak "s/REPORT_DATE_PLACEHOLDER/$REPORT_DATE/g" "$OUTPUT_FILE" && rm -f "$OUTPUT_FILE.bak"

echo "✓ 详细报告已生成: $OUTPUT_FILE"
echo ""
echo "查看报告: cat $OUTPUT_FILE"
