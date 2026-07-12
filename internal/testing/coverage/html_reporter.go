// Package coverage HTML覆盖率报告生成器
package coverage

import (
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"
)

// ============================================================================
// HTMLReporter 类型定义
// ============================================================================

// HTMLReporter HTML报告生成器
type HTMLReporter struct {
	analyzer *CoverageAnalyzer
	template *template.Template
}

// NewHTMLReporter 创建HTML报告生成器
func NewHTMLReporter(analyzer *CoverageAnalyzer) *HTMLReporter {
	tmpl := template.Must(template.New("coverage").Funcs(template.FuncMap{
		"formatPercent": func(v float64) string {
			return fmt.Sprintf("%.2f%%", v)
		},
		"formatInt": func(v int) string {
			return fmt.Sprintf("%d", v)
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"coverageClass": func(coverage, threshold float64) string {
			if coverage >= threshold {
				return "success"
			} else if coverage >= threshold-10 {
				return "warning"
			}
			return "danger"
		},
		"criticalityClass": func(level CriticalityLevel) string {
			switch level {
			case CriticalityHigh:
				return "danger"
			case CriticalityMedium:
				return "warning"
			case CriticalityLow:
				return "info"
			default:
				return "info"
			}
		},
		"priorityClass": func(priority string) string {
			switch priority {
			case "High":
				return "danger"
			case "Medium":
				return "warning"
			default:
				return "info"
			}
		},
		"shortPath": func(path string, maxLen int) string {
			if len(path) <= maxLen {
				return path
			}
			parts := strings.Split(path, "/")
			if len(parts) > 2 {
				return ".../" + strings.Join(parts[len(parts)-2:], "/")
			}
			return path
		},
	}).Parse(htmlTemplate))

	return &HTMLReporter{
		analyzer: analyzer,
		template: tmpl,
	}
}

// ============================================================================
// 报告生成方法
// ============================================================================

// GenerateHTML 生成HTML覆盖率报告
// 参数：
//   - w: 输出流
//   - report: 覆盖率报告
//
// 返回：
//   - error: 错误信息
func (hr *HTMLReporter) GenerateHTML(w io.Writer, report *CoverageReport) error {
	// 准备报告数据
	data := hr.prepareReportData(report)

	// 渲染模板
	if err := hr.template.Execute(w, data); err != nil {
		return fmt.Errorf("failed to render HTML template: %w", err)
	}

	return nil
}

// prepareReportData 准备报告数据
func (hr *HTMLReporter) prepareReportData(report *CoverageReport) map[string]interface{} {
	// 获取包缺口信息
	deficits := hr.analyzer.GetPackagesBelowThreshold(report)

	// 获取通过阈值的包
	passedPackages := hr.getPassedPackages(report)

	// 获取改进计划
	var plan *RemediationPlan
	if len(deficits) > 0 {
		plan = hr.analyzer.CalculateRemediationPlan(report)
	}

	// 获取文件详情列表（按覆盖率排序）
	fileDetails := hr.getSortedFileDetails(report)

	return map[string]interface{}{
		"GeneratedAt":      time.Now().Format("2006-01-02 15:04:05"),
		"Report":           report,
		"Threshold":        hr.analyzer.threshold,
		"Passed":           report.OverallCoverage >= hr.analyzer.threshold,
		"Deficits":         deficits,
		"PassedPackages":   passedPackages,
		"Plan":             plan,
		"FileDetails":      fileDetails,
		"HasCriticalGaps":  len(report.UncoveredCritical) > 0,
		"CriticalGapsTop5": hr.getTopCriticalGaps(report, 5),
	}
}

// getPassedPackages 获取通过阈值的包列表
func (hr *HTMLReporter) getPassedPackages(report *CoverageReport) []PackageInfo {
	packages := make([]PackageInfo, 0)

	// 统计每个包的语句数
	packageStats := make(map[string]*packageCoverage)
	for _, detail := range report.FileDetails {
		pkgPath := getPackagePath(detail.FilePath)
		stats, exists := packageStats[pkgPath]
		if !exists {
			stats = &packageCoverage{}
			packageStats[pkgPath] = stats
		}
		stats.totalStmt += detail.TotalStatements
		stats.coveredStmt += detail.CoveredStatements
	}

	// 找出通过阈值的包
	for pkgPath, stats := range packageStats {
		if stats.totalStmt == 0 {
			continue
		}

		coverage := float64(stats.coveredStmt) / float64(stats.totalStmt) * 100.0
		if coverage >= hr.analyzer.threshold {
			packages = append(packages, PackageInfo{
				Path:     pkgPath,
				Coverage: coverage,
				Total:    stats.totalStmt,
				Covered:  stats.coveredStmt,
			})
		}
	}

	// 按覆盖率从高到低排序
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Coverage > packages[j].Coverage
	})

	return packages
}

