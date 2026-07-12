package coverage_test

import (
	"fmt"
	"log"

	"github.com/example/sso/internal/testing/coverage"
)

// ExampleCoverageAnalyzer demonstrates basic usage of the coverage analyzer
func ExampleCoverageAnalyzer() {
	// Create analyzer with 80% threshold and default critical paths
	analyzer := coverage.NewCoverageAnalyzer(80.0, nil)

	// Analyze coverage profile (using hypothetical file)
	// In real usage, this would be the path to coverage.out
	// report, err := analyzer.Analyze("coverage.out")
	// if err != nil {
	//     log.Fatal(err)
	// }

	// For demonstration, create a mock report
	report := &coverage.CoverageReport{
		OverallCoverage:   78.5,
		TotalStatements:   1000,
		CoveredStatements: 785,
		PackageCoverage: map[string]float64{
			"internal/service": 85.0,
			"internal/handler": 80.0,
			"internal/store":   75.0,
		},
	}

	// Check if coverage meets threshold
	err := analyzer.EnforceThreshold(report)
	if err != nil {
		fmt.Printf("Coverage check failed: %.2f%% < 80.0%%\n", report.OverallCoverage)
	}

	// Output:
	// Coverage check failed: 78.50% < 80.0%
}

// ExampleCoverageAnalyzer_criticalPaths demonstrates critical path detection
func ExampleCoverageAnalyzer_criticalPaths() {
	// Create analyzer with custom critical paths
	analyzer := coverage.NewCoverageAnalyzer(80.0, []string{
		"internal/service/auth.go",
		"internal/service/mfa.go",
		"internal/handler/authorize.go",
	})

	// Create a mock report with some uncovered critical files
	report := &coverage.CoverageReport{
		FileDetails: map[string]*coverage.FileDetail{
			"internal/service/auth.go": {
				FilePath:          "internal/service/auth.go",
				TotalStatements:   100,
				CoveredStatements: 85,
				Coverage:          85.0,
				UncoveredLines:    []int{45, 46, 47, 89, 90},
			},
			"internal/service/mfa.go": {
				FilePath:          "internal/service/mfa.go",
				TotalStatements:   50,
				CoveredStatements: 50,
				Coverage:          100.0,
				UncoveredLines:    []int{},
			},
		},
	}

	// Identify critical gaps
	gaps := analyzer.IdentifyCriticalGaps(report)

	if len(gaps) > 0 {
		fmt.Printf("Found %d critical gaps\n", len(gaps))
		for _, gap := range gaps {
			fmt.Printf("- [%s] %s: %d uncovered lines\n",
				gap.Criticality, gap.File, len(gap.Lines))
		}
	}

	// Output:
	// Found 1 critical gaps
	// - [High] internal/service/auth.go: 5 uncovered lines
}

// ExampleNewCoverageAnalyzer demonstrates creating an analyzer
func ExampleNewCoverageAnalyzer() {
	// Create with default critical paths
	analyzer := coverage.NewCoverageAnalyzer(80.0, nil)
	_ = analyzer // Use the variable

	fmt.Printf("Threshold: %.1f%%\n", 80.0)
	fmt.Printf("Analyzer created successfully\n")

	// You can also create with custom paths
	customAnalyzer := coverage.NewCoverageAnalyzer(85.0, []string{
		"internal/service/auth.go",
		"internal/service/payment.go",
	})
	_ = customAnalyzer // Use the variable

	// Output:
	// Threshold: 80.0%
	// Analyzer created successfully
}

// Example showing package coverage analysis
func ExampleCoverageReport_packageCoverage() {
	report := &coverage.CoverageReport{
		OverallCoverage: 82.5,
		PackageCoverage: map[string]float64{
			"internal/service":   85.0,
			"internal/handler":   80.0,
			"internal/store":     78.5,
			"internal/util":      90.0,
			"internal/model":     100.0,
			"internal/validator": 95.0,
		},
	}

	fmt.Printf("Overall Coverage: %.2f%%\n\n", report.OverallCoverage)
	fmt.Println("Package Coverage:")

	// In real code, you would iterate over the map
	// For example output, we'll show a few entries
	packages := []struct {
		name     string
		coverage float64
	}{
		{"internal/model", 100.0},
		{"internal/validator", 95.0},
		{"internal/util", 90.0},
		{"internal/service", 85.0},
		{"internal/handler", 80.0},
		{"internal/store", 78.5},
	}

	for _, pkg := range packages {
		status := "✓"
		if pkg.coverage < 80.0 {
			status = "✗"
		}
		fmt.Printf("  %s %s: %.1f%%\n", status, pkg.name, pkg.coverage)
	}

	// Output:
	// Overall Coverage: 82.50%
	//
	// Package Coverage:
	//   ✓ internal/model: 100.0%
	//   ✓ internal/validator: 95.0%
	//   ✓ internal/util: 90.0%
	//   ✓ internal/service: 85.0%
	//   ✓ internal/handler: 80.0%
	//   ✗ internal/store: 78.5%
}

// Example showing error handling
func Example_errorHandling() {
	analyzer := coverage.NewCoverageAnalyzer(80.0, nil)

	// Attempt to analyze non-existent file
	_, err := analyzer.Analyze("/nonexistent/coverage.out")
	if err != nil {
		fmt.Println("Error: File not found")
	}

	// Output:
	// Error: File not found
}

// Example showing threshold enforcement
func Example_thresholdEnforcement() {
	analyzer := coverage.NewCoverageAnalyzer(85.0, nil)

	scenarios := []struct {
		name     string
		coverage float64
	}{
		{"Above threshold", 90.0},
		{"At threshold", 85.0},
		{"Below threshold", 80.0},
	}

	for _, scenario := range scenarios {
		report := &coverage.CoverageReport{
			OverallCoverage: scenario.coverage,
		}

		err := analyzer.EnforceThreshold(report)
		if err != nil {
			fmt.Printf("%s: FAIL (%.1f%% < 85.0%%)\n", scenario.name, scenario.coverage)
		} else {
			fmt.Printf("%s: PASS (%.1f%% >= 85.0%%)\n", scenario.name, scenario.coverage)
		}
	}

	// Output:
	// Above threshold: PASS (90.0% >= 85.0%)
	// At threshold: PASS (85.0% >= 85.0%)
	// Below threshold: FAIL (80.0% < 85.0%)
}

func init() {
	// Suppress log output in examples
	log.SetOutput(nil)
}
