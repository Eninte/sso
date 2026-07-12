// Package coverage 代码覆盖率分析工具
// 提供覆盖率解析、阈值检查和关键路径检测功能
package coverage

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	apperrors "github.com/example/sso/internal/errors"
)

// ============================================================================
// 类型定义
// ============================================================================

// CriticalityLevel 关键性级别
type CriticalityLevel string

const (
	CriticalityHigh   CriticalityLevel = "High"   // 高关键性（认证、授权、MFA）
	CriticalityMedium CriticalityLevel = "Medium" // 中等关键性（用户管理、配置）
	CriticalityLow    CriticalityLevel = "Low"    // 低关键性（工具函数）
)

// UncoveredPath 未覆盖的代码路径
type UncoveredPath struct {
	File        string           `json:"file"`        // 文件路径
	Lines       []int            `json:"lines"`       // 未覆盖的行号
	Function    string           `json:"function"`    // 函数名
	Criticality CriticalityLevel `json:"criticality"` // 关键性级别
}

// CoverageReport 覆盖率报告
type CoverageReport struct {
	OverallCoverage   float64                `json:"overall_coverage"`   // 整体覆盖率
	PackageCoverage   map[string]float64     `json:"package_coverage"`   // 包级别覆盖率
	UncoveredCritical []UncoveredPath        `json:"uncovered_critical"` // 未覆盖的关键路径
	CoverageDeficit   float64                `json:"coverage_deficit"`   // 覆盖率缺口
	TotalStatements   int                    `json:"total_statements"`   // 总语句数
	CoveredStatements int                    `json:"covered_statements"` // 已覆盖语句数
	FileDetails       map[string]*FileDetail `json:"file_details"`       // 文件详情
}

// FileDetail 文件覆盖率详情
type FileDetail struct {
	FilePath          string  `json:"file_path"`          // 文件路径
	TotalStatements   int     `json:"total_statements"`   // 总语句数
	CoveredStatements int     `json:"covered_statements"` // 已覆盖语句数
	Coverage          float64 `json:"coverage"`           // 覆盖率
	UncoveredLines    []int   `json:"uncovered_lines"`    // 未覆盖的行号
}

// PackageDeficit 包覆盖率缺口
type PackageDeficit struct {
	PackagePath     string  `json:"package_path"`     // 包路径
	CurrentCoverage float64 `json:"current_coverage"` // 当前覆盖率
	Deficit         float64 `json:"deficit"`          // 覆盖率缺口
	UncoveredStmt   int     `json:"uncovered_stmt"`   // 未覆盖语句数
	TotalStmt       int     `json:"total_stmt"`       // 总语句数
	EstimatedTests  int     `json:"estimated_tests"`  // 估算需要的测试数量
}

// RemediationPlan 改进计划
type RemediationPlan struct {
	CurrentCoverage     float64         `json:"current_coverage"`      // 当前覆盖率
	TargetCoverage      float64         `json:"target_coverage"`       // 目标覆盖率
	Deficit             float64         `json:"deficit"`               // 覆盖率缺口
	TotalEstimatedTests int             `json:"total_estimated_tests"` // 总估算测试数
	PackageActions      []PackageAction `json:"package_actions"`       // 包级别改进计划
}

// PackageAction 包级别改进行动
type PackageAction struct {
	PackagePath     string   `json:"package_path"`     // 包路径
	CurrentCoverage float64  `json:"current_coverage"` // 当前覆盖率
	TargetCoverage  float64  `json:"target_coverage"`  // 目标覆盖率
	RequiredTests   int      `json:"required_tests"`   // 需要的测试数量
	Priority        string   `json:"priority"`         // 优先级 (High/Medium/Low)
	Suggestions     []string `json:"suggestions"`      // 具体建议
}

// CoverageAnalyzer 覆盖率分析器
type CoverageAnalyzer struct {
	threshold     float64  // 覆盖率阈值（例如 80.0）
	criticalPaths []string // 关键路径模式列表
}

// ============================================================================
// 构造函数
// ============================================================================

