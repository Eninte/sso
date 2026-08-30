// Command qualitymetrics 将测试覆盖率与 JUnit 测试结果回写到 quality_metrics 表
//
// 背景：质量仪表盘（/api/v1/admin/quality/api/*）读取 quality_metrics 表，
// 但此前全仓库没有任何写入方，仪表盘/周报永远空转。本命令作为采集器补上生产者：
// CI test job 在覆盖率阈值检查通过后调用，也支持本地 make quality-metrics-store。
//
// 口径一致性：
//   - 打分复用 internal/quality/dashboard.CalculateQualityScore（与仪表盘同一公式）
//   - 落库复用 internal/store/postgres.StoreMetrics（与仪表盘同一 SQL）
//
// 用法：
//
//	go run ./scripts/qualitymetrics -coverage coverage.out -junit test-results.xml -commit "$GITHUB_SHA"
//
// 静态分析计数（lint/gosec/gocyclo）在 CI 中由独立 job 产生，跨 job 回传成本高，
// 默认 0；本地可经 make analyze-* 生成后通过 flag 回填。
package main

import (
	"bufio"
	"context"
	"encoding/xml"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/example/sso/internal/quality/dashboard"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/store/postgres"
)

func main() {
	coveragePath := flag.String("coverage", "coverage.out", "go test -coverprofile 产物路径")
	junitPath := flag.String("junit", "", "gotestsum --junitfile 产物路径（可选，缺省按全部通过计）")
	databaseURL := flag.String("database", os.Getenv("DATABASE_URL"), "PostgreSQL DSN（默认读 DATABASE_URL 环境变量）")
	commit := flag.String("commit", os.Getenv("GITHUB_SHA"), "git commit SHA（默认读 GITHUB_SHA）")
	lintViolations := flag.Int("lint-violations", 0, "lint 违规数")
	gosecViolations := flag.Int("gosec-violations", 0, "gosec 违规数")
	gocycloViolations := flag.Int("gocyclo-violations", 0, "gocyclo 违规数")
	flag.Parse()

	if *databaseURL == "" {
		fatalf("未提供数据库连接：请设置 DATABASE_URL 或传 -database")
	}

	coverage, err := parseCoverageProfile(*coveragePath)
	if err != nil {
		fatalf("解析覆盖率文件 %s 失败: %v", *coveragePath, err)
	}

	metrics := &store.QualityMetrics{
		RecordedAt:        time.Now().UTC(),
		GitCommitSHA:      *commit,
		CoveragePercent:   coverage.percent,
		LintViolations:    *lintViolations,
		GosecViolations:   *gosecViolations,
		GocycloViolations: *gocycloViolations,
		Metadata: map[string]interface{}{
			"source": "test-job-collector",
		},
	}

	if *junitPath != "" {
		junit, err := parseJUnit(*junitPath)
		if err != nil {
			fatalf("解析 JUnit 文件 %s 失败: %v", *junitPath, err)
		}
		metrics.TotalTests = junit.total
		metrics.PassedTests = junit.passed
		metrics.FailedTests = junit.failed
		if junit.total > 0 {
			metrics.TestPassRate = round2(float64(junit.passed) / float64(junit.total) * 100)
		} else {
			// JUnit 无测试数据（如只跑了无用例的包）时按全部通过计，计数保持 0 便于识别
			metrics.TestPassRate = 100
		}
	} else {
		metrics.TestPassRate = 100
	}

	score := dashboard.CalculateQualityScore(metrics)
	metrics.QualityScore = score.Score
	metrics.Metadata["grade"] = score.Grade

	db, err := postgres.NewFromURL(*databaseURL)
	if err != nil {
		fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.StoreMetrics(ctx, metrics); err != nil {
		fatalf("写入质量指标失败: %v", err)
	}

	fmt.Printf("✅ 质量指标已写入: coverage=%.2f%% passRate=%.2f%% tests=%d score=%.2f(%s)\n",
		metrics.CoveragePercent, metrics.TestPassRate, metrics.TotalTests, metrics.QualityScore, score.Grade)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "❌ "+format+"\n", args...)
	os.Exit(1)
}

// round2 四舍五入到两位小数（与 dashboard.CalculateQualityScore 的口径一致）
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// ============================================================================
// 覆盖率 profile 解析
// ============================================================================

// coverageStats 覆盖率统计结果
type coverageStats struct {
	percent float64
}

// parseCoverageProfile 解析 go test -coverprofile 产物（mode: set/count/atomic）
//
// 统计口径与 go tool cover -func 的 total 一致：
// 每行 "文件:起止位置 语句数 命中数"，命中数 > 0 视为该块语句全部覆盖
func parseCoverageProfile(path string) (*coverageStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var totalStmts, coveredStmts int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") || line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 3 {
			return nil, fmt.Errorf("格式异常的覆盖率行: %q", line)
		}
		stmts, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("语句数解析失败 %q: %w", parts[1], err)
		}
		count, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("命中数解析失败 %q: %w", parts[2], err)
		}

		totalStmts += stmts
		if count > 0 {
			coveredStmts += stmts
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if totalStmts == 0 {
		return nil, fmt.Errorf("覆盖率文件无有效数据")
	}

	return &coverageStats{percent: round2(float64(coveredStmts) / float64(totalStmts) * 100)}, nil
}

// ============================================================================
// JUnit XML 解析
// ============================================================================

// junitStats JUnit 测试统计结果
type junitStats struct {
	total  int
	passed int
	failed int
}

// junitSuites gotestsum --junitfile 产物的最小解析结构（兼容 testsuites 根节点）
// 不设 XMLName：单 testsuite 根节点时根元素本身即匹配 testsuite 字段
type junitSuites struct {
	Suites []junitSuite `xml:"testsuite"`
}

// junitSuite 单个 testsuite 的计数属性
type junitSuite struct {
	Tests    int `xml:"tests,attr"`
	Failures int `xml:"failures,attr"`
	Errors   int `xml:"errors,attr"`
	Skipped  int `xml:"skipped,attr"`
}

// parseJUnit 解析 JUnit XML，返回总数/通过数/失败数（skipped 不计入通过率分母）
func parseJUnit(path string) (*junitStats, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var suites junitSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		return nil, err
	}

	// 兼容单 testsuite 根节点（xml 包不会把根元素匹配到字段上）
	if len(suites.Suites) == 0 {
		var single junitSuite
		if err := xml.Unmarshal(data, &single); err == nil && single.Tests > 0 {
			suites.Suites = append(suites.Suites, single)
		}
	}

	stats := &junitStats{}
	skipped := 0
	for _, s := range suites.Suites {
		stats.total += s.Tests
		stats.failed += s.Failures + s.Errors
		skipped += s.Skipped
	}
	stats.passed = stats.total - stats.failed - skipped
	if stats.passed < 0 {
		stats.passed = 0
	}
	return stats, nil
}
