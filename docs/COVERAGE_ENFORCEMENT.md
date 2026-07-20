# Coverage Threshold Enforcement

This document explains the coverage threshold enforcement mechanism used in the SSO project.

## Overview

Coverage enforcement uses the standard Go toolchain (`go test -coverprofile` + `go tool cover`) and Makefile targets. The build fails when overall coverage falls below 80%.

## Usage

### Generate Coverage Report and Check Threshold

```bash
# Run tests, generate HTML report, and enforce 80% threshold
make test-coverage

# Quick threshold check only (no HTML)
make test-coverage-check
```

Both targets:

1. Run `go test -coverprofile=coverage.out` (excluding `internal/store/mock` and `internal/app` for `test-coverage-check`).
2. Extract the overall coverage percentage from `go tool cover -func=coverage.out`.
3. Compare against the 80% threshold using `awk`; exit non-zero on failure.

### Manual Inspection

```bash
# Function-level coverage in the terminal
go tool cover -func=coverage.out

# HTML report for browsing uncovered lines
go tool cover -html=coverage.out -o coverage.html
```

## Merging Coverage Profiles

To combine unit, integration, and E2E coverage into a single report:

```bash
make test-coverage-full
```

This target runs unit/integration and E2E tests separately, then merges the
profiles with `scripts/merge_coverage.go` (a union merger for mode:set
profiles, since `go tool cover` has no built-in merge support) and emits
`full-coverage.html`.

## CI Integration

The CI workflow (`.github/workflows/ci.yml`) enforces the 80% threshold in
the `test` job:

```bash
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
if [ $(echo "$COVERAGE < 80" | bc) -eq 1 ]; then exit 1; fi
```

## Excluded Paths

Coverage statistics exclude:

- `internal/app` — assembled by E2E tests
- `internal/store/mock` — generated mock code
- `internal/testing/` — test infrastructure
- `cmd/` — entry points
- `sdks/` — client SDKs

## Threshold Adjustment

To temporarily adjust the threshold (e.g., during a large refactor), edit
the `awk` comparison in the `test-coverage` and `test-coverage-check`
targets of the `Makefile`. Do not lower the threshold permanently without
team review.
