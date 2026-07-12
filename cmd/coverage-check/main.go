// Package main 代码覆盖率检查命令
// 用于CI/CD流程中的覆盖率阈值验证和构建失败控制
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/example/sso/internal/testing/coverage"
)

// ============================================================================
// 命令行参数
// ============================================================================

var (
	profilePath    = flag.String("profile", "coverage.out", "Coverage profile file path")
	threshold      = flag.Float64("threshold", 80.0, "Coverage threshold percentage (0-100)")
	verbose        = flag.Bool("verbose", false, "Enable verbose output")
	criticalPaths  = flag.String("critical-paths", "", "Comma-separated list of critical paths")
	htmlOutput     = flag.String("html", "", "Output HTML report to file")
	jsonOutput     = flag.String("json", "", "Output JSON report to file")
	diffPrevious   = flag.String("diff", "", "Compare with previous coverage profile")
	trendData      = flag.String("trend-data", "", "Export trend data to file")
	trendChartData = flag.String("trend-chart", "", "Export chart data to file")
)

// ============================================================================
// 主函数
// ============================================================================

func main() {
	flag.Parse()

	// 验证参数
	if *threshold < 0 || *threshold > 100 {
		fmt.Fprintf(os.Stderr, "❌ Error: threshold must be between 0 and 100, got %.2f\n", *threshold)
		os.Exit(1)
	}

	// 检查覆盖率文件是否存在
	if _, err := os.Stat(*profilePath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "❌ Error: coverage profile not found: %s\n", *profilePath)
		fmt.Fprintf(os.Stderr, "   Run 'go test -coverprofile=%s ./...' first\n", *profilePath)
		os.Exit(1)
	}

	// 解析关键路径
	var criticalPathsList []string
	if *criticalPaths != "" {
		// 由用户提供（用于测试或自定义场景）
		criticalPathsList = parseCommaSeparated(*criticalPaths)
	}
	// 否则使用默认值（在 NewCoverageAnalyzer 中）

	// 创建分析器
	analyzer := coverage.NewCoverageAnalyzer(*threshold, criticalPathsList)

	// 分析覆盖率
	report, err := analyzer.Analyze(*profilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: failed to analyze coverage: %v\n", err)
		os.Exit(1)
	}

	// 创建报告生成器
	reporter := coverage.NewReporter(analyzer)

	// 生成HTML报告（如果指定）
	if *htmlOutput != "" {
		if err := generateHTMLReport(reporter, report, *htmlOutput); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to generate HTML report: %v\n", err)
		} else if *verbose {
			fmt.Printf("✓ HTML report generated: %s\n", *htmlOutput)
		}
	}

	// 生成JSON报告（如果指定）
	if *jsonOutput != "" {
		if err := generateJSONReport(reporter, report, *jsonOutput); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to generate JSON report: %v\n", err)
		} else if *verbose {
			fmt.Printf("✓ JSON report generated: %s\n", *jsonOutput)
		}
	}

	// 差异分析（如果指定）
	if *diffPrevious != "" {
		if err := performDiffAnalysis(analyzer, report, *diffPrevious); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to perform diff analysis: %v\n", err)
		}
	}

	// 导出趋势数据（如果指定）
	if *trendData != "" {
		if err := exportTrendData(report, *trendData); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to export trend data: %v\n", err)
		} else if *verbose {
			fmt.Printf("✓ Trend data exported: %s\n", *trendData)
		}
	}

	// 导出图表数据（如果指定）
	if *trendChartData != "" {
		if err := exportChartData(report, *trendChartData); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to export chart data: %v\n", err)
		} else if *verbose {
			fmt.Printf("✓ Chart data exported: %s\n", *trendChartData)
		}
	}

	// 生成报告
	reportText := reporter.FormatDeficitReport(report)

	// 输出报告
	if *verbose {
		fmt.Println(reportText)
	} else {
		// 非详细模式下只输出关键信息
		fmt.Printf("Overall Coverage: %.2f%%\n", report.OverallCoverage)
		if report.OverallCoverage < *threshold {
			fmt.Printf("❌ Coverage below threshold: %.2f%% (deficit: %.2f%%)\n",
				*threshold, report.CoverageDeficit)
		} else {
			fmt.Printf("✓ Coverage meets threshold: %.2f%%\n", *threshold)
		}
	}

	// 执行阈值检查
	if err := analyzer.EnforceThreshold(report); err != nil {
		// 在非详细模式下，输出完整报告以便调试
		if !*verbose {
			fmt.Println("\n" + reportText)
		}

		// 返回非零退出码以使构建失败
		os.Exit(1)
	}

	// 成功
	if *verbose {
		fmt.Println("\n✓ Coverage check passed")
	}
	os.Exit(0)
}