// getSortedFileDetails 获取排序后的文件详情列表
func (hr *HTMLReporter) getSortedFileDetails(report *CoverageReport) []FileDetail {
	files := make([]FileDetail, 0, len(report.FileDetails))
	for _, detail := range report.FileDetails {
		files = append(files, *detail)
	}

	// 按覆盖率从低到高排序（优先显示需要改进的文件）
	sort.Slice(files, func(i, j int) bool {
		return files[i].Coverage < files[j].Coverage
	})

	return files
}

// getTopCriticalGaps 获取前N个关键未覆盖路径
func (hr *HTMLReporter) getTopCriticalGaps(report *CoverageReport, n int) []UncoveredPath {
	if len(report.UncoveredCritical) <= n {
		return report.UncoveredCritical
	}
	return report.UncoveredCritical[:n]
}

// ============================================================================
// 辅助类型
// ============================================================================

// PackageInfo 包信息
type PackageInfo struct {
	Path     string  // 包路径
	Coverage float64 // 覆盖率
	Total    int     // 总语句数
	Covered  int     // 已覆盖语句数
}

// ============================================================================
// HTML模板
// ============================================================================

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Code Coverage Report</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            background: #f5f5f5;
            padding: 20px;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .header {
            border-bottom: 3px solid #1e88e5;
            padding-bottom: 20px;
            margin-bottom: 30px;
        }
        h1 {
            color: #1e88e5;
            font-size: 32px;
            margin-bottom: 10px;
        }
        .timestamp {
            color: #666;
            font-size: 14px;
        }
        .summary {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .metric {
            padding: 20px;
            border-radius: 6px;
            border-left: 4px solid #1e88e5;
            background: #f8f9fa;
        }
        .metric-label {
            font-size: 14px;
            color: #666;
            margin-bottom: 5px;
        }
        .metric-value {
            font-size: 28px;
            font-weight: bold;
            color: #333;
        }
        .metric.success {
            border-left-color: #4caf50;
        }
        .metric.warning {
            border-left-color: #ff9800;
        }
        .metric.danger {
            border-left-color: #f44336;
        }
        .section {
            margin-bottom: 40px;
        }
        .section-title {
            font-size: 24px;
            color: #333;
            margin-bottom: 15px;
            padding-bottom: 10px;
            border-bottom: 2px solid #e0e0e0;
        }
        .status-badge {
            display: inline-block;
            padding: 6px 12px;
            border-radius: 4px;
            font-size: 14px;
            font-weight: bold;
            margin-left: 10px;
        }
        .status-badge.success {
            background: #4caf50;
            color: white;
        }
        .status-badge.danger {
            background: #f44336;
            color: white;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 15px;
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #e0e0e0;
        }
        th {
            background: #f8f9fa;
            font-weight: 600;
            color: #555;
        }
        tr:hover {
            background: #f8f9fa;
        }
        .coverage-bar {
            height: 20px;
            border-radius: 4px;
            background: #e0e0e0;
            overflow: hidden;
            position: relative;
        }
        .coverage-fill {
            height: 100%;
            background: #4caf50;
            transition: width 0.3s ease;
        }
        .coverage-fill.warning {
            background: #ff9800;
        }
        .coverage-fill.danger {
            background: #f44336;
        }
        .badge {
            display: inline-block;
            padding: 4px 8px;
            border-radius: 3px;
            font-size: 12px;
            font-weight: 600;
        }
        .badge.success {
            background: #e8f5e9;
            color: #2e7d32;
        }
        .badge.warning {
            background: #fff3e0;
            color: #ef6c00;
        }
        .badge.danger {
            background: #ffebee;
            color: #c62828;
        }
        .badge.info {
            background: #e3f2fd;
            color: #1565c0;
        }
        .plan-card {
            background: #f8f9fa;
            padding: 20px;
            border-radius: 6px;
            margin-bottom: 15px;
            border-left: 4px solid #1e88e5;
        }
        .plan-card.high {
            border-left-color: #f44336;
        }
        .plan-card.medium {
            border-left-color: #ff9800;
        }
        .plan-card.low {
            border-left-color: #4caf50;
        }
        .suggestions {
            list-style: none;
            margin-top: 10px;
        }
        .suggestions li {
            padding: 5px 0;
            padding-left: 20px;
            position: relative;
        }
        .suggestions li:before {
            content: "•";
            position: absolute;
            left: 0;
            color: #1e88e5;
            font-weight: bold;
        }
        .critical-gap {
            background: #fff3e0;
            padding: 15px;
            border-radius: 6px;
            margin-bottom: 10px;
            border-left: 4px solid #ff9800;
        }
        .critical-gap.high {
            background: #ffebee;
            border-left-color: #f44336;
        }
        .code-path {
            font-family: "Courier New", monospace;
            font-size: 13px;
            color: #d32f2f;
        }
        .line-numbers {
            color: #666;
            font-size: 12px;
        }
        @media print {
            body {
                background: white;
                padding: 0;
            }
            .container {
                box-shadow: none;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Code Coverage Report</h1>
            <div class="timestamp">Generated: {{.GeneratedAt}}</div>
        </div>

        <!-- Summary Section -->
        <div class="summary">
            <div class="metric {{coverageClass .Report.OverallCoverage .Threshold}}">
                <div class="metric-label">Overall Coverage</div>
                <div class="metric-value">{{formatPercent .Report.OverallCoverage}}</div>
            </div>
            <div class="metric">
                <div class="metric-label">Threshold</div>
                <div class="metric-value">{{formatPercent .Threshold}}</div>
            </div>
            <div class="metric">
                <div class="metric-label">Covered Statements</div>
                <div class="metric-value">{{formatInt .Report.CoveredStatements}} / {{formatInt .Report.TotalStatements}}</div>
            </div>
            {{if not .Passed}}
            <div class="metric danger">
                <div class="metric-label">Coverage Deficit</div>
                <div class="metric-value">{{formatPercent .Report.CoverageDeficit}}</div>
            </div>
            {{end}}
        </div>

        <!-- Status Section -->
        <div class="section">
            <h2 class="section-title">
                Build Status
                {{if .Passed}}
                <span class="status-badge success">✓ PASSED</span>
                {{else}}
                <span class="status-badge danger">✗ FAILED</span>
                {{end}}
            </h2>
            {{if .Passed}}
            <p>✅ Coverage meets the threshold of {{formatPercent .Threshold}}. All quality checks passed.</p>
            {{else}}
            <p>❌ Coverage ({{formatPercent .Report.OverallCoverage}}) is below the threshold ({{formatPercent .Threshold}}). 
               Additional {{formatInt .Plan.TotalEstimatedTests}} test case(s) needed.</p>
            {{end}}
        </div>

        <!-- Package Coverage Section -->
        {{if .Deficits}}
        <div class="section">
            <h2 class="section-title">Packages Below Threshold ({{len .Deficits}})</h2>
            <table>
                <thead>
                    <tr>
                        <th>Package</th>
                        <th style="width: 200px">Coverage</th>
                        <th style="text-align: center">Deficit</th>
                        <th style="text-align: center">Tests Needed</th>
                    </tr>
                </thead>
                <tbody>
                {{range .Deficits}}
                    <tr>
                        <td>{{shortPath .PackagePath 60}}</td>
                        <td>
                            <div class="coverage-bar">
                                <div class="coverage-fill {{coverageClass .CurrentCoverage $.Threshold}}" 
                                     style="width: {{.CurrentCoverage}}%"></div>
                            </div>
                            <small>{{formatPercent .CurrentCoverage}}</small>
                        </td>
                        <td style="text-align: center">{{formatPercent .Deficit}}</td>
                        <td style="text-align: center">~{{formatInt .EstimatedTests}}</td>
                    </tr>
                {{end}}
                </tbody>
            </table>
        </div>
        {{end}}

        <!-- Passed Packages Section -->
        {{if .PassedPackages}}
        <div class="section">
            <h2 class="section-title">Packages Meeting Threshold ({{len .PassedPackages}})</h2>
            <table>
                <thead>
                    <tr>
                        <th>Package</th>
                        <th style="width: 200px">Coverage</th>
                        <th style="text-align: center">Statements</th>
                    </tr>
                </thead>
                <tbody>
                {{range .PassedPackages}}
                    <tr>
                        <td>{{shortPath .Path 60}}</td>
                        <td>
                            <div class="coverage-bar">
                                <div class="coverage-fill success" style="width: {{.Coverage}}%"></div>
                            </div>
                            <small>{{formatPercent .Coverage}}</small>
                        </td>
                        <td style="text-align: center">{{formatInt .Covered}} / {{formatInt .Total}}</td>
                    </tr>
                {{end}}
                </tbody>
            </table>
        </div>
        {{end}}

        <!-- Critical Gaps Section -->
        {{if .HasCriticalGaps}}
        <div class="section">
            <h2 class="section-title">⚠️ Uncovered Critical Paths</h2>
            {{range .CriticalGapsTop5}}
            <div class="critical-gap {{criticalityClass .Criticality}}">
                <div><span class="badge {{criticalityClass .Criticality}}">{{.Criticality}}</span></div>
                <div class="code-path">{{.File}}</div>
                <div class="line-numbers">Lines: {{range $i, $line := .Lines}}{{if $i}}, {{end}}{{$line}}{{end}}</div>
            </div>
            {{end}}
            {{if gt (len .Report.UncoveredCritical) 5}}
            <p><small>... and {{sub (len .Report.UncoveredCritical) 5}} more critical paths</small></p>
            {{end}}
        </div>
        {{end}}

        <!-- Remediation Plan Section -->
        {{if .Plan}}
        <div class="section">
            <h2 class="section-title">Remediation Plan</h2>
            <p>To reach {{formatPercent .Plan.TargetCoverage}} coverage, add approximately <strong>{{formatInt .Plan.TotalEstimatedTests}} test case(s)</strong>.</p>
            
            {{range .Plan.PackageActions}}
            <div class="plan-card {{.Priority}}">
                <div>
                    <span class="badge {{priorityClass .Priority}}">{{.Priority}} Priority</span>
                    <strong>{{shortPath .PackagePath 80}}</strong>
                </div>
                <div style="margin: 10px 0">
                    Current: <strong>{{formatPercent .CurrentCoverage}}</strong> → 
                    Target: <strong>{{formatPercent .TargetCoverage}}</strong> 
                    (add ~{{formatInt .RequiredTests}} tests)
                </div>
                <ul class="suggestions">
                {{range .Suggestions}}
                    <li>{{.}}</li>
                {{end}}
                </ul>
            </div>
            {{end}}
        </div>
        {{end}}

        <!-- File Details Section -->
        <div class="section">
            <h2 class="section-title">File Coverage Details</h2>
            <table>
                <thead>
                    <tr>
                        <th>File</th>
                        <th style="width: 200px">Coverage</th>
                        <th style="text-align: center">Covered / Total</th>
                        <th style="text-align: center">Uncovered Lines</th>
                    </tr>
                </thead>
                <tbody>
                {{range .FileDetails}}
                    <tr>
                        <td>{{shortPath .FilePath 70}}</td>
                        <td>
                            <div class="coverage-bar">
                                <div class="coverage-fill {{coverageClass .Coverage $.Threshold}}" 
                                     style="width: {{.Coverage}}%"></div>
                            </div>
                            <small>{{formatPercent .Coverage}}</small>
                        </td>
                        <td style="text-align: center">{{formatInt .CoveredStatements}} / {{formatInt .TotalStatements}}</td>
                        <td style="text-align: center">{{len .UncoveredLines}}</td>
                    </tr>
                {{end}}
                </tbody>
            </table>
        </div>
    </div>
</body>
</html>
`
