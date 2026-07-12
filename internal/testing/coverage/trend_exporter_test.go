// Package coverage 趋势导出器测试
package coverage

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

// ============================================================================
// TrendExporter 测试
// ============================================================================

func TestTrendExporter_ExportTrendData(t *testing.T) {
	exporter := NewTrendExporter()

	// 创建趋势数据点
	dataPoints := []TrendDataPoint{
		{
			Timestamp:         "2024-01-01T00:00:00Z",
			OverallCoverage:   75.0,
			TotalStatements:   1000,
			CoveredStatements: 750,
			PackageCoverage:   map[string]float64{"pkg1": 75.0},
			CriticalGapsCount: 5,
			Metadata:          map[string]string{"commit": "abc123"},
		},
		{
			Timestamp:         "2024-01-02T00:00:00Z",
			OverallCoverage:   80.0,
			TotalStatements:   1100,
			CoveredStatements: 880,
			PackageCoverage:   map[string]float64{"pkg1": 80.0},
			CriticalGapsCount: 3,
			Metadata:          map[string]string{"commit": "def456"},
		},
	}

	// 导出趋势数据
	var buf bytes.Buffer
	err := exporter.ExportTrendData(&buf, dataPoints)
	if err != nil {
		t.Fatalf("ExportTrendData failed: %v", err)
	}

	// 解析JSON
	var exportData TrendExportData
	if err := json.Unmarshal(buf.Bytes(), &exportData); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// 验证数据点
	if len(exportData.DataPoints) != 2 {
		t.Errorf("Expected 2 data points, got %d", len(exportData.DataPoints))
	}

	// 验证指标
	if exportData.Metrics.CurrentCoverage != 80.0 {
		t.Errorf("Expected current coverage 80.0, got %.2f", exportData.Metrics.CurrentCoverage)
	}

	if exportData.Metrics.Trend != "improving" {
		t.Errorf("Expected trend improving, got %s", exportData.Metrics.Trend)
	}

	// 验证导出时间
	if exportData.ExportedAt == "" {
		t.Error("ExportedAt should not be empty")
	}
}

func TestTrendExporter_CreateDataPoint(t *testing.T) {
	exporter := NewTrendExporter()

	report := &CoverageReport{
		OverallCoverage:   85.0,
		TotalStatements:   1000,
		CoveredStatements: 850,
		PackageCoverage: map[string]float64{
			"pkg1": 90.0,
			"pkg2": 80.0,
		},
		UncoveredCritical: []UncoveredPath{
			{File: "file1.go"},
			{File: "file2.go"},
		},
	}

	metadata := map[string]string{
		"commit": "abc123",
		"branch": "main",
	}

	timestamp := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	dataPoint := exporter.CreateDataPoint(report, timestamp, metadata)

	// 验证数据点
	if dataPoint.Timestamp != "2024-01-01T00:00:00Z" {
		t.Errorf("Expected timestamp 2024-01-01T00:00:00Z, got %s", dataPoint.Timestamp)
	}

	if dataPoint.OverallCoverage != 85.0 {
		t.Errorf("Expected coverage 85.0, got %.2f", dataPoint.OverallCoverage)
	}

	if dataPoint.TotalStatements != 1000 {
		t.Errorf("Expected 1000 statements, got %d", dataPoint.TotalStatements)
	}

	if dataPoint.CoveredStatements != 850 {
		t.Errorf("Expected 850 covered statements, got %d", dataPoint.CoveredStatements)
	}

	if len(dataPoint.PackageCoverage) != 2 {
		t.Errorf("Expected 2 packages, got %d", len(dataPoint.PackageCoverage))
	}

	if dataPoint.CriticalGapsCount != 2 {
		t.Errorf("Expected 2 critical gaps, got %d", dataPoint.CriticalGapsCount)
	}

	if dataPoint.Metadata["commit"] != "abc123" {
		t.Errorf("Expected commit abc123, got %s", dataPoint.Metadata["commit"])
	}
}

func TestTrendExporter_CreateDataPoint_DefaultValues(t *testing.T) {
	exporter := NewTrendExporter()

	report := &CoverageReport{
		OverallCoverage: 85.0,
	}

	// 使用零值时间和nil元数据
	dataPoint := exporter.CreateDataPoint(report, time.Time{}, nil)

	// 验证默认时间戳（应该是当前时间）
	if dataPoint.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}

	// 验证元数据已初始化
	if dataPoint.Metadata == nil {
		t.Error("Metadata should be initialized")
	}
}

