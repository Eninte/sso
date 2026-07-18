// Package dashboard 仪表盘HTTP接口测试
package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/sso/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// fakeQualityStore 仅实现 store.QualityMetricsStore 用于 HTTP handler 测试
//
// 通过 fake 而非 mock.Store，是因为：
// 1. mock.Store 不实现 QualityMetricsStore 接口
// 2. 接口只暴露 4 个方法，fake 更轻量且可控
// 3. 测试需要精确控制返回的数据与错误
// ============================================================================

type fakeQualityStore struct {
	// 返回数据
	metricsRange     []store.QualityMetrics
	weeklyComparison *store.WeeklyComparison

	// 错误注入
	metricsRangeErr  error
	weeklyCompareErr error

	// 调用记录
	metricsRangeArgs struct {
		from time.Time
		to   time.Time
	}
	weeklyCalled bool
}

func (f *fakeQualityStore) StoreMetrics(ctx context.Context, m *store.QualityMetrics) error {
	return nil
}

func (f *fakeQualityStore) GetLatestMetrics(ctx context.Context) (*store.QualityMetrics, error) {
	return nil, nil
}

func (f *fakeQualityStore) GetMetricsRange(ctx context.Context, from, to time.Time) ([]store.QualityMetrics, error) {
	f.metricsRangeArgs.from = from
	f.metricsRangeArgs.to = to
	return f.metricsRange, f.metricsRangeErr
}

func (f *fakeQualityStore) GetWeeklyComparison(ctx context.Context) (*store.WeeklyComparison, error) {
	f.weeklyCalled = true
	return f.weeklyComparison, f.weeklyCompareErr
}

// ============================================================================
// HandleMetricsAPI 单元测试
// 目标端点：GET /api/v1/admin/quality/api/metrics
// 期望行为：
//   - 返回近 30 天质量指标数据
//   - 响应体为 { "metrics": [...], "count": N }
//   - store 错误时返回 500，响应体为 { "error": "获取指标数据失败" }
// ============================================================================

func newTestServer(fake *fakeQualityStore) *Server {
	// 错误路径会调用 s.logger.Error，必须传真实 logger 避免 nil panic
	return NewServer(fake, slog.Default())
}

func TestHandleMetricsAPI_ReturnsMetricsAndCount(t *testing.T) {
	// 准备：store 返回 3 条指标
	now := time.Now()
	fake := &fakeQualityStore{
		metricsRange: []store.QualityMetrics{
			{ID: "m1", RecordedAt: now.AddDate(0, 0, -1), CoveragePercent: 85.5, QualityScore: 88.0},
			{ID: "m2", RecordedAt: now.AddDate(0, 0, -2), CoveragePercent: 85.0, QualityScore: 87.5},
			{ID: "m3", RecordedAt: now.AddDate(0, 0, -3), CoveragePercent: 84.5, QualityScore: 87.0},
		},
	}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/quality/api/metrics", nil)
	rr := httptest.NewRecorder()

	srv.HandleMetricsAPI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "应返回 200")
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json",
		"Content-Type 应为 application/json")

	// 验证响应结构
	var body struct {
		Metrics []store.QualityMetrics `json:"metrics"`
		Count   int                    `json:"count"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, 3, body.Count, "count 应为 3")
	require.Len(t, body.Metrics, 3, "metrics 应有 3 条记录")
	assert.Equal(t, "m1", body.Metrics[0].ID, "应返回第 1 条记录")
	assert.Equal(t, "m3", body.Metrics[2].ID, "应返回第 3 条记录")
}

func TestHandleMetricsAPI_EmptyResult_ReturnsZeroCount(t *testing.T) {
	// 准备：store 返回空切片
	fake := &fakeQualityStore{metricsRange: []store.QualityMetrics{}}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/quality/api/metrics", nil)
	rr := httptest.NewRecorder()

	srv.HandleMetricsAPI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "空结果应仍返回 200")

	var body struct {
		Metrics []store.QualityMetrics `json:"metrics"`
		Count   int                    `json:"count"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, 0, body.Count, "count 应为 0")
	assert.Empty(t, body.Metrics, "metrics 应为空数组")
}

func TestHandleMetricsAPI_StoreError_Returns500(t *testing.T) {
	// 准备：store 返回错误
	fake := &fakeQualityStore{
		metricsRangeErr: errors.New("connection refused"),
	}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/quality/api/metrics", nil)
	rr := httptest.NewRecorder()

	srv.HandleMetricsAPI(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code, "store 错误应返回 500")

	var body map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "获取指标数据失败", body["error"], "应返回中文错误消息")
}

func TestHandleMetricsAPI_QueryWindowIs30Days(t *testing.T) {
	// 验证：handler 请求 store 的 from/to 时间差约为 30 天
	fake := &fakeQualityStore{}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/quality/api/metrics", nil)
	rr := httptest.NewRecorder()

	srv.HandleMetricsAPI(rr, req)

	delta := fake.metricsRangeArgs.to.Sub(fake.metricsRangeArgs.from)
	// 期望约 30 天 = 720 小时，允许 1 小时误差
	assert.InDelta(t, 720.0, delta.Hours(), 1.0,
		"查询窗口应约为 30 天（允许 1 小时误差）")
}

