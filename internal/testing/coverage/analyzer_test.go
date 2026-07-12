package coverage

import (
	"os"
	"path/filepath"
	"testing"
)

// ============================================================================
// 测试: NewCoverageAnalyzer
// ============================================================================

func TestNewCoverageAnalyzer(t *testing.T) {
	tests := []struct {
		name          string
		threshold     float64
		criticalPaths []string
		wantThreshold float64
		wantPathsLen  int
	}{
		{
			name:          "with custom critical paths",
			threshold:     80.0,
			criticalPaths: []string{"internal/service/auth.go", "internal/handler/auth.go"},
			wantThreshold: 80.0,
			wantPathsLen:  2,
		},
		{
			name:          "with default critical paths",
			threshold:     85.0,
			criticalPaths: nil,
			wantThreshold: 85.0,
			wantPathsLen:  8, // 默认关键路径数量
		},
		{
			name:          "with empty critical paths",
			threshold:     90.0,
			criticalPaths: []string{},
			wantThreshold: 90.0,
			wantPathsLen:  8, // 默认关键路径数量
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewCoverageAnalyzer(tt.threshold, tt.criticalPaths)

			if analyzer.threshold != tt.wantThreshold {
				t.Errorf("threshold = %v, want %v", analyzer.threshold, tt.wantThreshold)
			}

			if len(analyzer.criticalPaths) != tt.wantPathsLen {
				t.Errorf("critical paths length = %v, want %v", len(analyzer.criticalPaths), tt.wantPathsLen)
			}
		})
	}
}

// ============================================================================
// 测试: parseCoverageLine
// ============================================================================

func TestParseCoverageLine(t *testing.T) {
	analyzer := NewCoverageAnalyzer(80.0, nil)

	tests := []struct {
		name          string
		line          string
		wantFilePath  string
		wantStartLine int
		wantEndLine   int
		wantNumStmt   int
		wantCount     int
		wantCovered   bool
		wantErr       bool
	}{
		{
			name:          "valid covered line",
			line:          "github.com/example/sso/internal/service/auth.go:45.2,47.3 2 5",
			wantFilePath:  "github.com/example/sso/internal/service/auth.go",
			wantStartLine: 45,
			wantEndLine:   47,
			wantNumStmt:   2,
			wantCount:     5,
			wantCovered:   true,
			wantErr:       false,
		},
		{
			name:          "valid uncovered line",
			line:          "github.com/example/sso/internal/handler/oauth.go:100.5,105.10 3 0",
			wantFilePath:  "github.com/example/sso/internal/handler/oauth.go",
			wantStartLine: 100,
			wantEndLine:   105,
			wantNumStmt:   3,
			wantCount:     0,
			wantCovered:   false,
			wantErr:       false,
		},
		{
			name:    "invalid format - missing fields",
			line:    "invalid line",
			wantErr: true,
		},
		{
			name:    "invalid format - bad location",
			line:    "badfile 2 5",
			wantErr: true,
		},
		{
			name:    "invalid format - bad line number",
			line:    "file.go:abc.2,47.3 2 5",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := analyzer.parseCoverageLine(tt.line)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if entry.filePath != tt.wantFilePath {
				t.Errorf("filePath = %v, want %v", entry.filePath, tt.wantFilePath)
			}
			if entry.startLine != tt.wantStartLine {
				t.Errorf("startLine = %v, want %v", entry.startLine, tt.wantStartLine)
			}
			if entry.endLine != tt.wantEndLine {
				t.Errorf("endLine = %v, want %v", entry.endLine, tt.wantEndLine)
			}
			if entry.numStmt != tt.wantNumStmt {
				t.Errorf("numStmt = %v, want %v", entry.numStmt, tt.wantNumStmt)
			}
			if entry.count != tt.wantCount {
				t.Errorf("count = %v, want %v", entry.count, tt.wantCount)
			}
			if entry.isCovered != tt.wantCovered {
				t.Errorf("isCovered = %v, want %v", entry.isCovered, tt.wantCovered)
			}
		})
	}
}

// ============================================================================
// 测试: Analyze
// ============================================================================