// NewCoverageAnalyzer 创建覆盖率分析器
func NewCoverageAnalyzer(threshold float64, criticalPaths []string) *CoverageAnalyzer {
	// 如果未指定关键路径，使用默认值
	if len(criticalPaths) == 0 {
		criticalPaths = []string{
			"internal/service/auth.go",
			"internal/service/mfa.go",
			"internal/service/oauth.go",
			"internal/service/token.go",
			"internal/handler/auth.go",
			"internal/handler/mfa.go",
			"internal/handler/oauth.go",
			"internal/middleware/auth.go",
		}
	}

	return &CoverageAnalyzer{
		threshold:     threshold,
		criticalPaths: criticalPaths,
	}
}

// ============================================================================
// 核心方法
// ============================================================================

// Analyze 分析覆盖率数据
// 参数：
//   - profilePath: 覆盖率文件路径（go test -coverprofile生成）
//
// 返回：
//   - *CoverageReport: 覆盖率报告
//   - error: 错误信息
func (ca *CoverageAnalyzer) Analyze(profilePath string) (*CoverageReport, error) {
	// 打开覆盖率文件
	file, err := os.Open(profilePath)
	if err != nil {
		return nil, apperrors.Wrap(
			apperrors.ErrCodeNotFound,
			"Failed to open coverage profile",
			500,
			err,
		)
	}
	defer file.Close()

	// 初始化报告
	report := &CoverageReport{
		PackageCoverage:   make(map[string]float64),
		UncoveredCritical: make([]UncoveredPath, 0),
		FileDetails:       make(map[string]*FileDetail),
	}

	// 用于统计包级别覆盖率
	packageStats := make(map[string]*packageCoverage)

	// 流式处理覆盖率数据
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// 跳过首行（mode: set/count/atomic）
		if lineNum == 1 {
			if !strings.HasPrefix(line, "mode:") {
				return nil, apperrors.New(
					apperrors.ErrCodeBadRequest,
					"Invalid coverage profile format: missing mode line",
					422,
				)
			}
			continue
		}

		// 解析覆盖率行
		// 格式: file.go:startLine.startCol,endLine.endCol numStmt count
		entry, err := ca.parseCoverageLine(line)
		if err != nil {
			return nil, apperrors.Wrap(
				apperrors.ErrCodeBadRequest,
				fmt.Sprintf("Failed to parse coverage line %d", lineNum),
				422,
				err,
			)
		}

		// 更新文件详情
		ca.updateFileDetail(report, entry)

		// 更新包统计
		ca.updatePackageStats(packageStats, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, apperrors.Wrap(
			apperrors.ErrCodeInternal,
			"Failed to read coverage profile",
			500,
			err,
		)
	}

	// 计算整体覆盖率
	ca.calculateOverallCoverage(report)

	// 计算包级别覆盖率
	ca.calculatePackageCoverage(report, packageStats)

	// 识别未覆盖的关键路径
	ca.identifyCriticalGaps(report)

	// 计算覆盖率缺口
	report.CoverageDeficit = ca.threshold - report.OverallCoverage
	if report.CoverageDeficit < 0 {
		report.CoverageDeficit = 0
	}

	return report, nil
}

// EnforceThreshold 检查覆盖率是否达到阈值
// 参数：
//   - report: 覆盖率报告
//
// 返回：
//   - error: 如果未达到阈值返回错误，否则返回 nil
func (ca *CoverageAnalyzer) EnforceThreshold(report *CoverageReport) error {
	if report.OverallCoverage < ca.threshold {
		return apperrors.New(
			apperrors.ErrCodeBadRequest,
			fmt.Sprintf("Coverage %.2f%% is below threshold %.2f%%",
				report.OverallCoverage, ca.threshold),
			422,
		)
	}
	return nil
}

