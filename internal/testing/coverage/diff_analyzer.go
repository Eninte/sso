// Package coverage 覆盖率差异分析工具
package coverage

import (
	"fmt"
	"sort"
)

// ============================================================================
// DiffAnalyzer 类型定义
// ============================================================================

// DiffAnalyzer 覆盖率差异分析器
type DiffAnalyzer struct{}

// NewDiffAnalyzer 创建差异分析器
func NewDiffAnalyzer() *DiffAnalyzer {
	return &DiffAnalyzer{}
}

// ============================================================================
// 差异分析方法
// ============================================================================

// CompareCoverage 对比两个覆盖率报告
// 参数：
//   - current: 当前覆盖率报告
//   - previous: 之前的覆盖率报告
//
// 返回：
//   - *CoverageDiff: 覆盖率差异报告
func (da *DiffAnalyzer) CompareCoverage(current, previous *CoverageReport) *CoverageDiff {
	diff := &CoverageDiff{
		Current:  current,
		Previous: previous,
		Overall:  da.compareOverall(current, previous),
		Packages: da.comparePackages(current, previous),
		Files:    da.compareFiles(current, previous),
	}

	// 计算总体改进/退步
	diff.Summary = da.generateSummary(diff)

	return diff
}

// compareOverall 对比整体覆盖率
func (da *DiffAnalyzer) compareOverall(current, previous *CoverageReport) OverallDiff {
	diff := OverallDiff{
		CurrentCoverage:  current.OverallCoverage,
		PreviousCoverage: previous.OverallCoverage,
		Delta:            current.OverallCoverage - previous.OverallCoverage,
		StatementsAdded:  current.TotalStatements - previous.TotalStatements,
		CoverageAdded:    current.CoveredStatements - previous.CoveredStatements,
	}

	// 判断趋势
	if diff.Delta > 0 {
		diff.Trend = TrendImproved
	} else if diff.Delta < 0 {
		diff.Trend = TrendRegressed
	} else {
		diff.Trend = TrendUnchanged
	}

	return diff
}

// comparePackages 对比包级别覆盖率
func (da *DiffAnalyzer) comparePackages(current, previous *CoverageReport) []PackageDiff {
	diffs := make([]PackageDiff, 0)

	// 所有包的集合
	allPackages := make(map[string]bool)
	for pkg := range current.PackageCoverage {
		allPackages[pkg] = true
	}
	for pkg := range previous.PackageCoverage {
		allPackages[pkg] = true
	}

	// 对比每个包
	for pkg := range allPackages {
		currentCov, currentExists := current.PackageCoverage[pkg]
		previousCov, previousExists := previous.PackageCoverage[pkg]

		var diff PackageDiff
		diff.PackagePath = pkg

		if currentExists && previousExists {
			// 包存在于两个报告中
			diff.CurrentCoverage = currentCov
			diff.PreviousCoverage = previousCov
			diff.Delta = currentCov - previousCov
			diff.Status = PackageStatusChanged

			if diff.Delta > 0 {
				diff.Trend = TrendImproved
			} else if diff.Delta < 0 {
				diff.Trend = TrendRegressed
			} else {
				diff.Trend = TrendUnchanged
			}
		} else if currentExists {
			// 新增的包
			diff.CurrentCoverage = currentCov
			diff.PreviousCoverage = 0
			diff.Delta = currentCov
			diff.Status = PackageStatusNew
			diff.Trend = TrendImproved // 新增代码默认为改进
		} else {
			// 删除的包
			diff.CurrentCoverage = 0
			diff.PreviousCoverage = previousCov
			diff.Delta = -previousCov
			diff.Status = PackageStatusRemoved
			diff.Trend = TrendUnchanged // 删除代码不算退步
		}

		diffs = append(diffs, diff)
	}

	// 按Delta排序（从大到小，先显示改进最大的）
	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Delta > diffs[j].Delta
	})

	return diffs
}

