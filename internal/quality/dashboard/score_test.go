// Package dashboard 质量分数计算测试
package dashboard

import (
	"testing"

	"github.com/example/sso/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestCalculateQualityScore(t *testing.T) {
	t.Run("nil指标返回零分", func(t *testing.T) {
		score := CalculateQualityScore(nil)
		assert.Equal(t, 0.0, score.Score)
		assert.Equal(t, "N/A", score.Grade)
	})

	t.Run("完美指标返回A级", func(t *testing.T) {
		m := &store.QualityMetrics{
			CoveragePercent: 100,
			TestPassRate:    100,
			LintViolations:  0,
		}
		score := CalculateQualityScore(m)
		assert.Equal(t, 100.0, score.Score)
		assert.Equal(t, "A", score.Grade)
		assert.Equal(t, 100.0, score.CoverageScore)
		assert.Equal(t, 100.0, score.PassRateScore)
		assert.Equal(t, 100.0, score.ViolationsScore)
	})

	t.Run("80%覆盖率和通过率返回B级", func(t *testing.T) {
		m := &store.QualityMetrics{
			CoveragePercent: 80,
			TestPassRate:    80,
			LintViolations:  0,
		}
		score := CalculateQualityScore(m)
		assert.Equal(t, "B", score.Grade)
		// 80*0.4 + 80*0.4 + 100*0.2 = 32 + 32 + 20 = 84
		assert.Equal(t, 84.0, score.Score)
	})

	t.Run("违规影响分数", func(t *testing.T) {
		m := &store.QualityMetrics{
			CoveragePercent: 100,
			TestPassRate:    100,
			LintViolations:  500, // 50% of max
		}
		score := CalculateQualityScore(m)
		// coverage: 100*0.4 = 40, passRate: 100*0.4 = 40, violations: (1-0.5)*100*0.2 = 10
		assert.Equal(t, 90.0, score.Score)
		assert.Equal(t, 50.0, score.ViolationsScore)
	})

	t.Run("低覆盖率返回低等级", func(t *testing.T) {
		m := &store.QualityMetrics{
			CoveragePercent: 50,
			TestPassRate:    50,
			LintViolations:  0,
		}
		score := CalculateQualityScore(m)
		// 50*0.4 + 50*0.4 + 100*0.2 = 20 + 20 + 20 = 60
		assert.Equal(t, 60.0, score.Score)
		assert.Equal(t, "D", score.Grade)
	})
}

func TestGetGrade(t *testing.T) {
	tests := []struct {
		score    float64
		expected string
	}{
		{95, "A"},
		{90, "A"},
		{85, "B"},
		{80, "B"},
		{75, "C"},
		{70, "C"},
		{65, "D"},
		{60, "D"},
		{55, "F"},
		{0, "F"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, getGrade(tt.score))
		})
	}
}

func TestGenerateWeeklySummary(t *testing.T) {
	t.Run("无数据", func(t *testing.T) {
		report := GenerateWeeklySummary(&store.WeeklyComparison{})
		assert.Equal(t, "暂无质量数据", report.Summary)
	})

	t.Run("首次采集", func(t *testing.T) {
		comparison := &store.WeeklyComparison{
			Current: &store.QualityMetrics{
				CoveragePercent: 85.5,
				TestPassRate:    95.0,
				QualityScore:    88.0,
			},
		}
		report := GenerateWeeklySummary(comparison)
		assert.Contains(t, report.Summary, "首次采集")
		assert.Contains(t, report.Summary, "85.5%")
	})

	t.Run("周对比-改进", func(t *testing.T) {
		comparison := &store.WeeklyComparison{
			Current: &store.QualityMetrics{
				CoveragePercent: 85.0,
				TestPassRate:    95.0,
				QualityScore:    88.0,
			},
			Previous: &store.QualityMetrics{
				CoveragePercent: 80.0,
				TestPassRate:    90.0,
				QualityScore:    84.0,
			},
			Delta: &store.QualityDelta{
				CoverageDelta: 5.0,
				PassRateDelta: 5.0,
				ScoreDelta:    4.0,
				LintDelta:     -10,
			},
		}
		report := GenerateWeeklySummary(comparison)
		assert.Len(t, report.Improvements, 3)
		assert.Len(t, report.Regressions, 0)
	})

	t.Run("周对比-退步", func(t *testing.T) {
		comparison := &store.WeeklyComparison{
			Current: &store.QualityMetrics{
				CoveragePercent: 75.0,
				TestPassRate:    85.0,
				QualityScore:    78.0,
			},
			Previous: &store.QualityMetrics{
				CoveragePercent: 80.0,
				TestPassRate:    90.0,
				QualityScore:    84.0,
			},
			Delta: &store.QualityDelta{
				CoverageDelta: -5.0,
				PassRateDelta: -5.0,
				ScoreDelta:    -6.0,
				LintDelta:     20,
			},
		}
		report := GenerateWeeklySummary(comparison)
		assert.Len(t, report.Improvements, 0)
		assert.Len(t, report.Regressions, 3)
	})
}
