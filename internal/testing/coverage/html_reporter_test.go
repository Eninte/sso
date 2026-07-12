// Package coverage HTML报告生成器测试
package coverage

import (
	"bytes"
	"strings"
	"testing"
)

// ============================================================================
// HTMLReporter 测试
// ============================================================================

func TestHTMLReporter_GenerateHTML(t *testing.T) {
	// 创建测试数据
	analyzer := NewCoverageAnalyzer(80.0, nil)
	reporter := NewHTMLReporter(analyzer)

	report := &CoverageReport{
		OverallCoverage:   75.5,
		TotalStatements:   1000,
		CoveredStatements: 755,
		CoverageDeficit:   4.5,
		PackageCoverage: map[string]float64{
			"internal/handler": 85.0,
			"internal/service": 70.0,
			"internal/store":   65.0,
		},
		FileDetails: map[string]*FileDetail{
			"internal/handler/auth.go": {
				FilePath:          "internal/handler/auth.go",
				TotalStatements:   100,
				CoveredStatements: 85,
				Coverage:          85.0,
				UncoveredLines:    []int{10, 20, 30},
			},
			"internal/service/auth.go": {
				FilePath:          "internal/service/auth.go",
				TotalStatements:   200,
				CoveredStatements: 140,
				Coverage:          70.0,
				UncoveredLines:    []int{50, 60, 70},
			},
		},
		UncoveredCritical: []UncoveredPath{
			{
				File:        "internal/service/auth.go",
				Lines:       []int{50, 60, 70},
				Function:    "Authenticate",
				Criticality: CriticalityHigh,
			},
		},
	}

	// 生成HTML报告
	var buf bytes.Buffer
	err := reporter.GenerateHTML(&buf, report)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	// 验证HTML内容
	html := buf.String()

	// 检查HTML结构
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("HTML report should start with DOCTYPE")
	}

	if !strings.Contains(html, "<title>Code Coverage Report</title>") {
		t.Error("HTML report should have title")
	}

	// 检查覆盖率数据
	if !strings.Contains(html, "75.5") {
		t.Error("HTML report should contain overall coverage")
	}

	if !strings.Contains(html, "80.0") {
		t.Error("HTML report should contain threshold")
	}

	// 检查状态
	if !strings.Contains(html, "FAILED") {
		t.Error("HTML report should show FAILED status for coverage below threshold")
	}

	// 检查包信息
	if !strings.Contains(html, "internal/handler") {
		t.Error("HTML report should contain package names")
	}

	// 检查关键路径
	if !strings.Contains(html, "Uncovered Critical Paths") {
		t.Error("HTML report should contain critical paths section")
	}
}

func TestHTMLReporter_GenerateHTML_Passed(t *testing.T) {
	// 创建通过阈值的测试数据
	analyzer := NewCoverageAnalyzer(80.0, nil)
	reporter := NewHTMLReporter(analyzer)

	report := &CoverageReport{
		OverallCoverage:   85.0,
		TotalStatements:   1000,
		CoveredStatements: 850,
		CoverageDeficit:   0,
		PackageCoverage: map[string]float64{
			"internal/handler": 85.0,
			"internal/service": 83.0,
			"internal/store":   88.0,
		},
		FileDetails:       map[string]*FileDetail{},
		UncoveredCritical: []UncoveredPath{},
	}

	// 生成HTML报告
	var buf bytes.Buffer
	err := reporter.GenerateHTML(&buf, report)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	// 验证HTML内容
	html := buf.String()

	// 检查状态
	if !strings.Contains(html, "PASSED") {
		t.Error("HTML report should show PASSED status for coverage above threshold")
	}

	// 不应该有remediation plan
	if strings.Contains(html, "Remediation Plan") {
		t.Error("HTML report should not show remediation plan when coverage is sufficient")
	}
}

func TestHTMLReporter_PrepareReportData(t *testing.T) {
	analyzer := NewCoverageAnalyzer(80.0, nil)
	reporter := NewHTMLReporter(analyzer)

	report := &CoverageReport{
		OverallCoverage:   75.5,
		TotalStatements:   1000,
		CoveredStatements: 755,
		CoverageDeficit:   4.5,
		PackageCoverage: map[string]float64{
			"internal/handler": 85.0,
			"internal/service": 70.0,
		},
		FileDetails: map[string]*FileDetail{
			"internal/handler/auth.go": {
				FilePath:          "internal/handler/auth.go",
				TotalStatements:   100,
				CoveredStatements: 85,
				Coverage:          85.0,
			},
			"internal/service/auth.go": {
				FilePath:          "internal/service/auth.go",
				TotalStatements:   200,
				CoveredStatements: 140,
				Coverage:          70.0,
			},
		},
		UncoveredCritical: []UncoveredPath{},
	}

	data := reporter.prepareReportData(report)

	// 验证数据结构
	if data["Report"] != report {
		t.Error("Report data should be set")
	}

	if data["Threshold"] != 80.0 {
		t.Error("Threshold should be 80.0")
	}

	if data["Passed"] != false {
		t.Error("Passed should be false")
	}

	// 验证deficits
	deficits, ok := data["Deficits"].([]PackageDeficit)
	if !ok {
		t.Fatal("Deficits should be []PackageDeficit")
	}

	if len(deficits) != 1 {
		t.Errorf("Expected 1 deficit package, got %d", len(deficits))
	}

	// 验证通过的包
	passedPackages, ok := data["PassedPackages"].([]PackageInfo)
	if !ok {
		t.Fatal("PassedPackages should be []PackageInfo")
	}

	if len(passedPackages) != 1 {
		t.Errorf("Expected 1 passed package, got %d", len(passedPackages))
	}
}