func TestAnalyze(t *testing.T) {
	analyzer := NewCoverageAnalyzer(80.0, []string{"internal/service/auth.go"})

	// 创建临时覆盖率文件
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "coverage.out")

	// 写入测试数据
	content := `mode: set
github.com/example/sso/internal/service/auth.go:10.1,12.2 2 1
github.com/example/sso/internal/service/auth.go:15.1,17.2 2 0
github.com/example/sso/internal/handler/user.go:20.1,22.2 2 1
`
	if err := os.WriteFile(profilePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// 执行分析
	report, err := analyzer.Analyze(profilePath)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	// 验证结果
	if report.TotalStatements != 6 {
		t.Errorf("TotalStatements = %v, want 6", report.TotalStatements)
	}
	if report.CoveredStatements != 4 {
		t.Errorf("CoveredStatements = %v, want 4", report.CoveredStatements)
	}

	expectedCoverage := (4.0 / 6.0) * 100.0
	if report.OverallCoverage < expectedCoverage-0.01 || report.OverallCoverage > expectedCoverage+0.01 {
		t.Errorf("OverallCoverage = %v, want ~%v", report.OverallCoverage, expectedCoverage)
	}

	// 验证文件详情
	if len(report.FileDetails) != 2 {
		t.Errorf("FileDetails count = %v, want 2", len(report.FileDetails))
	}
}

func TestAnalyze_FileNotFound(t *testing.T) {
	analyzer := NewCoverageAnalyzer(80.0, nil)

	_, err := analyzer.Analyze("/nonexistent/file.out")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestAnalyze_InvalidFormat(t *testing.T) {
	analyzer := NewCoverageAnalyzer(80.0, nil)

	// 创建临时文件，内容格式无效
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "invalid.out")

	content := `invalid content without mode line`
	if err := os.WriteFile(profilePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := analyzer.Analyze(profilePath)
	if err == nil {
		t.Error("expected error for invalid format, got nil")
	}
}

// ============================================================================
// 测试: EnforceThreshold
// ============================================================================

func TestEnforceThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		coverage  float64
		wantErr   bool
	}{
		{
			name:      "coverage meets threshold",
			threshold: 80.0,
			coverage:  85.0,
			wantErr:   false,
		},
		{
			name:      "coverage equals threshold",
			threshold: 80.0,
			coverage:  80.0,
			wantErr:   false,
		},
		{
			name:      "coverage below threshold",
			threshold: 80.0,
			coverage:  75.0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewCoverageAnalyzer(tt.threshold, nil)
			report := &CoverageReport{
				OverallCoverage: tt.coverage,
			}

			err := analyzer.EnforceThreshold(report)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// ============================================================================
// 测试: IdentifyCriticalGaps
// ============================================================================

func TestIdentifyCriticalGaps(t *testing.T) {
	analyzer := NewCoverageAnalyzer(80.0, []string{
		"internal/service/auth.go",
		"internal/service/mfa.go",
		"internal/handler/user.go",
	})

	report := &CoverageReport{
		FileDetails: map[string]*FileDetail{
			"github.com/example/sso/internal/service/auth.go": {
				FilePath:       "github.com/example/sso/internal/service/auth.go",
				UncoveredLines: []int{45, 46, 47},
			},
			"github.com/example/sso/internal/service/mfa.go": {
				FilePath:       "github.com/example/sso/internal/service/mfa.go",
				UncoveredLines: []int{100, 101},
			},
			"github.com/example/sso/internal/handler/user.go": {
				FilePath:       "github.com/example/sso/internal/handler/user.go",
				UncoveredLines: []int{},
			},
			"github.com/example/sso/internal/util/helper.go": {
				FilePath:       "github.com/example/sso/internal/util/helper.go",
				UncoveredLines: []int{50},
			},
		},
	}

	gaps := analyzer.IdentifyCriticalGaps(report)

	// 应该只包含关键路径文件
	// auth.go 和 mfa.go 有未覆盖的行，user.go 没有未覆盖的行
	// helper.go 不是关键路径
	if len(gaps) != 2 {
		t.Errorf("expected 2 critical gaps, got %v", len(gaps))
	}

	// 验证第一个gap是auth.go
	foundAuth := false
	foundMFA := false
	for _, gap := range gaps {
		if gap.File == "github.com/example/sso/internal/service/auth.go" {
			foundAuth = true
			if gap.Criticality != CriticalityHigh {
				t.Errorf("auth.go criticality = %v, want %v", gap.Criticality, CriticalityHigh)
			}
			if len(gap.Lines) != 3 {
				t.Errorf("auth.go uncovered lines = %v, want 3", len(gap.Lines))
			}
		}
		if gap.File == "github.com/example/sso/internal/service/mfa.go" {
			foundMFA = true
			if gap.Criticality != CriticalityHigh {
				t.Errorf("mfa.go criticality = %v, want %v", gap.Criticality, CriticalityHigh)
			}
		}
	}

	if !foundAuth {
		t.Error("expected to find auth.go in critical gaps")
	}
	if !foundMFA {
		t.Error("expected to find mfa.go in critical gaps")
	}
}

// ============================================================================
// 测试: getCriticality
// ============================================================================

func TestGetCriticality(t *testing.T) {
	analyzer := NewCoverageAnalyzer(80.0, []string{
		"internal/service/auth.go",
		"internal/service/user.go",
		"internal/handler/oauth.go",
	})

	tests := []struct {
		name     string
		filePath string
		want     CriticalityLevel
	}{
		{
			name:     "high criticality - auth",
			filePath: "github.com/example/sso/internal/service/auth.go",
			want:     CriticalityHigh,
		},
		{
			name:     "high criticality - mfa",
			filePath: "github.com/example/sso/internal/service/mfa.go",
			want:     "", // mfa.go 不在关键路径列表中
		},
		{
			name:     "high criticality - oauth",
			filePath: "github.com/example/sso/internal/handler/oauth.go",
			want:     CriticalityHigh,
		},
		{
			name:     "medium criticality - user",
			filePath: "github.com/example/sso/internal/service/user.go",
			want:     CriticalityMedium,
		},
		{
			name:     "not critical",
			filePath: "github.com/example/sso/internal/util/helper.go",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := analyzer.getCriticality(tt.filePath)
			if got != tt.want {
				t.Errorf("getCriticality(%v) = %v, want %v", tt.filePath, got, tt.want)
			}
		})
	}
}

// ============================================================================
// 测试: contains
// ============================================================================

func TestContains(t *testing.T) {
	tests := []struct {
		name  string
		slice []int
		item  int
		want  bool
	}{
		{
			name:  "contains item",
			slice: []int{1, 2, 3, 4, 5},
			item:  3,
			want:  true,
		},
		{
			name:  "does not contain item",
			slice: []int{1, 2, 3, 4, 5},
			item:  10,
			want:  false,
		},
		{
			name:  "empty slice",
			slice: []int{},
			item:  1,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.slice, tt.item)
			if got != tt.want {
				t.Errorf("contains(%v, %v) = %v, want %v", tt.slice, tt.item, got, tt.want)
			}
		})
	}
}

