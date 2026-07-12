# Coverage Reporting Tools

This package provides comprehensive code coverage analysis, reporting, and visualization tools for the SSO service.

## Features

### 1. Coverage Analysis
- Parse and analyze Go coverage profiles
- Calculate package-level and file-level coverage
- Identify critical uncovered paths (authentication, MFA, OAuth flows)
- Enforce coverage thresholds

### 2. Report Generation
- **HTML Reports**: Beautiful, interactive HTML reports with charts and drill-down
- **JSON Reports**: Machine-readable JSON for CI/CD integration
- **Text Reports**: Detailed console output with remediation plans

### 3. Diff Analysis
- Compare current vs. previous coverage
- Identify improved and regressed packages/files
- Track coverage trends over time

### 4. Trend Visualization
- Export trend data for dashboards
- Generate chart data for visualization libraries
- Calculate coverage metrics (peak, average, delta)

## Usage

### Command-Line Tool

```bash
# Basic coverage check (fails build if < 80%)
go run cmd/coverage-check/main.go -profile coverage.out -threshold 80.0

# Generate HTML report
go run cmd/coverage-check/main.go -profile coverage.out -html coverage-report.html

# Generate JSON report (for CI/CD)
go run cmd/coverage-check/main.go -profile coverage.out -json coverage-report.json

# Compare with previous coverage
go run cmd/coverage-check/main.go -profile coverage.out -diff coverage-previous.out

# Export trend data
go run cmd/coverage-check/main.go -profile coverage.out -trend-data trend.json

# Export chart data (for front-end visualization)
go run cmd/coverage-check/main.go -profile coverage.out -trend-chart chart.json

# Verbose output
go run cmd/coverage-check/main.go -profile coverage.out -verbose
```

### Programmatic Usage

```go
package main

import (
    "os"
    "github.com/example/sso/internal/testing/coverage"
)

func main() {
    // Create analyzer
    analyzer := coverage.NewCoverageAnalyzer(80.0, nil)
    
    // Analyze coverage
    report, err := analyzer.Analyze("coverage.out")
    if err != nil {
        panic(err)
    }
    
    // Create reporter
    reporter := coverage.NewReporter(analyzer)
    
    // Generate HTML report
    htmlFile, _ := os.Create("coverage.html")
    defer htmlFile.Close()
    reporter.GenerateHTML(htmlFile, report)
    
    // Generate JSON report
    jsonFile, _ := os.Create("coverage.json")
    defer jsonFile.Close()
    reporter.GenerateJSON(jsonFile, report)
    
    // Enforce threshold
    if err := analyzer.EnforceThreshold(report); err != nil {
        // Coverage below threshold
        os.Exit(1)
    }
}
```

### Diff Analysis

```go
// Create diff analyzer
diffAnalyzer := coverage.NewDiffAnalyzer()

// Analyze current and previous coverage
current, _ := analyzer.Analyze("coverage.out")
previous, _ := analyzer.Analyze("coverage-previous.out")

// Compare
diff := diffAnalyzer.CompareCoverage(current, previous)

// Format diff report
formatted := diffAnalyzer.FormatDiff(diff)
fmt.Println(formatted)
```

### Trend Data Export

```go
// Create trend exporter
exporter := coverage.NewTrendExporter()

// Create data point from report
dataPoint := exporter.CreateDataPoint(report, time.Now(), map[string]string{
    "commit": "abc123",
    "branch": "main",
})

// Export trend data
trendFile, _ := os.Create("trend.json")
defer trendFile.Close()
exporter.ExportTrendData(trendFile, []coverage.TrendDataPoint{dataPoint})

// Export chart data
chartFile, _ := os.Create("chart.json")
defer chartFile.Close()
exporter.ExportChartData(chartFile, []coverage.TrendDataPoint{dataPoint})
```

## Integration with Makefile

Add to your `Makefile`:

```makefile
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	@echo "Generating coverage reports..."
	go run cmd/coverage-check/main.go \
		-profile coverage.out \
		-threshold 80.0 \
		-html coverage.html \
		-json coverage.json \
		-verbose

.PHONY: coverage-diff
coverage-diff:
	@echo "Comparing coverage with previous..."
	go run cmd/coverage-check/main.go \
		-profile coverage.out \
		-diff coverage-previous.out
```

## Integration with CI/CD

### GitHub Actions

```yaml
- name: Test with coverage
  run: make test-coverage

- name: Upload coverage report
  uses: actions/upload-artifact@v3
  with:
    name: coverage-report
    path: coverage.html

- name: Upload coverage JSON
  uses: actions/upload-artifact@v3
  with:
    name: coverage-json
    path: coverage.json
```

### GitLab CI

```yaml
test:
  script:
    - make test-coverage
  artifacts:
    paths:
      - coverage.html
      - coverage.json
    reports:
      coverage_report:
        coverage_format: cobertura
        path: coverage.json
```

## Report Examples

### HTML Report
The HTML report includes:
- Overall coverage metrics with visual indicators
- Package-level breakdown with coverage bars
- File-level details
- Uncovered critical paths
- Remediation plan with actionable suggestions
- Responsive design for mobile/desktop

### JSON Report
The JSON report provides:
```json
{
  "timestamp": "2024-01-01T00:00:00Z",
  "threshold": 80.0,
  "passed": false,
  "overall_coverage": 78.74,
  "coverage_deficit": 1.26,
  "total_statements": 5253,
  "covered_statements": 4136,
  "package_coverage": {...},
  "packages_below": [...],
  "critical_gaps": [...],
  "remediation_plan": {...}
}
```

### Diff Report
```
================================================================================
  Coverage Diff Report
================================================================================

Overall Coverage:
  Previous: 75.00%
  Current:  80.00%
  Delta:    +5.00% ↑

Summary:
  Packages: 3 improved, 1 regressed, 2 added, 0 removed
  Files:    12 improved, 2 regressed, 5 added, 0 removed
```

## Critical Paths

The analyzer automatically identifies critical paths in:
- Authentication: `internal/service/auth.go`, `internal/handler/auth.go`
- MFA: `internal/service/mfa.go`, `internal/handler/mfa.go`
- OAuth: `internal/service/oauth.go`, `internal/handler/oauth.go`
- Token Management: `internal/service/token.go`
- Middleware: `internal/middleware/auth.go`

You can customize critical paths:

```go
analyzer := coverage.NewCoverageAnalyzer(80.0, []string{
    "internal/service/custom.go",
    "internal/handler/custom.go",
})
```

## Architecture

```
├── analyzer.go          # Core coverage analysis engine
├── reporter.go          # Text report generation
├── html_reporter.go     # HTML report generation
├── json_reporter.go     # JSON report generation (CI/CD)
├── diff_analyzer.go     # Coverage diff analysis
└── trend_exporter.go    # Trend data export for dashboards
```

## Testing

Run the test suite:

```bash
go test -v ./internal/testing/coverage/...
```

Test coverage for this package:

```bash
go test -coverprofile=coverage.out ./internal/testing/coverage/...
go tool cover -func=coverage.out
```

## Requirements

- Go 1.26+
- Coverage profile generated by `go test -coverprofile=coverage.out`

## See Also

- [COVERAGE_ENFORCEMENT.md](../../../docs/COVERAGE_ENFORCEMENT.md) - Coverage enforcement strategy
- [TESTING.md](../../../docs/TESTING.md) - Testing guidelines
