// Package dashboard 质量仪表盘服务
package dashboard

import (
	"github.com/example/sso/internal/store"
)

// ============================================================================
// 质量仪表盘配置
// ============================================================================

// Config 仪表盘配置
type Config struct {
	Enabled bool `json:"enabled"`
}

// ============================================================================
// 质量分数权重
// ============================================================================

const (
	// WeightCoverage 覆盖率权重
	WeightCoverage = 0.4
	// WeightPassRate 测试通过率权重
	WeightPassRate = 0.4
	// WeightViolations 违规权重（反向）
	WeightViolations = 0.2
	// MaxViolationsForScore 违规数最大值（超过此值违规分数为0）
	MaxViolationsForScore = 1000
)

// ============================================================================
// 仪表盘响应类型
// ============================================================================

// DashboardData 仪表盘数据
type Data struct {
	Current   *store.QualityMetrics   `json:"current"`
	Weekly    *store.WeeklyComparison `json:"weekly"`
	Trend     []store.QualityMetrics  `json:"trend"`
	ScoreInfo *ScoreInfo              `json:"score_info"`
}

// ScoreInfo 质量分数详情
type ScoreInfo struct {
	Score           float64 `json:"score"`
	CoverageScore   float64 `json:"coverage_score"`
	PassRateScore   float64 `json:"pass_rate_score"`
	ViolationsScore float64 `json:"violations_score"`
	Grade           string  `json:"grade"`
}

// WeeklyReportData 周报数据
type WeeklyReportData struct {
	Comparison   *store.WeeklyComparison `json:"comparison"`
	Summary      string                  `json:"summary"`
	Improvements []string                `json:"improvements"`
	Regressions  []string                `json:"regressions"`
}
