// Package coverage 代码覆盖率报告生成器
package coverage

import (
	"fmt"
	"io"
	"strings"
)

// ============================================================================
// Reporter 类型定义
// ============================================================================

// Reporter 覆盖率报告生成器
type Reporter struct {
	analyzer     *CoverageAnalyzer
	htmlReporter *HTMLReporter
	jsonReporter *JSONReporter
}

// NewReporter 创建报告生成器
func NewReporter(analyzer *CoverageAnalyzer) *Reporter {
	return &Reporter{
		analyzer:     analyzer,
		htmlReporter: NewHTMLReporter(analyzer),
		jsonReporter: NewJSONReporter(analyzer),
	}
}

// ============================================================================
// 报告格式化方法
// ============================================================================

// FormatDeficitReport 生成详细的覆盖率缺口报告
// 参数：
//   - report: 覆盖率报告
//
// 返回：
//   - string: 格式化的报告文本
func (r *Reporter) FormatDeficitReport(report *CoverageReport) string {
	var sb strings.Builder

	// 标题
	sb.WriteString("=" + strings.Repeat("=", 79) + "\n")
	sb.WriteString("  Code Coverage Analysis Report\n")
	sb.WriteString("=" + strings.Repeat("=", 79) + "\n\n")

	// 整体覆盖率
	sb.WriteString("Overall Coverage:\n")
	sb.WriteString(fmt.Sprintf("  Current:  %.2f%% (%d/%d statements)\n",
		report.OverallCoverage,
		report.CoveredStatements,
		report.TotalStatements,
	))
	sb.WriteString(fmt.Sprintf("  Threshold: %.2f%%\n", r.analyzer.threshold))

	if report.OverallCoverage >= r.analyzer.threshold {
		sb.WriteString("  Status:    ✅ PASSED\n\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("  Status:    ❌ FAILED (deficit: %.2f%%)\n\n", report.CoverageDeficit))

	// 包级别覆盖率
	sb.WriteString("-" + strings.Repeat("-", 79) + "\n")
	sb.WriteString("Package Coverage Breakdown:\n")
	sb.WriteString("-" + strings.Repeat("-", 79) + "\n\n")

	// 按覆盖率排序显示包
	deficits := r.analyzer.GetPackagesBelowThreshold(report)

	if len(deficits) == 0 {
		sb.WriteString("  All packages meet the threshold ✓\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("%-50s %10s %10s %8s\n",
			"Package", "Coverage", "Deficit", "Tests"))
		sb.WriteString(strings.Repeat("-", 80) + "\n")

		for _, deficit := range deficits {
			status := "✗"
			sb.WriteString(fmt.Sprintf("%-50s %9.2f%% %9.2f%% %7d %s\n",
				truncatePackagePath(deficit.PackagePath, 50),
				deficit.CurrentCoverage,
				deficit.Deficit,
				deficit.EstimatedTests,
				status,
			))
		}
		sb.WriteString("\n")
	}

	// 通过阈值的包（仅显示汇总）
	passedPackages := r.getPassedPackages(report)
	if len(passedPackages) > 0 {
		sb.WriteString(fmt.Sprintf("Packages meeting threshold: %d ✓\n\n", len(passedPackages)))
	}

	// 改进计划
	if len(deficits) > 0 {
		plan := r.analyzer.CalculateRemediationPlan(report)
		sb.WriteString(r.formatRemediationPlan(plan))
	}

	// 关键路径未覆盖
	if len(report.UncoveredCritical) > 0 {
		sb.WriteString("-" + strings.Repeat("-", 79) + "\n")
		sb.WriteString("⚠️  Uncovered Critical Paths:\n")
		sb.WriteString("-" + strings.Repeat("-", 79) + "\n\n")

		for i, path := range report.UncoveredCritical {
			if i >= 5 {
				sb.WriteString(fmt.Sprintf("  ... and %d more critical paths\n", len(report.UncoveredCritical)-5))
				break
			}
			sb.WriteString(fmt.Sprintf("  [%s] %s (lines: %v)\n",
				path.Criticality,
				path.File,
				formatLineNumbers(path.Lines),
			))
		}
		sb.WriteString("\n")
	}

	// 构建失败消息
	sb.WriteString("=" + strings.Repeat("=", 79) + "\n")
	sb.WriteString("❌ BUILD FAILED: Coverage below threshold\n")
	sb.WriteString("=" + strings.Repeat("=", 79) + "\n")

	return sb.String()
}

// formatRemediationPlan 格式化改进计划
func (r *Reporter) formatRemediationPlan(plan *RemediationPlan) string {
	var sb strings.Builder

	sb.WriteString("-" + strings.Repeat("-", 79) + "\n")
	sb.WriteString("Remediation Plan:\n")
	sb.WriteString("-" + strings.Repeat("-", 79) + "\n\n")

	sb.WriteString(fmt.Sprintf("To reach %.2f%% coverage, add approximately %d test case(s)\n\n",
		plan.TargetCoverage,
		plan.TotalEstimatedTests,
	))

	// 按优先级分组
	highPriority := filterByPriority(plan.PackageActions, "High")
	mediumPriority := filterByPriority(plan.PackageActions, "Medium")
	lowPriority := filterByPriority(plan.PackageActions, "Low")

	if len(highPriority) > 0 {
		sb.WriteString("🔴 High Priority Packages:\n")
		sb.WriteString(strings.Repeat("-", 80) + "\n")
		for _, action := range highPriority {
			sb.WriteString(r.formatPackageAction(action))
		}
		sb.WriteString("\n")
	}

	if len(mediumPriority) > 0 {
		sb.WriteString("🟡 Medium Priority Packages:\n")
		sb.WriteString(strings.Repeat("-", 80) + "\n")
		for _, action := range mediumPriority {
			sb.WriteString(r.formatPackageAction(action))
		}
		sb.WriteString("\n")
	}

	if len(lowPriority) > 0 {
		sb.WriteString("🟢 Low Priority Packages:\n")
		sb.WriteString(strings.Repeat("-", 80) + "\n")
		for _, action := range lowPriority {
			sb.WriteString(r.formatPackageAction(action))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatPackageAction 格式化包改进行动
func (r *Reporter) formatPackageAction(action PackageAction) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n📦 %s\n", action.PackagePath))
	sb.WriteString(fmt.Sprintf("   Current: %.2f%% → Target: %.2f%% (add ~%d tests)\n",
		action.CurrentCoverage,
		action.TargetCoverage,
		action.RequiredTests,
	))

	sb.WriteString("   Suggestions:\n")
	for _, suggestion := range action.Suggestions {
		sb.WriteString(fmt.Sprintf("     • %s\n", suggestion))
	}

	return sb.String()
}

// FormatSummary 生成简洁摘要
func (r *Reporter) FormatSummary(report *CoverageReport) string {
	if report.OverallCoverage >= r.analyzer.threshold {
		return fmt.Sprintf("✅ Coverage: %.2f%% (threshold: %.2f%%)",
			report.OverallCoverage, r.analyzer.threshold)
	}

	deficits := r.analyzer.GetPackagesBelowThreshold(report)
	return fmt.Sprintf("❌ Coverage: %.2f%% (threshold: %.2f%%, deficit: %.2f%%, %d packages below threshold)",
		report.OverallCoverage,
		r.analyzer.threshold,
		report.CoverageDeficit,
		len(deficits),
	)
}

// GenerateHTML 生成HTML覆盖率报告
// 参数：
//   - w: 输出流
//   - report: 覆盖率报告
//
// 返回：
//   - error: 错误信息
func (r *Reporter) GenerateHTML(w io.Writer, report *CoverageReport) error {
	return r.htmlReporter.GenerateHTML(w, report)
}

// GenerateJSON 生成JSON覆盖率报告（用于CI/CD）
// 参数：
//   - w: 输出流
//   - report: 覆盖率报告
//
// 返回：
//   - error: 错误信息
func (r *Reporter) GenerateJSON(w io.Writer, report *CoverageReport) error {
	return r.jsonReporter.GenerateJSON(w, report)
}

// GenerateCompactJSON 生成紧凑JSON报告（无缩进）
// 参数：
//   - w: 输出流
//   - report: 覆盖率报告
//
// 返回：
//   - error: 错误信息
func (r *Reporter) GenerateCompactJSON(w io.Writer, report *CoverageReport) error {
	return r.jsonReporter.GenerateCompactJSON(w, report)
}

// ============================================================================
// 辅助函数
// ============================================================================

// getPassedPackages 获取通过阈值的包列表
func (r *Reporter) getPassedPackages(report *CoverageReport) []string {
	passedPackages := make([]string, 0)

	for pkgPath, coverage := range report.PackageCoverage {
		if coverage >= r.analyzer.threshold {
			passedPackages = append(passedPackages, pkgPath)
		}
	}

	return passedPackages
}

// truncatePackagePath 截断包路径以适应显示宽度
func truncatePackagePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}

	// 保留开头和结尾，中间用...替代
	if maxLen < 10 {
		return path[:maxLen]
	}

	prefixLen := (maxLen - 3) / 2
	suffixLen := maxLen - 3 - prefixLen

	return path[:prefixLen] + "..." + path[len(path)-suffixLen:]
}

// formatLineNumbers 格式化行号列表
func formatLineNumbers(lines []int) string {
	if len(lines) == 0 {
		return "none"
	}

	if len(lines) <= 5 {
		return fmt.Sprintf("%v", lines)
	}

	// 只显示前5个
	return fmt.Sprintf("%v... (%d more)", lines[:5], len(lines)-5)
}

// filterByPriority 按优先级过滤包行动
func filterByPriority(actions []PackageAction, priority string) []PackageAction {
	result := make([]PackageAction, 0)
	for _, action := range actions {
		if action.Priority == priority {
			result = append(result, action)
		}
	}
	return result
}
