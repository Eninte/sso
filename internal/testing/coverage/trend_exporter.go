// Package coverage 覆盖率趋势数据导出工具（为仪表板准备数据）
package coverage

import (
	"encoding/json"
	"io"
	"time"
)

// ============================================================================
// TrendExporter 类型定义
// ============================================================================

// TrendExporter 趋势数据导出器
type TrendExporter struct{}

// NewTrendExporter 创建趋势数据导出器
func NewTrendExporter() *TrendExporter {
	return &TrendExporter{}
}

// ============================================================================
// 趋势数据导出方法
// ============================================================================

// ExportTrendData 导出趋势数据（用于仪表板可视化）
// 参数：
//   - w: 输出流
//   - dataPoints: 趋势数据点列表（按时间排序）
//
// 返回：
//   - error: 错误信息
func (te *TrendExporter) ExportTrendData(w io.Writer, dataPoints []TrendDataPoint) error {
	// 计算趋势指标
	trend := te.calculateTrend(dataPoints)

	// 导出数据
	data := &TrendExportData{
		DataPoints: dataPoints,
		Metrics:    trend,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// 编码为JSON（带缩进）
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(data); err != nil {
		return err
	}

	return nil
}

// CreateDataPoint 从覆盖率报告创建趋势数据点
// 参数：
//   - report: 覆盖率报告
//   - timestamp: 时间戳（可选，默认当前时间）
//   - metadata: 附加元数据（可选，例如git commit SHA）
//
// 返回：
//   - TrendDataPoint: 趋势数据点
func (te *TrendExporter) CreateDataPoint(report *CoverageReport, timestamp time.Time, metadata map[string]string) TrendDataPoint {
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	if metadata == nil {
		metadata = make(map[string]string)
	}

	return TrendDataPoint{
		Timestamp:         timestamp.UTC().Format(time.RFC3339),
		OverallCoverage:   report.OverallCoverage,
		TotalStatements:   report.TotalStatements,
		CoveredStatements: report.CoveredStatements,
		PackageCoverage:   report.PackageCoverage,
		CriticalGapsCount: len(report.UncoveredCritical),
		Metadata:          metadata,
	}
}

// calculateTrend 计算趋势指标
func (te *TrendExporter) calculateTrend(dataPoints []TrendDataPoint) TrendMetrics {
	metrics := TrendMetrics{}

	if len(dataPoints) == 0 {
		return metrics
	}

	// 当前覆盖率
	metrics.CurrentCoverage = dataPoints[len(dataPoints)-1].OverallCoverage

	// 如果只有一个数据点，无法计算趋势
	if len(dataPoints) == 1 {
		metrics.DeltaLastWeek = 0
		metrics.DeltaLastMonth = 0
		metrics.AverageCoverage = metrics.CurrentCoverage
		metrics.Trend = "stable"
		return metrics
	}

	// 最早覆盖率
	firstCoverage := dataPoints[0].OverallCoverage

	// 计算平均覆盖率
	sum := 0.0
	for _, dp := range dataPoints {
		sum += dp.OverallCoverage
	}
	metrics.AverageCoverage = sum / float64(len(dataPoints))

	// 计算最近一周的变化（如果有7天以上的数据）
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)
	weekAgoPoint := te.findNearestDataPoint(dataPoints, weekAgo)
	if weekAgoPoint != nil {
		metrics.DeltaLastWeek = metrics.CurrentCoverage - weekAgoPoint.OverallCoverage
	}

	// 计算最近一个月的变化（如果有30天以上的数据）
	monthAgo := now.AddDate(0, -1, 0)
	monthAgoPoint := te.findNearestDataPoint(dataPoints, monthAgo)
	if monthAgoPoint != nil {
		metrics.DeltaLastMonth = metrics.CurrentCoverage - monthAgoPoint.OverallCoverage
	}

	// 判断整体趋势
	totalDelta := metrics.CurrentCoverage - firstCoverage
	if totalDelta > 1.0 {
		metrics.Trend = "improving"
	} else if totalDelta < -1.0 {
		metrics.Trend = "declining"
	} else {
		metrics.Trend = "stable"
	}

	// 计算最高和最低覆盖率
	metrics.PeakCoverage = metrics.CurrentCoverage
	metrics.LowestCoverage = metrics.CurrentCoverage
	for _, dp := range dataPoints {
		if dp.OverallCoverage > metrics.PeakCoverage {
			metrics.PeakCoverage = dp.OverallCoverage
		}
		if dp.OverallCoverage < metrics.LowestCoverage {
			metrics.LowestCoverage = dp.OverallCoverage
		}
	}

	return metrics
}

