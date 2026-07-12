# Coverage Threshold Enforcement

This document explains the coverage threshold enforcement system implemented in the SSO project.

## Overview

The coverage enforcement system ensures that code coverage remains above 80% by:

1. **Detailed Package Breakdown** - Shows which packages are below threshold
2. **Coverage Deficit Calculation** - Calculates how much coverage is needed per package
3. **Actionable Remediation Suggestions** - Provides specific guidance on how to improve coverage
4. **Build Failure Mechanism** - Fails the build when coverage is insufficient
5. **Priority-Based Recommendations** - Prioritizes critical packages (auth, MFA, OAuth)

## Usage

### Running Coverage Analysis

```bash
# Generate coverage report and check threshold
make test-coverage

# Quick coverage check only (without regenerating HTML)
make test-coverage-check
```

### Command-Line Tool

The coverage check tool can also be run directly:

```bash
# Basic usage
go run cmd/coverage-check/main.go -profile coverage.out -threshold 80.0

# Verbose output with detailed remediation plan
go run cmd/coverage-check/main.go -profile coverage.out -threshold 80.0 -verbose

# Custom critical paths
go run cmd/coverage-check/main.go \
  -profile coverage.out \
  -threshold 80.0 \
  -critical-paths "internal/service/auth.go,internal/handler/oauth.go"
```

## Report Format

When coverage is below threshold, the report includes:

### 1. Overall Coverage Summary

```
Overall Coverage:
  Current:  78.30% (3655/4668 statements)
  Threshold: 80.00%
  Status:    ❌ FAILED (deficit: 1.70%)
```

### 2. Package Coverage Breakdown

Shows all packages below threshold, sorted by coverage (lowest first):

```
Package                                              Coverage    Deficit    Tests
--------------------------------------------------------------------------------
github.com/example/sso/internal/app                     1.74%     78.26%      41 ✗
github.com/example/sso/internal/util/testutil          27.45%     52.55%       6 ✗
github.com/example/sso/internal/config                 78.45%      1.55%       6 ✗
```

### 3. Remediation Plan

Provides actionable guidance organized by priority:

```
Remediation Plan:
To reach 80.00% coverage, add approximately 67 test case(s)

🔴 High Priority Packages:
  (Auth, MFA, OAuth, Token services)
  
🟡 Medium Priority Packages:
  (Handlers, User services, Config)
  
🟢 Low Priority Packages:
  (Utilities, Helpers)
```

Each package includes:
- Current and target coverage
- Estimated number of tests needed
- Specific suggestions based on package type

### 4. Uncovered Critical Paths

Lists critical code paths that lack coverage:

```
⚠️  Uncovered Critical Paths:
  [High] internal/service/oauth.go (lines: [103 104 105 106 107]...)
  [High] internal/middleware/auth.go (lines: [91 92 93 94 95]...)
```

## Package-Specific Suggestions

The system provides tailored suggestions based on package type:

### Handler Packages
- Focus on HTTP handler test cases with table-driven tests
- Test both success and error response paths
- Verify request validation and error handling

### Service Packages
- Add unit tests for business logic functions
- Test error conditions and edge cases
- Mock Store layer dependencies using `internal/store/mock`

### Store Packages
- Add database integration tests
- Test CRUD operations and error handling
- Verify transaction rollback behavior

### Middleware Packages
- Test middleware with mock HTTP handlers
- Verify request/response transformation
- Test authentication and authorization logic

## Priority Levels

Packages are automatically prioritized based on their role:

### High Priority
- Authentication services (`internal/service/auth`)
- MFA services (`internal/service/mfa`)
- OAuth services (`internal/service/oauth`)
- Token services (`internal/service/token`)
- Auth middleware (`internal/middleware/auth`)

### Medium Priority
- All handlers (`internal/handler/*`)
- User services (`internal/service/user`)
- Admin services (`internal/service/admin`)
- Configuration (`internal/config`)

### Low Priority
- Utility packages (`internal/util/*`)
- Helper functions
- Test utilities

## CI/CD Integration

The coverage check is integrated into the CI/CD pipeline through the Makefile:

```makefile
.PHONY: test-coverage
test-coverage: ## 生成测试覆盖率报告（HTML）并执行阈值检查
	@go test -coverprofile=coverage.out ./internal/...
	@go tool cover -func=coverage.out | grep "total:"
	@go tool cover -html=coverage.out -o coverage.html
	@go run cmd/coverage-check/main.go -profile coverage.out -threshold 80.0 -verbose || { \
		echo ""; \
		echo "❌ 覆盖率未达到80%阈值"; \
		exit 1; \
	}
	@echo "✅ 覆盖率检查通过"
```

**Key Features:**
- Generates coverage profile
- Creates HTML report for visualization
- Runs threshold enforcement
- Fails build if coverage < 80%
- Provides detailed error messages

## Example Output

### Passing Coverage

```
Overall Coverage:
  Current:  82.50% (3850/4668 statements)
  Threshold: 80.00%
  Status:    ✅ PASSED
```

### Failing Coverage (with remediation)

```
Overall Coverage:
  Current:  78.30% (3655/4668 statements)
  Threshold: 80.00%
  Status:    ❌ FAILED (deficit: 1.70%)

Package Coverage Breakdown:
  internal/config: 78.45% (deficit: 1.55%, add ~6 tests)

Remediation Plan:
  📦 internal/config
     Current: 78.45% → Target: 80.00% (add ~6 tests)
     Suggestions:
       • Add 6 test case(s) to cover 39 uncovered statements
       • Review uncovered code paths and add relevant tests
       • Consider table-driven tests for multiple scenarios

❌ BUILD FAILED: Coverage below threshold
```

## Configuration

### Critical Paths

The default critical paths are:

```go
[]string{
    "internal/service/auth.go",
    "internal/service/mfa.go",
    "internal/service/oauth.go",
    "internal/service/token.go",
    "internal/handler/auth.go",
    "internal/handler/mfa.go",
    "internal/handler/oauth.go",
    "internal/middleware/auth.go",
}
```

These can be overridden using the `-critical-paths` flag.

### Threshold

The default threshold is 80%. This can be changed:

```bash
# Set threshold to 85%
go run cmd/coverage-check/main.go -profile coverage.out -threshold 85.0
```

## Best Practices

1. **Run coverage checks before committing**
   ```bash
   make test-coverage
   ```

2. **Address high-priority packages first**
   - Focus on authentication, authorization, and security-critical code
   - These have the highest impact on system security

3. **Use table-driven tests**
   - More efficient for covering multiple scenarios
   - Easier to maintain and extend

4. **Aim for meaningful coverage**
   - Don't just add tests to hit the threshold
   - Ensure tests validate actual behavior and edge cases

5. **Review uncovered critical paths regularly**
   - These represent the highest risk areas
   - Should be addressed as priority

## Troubleshooting

### Coverage report not found

```bash
# Generate coverage profile first
go test -coverprofile=coverage.out ./internal/...

# Then run coverage check
go run cmd/coverage-check/main.go -profile coverage.out -threshold 80.0
```

### Incorrect coverage percentages

```bash
# Exclude mock packages
go test -coverprofile=coverage.out $(go list ./internal/... | grep -v '/store/mock')
```

### Build fails in CI but passes locally

- Ensure test data is properly set up in CI
- Check that all dependencies are available
- Verify environment variables are configured

## Related Documentation

- [Testing Guide](TESTING.md)
- [Development Workflow](../AGENTS.md#开发工作流)
- [Code Quality Standards](../AGENTS.md#代码审查检查清单)