func TestHandleMetricsAPI_ResponseDoesNotLeakInternalError(t *testing.T) {
	// 安全要求：错误响应不应泄漏内部错误详情（DB 驱动名、地址、凭据等）
	fake := &fakeQualityStore{
		metricsRangeErr: errors.New("pgx: connection refused 127.0.0.1:5432 password=secret123"),
	}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/quality/api/metrics", nil)
	rr := httptest.NewRecorder()

	srv.HandleMetricsAPI(rr, req)

	bodyText := rr.Body.String()
	assert.NotContains(t, bodyText, "pgx", "不应泄漏驱动名")
	assert.NotContains(t, bodyText, "127.0.0.1", "不应泄漏数据库地址")
	assert.NotContains(t, bodyText, "password", "不应泄漏凭据")
	assert.NotContains(t, bodyText, "secret123", "不应泄漏密码")
}

// ============================================================================
// HandleWeeklyReportAPI 单元测试
// 目标端点：GET /api/v1/admin/quality/api/report/weekly
// 期望行为：
//   - 返回本周与上周的对比数据，含改进与退步项
//   - store 错误时返回 500
//   - 数据缺失时返回 "暂无质量数据" 而不是空指针 panic
// ============================================================================

func TestHandleWeeklyReportAPI_ReturnsWeeklyComparison(t *testing.T) {
	// 准备：store 返回有对比数据的周报
	now := time.Now()
	lastWeek := now.AddDate(0, 0, -7)
	fake := &fakeQualityStore{
		weeklyComparison: &store.WeeklyComparison{
			Current: &store.QualityMetrics{
				RecordedAt:      now,
				CoveragePercent: 90.0,
				TestPassRate:    100.0,
				QualityScore:    92.0,
				LintViolations:  5,
			},
			Previous: &store.QualityMetrics{
				RecordedAt:      lastWeek,
				CoveragePercent: 85.0,
				TestPassRate:    100.0,
				QualityScore:    88.0,
				LintViolations:  10,
			},
			Delta: &store.QualityDelta{
				CoverageDelta: 5.0,
				PassRateDelta: 0.0,
				LintDelta:     -5, // Lint 违规减少 5 个
				ScoreDelta:    4.0,
			},
		},
	}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/quality/api/report/weekly", nil)
	rr := httptest.NewRecorder()

	srv.HandleWeeklyReportAPI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "应返回 200")
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json",
		"Content-Type 应为 application/json")

	// 验证响应结构
	var body WeeklyReportData
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.NotNil(t, body.Comparison, "comparison 不应为空")
	assert.Equal(t, 90.0, body.Comparison.Current.CoveragePercent)
	assert.NotEmpty(t, body.Summary, "summary 不应为空（有数据时应有摘要）")
	assert.NotEmpty(t, body.Improvements, "覆盖率提升 5% 应识别为改进项")
	assert.Empty(t, body.Regressions, "无退步项")
}

func TestHandleWeeklyReportAPI_NoData_ReturnsEmptySummary(t *testing.T) {
	// 准备：store 返回 nil（首次部署、无数据）
	fake := &fakeQualityStore{
		weeklyComparison: &store.WeeklyComparison{
			Current:  nil,
			Previous: nil,
			Delta:    nil,
		},
	}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/quality/api/report/weekly", nil)
	rr := httptest.NewRecorder()

	srv.HandleWeeklyReportAPI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "无数据仍应返回 200")

	var body WeeklyReportData
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "暂无质量数据", body.Summary, "summary 应为占位文案")
	assert.Empty(t, body.Improvements, "无改进项")
	assert.Empty(t, body.Regressions, "无退步项")
}

func TestHandleWeeklyReportAPI_FirstWeekData_ReturnsFirstSummary(t *testing.T) {
	// 准备：只有当前数据，无上周对比
	now := time.Now()
	fake := &fakeQualityStore{
		weeklyComparison: &store.WeeklyComparison{
			Current: &store.QualityMetrics{
				RecordedAt:      now,
				CoveragePercent: 80.0,
				TestPassRate:    100.0,
				QualityScore:    85.0,
			},
			Previous: nil,
			Delta:    nil,
		},
	}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/quality/api/report/weekly", nil)
	rr := httptest.NewRecorder()

	srv.HandleWeeklyReportAPI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var body WeeklyReportData
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Contains(t, body.Summary, "首次采集", "首周数据应为'首次采集'摘要")
	assert.Contains(t, body.Summary, "80.0%", "应包含覆盖率")
}

func TestHandleWeeklyReportAPI_StoreError_Returns500(t *testing.T) {
	fake := &fakeQualityStore{
		weeklyCompareErr: errors.New("query failed"),
	}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/quality/api/report/weekly", nil)
	rr := httptest.NewRecorder()

	srv.HandleWeeklyReportAPI(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code, "store 错误应返回 500")

	var body map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "获取周报数据失败", body["error"], "应返回中文错误消息")
}

func TestHandleWeeklyReportAPI_ResponseDoesNotLeakInternalError(t *testing.T) {
	// 安全要求：错误响应不应泄漏内部错误详情
	fake := &fakeQualityStore{
		weeklyCompareErr: errors.New("pgx: connection refused 127.0.0.1:5432 password=secret123"),
	}
	srv := newTestServer(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/quality/api/report/weekly", nil)
	rr := httptest.NewRecorder()

	srv.HandleWeeklyReportAPI(rr, req)

	bodyText := rr.Body.String()
	assert.NotContains(t, bodyText, "pgx", "不应泄漏驱动名")
	assert.NotContains(t, bodyText, "127.0.0.1", "不应泄漏数据库地址")
	assert.NotContains(t, bodyText, "password", "不应泄漏凭据")
	assert.NotContains(t, bodyText, "secret123", "不应泄漏密码")
}
