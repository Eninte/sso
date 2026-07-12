package coverage

import (
	"strings"
	"testing"
)

// ============================================================================
// Reporter 测试
// ============================================================================

func TestReporter_FormatDeficitReport(t *testing.T) {
	tests := []struct {
		name           string
		threshold      float64
		report         *CoverageReport
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:      "Coverage above threshold",
			threshold: 80.0,
			report: &CoverageReport{
				OverallCoverage:   85.5,
				TotalStatements:   1000,
				CoveredStatements: 855,
				CoverageDeficit:   0,
				PackageCoverage: map[string]float64{
					"internal/handler": 90.0,
					"internal/service": 85.0,
				},
				FileDetails: map[string]*FileDetail{
					"internal/handler/auth.go": {
						FilePath:          "internal/handler/auth.go",
						TotalStatements:   100,
						CoveredStatements: 90,
						Coverage:          90.0,
					},
				},
			},
			wantContains: []string{
				"Code Coverage Analysis Report",
				"Overall Coverage:",
				"85.50%",
				"✅ PASSED",
			},
			wantNotContain: []string{
				"❌ FAILED",
				"Remediation Plan",
			},
		},
		{
			name:      "Coverage below threshold",
			threshold: 80.0,
			report: &CoverageReport{
				OverallCoverage:   75.0,
				TotalStatements:   1000,
				CoveredStatements: 750,
				CoverageDeficit:   5.0,
				PackageCoverage: map[string]float64{
					"internal/handler": 70.0,
					"internal/service": 75.0,
				},
				FileDetails: map[string]*FileDetail{
					"internal/handler/auth.go": {
						FilePath:          "internal/handler/auth.go",
						TotalStatements:   100,
						CoveredStatements: 70,
						Coverage:          70.0,
						UncoveredLines:    []int{10, 20, 30},
					},
					"internal/service/auth.go": {
						FilePath:          "internal/service/auth.go",
						TotalStatements:   100,
						CoveredStatements: 75,
						Coverage:          75.0,
						UncoveredLines:    []int{15, 25},
					},
				},
			},
			wantContains: []string{
				"Code Coverage Analysis Report",
				"Overall Coverage:",
				"75.00%",
				"❌ FAILED",
				"deficit: 5.00%",
				"Package Coverage Breakdown",
				"Remediation Plan",
				"BUILD FAILED",
			},
		},
		{
			name:      "Multiple packages with different priorities",
			threshold: 80.0,
			report: &CoverageReport{
				OverallCoverage:   70.0,
				TotalStatements:   1000,
				CoveredStatements: 700,
				CoverageDeficit:   10.0,
				PackageCoverage: map[string]float64{
					"internal/service": 65.0,
					"internal/handler": 70.0,
					"internal/util":    75.0,
				},
				FileDetails: map[string]*FileDetail{
					"internal/service/auth.go": {
						FilePath:          "internal/service/auth.go",
						TotalStatements:   200,
						CoveredStatements: 130,
						Coverage:          65.0,
					},
					"internal/handler/auth.go": {
						FilePath:          "internal/handler/auth.go",
						TotalStatements:   100,
						CoveredStatements: 70,
						Coverage:          70.0,
					},
					"internal/util/helper.go": {
						FilePath:          "internal/util/helper.go",
						TotalStatements:   100,
						CoveredStatements: 75,
						Coverage:          75.0,
					},
				},
			},
			wantContains: []string{
				"Medium Priority",
				"Low Priority",
				"internal/service",
				"internal/handler",
			},
		},
		{
			name:      "With uncovered critical paths",
			threshold: 80.0,
			report: &CoverageReport{
				OverallCoverage:   75.0,
				TotalStatements:   1000,
				CoveredStatements: 750,
				CoverageDeficit:   5.0,
				PackageCoverage: map[string]float64{
					"internal/service/auth": 70.0,
				},
				FileDetails: map[string]*FileDetail{
					"internal/service/auth.go": {
						FilePath:          "internal/service/auth.go",
						TotalStatements:   100,
						CoveredStatements: 70,
						Coverage:          70.0,
					},
				},
				UncoveredCritical: []UncoveredPath{
					{
						File:        "internal/service/auth.go",
						Lines:       []int{156, 157, 158},
						Function:    "ValidateMFA",
						Criticality: CriticalityHigh,
					},
				},
			},
			wantContains: []string{
				"Uncovered Critical Paths",
				"internal/service/auth.go",
				"High",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewCoverageAnalyzer(tt.threshold, nil)
			reporter := NewReporter(analyzer)

			got := reporter.FormatDeficitReport(tt.report)

			// 检查期望包含的内容
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("FormatDeficitReport() output missing expected string:\nwant: %q\ngot:\n%s", want, got)
				}
			}

			// 检查不应包含的内容
			for _, notWant := range tt.wantNotContain {
				if strings.Contains(got, notWant) {
					t.Errorf("FormatDeficitReport() output contains unexpected string:\nnot want: %q\ngot:\n%s", notWant, got)
				}
			}
		})
	}
}