// compareFiles 对比文件级别覆盖率
func (da *DiffAnalyzer) compareFiles(current, previous *CoverageReport) []FileDiff {
	diffs := make([]FileDiff, 0)

	// 所有文件的集合
	allFiles := make(map[string]bool)
	for file := range current.FileDetails {
		allFiles[file] = true
	}
	for file := range previous.FileDetails {
		allFiles[file] = true
	}

	// 对比每个文件
	for file := range allFiles {
		currentDetail, currentExists := current.FileDetails[file]
		previousDetail, previousExists := previous.FileDetails[file]

		var diff FileDiff
		diff.FilePath = file

		if currentExists && previousExists {
			// 文件存在于两个报告中
			diff.CurrentCoverage = currentDetail.Coverage
			diff.PreviousCoverage = previousDetail.Coverage
			diff.Delta = currentDetail.Coverage - previousDetail.Coverage
			diff.StatementsAdded = currentDetail.TotalStatements - previousDetail.TotalStatements
			diff.CoverageAdded = currentDetail.CoveredStatements - previousDetail.CoveredStatements
			diff.Status = FileStatusChanged

			if diff.Delta > 0 {
				diff.Trend = TrendImproved
			} else if diff.Delta < 0 {
				diff.Trend = TrendRegressed
			} else {
				diff.Trend = TrendUnchanged
			}
		} else if currentExists {
			// 新增的文件
			diff.CurrentCoverage = currentDetail.Coverage
			diff.PreviousCoverage = 0
			diff.Delta = currentDetail.Coverage
			diff.StatementsAdded = currentDetail.TotalStatements
			diff.CoverageAdded = currentDetail.CoveredStatements
			diff.Status = FileStatusNew
			diff.Trend = TrendImproved
		} else {
			// 删除的文件
			diff.CurrentCoverage = 0
			diff.PreviousCoverage = previousDetail.Coverage
			diff.Delta = -previousDetail.Coverage
			diff.StatementsAdded = -previousDetail.TotalStatements
			diff.CoverageAdded = -previousDetail.CoveredStatements
			diff.Status = FileStatusRemoved
			diff.Trend = TrendUnchanged
		}

		diffs = append(diffs, diff)
	}

	// 按Delta排序（从大到小）
	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Delta > diffs[j].Delta
	})

	return diffs
}

// generateSummary 生成差异摘要
func (da *DiffAnalyzer) generateSummary(diff *CoverageDiff) DiffSummary {
	summary := DiffSummary{
		OverallTrend: diff.Overall.Trend,
	}

	// 统计包变化
	for _, pkg := range diff.Packages {
		switch pkg.Status {
		case PackageStatusNew:
			summary.PackagesAdded++
		case PackageStatusRemoved:
			summary.PackagesRemoved++
		case PackageStatusChanged:
			if pkg.Trend == TrendImproved {
				summary.PackagesImproved++
			} else if pkg.Trend == TrendRegressed {
				summary.PackagesRegressed++
			}
		}
	}

	// 统计文件变化
	for _, file := range diff.Files {
		switch file.Status {
		case FileStatusNew:
			summary.FilesAdded++
		case FileStatusRemoved:
			summary.FilesRemoved++
		case FileStatusChanged:
			if file.Trend == TrendImproved {
				summary.FilesImproved++
			} else if file.Trend == TrendRegressed {
				summary.FilesRegressed++
			}
		}
	}

	return summary
}

