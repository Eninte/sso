// Package postgres 质量指标存储实现
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/sso/internal/store"
)

// ============================================================================
// QualityMetricsStore 实现（方法挂载在 *Store 上）
// ============================================================================

// StoreMetrics 存储质量指标
func (s *Store) StoreMetrics(ctx context.Context, m *store.QualityMetrics) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	var metadataJSON []byte
	if m.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(m.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO quality_metrics (
			recorded_at, git_commit_sha, coverage_percent, test_pass_rate,
			total_tests, passed_tests, failed_tests,
			lint_violations, gosec_violations, gocyclo_violations,
			quality_score, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at
	`

	return s.db.QueryRowContext(ctx, query,
		m.RecordedAt,
		sql.NullString{String: m.GitCommitSHA, Valid: m.GitCommitSHA != ""},
		m.CoveragePercent,
		m.TestPassRate,
		m.TotalTests,
		m.PassedTests,
		m.FailedTests,
		m.LintViolations,
		m.GosecViolations,
		m.GocycloViolations,
		m.QualityScore,
		metadataJSON,
	).Scan(&m.ID, &m.CreatedAt)
}

// GetLatestMetrics 获取最新的质量指标
func (s *Store) GetLatestMetrics(ctx context.Context) (*store.QualityMetrics, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		SELECT id, recorded_at, git_commit_sha, coverage_percent, test_pass_rate,
			   total_tests, passed_tests, failed_tests,
			   lint_violations, gosec_violations, gocyclo_violations,
			   quality_score, metadata, created_at
		FROM quality_metrics
		ORDER BY recorded_at DESC
		LIMIT 1
	`

	m := &store.QualityMetrics{}
	var gitSHA sql.NullString
	var metadataJSON []byte

	err := s.db.QueryRowContext(ctx, query).Scan(
		&m.ID, &m.RecordedAt, &gitSHA,
		&m.CoveragePercent, &m.TestPassRate,
		&m.TotalTests, &m.PassedTests, &m.FailedTests,
		&m.LintViolations, &m.GosecViolations, &m.GocycloViolations,
		&m.QualityScore, &metadataJSON, &m.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest metrics: %w", err)
	}

	if gitSHA.Valid {
		m.GitCommitSHA = gitSHA.String
	}
	if metadataJSON != nil {
		_ = json.Unmarshal(metadataJSON, &m.Metadata)
	}

	return m, nil
}

// GetMetricsRange 获取指定时间范围内的质量指标
func (s *Store) GetMetricsRange(ctx context.Context, from, to time.Time) ([]store.QualityMetrics, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		SELECT id, recorded_at, git_commit_sha, coverage_percent, test_pass_rate,
			   total_tests, passed_tests, failed_tests,
			   lint_violations, gosec_violations, gocyclo_violations,
			   quality_score, metadata, created_at
		FROM quality_metrics
		WHERE recorded_at BETWEEN $1 AND $2
		ORDER BY recorded_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, fmt.Errorf("get metrics range: %w", err)
	}
	defer rows.Close()

	var metrics []store.QualityMetrics
	for rows.Next() {
		var m store.QualityMetrics
		var gitSHA sql.NullString
		var metadataJSON []byte

		if err := rows.Scan(
			&m.ID, &m.RecordedAt, &gitSHA,
			&m.CoveragePercent, &m.TestPassRate,
			&m.TotalTests, &m.PassedTests, &m.FailedTests,
			&m.LintViolations, &m.GosecViolations, &m.GocycloViolations,
			&m.QualityScore, &metadataJSON, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan metrics: %w", err)
		}

		if gitSHA.Valid {
			m.GitCommitSHA = gitSHA.String
		}
		if metadataJSON != nil {
			_ = json.Unmarshal(metadataJSON, &m.Metadata)
		}

		metrics = append(metrics, m)
	}

	return metrics, rows.Err()
}

// GetWeeklyComparison 获取周对比数据
func (s *Store) GetWeeklyComparison(ctx context.Context) (*store.WeeklyComparison, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 获取本周最新数据
	current, err := s.GetLatestMetrics(ctx)
	if err != nil {
		if err == store.ErrNotFound {
			return &store.WeeklyComparison{}, nil
		}
		return nil, fmt.Errorf("get current metrics: %w", err)
	}

	// 获取一周前的数据
	query := `
		SELECT id, recorded_at, git_commit_sha, coverage_percent, test_pass_rate,
			   total_tests, passed_tests, failed_tests,
			   lint_violations, gosec_violations, gocyclo_violations,
			   quality_score, metadata, created_at
		FROM quality_metrics
		WHERE recorded_at <= $1
		ORDER BY recorded_at DESC
		LIMIT 1
	`

	weekAgo := current.RecordedAt.Add(-7 * 24 * time.Hour)
	previous := &store.QualityMetrics{}
	var gitSHA sql.NullString
	var metadataJSON []byte

	err = s.db.QueryRowContext(ctx, query, weekAgo).Scan(
		&previous.ID, &previous.RecordedAt, &gitSHA,
		&previous.CoveragePercent, &previous.TestPassRate,
		&previous.TotalTests, &previous.PassedTests, &previous.FailedTests,
		&previous.LintViolations, &previous.GosecViolations, &previous.GocycloViolations,
		&previous.QualityScore, &metadataJSON, &previous.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return &store.WeeklyComparison{Current: current}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get previous metrics: %w", err)
	}

	if gitSHA.Valid {
		previous.GitCommitSHA = gitSHA.String
	}
	if metadataJSON != nil {
		_ = json.Unmarshal(metadataJSON, &previous.Metadata)
	}

	// 计算变化量
	delta := &store.QualityDelta{
		CoverageDelta: current.CoveragePercent - previous.CoveragePercent,
		PassRateDelta: current.TestPassRate - previous.TestPassRate,
		ScoreDelta:    current.QualityScore - previous.QualityScore,
		LintDelta:     current.LintViolations - previous.LintViolations,
	}

	return &store.WeeklyComparison{
		Current:  current,
		Previous: previous,
		Delta:    delta,
	}, nil
}
