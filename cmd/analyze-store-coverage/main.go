package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/example/sso/internal/testing/coverage"
)

func main() {
	// 创建覆盖率分析器 - 专注于store层，阈值设为80%
	criticalPaths := []string{
		"internal/store/postgres/",
	}
	analyzer := coverage.NewCoverageAnalyzer(80.0, criticalPaths)

	// 分析覆盖率数据
	report, err := analyzer.Analyze("coverage_store.out")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error analyzing coverage: %v\n", err)
		os.Exit(1)
	}

	// 打印整体覆盖率
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("Store Layer Coverage Analysis")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Printf("\nOverall Coverage: %.2f%%\n", report.OverallCoverage)
	fmt.Printf("Threshold: 80.00%%\n")
	if report.CoverageDeficit > 0 {
		fmt.Printf("❌ Deficit: %.2f%%\n", report.CoverageDeficit)
	} else {
		fmt.Println("✅ Coverage meets threshold")
	}
	fmt.Printf("\nTotal Statements: %d\n", report.TotalStatements)
	fmt.Printf("Covered Statements: %d\n", report.CoveredStatements)
	fmt.Printf("Uncovered Statements: %d\n", report.TotalStatements-report.CoveredStatements)

	// 打印包级别覆盖率（仅Store层）
	fmt.Println("\n" + strings.Repeat("-", 80))
	fmt.Println("Package Coverage (Store Layer):")
	fmt.Println(strings.Repeat("-", 80))
	for pkgPath, coverage := range report.PackageCoverage {
		if strings.Contains(pkgPath, "internal/store") {
			status := "✅"
			if coverage < 80.0 {
				status = "❌"
			}
			fmt.Printf("%s %-50s %.2f%%\n", status, pkgPath, coverage)
		}
	}

	// 打印文件级别详情（仅Store层的postgres实现）
	fmt.Println("\n" + strings.Repeat("-", 80))
	fmt.Println("File-Level Coverage Details (internal/store/postgres/):")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("%-50s %10s %10s %10s\n", "File", "Total", "Covered", "Coverage")
	fmt.Println(strings.Repeat("-", 80))

	for _, detail := range report.FileDetails {
		if strings.Contains(detail.FilePath, "internal/store/postgres/") &&
			!strings.Contains(detail.FilePath, "_test.go") {
			status := "✅"
			if detail.Coverage < 80.0 {
				status = "❌"
			}
			fileName := detail.FilePath[strings.LastIndex(detail.FilePath, "/")+1:]
			fmt.Printf("%s %-47s %10d %10d %9.2f%%\n",
				status,
				fileName,
				detail.TotalStatements,
				detail.CoveredStatements,
				detail.Coverage,
			)
		}
	}

	// 识别未覆盖的关键路径
	criticalGaps := analyzer.IdentifyCriticalGaps(report)
	if len(criticalGaps) > 0 {
		fmt.Println("\n" + strings.Repeat("-", 80))
		fmt.Println("Uncovered Critical Paths in Store Layer:")
		fmt.Println(strings.Repeat("-", 80))
		for _, gap := range criticalGaps {
			if strings.Contains(gap.File, "internal/store/postgres/") {
				fmt.Printf("\n%s %s\n", gap.Criticality, gap.File)
				fmt.Printf("  Uncovered lines: %v\n", gap.Lines)
			}
		}
	}

	// 生成改进计划
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("Remediation Plan:")
	fmt.Println(strings.Repeat("=", 80))

	plan := analyzer.CalculateRemediationPlan(report)
	if plan.Deficit > 0 {
		fmt.Printf("\nCurrent Coverage: %.2f%%\n", plan.CurrentCoverage)
		fmt.Printf("Target Coverage: %.2f%%\n", plan.TargetCoverage)
		fmt.Printf("Deficit: %.2f%%\n", plan.Deficit)
		fmt.Printf("Total Estimated Tests Needed: %d\n", plan.TotalEstimatedTests)

		fmt.Println("\nPackage-Level Actions:")
		for _, action := range plan.PackageActions {
			if strings.Contains(action.PackagePath, "internal/store") {
				fmt.Printf("\n📦 Package: %s\n", action.PackagePath)
				fmt.Printf("   Current: %.2f%% | Target: %.2f%% | Priority: %s\n",
					action.CurrentCoverage, action.TargetCoverage, action.Priority)
				fmt.Printf("   Required Tests: %d\n", action.RequiredTests)
				fmt.Println("   Suggestions:")
				for _, suggestion := range action.Suggestions {
					fmt.Printf("     - %s\n", suggestion)
				}
			}
		}
	} else {
		fmt.Println("\n✅ No remediation needed - coverage meets threshold!")
	}

	// 列出需要补充测试的数据库操作
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("Uncovered Database Operations:")
	fmt.Println(strings.Repeat("=", 80))

	// 遍历所有store层文件，找出未覆盖的行
	uncoveredOps := make(map[string][]int)
	for _, detail := range report.FileDetails {
		if strings.Contains(detail.FilePath, "internal/store/postgres/") &&
			!strings.Contains(detail.FilePath, "_test.go") &&
			len(detail.UncoveredLines) > 0 {
			uncoveredOps[detail.FilePath] = detail.UncoveredLines
		}
	}

	if len(uncoveredOps) > 0 {
		for filePath, lines := range uncoveredOps {
			fileName := filePath[strings.LastIndex(filePath, "/")+1:]
			fmt.Printf("\n📄 %s\n", fileName)
			fmt.Printf("   Uncovered lines (%d): ", len(lines))

			// 打印行号范围，避免输出过长
			if len(lines) <= 20 {
				fmt.Printf("%v\n", lines)
			} else {
				fmt.Printf("%v ... (and %d more)\n", lines[:20], len(lines)-20)
			}
		}

		fmt.Println("\n💡 Recommendations:")
		fmt.Println("   1. Add tests for database connection failures and timeout scenarios")
		fmt.Println("   2. Add tests for constraint violations (unique, foreign key, not null)")
		fmt.Println("   3. Add tests for transaction rollback scenarios")
		fmt.Println("   4. Add tests for query parameter edge cases (NULL, empty strings, max values)")
		fmt.Println("   5. Add tests for concurrent operations and race conditions")
	} else {
		fmt.Println("\n✅ All database operations are covered!")
	}

	// 输出JSON报告（可选，用于自动化处理）
	if len(os.Args) > 1 && os.Args[1] == "--json" {
		jsonData, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n\nJSON Report:\n%s\n", string(jsonData))
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("Analysis Complete")
	fmt.Println(strings.Repeat("=", 80))
}