// FormatDiff 格式化差异报告
func (da *DiffAnalyzer) FormatDiff(diff *CoverageDiff) string {
	var result string

	// 标题
	result += "=" + repeatString("=", 79) + "\n"
	result += "  Coverage Diff Report\n"
	result += "=" + repeatString("=", 79) + "\n\n"

	// 整体差异
	result += "Overall Coverage:\n"
	result += fmt.Sprintf("  Previous: %.2f%%\n", diff.Overall.PreviousCoverage)
	result += fmt.Sprintf("  Current:  %.2f%%\n", diff.Overall.CurrentCoverage)
	result += fmt.Sprintf("  Delta:    %+.2f%% ", diff.Overall.Delta)
	result += formatTrendSymbol(diff.Overall.Trend) + "\n\n"

	// 摘要
	result += "Summary:\n"
	result += fmt.Sprintf("  Packages: %d improved, %d regressed, %d added, %d removed\n",
		diff.Summary.PackagesImproved,
		diff.Summary.PackagesRegressed,
		diff.Summary.PackagesAdded,
		diff.Summary.PackagesRemoved,
	)
	result += fmt.Sprintf("  Files:    %d improved, %d regressed, %d added, %d removed\n\n",
		diff.Summary.FilesImproved,
		diff.Summary.FilesRegressed,
		diff.Summary.FilesAdded,
		diff.Summary.FilesRemoved,
	)

	// 包级别差异（只显示有变化的前10个）
	changedPackages := filterChangedPackages(diff.Packages, 10)
	if len(changedPackages) > 0 {
		result += "-" + repeatString("-", 79) + "\n"
		result += "Package Coverage Changes:\n"
		result += "-" + repeatString("-", 79) + "\n\n"
		result += fmt.Sprintf("%-50s %10s %10s %10s\n", "Package", "Previous", "Current", "Delta")
		result += repeatString("-", 80) + "\n"

		for _, pkg := range changedPackages {
			symbol := formatTrendSymbol(pkg.Trend)
			result += fmt.Sprintf("%-50s %9.2f%% %9.2f%% %+9.2f%% %s\n",
				truncateString(pkg.PackagePath, 50),
				pkg.PreviousCoverage,
				pkg.CurrentCoverage,
				pkg.Delta,
				symbol,
			)
		}
		result += "\n"
	}

	// 文件级别差异（只显示退步最大的5个）
	regressedFiles := filterRegressedFiles(diff.Files, 5)
	if len(regressedFiles) > 0 {
		result += "-" + repeatString("-", 79) + "\n"
		result += "⚠️  Top Regressed Files:\n"
		result += "-" + repeatString("-", 79) + "\n\n"

		for _, file := range regressedFiles {
			result += fmt.Sprintf("  %s: %.2f%% → %.2f%% (%+.2f%%)\n",
				file.FilePath,
				file.PreviousCoverage,
				file.CurrentCoverage,
				file.Delta,
			)
		}
		result += "\n"
	}

	return result
}

// ============================================================================
// 数据结构
// ============================================================================

// CoverageDiff 覆盖率差异报告
type CoverageDiff struct {
	Current  *CoverageReport `json:"current"`  // 当前报告
	Previous *CoverageReport `json:"previous"` // 之前的报告
	Overall  OverallDiff     `json:"overall"`  // 整体差异
	Packages []PackageDiff   `json:"packages"` // 包级别差异
	Files    []FileDiff      `json:"files"`    // 文件级别差异
	Summary  DiffSummary     `json:"summary"`  // 差异摘要
}

// OverallDiff 整体覆盖率差异
type OverallDiff struct {
	CurrentCoverage  float64 `json:"current_coverage"`  // 当前覆盖率
	PreviousCoverage float64 `json:"previous_coverage"` // 之前的覆盖率
	Delta            float64 `json:"delta"`             // 差异
	StatementsAdded  int     `json:"statements_added"`  // 新增语句数
	CoverageAdded    int     `json:"coverage_added"`    // 新增覆盖语句数
	Trend            Trend   `json:"trend"`             // 趋势
}

// PackageDiff 包级别覆盖率差异
type PackageDiff struct {
	PackagePath      string        `json:"package_path"`      // 包路径
	CurrentCoverage  float64       `json:"current_coverage"`  // 当前覆盖率
	PreviousCoverage float64       `json:"previous_coverage"` // 之前的覆盖率
	Delta            float64       `json:"delta"`             // 差异
	Status           PackageStatus `json:"status"`            // 状态
	Trend            Trend         `json:"trend"`             // 趋势
}

