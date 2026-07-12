-- ============================================================================
-- 删除质量指标表
-- ============================================================================

DROP INDEX IF EXISTS idx_quality_metrics_git_commit;
DROP INDEX IF EXISTS idx_quality_metrics_recorded_at;
DROP TABLE IF EXISTS quality_metrics;
