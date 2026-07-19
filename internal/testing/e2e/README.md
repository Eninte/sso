# E2E Test Isolation Helpers

Test data isolation utilities for E2E tests in the SSO service. These helpers
keep tests independent by ensuring data created during a test is removed
afterwards, including data written asynchronously by the audit worker pool.

## Components

### IsolationHelper

Core utility for test data isolation:

- **Transaction Isolation** — `WithTransaction` runs a function inside a DB
  transaction that is always rolled back, ensuring no persistent test data.
- **Redis Namespace Isolation** — `WithRedisNamespace` wraps a Redis client
  to prefix all keys with a namespace, then cleans them up automatically.
- **Pattern-based Cleanup** — `CleanupTestDataByPattern` deletes rows
  matching a testID pattern across all tables in dependency order, with
  special handling for audit logs that reference server-generated UUIDs.
- **Retry Cleanup** — `CleanupTestDataByPatternWithRetry` re-runs the full
  cleanup to catch audit logs written asynchronously after the first pass.

### Test Identifier Helpers

- **`GenerateTestID(testName)`** — produces a unique identifier
  (`e2e_<unix_nano>_<sanitized_name>`) that can be embedded in test data
  (emails, client IDs) so pattern-based cleanup can find and remove it.
- **`SanitizeTestName(name)`** — converts a test name into a safe
  identifier component by replacing non-alphanumeric characters with hyphens.

## Usage

### Basic Setup

```go
package e2e_test

import (
    "context"
    "database/sql"

    "github.com/example/sso/internal/testing/e2e"
    "github.com/redis/go-redis/v9"
)

func TestExample(t *testing.T) {
    db, _ := sql.Open("pgx", "postgres://...")
    rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

    helper := e2e.NewIsolationHelper(db, rdb)
    testID := e2e.GenerateTestID(t.Name())
    email := fmt.Sprintf("user-%s@example.com", testID)

    defer func() {
        ctx := context.Background()
        _ = helper.CleanupTestDataByPatternWithRetry(ctx, "%"+testID+"%", 2*time.Second, 3)
    }()

    // ... create user with this email, run assertions ...
}
```

### Transaction Isolation

```go
err := helper.WithTransaction(ctx, func(tx *sql.Tx) error {
    // Operations on tx are rolled back automatically after fn returns.
    return nil
})
```

### Redis Namespace Isolation

```go
err := helper.WithRedisNamespace(ctx, "test:mytest", func(ns *e2e.NamespacedRedisClient) error {
    // All keys are prefixed with "test:mytest:" and removed on return.
    return ns.Set(ctx, "foo", "bar", time.Minute)
})
```

## Integration with Existing E2E Tests

`test/e2e/helpers.go` uses these helpers in `TestMain` and registers
per-test cleanup via `t.Cleanup`. New E2E tests should:

1. Call `e2e.GenerateTestID(t.Name())` at the start of each test.
2. Embed the testID in created entities (emails, client IDs, etc.).
3. Register `CleanupTestDataByPatternWithRetry` via `t.Cleanup`.

## Audit Log Cleanup

Audit logs are written asynchronously by a worker pool. The helper handles
this in three phases:

1. Collect user UUIDs matching the pattern.
2. Delete audit logs by those UUIDs.
3. Sweep loop polls for late writes; `CleanupTestDataByPatternWithRetry`
   re-runs the full cleanup to catch any residual audit logs.

## Testing

Run the unit tests:

```bash
go test -v ./internal/testing/e2e/
```