// FileDiff 文件级别覆盖率差异
type FileDiff struct {
	FilePath         string     `json:"file_path"`         // 文件路径
	CurrentCoverage  float64    `json:"current_coverage"`  // 当前覆盖率
	PreviousCoverage float64    `json:"previous_coverage"` // 之前的覆盖率
	Delta            float64    `json:"delta"`             // 差异
	StatementsAdded  int        `json:"statements_added"`  // 新增语句数
	CoverageAdded    int        `json:"coverage_added"`    // 新增覆盖语句数
	Status           FileStatus `json:"status"`            // 状态
	Trend            Trend      `json:"trend"`             // 趋势
}

// DiffSummary 差异摘要
type DiffSummary struct {
	OverallTrend      Trend `json:"overall_trend"`      // 整体趋势
	PackagesImproved  int   `json:"packages_improved"`  // 改进的包数
	PackagesRegressed int   `json:"packages_regressed"` // 退步的包数
	PackagesAdded     int   `json:"packages_added"`     // 新增的包数
	PackagesRemoved   int   `json:"packages_removed"`   // 删除的包数
	FilesImproved     int   `json:"files_improved"`     // 改进的文件数
	FilesRegressed    int   `json:"files_regressed"`    // 退步的文件数
	FilesAdded        int   `json:"files_added"`        // 新增的文件数
	FilesRemoved      int   `json:"files_removed"`      // 删除的文件数
}

// Trend 趋势
type Trend string

const (
	TrendImproved  Trend = "improved"  // 改进
	TrendRegressed Trend = "regressed" // 退步
	TrendUnchanged Trend = "unchanged" // 不变
)

// PackageStatus 包状态
type PackageStatus string

const (
	PackageStatusNew     PackageStatus = "new"     // 新增
	PackageStatusRemoved PackageStatus = "removed" // 删除
	PackageStatusChanged PackageStatus = "changed" // 变更
)

// FileStatus 文件状态
type FileStatus string

const (
	FileStatusNew     FileStatus = "new"     // 新增
	FileStatusRemoved FileStatus = "removed" // 删除
	FileStatusChanged FileStatus = "changed" // 变更
)

// ============================================================================
// 辅助函数
// ============================================================================

// repeatString 重复字符串
func repeatString(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 10 {
		return s[:maxLen]
	}
	prefixLen := (maxLen - 3) / 2
	suffixLen := maxLen - 3 - prefixLen
	return s[:prefixLen] + "..." + s[len(s)-suffixLen:]
}

// formatTrendSymbol 格式化趋势符号
func formatTrendSymbol(trend Trend) string {
	switch trend {
	case TrendImproved:
		return "↑"
	case TrendRegressed:
		return "↓"
	case TrendUnchanged:
		return "→"
	default:
		return "→"
	}
}

// filterChangedPackages 过滤有变化的包（取前N个）
func filterChangedPackages(packages []PackageDiff, n int) []PackageDiff {
	result := make([]PackageDiff, 0)
	for _, pkg := range packages {
		if pkg.Status == PackageStatusChanged && pkg.Delta != 0 {
			result = append(result, pkg)
			if len(result) >= n {
				break
			}
		}
	}
	return result
}

// filterRegressedFiles 过滤退步的文件（取前N个）
func filterRegressedFiles(files []FileDiff, n int) []FileDiff {
	result := make([]FileDiff, 0)
	for _, file := range files {
		if file.Trend == TrendRegressed {
			result = append(result, file)
			if len(result) >= n {
				break
			}
		}
	}
	// 按Delta从小到大排序（退步最大的在前）
	sort.Slice(result, func(i, j int) bool {
		return result[i].Delta < result[j].Delta
	})
	return result
}
