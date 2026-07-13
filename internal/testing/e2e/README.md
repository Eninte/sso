# E2E Test Runner

The E2E Test Runner component provides infrastructure for stabilizing end-to-end test execution in the SSO service. It addresses the goal of achieving 100% E2E test pass rate (156/156 tests) through improved test isolation, environment validation, and detailed failure logging.

## Components

### TestRunner

The core orchestrator for E2E test execution. Provides:

- **Environment Validation**: Pre-flight checks for PostgreSQL triggers and Redis connectivity
- **Test Isolation**: Database transaction rollback and Redis namespace isolation
- **Failure Logging**: Detailed debugging information including stack traces, HTTP requests/responses, and environment state
- **Sequential Execution**: Runs tests with proper cleanup between each test

### Test Results

- `TestResult`: Captures test execution outcome (PASS/FAIL/SKIP), duration, and optional failure details
- `FailureLog`: Comprehensive debugging information for failed tests
- `TestStatus`: Status enumeration (PASS, FAIL, SKIP)

## Usage

### Basic Setup

```go
package main

import (
    "context"
    "database/sql"

    _ "github.com/jackc/pgx/v5/stdlib"

    "github.com/example/sso/internal/testing/e2e"
    "github.com/redis/go-redis/v9"
)

func main() {
    // Initialize dependencies
    db, _ := sql.Open("pgx", "postgres://...")
    redis := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    
    // Create runner with default config
    runner := e2e.NewTestRunner(db, redis, "http://localhost:9090", nil)
    
    // Validate environment before running tests
    ctx := context.Background()
    if err := runner.ValidateEnvironment(ctx); err != nil {
        log.Fatalf("Environment validation failed: %v", err)
    }
    
    // Define tests
    tests := []e2e.Test{
        {
            Name: "Registration Flow",
            Run: func(ctx context.Context, tr *e2e.TestRunner) error {
                // Test implementation
                return nil
            },
        },
    }
    
    // Execute tests
    results := runner.Run(ctx, tests)
    
    // Process results
    for _, result := range results {
        if result.Status == e2e.TestStatusFail {
            log.Printf("Test %s failed: %s", result.Name, result.FailureLog.ErrorMessage)
        }
    }
}
```

### Custom Configuration

```go
config := &e2e.RunnerConfig{
    ValidatePostgresTriggers: true,   // Check for @example.com auto-verify trigger
    ValidateRedisConnection:  true,   // Verify Redis connectivity
    UseDBTransactions:        true,   // Wrap tests in DB transactions
    RedisNamespaceMode:       true,   // Use test-specific Redis namespaces
    CaptureStackTraces:       true,   // Capture goroutine stacks on failure
    LogEnvironmentVars:       false,  // Don't log env vars (may contain secrets)
}

runner := e2e.NewTestRunner(db, redis, baseURL, config)
```

### Accessing Runner Resources

```go
test := e2e.Test{
    Name: "Custom Test",
    Run: func(ctx context.Context, tr *e2e.TestRunner) error {
        // Get HTTP client for making requests
        client := tr.GetHTTPClient()
        
        // Get base URL
        url := tr.GetBaseURL() + "/api/auth/login"
        
        // Direct database access if needed
        db := tr.GetDB()
        
        // Direct Redis access if needed
        redis := tr.GetRedis()
        
        return nil
    },
}
```

## Environment Validation

The TestRunner performs pre-flight checks to ensure the test environment is properly configured:

### PostgreSQL Trigger Check

Verifies that the `trigger_auto_verify_test_email` trigger exists and is enabled on the `users` table. This trigger automatically verifies test users with `@example.com` email addresses.

**Remediation**: If validation fails, run:
```bash
make test-e2e-prepare
```

### Redis Connection Check

Verifies that Redis is accessible and responding to PING commands.

**Remediation**: Ensure Redis is running and the connection string is correct in your environment configuration.

### Smart Test Skipping

**Validates: Requirements 1.3**

The TestRunner can gracefully skip tests when required environment dependencies are missing, preventing false test failures. This is particularly useful for:

- **Email tests** requiring SMTP credentials
- **OAuth tests** requiring provider configurations

