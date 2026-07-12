// Package dashboard 质量分数计算
package dashboard

import (
	"fmt"
	"math"

	"github.com/example/sso/internal/store"
)

// CalculateQualityScore 计算综合质量分数
// 公式: (coverage * 0.4) + (passRate * 0.4) + ((1 - violations/1000) * 0.2)
func CalculateQualityScore(m *store.QualityMetrics) *ScoreInfo {
	if m == nil {
		return &ScoreInfo{Score: 0, Grade: "N/A"}
	}

	// 覆盖率分数 (0-100)
	coverageScore := math.Min(m.CoveragePercent, 100)

	// 测试通过率分数 (0-100)
	passRateScore := math.Min(m.TestPassRate, 100)

	// 违规分数 (反向，违规越少分数越高)
	totalViolations := float64(m.LintViolations + m.GosecViolations + m.GocycloViolations)
	violationsRatio := math.Min(totalViolations/MaxViolationsForScore, 1.0)
	violationsScore := (1 - violationsRatio) * 100

	// 综合分数
	score := (coverageScore * WeightCoverage) +
		(passRateScore * WeightPassRate) +
		(violationsScore * WeightViolations)

	// 四舍五入到两位小数
	score = math.Round(score*100) / 100

	return &ScoreInfo{
		Score:           score,
		CoverageScore:   math.Round(coverageScore*100) / 100,
		PassRateScore:   math.Round(passRateScore*100) / 100,
		ViolationsScore: math.Round(violationsScore*100) / 100,
		Grade:           getGrade(score),
	}
}

// getGrade 根据分数获取等级
func getGrade(score float64) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

// GenerateWeeklySummary 生成周报摘要
func GenerateWeeklySummary(comparison *store.WeeklyComparison) *WeeklyReportData {
	report := &WeeklyReportData{
		Comparison: comparison,
	}

	if comparison.Current == nil {
		report.Summary = "暂无质量数据"
		return report
	}

	if comparison.Previous == nil {
		report.Summary = fmt.Sprintf("首次采集：覆盖率 %.1f%%，测试通过率 %.1f%%，质量分数 %.1f",
			comparison.Current.CoveragePercent,
			comparison.Current.TestPassRate,
			comparison.Current.QualityScore)
		return report
	}

	// 生成摘要
	current := comparison.Current
	previous := comparison.Previous
	delta := comparison.Delta

	summary := fmt.Sprintf("本周质量分数 %.1f（%s），覆盖率 %.1f%%，测试通过率 %.1f%%",
		current.QualityScore,
		formatDelta(delta.ScoreDelta),
		current.CoveragePercent,
		current.TestPassRate)

	report.Summary = summary

	// 识别改进和退步
	if delta.CoverageDelta > 0 {
		report.Improvements = append(report.Improvements,
			fmt.Sprintf("覆盖率提升 %.1f%%（%.1f%% → %.1f%%）",
				delta.CoverageDelta, previous.CoveragePercent, current.CoveragePercent))
	} else if delta.CoverageDelta < 0 {
		report.Regressions = append(report.Regressions,
			fmt.Sprintf("覆盖率下降 %.1f%%（%.1f%% → %.1f%%）",
				-delta.CoverageDelta, previous.CoveragePercent, current.CoveragePercent))
	}

	if delta.PassRateDelta > 0 {
		report.Improvements = append(report.Improvements,
			fmt.Sprintf("测试通过率提升 %.1f%%（%.1f%% → %.1f%%）",
				delta.PassRateDelta, previous.TestPassRate, current.TestPassRate))
	} else if delta.PassRateDelta < 0 {
		report.Regressions = append(report.Regressions,
			fmt.Sprintf("测试通过率下降 %.1f%%（%.1f%% → %.1f%%）",
				-delta.PassRateDelta, previous.TestPassRate, current.TestPassRate))
	}

	if delta.LintDelta < 0 {
		report.Improvements = append(report.Improvements,
			fmt.Sprintf("Lint违规减少 %d 个", -delta.LintDelta))
	} else if delta.LintDelta > 0 {
		report.Regressions = append(report.Regressions,
			fmt.Sprintf("Lint违规增加 %d 个", delta.LintDelta))
	}

	return report
}

// formatDelta 格式化变化量
func formatDelta(delta float64) string {
	if delta > 0 {
		return fmt.Sprintf("+%.1f", delta)
	}
	return fmt.Sprintf("%.1f", delta)
}
