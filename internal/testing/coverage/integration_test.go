package coverage

import (
	"os"
	"testing"
)

// TestAnalyzeRealCoverageFile 测试使用真实覆盖率文件
func TestAnalyzeRealCoverageFile(t *testing.T) {
	// 检查是否存在覆盖率文件
	coverageFile := "../../../coverage.out"
	if _, err := os.Stat(coverageFile); os.IsNotExist(err) {
		t.Skip("coverage.out not found, skipping integration test")
	}

	// 创建分析器
	analyzer := NewCoverageAnalyzer(80.0, []string{
		"internal/service/auth.go",
		"internal/service/mfa.go",
		"internal/service/oauth.go",
		"internal/handler/authorize.go",
		"internal/handler/login.go",
		"internal/handler/mfa.go",
		"internal/middleware/auth.go",
	})

	// 分析覆盖率
	report, err := analyzer.Analyze(coverageFile)
	if err != nil {
		t.Fatalf("Analyze() failed: %v", err)
	}

	// 验证基本报告结构
	if report.TotalStatements == 0 {
		t.Error("TotalStatements should not be 0")
	}

	if report.OverallCoverage < 0 || report.OverallCoverage > 100 {
		t.Errorf("OverallCoverage %v should be between 0 and 100", report.OverallCoverage)
	}

	if len(report.FileDetails) == 0 {
		t.Error("FileDetails should not be empty")
	}

	// 识别关键路径缺口
	gaps := analyzer.IdentifyCriticalGaps(report)

	t.Logf("Overall Coverage: %.2f%%", report.OverallCoverage)
	t.Logf("Total Statements: %d", report.TotalStatements)
	t.Logf("Covered Statements: %d", report.CoveredStatements)
	t.Logf("Critical Gaps Found: %d", len(gaps))

	// 输出包级别覆盖率
	if len(report.PackageCoverage) > 0 {
		t.Log("\nPackage Coverage:")
		for pkg, coverage := range report.PackageCoverage {
			t.Logf("  %s: %.2f%%", pkg, coverage)
		}
	}

	// 输出关键路径缺口
	if len(gaps) > 0 {
		t.Log("\nCritical Gaps:")
		for i, gap := range gaps {
			if i >= 5 {
				t.Logf("  ... and %d more", len(gaps)-5)
				break
			}
			t.Logf("  [%s] %s: %d uncovered lines", gap.Criticality, gap.File, len(gap.Lines))
		}
	}
}