// ============================================================================
// 测试: 完整的覆盖率分析流程
// ============================================================================

func TestFullCoverageAnalysisWorkflow(t *testing.T) {
	// 创建分析器，设置80%阈值
	analyzer := NewCoverageAnalyzer(80.0, []string{
		"internal/service/auth.go",
		"internal/handler/auth.go",
	})

	// 创建临时覆盖率文件
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "coverage.out")

	// 模拟覆盖率数据：总体覆盖率 75%（低于阈值）
	content := `mode: set
github.com/example/sso/internal/service/auth.go:10.1,12.2 2 1
github.com/example/sso/internal/service/auth.go:15.1,17.2 2 0
github.com/example/sso/internal/handler/auth.go:20.1,22.2 2 1
github.com/example/sso/internal/handler/auth.go:25.1,27.2 2 0
github.com/example/sso/internal/util/helper.go:30.1,32.2 2 1
github.com/example/sso/internal/util/helper.go:35.1,37.2 2 1
`
	if err := os.WriteFile(profilePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Step 1: 分析覆盖率
	report, err := analyzer.Analyze(profilePath)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	// Step 2: 验证报告内容
	expectedCoverage := (8.0 / 12.0) * 100.0 // 66.67%
	if report.OverallCoverage < expectedCoverage-1.0 || report.OverallCoverage > expectedCoverage+1.0 {
		t.Errorf("OverallCoverage = %v, want ~%v", report.OverallCoverage, expectedCoverage)
	}

	// Step 3: 检查阈值（应该失败，因为覆盖率低于80%）
	err = analyzer.EnforceThreshold(report)
	if err == nil {
		t.Error("expected threshold enforcement to fail, got nil")
	}

	// Step 4: 识别关键路径缺口
	gaps := analyzer.IdentifyCriticalGaps(report)
	if len(gaps) != 2 {
		t.Errorf("expected 2 critical gaps, got %v", len(gaps))
	}

	// 验证关键路径是高优先级
	for _, gap := range gaps {
		if gap.Criticality != CriticalityHigh {
			t.Errorf("expected High criticality for %v, got %v", gap.File, gap.Criticality)
		}
		if len(gap.Lines) == 0 {
			t.Errorf("expected uncovered lines for %v", gap.File)
		}
	}

	// Step 5: 验证覆盖率缺口计算
	expectedDeficit := 80.0 - report.OverallCoverage
	if report.CoverageDeficit < expectedDeficit-1.0 || report.CoverageDeficit > expectedDeficit+1.0 {
		t.Errorf("CoverageDeficit = %v, want ~%v", report.CoverageDeficit, expectedDeficit)
	}
}