func TestTrendExporter_CalculateTrend(t *testing.T) {
	exporter := NewTrendExporter()

	tests := []struct {
		name          string
		dataPoints    []TrendDataPoint
		expectedTrend string
	}{
		{
			name: "Improving trend",
			dataPoints: []TrendDataPoint{
				{Timestamp: "2024-01-01T00:00:00Z", OverallCoverage: 70.0},
				{Timestamp: "2024-01-02T00:00:00Z", OverallCoverage: 75.0},
				{Timestamp: "2024-01-03T00:00:00Z", OverallCoverage: 82.0},
			},
			expectedTrend: "improving",
		},
		{
			name: "Declining trend",
			dataPoints: []TrendDataPoint{
				{Timestamp: "2024-01-01T00:00:00Z", OverallCoverage: 85.0},
				{Timestamp: "2024-01-02T00:00:00Z", OverallCoverage: 80.0},
				{Timestamp: "2024-01-03T00:00:00Z", OverallCoverage: 82.0},
			},
			expectedTrend: "declining",
		},
		{
			name: "Stable trend",
			dataPoints: []TrendDataPoint{
				{Timestamp: "2024-01-01T00:00:00Z", OverallCoverage: 80.0},
				{Timestamp: "2024-01-02T00:00:00Z", OverallCoverage: 80.5},
				{Timestamp: "2024-01-03T00:00:00Z", OverallCoverage: 80.2},
			},
			expectedTrend: "stable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := exporter.calculateTrend(tt.dataPoints)

			if metrics.Trend != tt.expectedTrend {
				t.Errorf("Expected trend %s, got %s", tt.expectedTrend, metrics.Trend)
			}

			// 验证当前覆盖率
			lastCoverage := tt.dataPoints[len(tt.dataPoints)-1].OverallCoverage
			if metrics.CurrentCoverage != lastCoverage {
				t.Errorf("Expected current coverage %.2f, got %.2f",
					lastCoverage, metrics.CurrentCoverage)
			}

			// 验证平均覆盖率
			var sum float64
			for _, dp := range tt.dataPoints {
				sum += dp.OverallCoverage
			}
			expectedAvg := sum / float64(len(tt.dataPoints))
			if metrics.AverageCoverage != expectedAvg {
				t.Errorf("Expected average coverage %.2f, got %.2f",
					expectedAvg, metrics.AverageCoverage)
			}
		})
	}
}

func TestTrendExporter_CalculateTrend_EmptyData(t *testing.T) {
	exporter := NewTrendExporter()

	metrics := exporter.calculateTrend([]TrendDataPoint{})

	// 验证空数据返回零值
	if metrics.CurrentCoverage != 0 {
		t.Errorf("Expected current coverage 0, got %.2f", metrics.CurrentCoverage)
	}

	if metrics.AverageCoverage != 0 {
		t.Errorf("Expected average coverage 0, got %.2f", metrics.AverageCoverage)
	}
}

func TestTrendExporter_CalculateTrend_SingleDataPoint(t *testing.T) {
	exporter := NewTrendExporter()

	dataPoints := []TrendDataPoint{
		{Timestamp: "2024-01-01T00:00:00Z", OverallCoverage: 80.0},
	}

	metrics := exporter.calculateTrend(dataPoints)

	// 验证单个数据点
	if metrics.CurrentCoverage != 80.0 {
		t.Errorf("Expected current coverage 80.0, got %.2f", metrics.CurrentCoverage)
	}

	if metrics.AverageCoverage != 80.0 {
		t.Errorf("Expected average coverage 80.0, got %.2f", metrics.AverageCoverage)
	}

	if metrics.DeltaLastWeek != 0 {
		t.Errorf("Expected delta last week 0, got %.2f", metrics.DeltaLastWeek)
	}

	if metrics.Trend != "stable" {
		t.Errorf("Expected trend stable, got %s", metrics.Trend)
	}
}

func TestTrendExporter_ExportChartData(t *testing.T) {
	exporter := NewTrendExporter()

	dataPoints := []TrendDataPoint{
		{
			Timestamp:       "2024-01-01T00:00:00Z",
			OverallCoverage: 75.0,
		},
		{
			Timestamp:       "2024-01-02T00:00:00Z",
			OverallCoverage: 80.0,
		},
		{
			Timestamp:       "2024-01-03T00:00:00Z",
			OverallCoverage: 85.0,
		},
	}

	// 导出图表数据
	var buf bytes.Buffer
	err := exporter.ExportChartData(&buf, dataPoints)
	if err != nil {
		t.Fatalf("ExportChartData failed: %v", err)
	}

	// 解析JSON
	var chartData ChartData
	if err := json.Unmarshal(buf.Bytes(), &chartData); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// 验证标签
	if len(chartData.Labels) != 3 {
		t.Errorf("Expected 3 labels, got %d", len(chartData.Labels))
	}

	if chartData.Labels[0] != "2024-01-01" {
		t.Errorf("Expected label 2024-01-01, got %s", chartData.Labels[0])
	}

	// 验证覆盖率数据
	if len(chartData.Coverage) != 3 {
		t.Errorf("Expected 3 coverage values, got %d", len(chartData.Coverage))
	}

	if chartData.Coverage[0] != 75.0 {
		t.Errorf("Expected coverage 75.0, got %.2f", chartData.Coverage[0])
	}

	if chartData.Coverage[1] != 80.0 {
		t.Errorf("Expected coverage 80.0, got %.2f", chartData.Coverage[1])
	}

	if chartData.Coverage[2] != 85.0 {
		t.Errorf("Expected coverage 85.0, got %.2f", chartData.Coverage[2])
	}
}

func TestTrendExporter_FindNearestDataPoint(t *testing.T) {
	exporter := NewTrendExporter()

	dataPoints := []TrendDataPoint{
		{Timestamp: "2024-01-01T00:00:00Z", OverallCoverage: 70.0},
		{Timestamp: "2024-01-05T00:00:00Z", OverallCoverage: 75.0},
		{Timestamp: "2024-01-10T00:00:00Z", OverallCoverage: 80.0},
	}

	// 查找最接近 2024-01-03 的数据点
	target := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	nearest := exporter.findNearestDataPoint(dataPoints, target)

	if nearest == nil {
		t.Fatal("Expected to find nearest data point")
	}

	// 应该找到 2024-01-01（最近的之前的点）或 2024-01-05（最近的之后的点）
	// 由于距离相等，可能是任意一个
	if nearest.Timestamp != "2024-01-01T00:00:00Z" && nearest.Timestamp != "2024-01-05T00:00:00Z" {
		t.Errorf("Expected nearest to be 2024-01-01 or 2024-01-05, got %s", nearest.Timestamp)
	}
}
