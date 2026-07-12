// Package coverage 差异分析器测试
package coverage

import (
	"strings"
	"testing"
)

// ============================================================================
// DiffAnalyzer 测试
// ============================================================================

func TestDiffAnalyzer_CompareCoverage(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	// 创建当前报告
	current := &CoverageReport{
		OverallCoverage:   80.0,
		TotalStatements:   1000,
		CoveredStatements: 800,
		PackageCoverage: map[string]float64{
			"internal/handler": 85.0,
			"internal/service": 75.0,
			"internal/store":   80.0,
		},
		FileDetails: map[string]*FileDetail{
			"internal/handler/auth.go": {
				FilePath:          "internal/handler/auth.go",
				Coverage:          85.0,
				TotalStatements:   100,
				CoveredStatements: 85,
			},
		},
	}

	// 创建之前的报告
	previous := &CoverageReport{
		OverallCoverage:   75.0,
		TotalStatements:   900,
		CoveredStatements: 675,
		PackageCoverage: map[string]float64{
			"internal/handler": 80.0,
			"internal/service": 70.0,
		},
		FileDetails: map[string]*FileDetail{
			"internal/handler/auth.go": {
				FilePath:          "internal/handler/auth.go",
				Coverage:          80.0,
				TotalStatements:   90,
				CoveredStatements: 72,
			},
		},
	}

	// 执行差异分析
	diff := analyzer.CompareCoverage(current, previous)

	// 验证整体差异
	if diff.Overall.CurrentCoverage != 80.0 {
		t.Errorf("Expected current coverage 80.0, got %.2f", diff.Overall.CurrentCoverage)
	}

	if diff.Overall.PreviousCoverage != 75.0 {
		t.Errorf("Expected previous coverage 75.0, got %.2f", diff.Overall.PreviousCoverage)
	}

	if diff.Overall.Delta != 5.0 {
		t.Errorf("Expected delta 5.0, got %.2f", diff.Overall.Delta)
	}

	if diff.Overall.Trend != TrendImproved {
		t.Errorf("Expected trend improved, got %s", diff.Overall.Trend)
	}

	// 验证包差异
	if len(diff.Packages) != 3 {
		t.Errorf("Expected 3 packages, got %d", len(diff.Packages))
	}

	// 验证文件差异
	if len(diff.Files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(diff.Files))
	}

	// 验证摘要
	if diff.Summary.OverallTrend != TrendImproved {
		t.Errorf("Expected overall trend improved, got %s", diff.Summary.OverallTrend)
	}

	if diff.Summary.PackagesImproved != 2 {
		t.Errorf("Expected 2 improved packages, got %d", diff.Summary.PackagesImproved)
	}

	if diff.Summary.PackagesAdded != 1 {
		t.Errorf("Expected 1 added package, got %d", diff.Summary.PackagesAdded)
	}
}

func TestDiffAnalyzer_CompareOverall(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	tests := []struct {
		name             string
		currentCoverage  float64
		previousCoverage float64
		expectedDelta    float64
		expectedTrend    Trend
	}{
		{
			name:             "Improved",
			currentCoverage:  85.0,
			previousCoverage: 75.0,
			expectedDelta:    10.0,
			expectedTrend:    TrendImproved,
		},
		{
			name:             "Regressed",
			currentCoverage:  70.0,
			previousCoverage: 80.0,
			expectedDelta:    -10.0,
			expectedTrend:    TrendRegressed,
		},
		{
			name:             "Unchanged",
			currentCoverage:  80.0,
			previousCoverage: 80.0,
			expectedDelta:    0.0,
			expectedTrend:    TrendUnchanged,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := &CoverageReport{
				OverallCoverage:   tt.currentCoverage,
				TotalStatements:   1000,
				CoveredStatements: int(tt.currentCoverage * 10),
			}

			previous := &CoverageReport{
				OverallCoverage:   tt.previousCoverage,
				TotalStatements:   1000,
				CoveredStatements: int(tt.previousCoverage * 10),
			}

			diff := analyzer.compareOverall(current, previous)

			if diff.Delta != tt.expectedDelta {
				t.Errorf("Expected delta %.2f, got %.2f", tt.expectedDelta, diff.Delta)
			}

			if diff.Trend != tt.expectedTrend {
				t.Errorf("Expected trend %s, got %s", tt.expectedTrend, diff.Trend)
			}
		})
	}
}