// ============================================================================
// 辅助函数
// ============================================================================

// parseCommaSeparated 解析逗号分隔的字符串
func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}

	var result []string
	for _, item := range splitByComma(s) {
		trimmed := trimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitByComma 按逗号分割字符串
func splitByComma(s string) []string {
	var result []string
	var current string

	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, current)
			current = ""
		} else {
			current += string(s[i])
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}

// trimSpace 移除字符串前后的空白字符
func trimSpace(s string) string {
	start := 0
	end := len(s)

	// 移除前导空白
	for start < end && isSpace(s[start]) {
		start++
	}

	// 移除尾随空白
	for end > start && isSpace(s[end-1]) {
		end--
	}

	return s[start:end]
}

// isSpace 检查字符是否为空白字符
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// generateHTMLReport 生成HTML报告
func generateHTMLReport(reporter *coverage.Reporter, report *coverage.CoverageReport, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create HTML file: %w", err)
	}
	defer file.Close()

	if err := reporter.GenerateHTML(file, report); err != nil {
		return fmt.Errorf("failed to generate HTML: %w", err)
	}

	return nil
}

// generateJSONReport 生成JSON报告
func generateJSONReport(reporter *coverage.Reporter, report *coverage.CoverageReport, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create JSON file: %w", err)
	}
	defer file.Close()

	if err := reporter.GenerateJSON(file, report); err != nil {
		return fmt.Errorf("failed to generate JSON: %w", err)
	}

	return nil
}

// performDiffAnalysis 执行差异分析
func performDiffAnalysis(analyzer *coverage.CoverageAnalyzer, current *coverage.CoverageReport, previousPath string) error {
	// 分析之前的覆盖率
	previous, err := analyzer.Analyze(previousPath)
	if err != nil {
		return fmt.Errorf("failed to analyze previous coverage: %w", err)
	}

	// 执行差异分析
	diffAnalyzer := coverage.NewDiffAnalyzer()
	diff := diffAnalyzer.CompareCoverage(current, previous)

	// 输出差异报告
	fmt.Println("\n" + diffAnalyzer.FormatDiff(diff))

	return nil
}

// exportTrendData 导出趋势数据
func exportTrendData(report *coverage.CoverageReport, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create trend data file: %w", err)
	}
	defer file.Close()

	// 创建趋势导出器
	exporter := coverage.NewTrendExporter()

	// 创建数据点（单个数据点，实际使用中应该读取历史数据）
	dataPoint := exporter.CreateDataPoint(report, *new(time.Time), nil)

	// 导出趋势数据
	if err := exporter.ExportTrendData(file, []coverage.TrendDataPoint{dataPoint}); err != nil {
		return fmt.Errorf("failed to export trend data: %w", err)
	}

	return nil
}

// exportChartData 导出图表数据
func exportChartData(report *coverage.CoverageReport, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create chart data file: %w", err)
	}
	defer file.Close()

	// 创建趋势导出器
	exporter := coverage.NewTrendExporter()

	// 创建数据点
	dataPoint := exporter.CreateDataPoint(report, *new(time.Time), nil)

	// 导出图表数据
	if err := exporter.ExportChartData(file, []coverage.TrendDataPoint{dataPoint}); err != nil {
		return fmt.Errorf("failed to export chart data: %w", err)
	}

	return nil
}
