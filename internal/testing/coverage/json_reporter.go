// Package coverage JSON覆盖率报告生成器（用于CI/CD集成）
package coverage

import (
	"encoding/json"
	"io"
	"time"
)

// ============================================================================
// JSONReporter 类型定义
// ============================================================================

// JSONReporter JSON报告生成器
type JSONReporter struct {
	analyzer *CoverageAnalyzer
}

// NewJSONReporter 创建JSON报告生成器
func NewJSONReporter(analyzer *CoverageAnalyzer) *JSONReporter {
	return &JSONReporter{
		analyzer: analyzer,
	}
}

// ============================================================================
// 报告生成方法
// ============================================================================

// GenerateJSON 生成JSON覆盖率报告（用于CI/CD）
// 参数：
//   - w: 输出流
//   - report: 覆盖率报告
//
// 返回：
//   - error: 错误信息
func (jr *JSONReporter) GenerateJSON(w io.Writer, report *CoverageReport) error {
	// 准备机器可读的报告数据
	data := jr.prepareJSONData(report)

	// 编码为JSON（带缩进，便于调试）
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(data); err != nil {
		return err
	}

	return nil
}

// GenerateCompactJSON 生成紧凑JSON报告（无缩进，用于日志）
// 参数：
//   - w: 输出流
//   - report: 覆盖率报告
//
// 返回：
//   - error: 错误信息
func (jr *JSONReporter) GenerateCompactJSON(w io.Writer, report *CoverageReport) error {
	// 准备机器可读的报告数据
	data := jr.prepareJSONData(report)

	// 编码为JSON（无缩进）
	encoder := json.NewEncoder(w)

	if err := encoder.Encode(data); err != nil {
		return err
	}

	return nil
}

// prepareJSONData 准备JSON报告数据
func (jr *JSONReporter) prepareJSONData(report *CoverageReport) *JSONCoverageReport {
	// 获取包缺口信息
	deficits := jr.analyzer.GetPackagesBelowThreshold(report)

	// 获取改进计划
	var plan *RemediationPlan
	if len(deficits) > 0 {
		plan = jr.analyzer.CalculateRemediationPlan(report)
	}

	// 判断是否通过阈值
	passed := report.OverallCoverage >= jr.analyzer.threshold

	return &JSONCoverageReport{
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		Threshold:         jr.analyzer.threshold,
		Passed:            passed,
		OverallCoverage:   report.OverallCoverage,
		CoverageDeficit:   report.CoverageDeficit,
		TotalStatements:   report.TotalStatements,
		CoveredStatements: report.CoveredStatements,
		PackageCoverage:   report.PackageCoverage,
		PackagesBelow:     deficits,
		CriticalGaps:      report.UncoveredCritical,
		RemediationPlan:   plan,
		FileDetails:       jr.convertFileDetails(report.FileDetails),
	}
}

// convertFileDetails 转换文件详情为数组格式（便于处理）
func (jr *JSONReporter) convertFileDetails(details map[string]*FileDetail) []FileDetail {
	result := make([]FileDetail, 0, len(details))
	for _, detail := range details {
		result = append(result, *detail)
	}
	return result
}

// ============================================================================
// JSON数据结构
// ============================================================================

// JSONCoverageReport JSON格式的覆盖率报告
type JSONCoverageReport struct {
	Timestamp         string             `json:"timestamp"`          // 生成时间（ISO 8601格式）
	Threshold         float64            `json:"threshold"`          // 覆盖率阈值
	Passed            bool               `json:"passed"`             // 是否通过阈值检查
	OverallCoverage   float64            `json:"overall_coverage"`   // 整体覆盖率
	CoverageDeficit   float64            `json:"coverage_deficit"`   // 覆盖率缺口
	TotalStatements   int                `json:"total_statements"`   // 总语句数
	CoveredStatements int                `json:"covered_statements"` // 已覆盖语句数
	PackageCoverage   map[string]float64 `json:"package_coverage"`   // 包级别覆盖率
	PackagesBelow     []PackageDeficit   `json:"packages_below"`     // 低于阈值的包
	CriticalGaps      []UncoveredPath    `json:"critical_gaps"`      // 未覆盖的关键路径
	RemediationPlan   *RemediationPlan   `json:"remediation_plan"`   // 改进计划（可为空）
	FileDetails       []FileDetail       `json:"file_details"`       // 文件详情
}