func TestDiffAnalyzer_ComparePackages(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	current := &CoverageReport{
		PackageCoverage: map[string]float64{
			"pkg1": 85.0, // Improved
			"pkg2": 75.0, // Unchanged
			"pkg3": 80.0, // New
		},
	}

	previous := &CoverageReport{
		PackageCoverage: map[string]float64{
			"pkg1": 80.0,
			"pkg2": 75.0,
			"pkg4": 70.0, // Removed
		},
	}

	diffs := analyzer.comparePackages(current, previous)

	// 验证包数量
	if len(diffs) != 4 {
		t.Fatalf("Expected 4 package diffs, got %d", len(diffs))
	}

	// 查找各个包
	pkgMap := make(map[string]PackageDiff)
	for _, d := range diffs {
		pkgMap[d.PackagePath] = d
	}

	// 验证pkg1（改进）
	if pkg1, ok := pkgMap["pkg1"]; ok {
		if pkg1.Status != PackageStatusChanged {
			t.Errorf("pkg1 status should be changed, got %s", pkg1.Status)
		}
		if pkg1.Trend != TrendImproved {
			t.Errorf("pkg1 trend should be improved, got %s", pkg1.Trend)
		}
		if pkg1.Delta != 5.0 {
			t.Errorf("pkg1 delta should be 5.0, got %.2f", pkg1.Delta)
		}
	} else {
		t.Error("pkg1 not found")
	}

	// 验证pkg2（不变）
	if pkg2, ok := pkgMap["pkg2"]; ok {
		if pkg2.Status != PackageStatusChanged {
			t.Errorf("pkg2 status should be changed, got %s", pkg2.Status)
		}
		if pkg2.Trend != TrendUnchanged {
			t.Errorf("pkg2 trend should be unchanged, got %s", pkg2.Trend)
		}
	} else {
		t.Error("pkg2 not found")
	}

	// 验证pkg3（新增）
	if pkg3, ok := pkgMap["pkg3"]; ok {
		if pkg3.Status != PackageStatusNew {
			t.Errorf("pkg3 status should be new, got %s", pkg3.Status)
		}
		if pkg3.Trend != TrendImproved {
			t.Errorf("pkg3 trend should be improved, got %s", pkg3.Trend)
		}
	} else {
		t.Error("pkg3 not found")
	}

	// 验证pkg4（删除）
	if pkg4, ok := pkgMap["pkg4"]; ok {
		if pkg4.Status != PackageStatusRemoved {
			t.Errorf("pkg4 status should be removed, got %s", pkg4.Status)
		}
	} else {
		t.Error("pkg4 not found")
	}
}

func TestDiffAnalyzer_FormatDiff(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	current := &CoverageReport{
		OverallCoverage:   80.0,
		TotalStatements:   1000,
		CoveredStatements: 800,
		PackageCoverage: map[string]float64{
			"internal/handler": 85.0,
			"internal/service": 70.0,
		},
		FileDetails: map[string]*FileDetail{},
	}

	previous := &CoverageReport{
		OverallCoverage:   75.0,
		TotalStatements:   900,
		CoveredStatements: 675,
		PackageCoverage: map[string]float64{
			"internal/handler": 80.0,
			"internal/service": 78.0,
		},
		FileDetails: map[string]*FileDetail{},
	}

	diff := analyzer.CompareCoverage(current, previous)
	formatted := analyzer.FormatDiff(diff)

	// 验证格式化输出包含关键信息
	if !strings.Contains(formatted, "Coverage Diff Report") {
		t.Error("Formatted diff should contain title")
	}

	if !strings.Contains(formatted, "Previous: 75.00%") {
		t.Error("Formatted diff should contain previous coverage")
	}

	if !strings.Contains(formatted, "Current:  80.00%") {
		t.Error("Formatted diff should contain current coverage")
	}

	if !strings.Contains(formatted, "Delta:    +5.00%") {
		t.Error("Formatted diff should contain delta")
	}

	if !strings.Contains(formatted, "↑") {
		t.Error("Formatted diff should contain improvement symbol")
	}

	if !strings.Contains(formatted, "Summary:") {
		t.Error("Formatted diff should contain summary")
	}
}

func TestDiffAnalyzer_GenerateSummary(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	diff := &CoverageDiff{
		Overall: OverallDiff{
			Trend: TrendImproved,
		},
		Packages: []PackageDiff{
			{Status: PackageStatusNew},
			{Status: PackageStatusRemoved},
			{Status: PackageStatusChanged, Trend: TrendImproved},
			{Status: PackageStatusChanged, Trend: TrendRegressed},
			{Status: PackageStatusChanged, Trend: TrendUnchanged},
		},
		Files: []FileDiff{
			{Status: FileStatusNew},
			{Status: FileStatusRemoved},
			{Status: FileStatusChanged, Trend: TrendImproved},
			{Status: FileStatusChanged, Trend: TrendRegressed},
		},
	}

	summary := analyzer.generateSummary(diff)

	if summary.OverallTrend != TrendImproved {
		t.Errorf("Expected overall trend improved, got %s", summary.OverallTrend)
	}

	if summary.PackagesAdded != 1 {
		t.Errorf("Expected 1 package added, got %d", summary.PackagesAdded)
	}

	if summary.PackagesRemoved != 1 {
		t.Errorf("Expected 1 package removed, got %d", summary.PackagesRemoved)
	}

	if summary.PackagesImproved != 1 {
		t.Errorf("Expected 1 package improved, got %d", summary.PackagesImproved)
	}

	if summary.PackagesRegressed != 1 {
		t.Errorf("Expected 1 package regressed, got %d", summary.PackagesRegressed)
	}

	if summary.FilesAdded != 1 {
		t.Errorf("Expected 1 file added, got %d", summary.FilesAdded)
	}

	if summary.FilesRemoved != 1 {
		t.Errorf("Expected 1 file removed, got %d", summary.FilesRemoved)
	}

	if summary.FilesImproved != 1 {
		t.Errorf("Expected 1 file improved, got %d", summary.FilesImproved)
	}

	if summary.FilesRegressed != 1 {
		t.Errorf("Expected 1 file regressed, got %d", summary.FilesRegressed)
	}
}

func TestFormatTrendSymbol(t *testing.T) {
	tests := []struct {
		trend    Trend
		expected string
	}{
		{TrendImproved, "↑"},
		{TrendRegressed, "↓"},
		{TrendUnchanged, "→"},
	}

	for _, tt := range tests {
		t.Run(string(tt.trend), func(t *testing.T) {
			result := formatTrendSymbol(tt.trend)
			if result != tt.expected {
				t.Errorf("Expected symbol %s, got %s", tt.expected, result)
			}
		})
	}
}