// findNearestDataPoint 查找最接近指定时间的数据点
func (te *TrendExporter) findNearestDataPoint(dataPoints []TrendDataPoint, targetTime time.Time) *TrendDataPoint {
	var nearest *TrendDataPoint
	var minDiff time.Duration

	for i := range dataPoints {
		dp := &dataPoints[i]
		t, err := time.Parse(time.RFC3339, dp.Timestamp)
		if err != nil {
			continue
		}

		diff := t.Sub(targetTime)
		if diff < 0 {
			diff = -diff
		}

		if nearest == nil || diff < minDiff {
			nearest = dp
			minDiff = diff
		}
	}

	return nearest
}

// ExportChartData 导出图表数据（简化格式，用于前端可视化）
// 参数：
//   - w: 输出流
//   - dataPoints: 趋势数据点列表
//
// 返回：
//   - error: 错误信息
func (te *TrendExporter) ExportChartData(w io.Writer, dataPoints []TrendDataPoint) error {
	// 转换为图表格式
	chartData := &ChartData{
		Labels:   make([]string, len(dataPoints)),
		Coverage: make([]float64, len(dataPoints)),
	}

	for i, dp := range dataPoints {
		// 解析时间戳为简短格式
		t, err := time.Parse(time.RFC3339, dp.Timestamp)
		if err != nil {
			chartData.Labels[i] = dp.Timestamp
		} else {
			chartData.Labels[i] = t.Format("2006-01-02")
		}
		chartData.Coverage[i] = dp.OverallCoverage
	}

	// 编码为JSON
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(chartData); err != nil {
		return err
	}

	return nil
}

// ============================================================================
// 数据结构
// ============================================================================

// TrendDataPoint 趋势数据点
type TrendDataPoint struct {
	Timestamp         string             `json:"timestamp"`           // 时间戳（ISO 8601格式）
	OverallCoverage   float64            `json:"overall_coverage"`    // 整体覆盖率
	TotalStatements   int                `json:"total_statements"`    // 总语句数
	CoveredStatements int                `json:"covered_statements"`  // 已覆盖语句数
	PackageCoverage   map[string]float64 `json:"package_coverage"`    // 包级别覆盖率
	CriticalGapsCount int                `json:"critical_gaps_count"` // 关键路径未覆盖数量
	Metadata          map[string]string  `json:"metadata,omitempty"`  // 附加元数据（例如git commit SHA）
}

// TrendExportData 趋势导出数据
type TrendExportData struct {
	DataPoints []TrendDataPoint `json:"data_points"` // 数据点列表
	Metrics    TrendMetrics     `json:"metrics"`     // 趋势指标
	ExportedAt string           `json:"exported_at"` // 导出时间
}

// TrendMetrics 趋势指标
type TrendMetrics struct {
	CurrentCoverage float64 `json:"current_coverage"` // 当前覆盖率
	AverageCoverage float64 `json:"average_coverage"` // 平均覆盖率
	PeakCoverage    float64 `json:"peak_coverage"`    // 最高覆盖率
	LowestCoverage  float64 `json:"lowest_coverage"`  // 最低覆盖率
	DeltaLastWeek   float64 `json:"delta_last_week"`  // 最近一周变化
	DeltaLastMonth  float64 `json:"delta_last_month"` // 最近一个月变化
	Trend           string  `json:"trend"`            // 趋势（improving/declining/stable）
}

// ChartData 图表数据（简化格式，用于前端）
type ChartData struct {
	Labels   []string  `json:"labels"`   // 时间标签
	Coverage []float64 `json:"coverage"` // 覆盖率数据
}