// GetPackagesBlowThreshold 获取低于阈值的包列表
// 参数：
//   - report: 覆盖率报告
//
// 返回：
//   - []PackageDeficit: 低于阈值的包列表（按覆盖率从低到高排序）
func (ca *CoverageAnalyzer) GetPackagesBelowThreshold(report *CoverageReport) []PackageDeficit {
	deficits := make([]PackageDeficit, 0)

	// 统计每个包的语句数
	packageStats := make(map[string]*packageCoverage)
	for _, detail := range report.FileDetails {
		pkgPath := getPackagePath(detail.FilePath)
		stats, exists := packageStats[pkgPath]
		if !exists {
			stats = &packageCoverage{}
			packageStats[pkgPath] = stats
		}
		stats.totalStmt += detail.TotalStatements
		stats.coveredStmt += detail.CoveredStatements
	}

	// 找出低于阈值的包
	for pkgPath, stats := range packageStats {
		if stats.totalStmt == 0 {
			continue
		}

		coverage := float64(stats.coveredStmt) / float64(stats.totalStmt) * 100.0
		if coverage < ca.threshold {
			deficit := ca.threshold - coverage
			uncoveredStmt := stats.totalStmt - stats.coveredStmt

			// 估算需要的测试数量（假设每个测试覆盖5-10行代码）
			estimatedTests := (uncoveredStmt + 6) / 7 // 平均7行/测试

			deficits = append(deficits, PackageDeficit{
				PackagePath:     pkgPath,
				CurrentCoverage: coverage,
				Deficit:         deficit,
				UncoveredStmt:   uncoveredStmt,
				TotalStmt:       stats.totalStmt,
				EstimatedTests:  estimatedTests,
			})
		}
	}

	// 按覆盖率从低到高排序
	sortPackageDeficits(deficits)

	return deficits
}

// CalculateRemediationPlan 计算改进计划
// 参数：
//   - report: 覆盖率报告
//
// 返回：
//   - *RemediationPlan: 改进计划
func (ca *CoverageAnalyzer) CalculateRemediationPlan(report *CoverageReport) *RemediationPlan {
	plan := &RemediationPlan{
		CurrentCoverage: report.OverallCoverage,
		TargetCoverage:  ca.threshold,
		Deficit:         report.CoverageDeficit,
		PackageActions:  make([]PackageAction, 0),
	}

	// 如果已达到阈值，无需改进
	if report.CoverageDeficit <= 0 {
		plan.TotalEstimatedTests = 0
		return plan
	}

	// 获取低于阈值的包
	deficits := ca.GetPackagesBelowThreshold(report)

	// 为每个包生成改进建议
	totalTests := 0
	for _, deficit := range deficits {
		action := PackageAction{
			PackagePath:     deficit.PackagePath,
			CurrentCoverage: deficit.CurrentCoverage,
			TargetCoverage:  ca.threshold,
			RequiredTests:   deficit.EstimatedTests,
			Priority:        ca.getPackagePriority(deficit.PackagePath),
			Suggestions:     ca.generateSuggestions(deficit),
		}
		plan.PackageActions = append(plan.PackageActions, action)
		totalTests += deficit.EstimatedTests
	}

	plan.TotalEstimatedTests = totalTests

	return plan
}

// IdentifyCriticalGaps 识别未覆盖的关键路径
// 参数：
//   - report: 覆盖率报告
//
// 返回：
//   - []UncoveredPath: 未覆盖的关键路径列表
func (ca *CoverageAnalyzer) IdentifyCriticalGaps(report *CoverageReport) []UncoveredPath {
	gaps := make([]UncoveredPath, 0)

	for _, detail := range report.FileDetails {
		// 检查是否是关键路径
		criticality := ca.getCriticality(detail.FilePath)
		if criticality == "" {
			continue // 不是关键路径
		}

		// 如果有未覆盖的行，添加到列表
		if len(detail.UncoveredLines) > 0 {
			gaps = append(gaps, UncoveredPath{
				File:        detail.FilePath,
				Lines:       detail.UncoveredLines,
				Function:    "", // 需要进一步解析代码才能获取函数名
				Criticality: criticality,
			})
		}
	}

	return gaps
}

// ============================================================================
// 辅助方法
// ============================================================================

// coverageEntry 覆盖率条目
type coverageEntry struct {
	filePath  string
	startLine int
	endLine   int
	numStmt   int
	count     int
	isCovered bool
}

// packageCoverage 包级别覆盖率统计
type packageCoverage struct {
	totalStmt   int
	coveredStmt int
}

