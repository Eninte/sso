-- ============================================================================
-- 质量指标表
-- 存储代码质量指标的历史数据，用于趋势分析和仪表盘展示
-- ============================================================================

CREATE TABLE IF NOT EXISTS quality_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recorded_at TIMESTAMP NOT NULL DEFAULT NOW(),
    git_commit_sha VARCHAR(40),
    coverage_percent DECIMAL(5,2),
    test_pass_rate DECIMAL(5,2),
    total_tests INTEGER,
    passed_tests INTEGER,
    failed_tests INTEGER,
    lint_violations INTEGER,
    gosec_violations INTEGER,
    gocyclo_violations INTEGER,
    quality_score DECIMAL(5,2),
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE quality_metrics IS '代码质量指标历史数据';
COMMENT ON COLUMN quality_metrics.id IS '主键ID';
COMMENT ON COLUMN quality_metrics.recorded_at IS '记录时间';
COMMENT ON COLUMN quality_metrics.git_commit_sha IS 'Git提交SHA';
COMMENT ON COLUMN quality_metrics.coverage_percent IS '测试覆盖率百分比';
COMMENT ON COLUMN quality_metrics.test_pass_rate IS '测试通过率百分比';
COMMENT ON COLUMN quality_metrics.total_tests IS '总测试数';
COMMENT ON COLUMN quality_metrics.passed_tests IS '通过测试数';
COMMENT ON COLUMN quality_metrics.failed_tests IS '失败测试数';
COMMENT ON COLUMN quality_metrics.lint_violations IS 'Lint违规总数';
COMMENT ON COLUMN quality_metrics.gosec_violations IS 'Gosec安全违规数';
COMMENT ON COLUMN quality_metrics.gocyclo_violations IS 'Gocyclo复杂度违规数';
COMMENT ON COLUMN quality_metrics.quality_score IS '综合质量分数';
COMMENT ON COLUMN quality_metrics.metadata IS '扩展元数据（JSON格式）';
COMMENT ON COLUMN quality_metrics.created_at IS '创建时间';

-- 按时间查询的索引
CREATE INDEX idx_quality_metrics_recorded_at ON quality_metrics(recorded_at DESC);

-- 按Git提交查询的索引
CREATE INDEX idx_quality_metrics_git_commit ON quality_metrics(git_commit_sha);