#### SMTP Dependency Detection

Configure a test to require SMTP:

```go
config := &e2e.RunnerConfig{
    RequireSMTP:              true,  // Skip test if SMTP not configured
    ValidatePostgresTriggers: false,
    ValidateRedisConnection:  false,
}

runner := e2e.NewTestRunner(db, redis, baseURL, config)
```

**SMTP Configuration Requirements:**
- `SMTP_HOST`: SMTP server address (e.g., smtp.example.com)
- `SMTP_USER`: SMTP username/email
- `SMTP_PASSWORD`: SMTP password or app-specific password

If any of these environment variables are missing or empty, the test will be skipped with a clear message:

```
Test skipped: SMTP credentials not configured. To run email tests, set environment variables:
  - SMTP_HOST: SMTP server address (e.g., smtp.example.com)
  - SMTP_USER: SMTP username/email
  - SMTP_PASSWORD: SMTP password or app-specific password
Example: export SMTP_HOST=smtp.gmail.com SMTP_USER=test@example.com SMTP_PASSWORD=yourpass
```

#### OAuth Dependency Detection

Configure a test to require OAuth:

```go
config := &e2e.RunnerConfig{
    RequireOAuth:             true,  // Skip test if OAuth not configured
    ValidatePostgresTriggers: false,
    ValidateRedisConnection:  false,
}

runner := e2e.NewTestRunner(db, redis, baseURL, config)
```

**OAuth Configuration Requirements (at least one provider must be configured):**

Google OAuth:
- `OAUTH_GOOGLE_CLIENT_ID`: Your Google OAuth client ID
- `OAUTH_GOOGLE_CLIENT_SECRET`: Your Google OAuth client secret

GitHub OAuth:
- `OAUTH_GITHUB_CLIENT_ID`: Your GitHub OAuth client ID
- `OAUTH_GITHUB_CLIENT_SECRET`: Your GitHub OAuth client secret

If no OAuth providers are configured, the test will be skipped with a clear message:

```
Test skipped: OAuth provider credentials not configured. To run OAuth tests, configure at least one provider:
  Google OAuth:
    - OAUTH_GOOGLE_CLIENT_ID: Your Google OAuth client ID
    - OAUTH_GOOGLE_CLIENT_SECRET: Your Google OAuth client secret
  GitHub OAuth:
    - OAUTH_GITHUB_CLIENT_ID: Your GitHub OAuth client ID
    - OAUTH_GITHUB_CLIENT_SECRET: Your GitHub OAuth client secret
Example: export OAUTH_GOOGLE_CLIENT_ID=your-id OAUTH_GOOGLE_CLIENT_SECRET=your-secret
```

#### Benefits of Smart Skipping

1. **No False Failures**: Tests skip gracefully instead of failing when dependencies are unavailable
2. **Clear Remediation**: Skip messages provide exact instructions for configuring missing dependencies
3. **Flexible Testing**: Run subsets of tests based on available infrastructure
4. **CI/CD Friendly**: Different environments can run different test suites based on configuration

## Test Isolation

### Database Transaction Isolation

When `UseDBTransactions` is enabled, each test runs within a database transaction that is rolled back after test completion. This ensures:

- Tests don't leave persistent data in the database
- Tests are isolated from each other's database changes
- Fast cleanup without manual data deletion

### Redis Namespace Isolation

When `RedisNamespaceMode` is enabled, tests use namespaced Redis keys to prevent interference between parallel or sequential tests.

## Failure Logging

When a test fails, the TestRunner captures comprehensive debugging information:

- **Error Message**: The primary error returned by the test
- **Stack Trace**: Full goroutine stack traces (if enabled)
- **HTTP Request**: Complete request details if applicable
- **HTTP Response**: Complete response including body (up to 10KB)
- **Environment State**: Safe environment variables (non-sensitive only)
- **Timestamp**: When the failure occurred

Example failure log:

```go
if result.Status == e2e.TestStatusFail {
    log := result.FailureLog
    fmt.Printf("Test failed: %s\n", log.ErrorMessage)
    fmt.Printf("Timestamp: %s\n", log.Timestamp)
    fmt.Printf("Stack trace:\n%s\n", log.StackTrace)
    
    if log.Request != nil {
        fmt.Printf("Request: %s %s\n", log.Request.Method, log.Request.URL)
    }
    
    if log.Response != nil {
        fmt.Printf("Response: %d\n", log.Response.StatusCode)
        fmt.Printf("Body: %s\n", log.ResponseBody)
    }
}
```

## Integration with Existing E2E Tests

The TestRunner is designed to complement the existing E2E test suite in `test/e2e/`. It can be integrated gradually:

1. Keep existing `test/e2e/*_test.go` files as-is
2. Use TestRunner for new tests that need enhanced isolation
3. Migrate problematic tests to TestRunner as needed
4. Use TestRunner's environment validation in `TestMain`

Example integration in `test/e2e/helpers.go`:

```go
func TestMain(m *testing.M) {
    // Existing checks...
    
    // Add TestRunner environment validation
    runner := e2e.NewTestRunner(e2eDB, redisClient, baseURL, nil)
    ctx := context.Background()
    if err := runner.ValidateEnvironment(ctx); err != nil {
        fmt.Printf("FATAL: Environment validation failed: %v\n", err)
        os.Exit(1)
    }
    
    os.Exit(m.Run())
}
```

## Requirements Mapping

This implementation addresses the following requirements from `requirements.md`:

- **Requirement 1.2**: Detailed failure logs including request/response data, environment state, and error stack traces
- **Requirement 1.3**: Smart test skipping with clear messages for environment issues
- **Requirement 1.5**: PostgreSQL trigger verification before test execution

## Design Alignment

Implementation follows the design specified in `design.md` Section 1:

- ✅ `TestRunner` struct with db, redis, httpClient, baseURL, logger
- ✅ `TestResult` with Name, Status, Duration, FailureLog
- ✅ `FailureLog` with Request, Response, Environment, StackTrace
- ✅ `TestStatus` enumeration (Pass, Fail, Skip)
- ✅ `ValidateEnvironment()` with PostgreSQL trigger and Redis checks
- ✅ `IsolateTest()` for test isolation setup
- ✅ `CleanupTest()` for post-test cleanup
- ✅ `Run()` for executing test collections

## Performance Considerations

- Environment validation runs once before test suite execution
- Test isolation overhead is minimal (transaction begin/rollback)
- Stack trace capture is optional (disable for faster execution)
- Environment variable logging is disabled by default to avoid leaking secrets

## Security Considerations

- Environment variable logging is **disabled by default** to prevent secret leakage
- Only safe environment variables are captured (no passwords, tokens, or keys)
- HTTP request/response bodies are truncated to 10KB to prevent memory exhaustion
- Database transactions prevent accidental data corruption

## Testing

Run the TestRunner unit tests:

```bash
go test -v ./internal/testing/e2e/
```

Expected output:
```
=== RUN   TestNewTestRunner
--- PASS: TestNewTestRunner (0.00s)
=== RUN   TestNewTestRunnerWithCustomConfig
--- PASS: TestNewTestRunnerWithCustomConfig (0.00s)
...
PASS
ok      github.com/example/sso/internal/testing/e2e     0.005s
```

## Future Enhancements

Potential improvements for Phase 2:

1. **Parallel Test Execution**: Support for running independent tests in parallel
2. **Transaction Context Management**: Store DB transactions in context for test access
3. **Redis Key Namespacing**: Automatic prefix injection for all Redis operations
4. **Metrics Collection**: Track test execution metrics (pass rate, duration trends)
5. **Retry Logic**: Automatic retry for flaky tests with exponential backoff
6. **Test Dependencies**: Support for test ordering based on dependencies

## References

- **Requirements**: `.kiro/specs/code-quality-comprehensive-improvements/requirements.md`
- **Design**: `.kiro/specs/code-quality-comprehensive-improvements/design.md`
- **Tasks**: `.kiro/specs/code-quality-comprehensive-improvements/tasks.md`
- **Existing E2E Tests**: `test/e2e/`
- **E2E Testing Docs**: `docs/E2E_TESTING.md`