// parseCoverageLine 解析覆盖率行
// 格式: file.go:startLine.startCol,endLine.endCol numStmt count
func (ca *CoverageAnalyzer) parseCoverageLine(line string) (*coverageEntry, error) {
	// 分割行
	parts := strings.Fields(line)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid coverage line format: expected 3 fields, got %d", len(parts))
	}

	// 解析文件路径和位置信息
	// 格式: file.go:startLine.startCol,endLine.endCol
	locationParts := strings.Split(parts[0], ":")
	if len(locationParts) != 2 {
		return nil, fmt.Errorf("invalid location format: %s", parts[0])
	}
	filePath := locationParts[0]

	// 解析行号范围
	rangeParts := strings.Split(locationParts[1], ",")
	if len(rangeParts) != 2 {
		return nil, fmt.Errorf("invalid range format: %s", locationParts[1])
	}

	// 解析起始行
	startParts := strings.Split(rangeParts[0], ".")
	if len(startParts) != 2 {
		return nil, fmt.Errorf("invalid start position: %s", rangeParts[0])
	}
	startLine, err := strconv.Atoi(startParts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid start line: %w", err)
	}

	// 解析结束行
	endParts := strings.Split(rangeParts[1], ".")
	if len(endParts) != 2 {
		return nil, fmt.Errorf("invalid end position: %s", rangeParts[1])
	}
	endLine, err := strconv.Atoi(endParts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid end line: %w", err)
	}

	// 解析语句数
	numStmt, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid statement count: %w", err)
	}

	// 解析执行次数
	count, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid execution count: %w", err)
	}

	return &coverageEntry{
		filePath:  filePath,
		startLine: startLine,
		endLine:   endLine,
		numStmt:   numStmt,
		count:     count,
		isCovered: count > 0,
	}, nil
}

// updateFileDetail 更新文件详情
func (ca *CoverageAnalyzer) updateFileDetail(report *CoverageReport, entry *coverageEntry) {
	detail, exists := report.FileDetails[entry.filePath]
	if !exists {
		detail = &FileDetail{
			FilePath:       entry.filePath,
			UncoveredLines: make([]int, 0),
		}
		report.FileDetails[entry.filePath] = detail
	}

	// 更新统计
	detail.TotalStatements += entry.numStmt
	if entry.isCovered {
		detail.CoveredStatements += entry.numStmt
	} else {
		// 记录未覆盖的行号
		for line := entry.startLine; line <= entry.endLine; line++ {
			if !contains(detail.UncoveredLines, line) {
				detail.UncoveredLines = append(detail.UncoveredLines, line)
			}
		}
	}

	// 计算文件覆盖率
	if detail.TotalStatements > 0 {
		detail.Coverage = float64(detail.CoveredStatements) / float64(detail.TotalStatements) * 100.0
	}
}

// updatePackageStats 更新包统计
func (ca *CoverageAnalyzer) updatePackageStats(packageStats map[string]*packageCoverage, entry *coverageEntry) {
	// 从文件路径提取包名
	// 例如: github.com/example/sso/internal/service/auth.go -> internal/service
	packagePath := filepath.Dir(entry.filePath)

	stats, exists := packageStats[packagePath]
	if !exists {
		stats = &packageCoverage{}
		packageStats[packagePath] = stats
	}

	stats.totalStmt += entry.numStmt
	if entry.isCovered {
		stats.coveredStmt += entry.numStmt
	}
}

// calculateOverallCoverage 计算整体覆盖率
func (ca *CoverageAnalyzer) calculateOverallCoverage(report *CoverageReport) {
	totalStmt := 0
	coveredStmt := 0

	for _, detail := range report.FileDetails {
		totalStmt += detail.TotalStatements
		coveredStmt += detail.CoveredStatements
	}

	report.TotalStatements = totalStmt
	report.CoveredStatements = coveredStmt

	if totalStmt > 0 {
		report.OverallCoverage = float64(coveredStmt) / float64(totalStmt) * 100.0
	}
}

// calculatePackageCoverage 计算包级别覆盖率
func (ca *CoverageAnalyzer) calculatePackageCoverage(report *CoverageReport, packageStats map[string]*packageCoverage) {
	for pkgPath, stats := range packageStats {
		if stats.totalStmt > 0 {
			coverage := float64(stats.coveredStmt) / float64(stats.totalStmt) * 100.0
			report.PackageCoverage[pkgPath] = coverage
		}
	}
}

// identifyCriticalGaps 识别未覆盖的关键路径
func (ca *CoverageAnalyzer) identifyCriticalGaps(report *CoverageReport) {
	report.UncoveredCritical = ca.IdentifyCriticalGaps(report)
}

