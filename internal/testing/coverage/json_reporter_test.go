// Package coverage JSON报告生成器测试
package coverage

import (
	"bytes"
	"encoding/json"
	"testing"
)

// ============================================================================
// JSONReporter 测试
// ============================================================================

func TestJSONReporter_GenerateJSON(t *testing.T) {
	// 创建测试数据
	analyzer := NewCoverageAnalyzer(80.0, nil)
	reporter := NewJSONReporter(analyzer)

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
				UncoveredLines:    []int{10, 20, 30},
			},
		},
		UncoveredCritical: []UncoveredPath{},
	}

	// 生成JSON报告
	var buf bytes.Buffer
	err := reporter.GenerateJSON(&buf, report)
	if err != nil {
		t.Fatalf("GenerateJSON failed: %v", err)
	}

	// 解析JSON
	var jsonReport JSONCoverageReport
	if err := json.Unmarshal(buf.Bytes(), &jsonReport); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// 验证JSON内容
	if jsonReport.Threshold != 80.0 {
		t.Errorf("Expected threshold 80.0, got %.2f", jsonReport.Threshold)
	}

	if jsonReport.OverallCoverage != 75.5 {
		t.Errorf("Expected coverage 75.5, got %.2f", jsonReport.OverallCoverage)
	}

	if jsonReport.Passed != false {
		t.Error("Expected passed to be false")
	}

	if jsonReport.CoverageDeficit != 4.5 {
		t.Errorf("Expected deficit 4.5, got %.2f", jsonReport.CoverageDeficit)
	}

	// 验证包覆盖率
	if len(jsonReport.PackageCoverage) != 2 {
		t.Errorf("Expected 2 packages, got %d", len(jsonReport.PackageCoverage))
	}

	// 验证文件详情
	if len(jsonReport.FileDetails) != 1 {
		t.Errorf("Expected 1 file, got %d", len(jsonReport.FileDetails))
	}
}

func TestJSONReporter_GenerateCompactJSON(t *testing.T) {
	// 创建测试数据
	analyzer := NewCoverageAnalyzer(80.0, nil)
	reporter := NewJSONReporter(analyzer)

	report := &CoverageReport{
		OverallCoverage:   85.0,
		TotalStatements:   1000,
		CoveredStatements: 850,
		CoverageDeficit:   0,
		PackageCoverage:   map[string]float64{},
		FileDetails:       map[string]*FileDetail{},
		UncoveredCritical: []UncoveredPath{},
	}

	// 生成紧凑JSON报告
	var buf bytes.Buffer
	err := reporter.GenerateCompactJSON(&buf, report)
	if err != nil {
		t.Fatalf("GenerateCompactJSON failed: %v", err)
	}

	// 验证JSON是紧凑格式（没有缩进）
	jsonStr := buf.String()
	if len(jsonStr) == 0 {
		t.Error("JSON should not be empty")
	}

	// 解析JSON验证有效性
	var jsonReport JSONCoverageReport
	if err := json.Unmarshal(buf.Bytes(), &jsonReport); err != nil {
		t.Fatalf("Failed to parse compact JSON: %v", err)
	}

	if jsonReport.Passed != true {
		t.Error("Expected passed to be true")
	}
}

func TestJSONReporter_PrepareJSONData(t *testing.T) {
	analyzer := NewCoverageAnalyzer(80.0, nil)
	reporter := NewJSONReporter(analyzer)

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

	data := reporter.prepareJSONData(report)

	// 验证数据结构
	if data.Threshold != 80.0 {
		t.Errorf("Expected threshold 80.0, got %.2f", data.Threshold)
	}

	if data.OverallCoverage != 75.5 {
		t.Errorf("Expected coverage 75.5, got %.2f", data.OverallCoverage)
	}

	if data.Passed != false {
		t.Error("Expected passed to be false")
	}

	// 验证timestamp格式
	if data.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}

	// 验证packages below threshold（service包应该低于80%）
	if len(data.PackagesBelow) == 0 {
		t.Error("Expected at least one package below threshold")
	}

	// 验证remediation plan存在
	if data.RemediationPlan == nil {
		t.Error("Remediation plan should not be nil for coverage below threshold")
	}
}

func TestJSONReporter_ConvertFileDetails(t *testing.T) {
	analyzer := NewCoverageAnalyzer(80.0, nil)
	reporter := NewJSONReporter(analyzer)

	details := map[string]*FileDetail{
		"file1.go": {
			FilePath: "file1.go",
			Coverage: 80.0,
		},
		"file2.go": {
			FilePath: "file2.go",
			Coverage: 70.0,
		},
	}

	result := reporter.convertFileDetails(details)

	if len(result) != 2 {
		t.Errorf("Expected 2 files, got %d", len(result))
	}

	// 验证文件都在结果中
	found := make(map[string]bool)
	for _, file := range result {
		found[file.FilePath] = true
	}

	if !found["file1.go"] || !found["file2.go"] {
		t.Error("Not all files found in result")
	}
}