func TestReporter_FormatSummary(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		report    *CoverageReport
		want      string
	}{
		{
			name:      "Coverage passes",
			threshold: 80.0,
			report: &CoverageReport{
				OverallCoverage: 85.0,
			},
			want: "✅ Coverage: 85.00% (threshold: 80.00%)",
		},
		{
			name:      "Coverage fails",
			threshold: 80.0,
			report: &CoverageReport{
				OverallCoverage: 75.0,
				CoverageDeficit: 5.0,
				FileDetails: map[string]*FileDetail{
					"internal/handler/auth.go": {
						FilePath:          "internal/handler/auth.go",
						TotalStatements:   100,
						CoveredStatements: 70,
						Coverage:          70.0,
					},
					"internal/service/auth.go": {
						FilePath:          "internal/service/auth.go",
						TotalStatements:   100,
						CoveredStatements: 75,
						Coverage:          75.0,
					},
				},
			},
			want: "❌ Coverage: 75.00% (threshold: 80.00%, deficit: 5.00%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewCoverageAnalyzer(tt.threshold, nil)
			reporter := NewReporter(analyzer)

			got := reporter.FormatSummary(tt.report)

			if !strings.Contains(got, tt.want) {
				t.Errorf("FormatSummary() = %q, want to contain %q", got, tt.want)
			}
		})
	}
}

func TestTruncatePackagePath(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		maxLen int
		want   string
	}{
		{
			name:   "Short path",
			path:   "internal/handler",
			maxLen: 50,
			want:   "internal/handler",
		},
		{
			name:   "Long path",
			path:   "github.com/example/sso/internal/service/authentication",
			maxLen: 30,
			want:   "github.com/e...thentication",
		},
		{
			name:   "Very short maxLen",
			path:   "internal/handler",
			maxLen: 8,
			want:   "internal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncatePackagePath(tt.path, tt.maxLen)

			if len(got) > tt.maxLen {
				t.Errorf("truncatePackagePath() length = %d, want <= %d", len(got), tt.maxLen)
			}

			if tt.path == tt.want && got != tt.want {
				t.Errorf("truncatePackagePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatLineNumbers(t *testing.T) {
	tests := []struct {
		name  string
		lines []int
		want  string
	}{
		{
			name:  "Empty",
			lines: []int{},
			want:  "none",
		},
		{
			name:  "Few lines",
			lines: []int{10, 20, 30},
			want:  "[10 20 30]",
		},
		{
			name:  "Many lines",
			lines: []int{10, 20, 30, 40, 50, 60, 70},
			want:  "[10 20 30 40 50]... (2 more)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLineNumbers(tt.lines)

			if got != tt.want {
				t.Errorf("formatLineNumbers() = %q, want %q", got, tt.want)
			}
		})
	}
}