// getCriticality 获取文件的关键性级别
func (ca *CoverageAnalyzer) getCriticality(filePath string) CriticalityLevel {
	// 检查是否匹配关键路径模式
	for _, pattern := range ca.criticalPaths {
		if strings.Contains(filePath, pattern) {
			// 判断关键性级别
			if strings.Contains(filePath, "auth") ||
				strings.Contains(filePath, "mfa") ||
				strings.Contains(filePath, "oauth") ||
				strings.Contains(filePath, "token") {
				return CriticalityHigh
			}
			if strings.Contains(filePath, "user") ||
				strings.Contains(filePath, "admin") ||
				strings.Contains(filePath, "config") {
				return CriticalityMedium
			}
			return CriticalityLow
		}
	}
	return "" // 不是关键路径
}

// contains 检查切片是否包含元素
func contains(slice []int, item int) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

// getPackagePath 从文件路径提取包路径
func getPackagePath(filePath string) string {
	// 从文件路径提取包名
	// 例如: github.com/example/sso/internal/service/auth.go -> internal/service
	idx := strings.LastIndex(filePath, "/")
	if idx == -1 {
		return "."
	}
	return filePath[:idx]
}

// sortPackageDeficits 按覆盖率从低到高排序包缺口列表
func sortPackageDeficits(deficits []PackageDeficit) {
	// 简单的冒泡排序（包数量较少，性能足够）
	n := len(deficits)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if deficits[j].CurrentCoverage > deficits[j+1].CurrentCoverage {
				deficits[j], deficits[j+1] = deficits[j+1], deficits[j]
			}
		}
	}
}

// getPackagePriority 获取包的优先级
func (ca *CoverageAnalyzer) getPackagePriority(packagePath string) string {
	// 首先根据内容判断是否是高优先级
	if strings.Contains(packagePath, "auth") ||
		strings.Contains(packagePath, "mfa") ||
		strings.Contains(packagePath, "oauth") ||
		strings.Contains(packagePath, "token") {
		// 检查是否在关键路径中
		for _, pattern := range ca.criticalPaths {
			patternDir := pattern
			if idx := strings.LastIndex(pattern, "/"); idx != -1 {
				if strings.Contains(pattern[idx+1:], ".") {
					patternDir = pattern[:idx]
				}
			}
			if strings.Contains(packagePath, patternDir) || strings.Contains(patternDir, packagePath) {
				return "High"
			}
		}
	}

	// 检查是否是中等优先级
	if strings.Contains(packagePath, "user") ||
		strings.Contains(packagePath, "admin") ||
		strings.Contains(packagePath, "config") ||
		strings.Contains(packagePath, "/handler") ||
		strings.Contains(packagePath, "/service") {
		return "Medium"
	}

	return "Low"
}

// generateSuggestions 生成改进建议
func (ca *CoverageAnalyzer) generateSuggestions(deficit PackageDeficit) []string {
	suggestions := make([]string, 0)

	// 基础建议
	suggestions = append(suggestions, fmt.Sprintf(
		"Add %d test case(s) to cover %d uncovered statements",
		deficit.EstimatedTests,
		deficit.UncoveredStmt,
	))

	// 根据包类型提供具体建议
	pkgPath := deficit.PackagePath

	if strings.Contains(pkgPath, "/handler") {
		suggestions = append(suggestions,
			"Focus on HTTP handler test cases with table-driven tests",
			"Test both success and error response paths",
			"Verify request validation and error handling",
		)
	} else if strings.Contains(pkgPath, "/service") {
		suggestions = append(suggestions,
			"Add unit tests for business logic functions",
			"Test error conditions and edge cases",
			"Mock Store layer dependencies using internal/store/mock",
		)
	} else if strings.Contains(pkgPath, "/store") {
		suggestions = append(suggestions,
			"Add database integration tests",
			"Test CRUD operations and error handling",
			"Verify transaction rollback behavior",
		)
	} else if strings.Contains(pkgPath, "/middleware") {
		suggestions = append(suggestions,
			"Test middleware with mock HTTP handlers",
			"Verify request/response transformation",
			"Test authentication and authorization logic",
		)
	} else {
		suggestions = append(suggestions,
			"Review uncovered code paths and add relevant tests",
			"Consider table-driven tests for multiple scenarios",
		)
	}

	// 高缺口的额外建议
	if deficit.Deficit > 20 {
		suggestions = append(suggestions,
			fmt.Sprintf("⚠️  High deficit (%.1f%%) - consider breaking down complex functions", deficit.Deficit),
		)
	}

	return suggestions
}
