# Smart Test Skipping Guide

## Overview

The TestRunner component includes smart test skipping logic that gracefully handles missing optional dependencies. Instead of failing tests when SMTP or OAuth credentials are not configured, tests are skipped with clear remediation instructions.

This feature validates **Requirement 1.3**: "WHEN an E2E test fails due to environment issues, THE SSO_Service SHALL skip the test with a clear message rather than reporting a failure"

## Features

### 1. SMTP Credential Detection

Tests requiring SMTP functionality will be skipped if credentials are missing.

**Required Environment Variables:**
- `SMTP_HOST`: SMTP server address
- `SMTP_USER`: SMTP username/email
- `SMTP_PASSWORD`: SMTP password or app-specific password

**Skip Message Example:**
```
SMTP credentials not configured. To run email tests, set environment variables:
  - SMTP_HOST: SMTP server address (e.g., smtp.example.com)
  - SMTP_USER: SMTP username/email
  - SMTP_PASSWORD: SMTP password or app-specific password
Example: export SMTP_HOST=smtp.gmail.com SMTP_USER=test@example.com SMTP_PASSWORD=yourpass
```

### 2. OAuth Provider Detection

Tests requiring OAuth functionality will be skipped if no OAuth providers are configured.

**Supported Providers:**
- **Google OAuth**: `OAUTH_GOOGLE_CLIENT_ID` + `OAUTH_GOOGLE_CLIENT_SECRET`
- **GitHub OAuth**: `OAUTH_GITHUB_CLIENT_ID` + `OAUTH_GITHUB_CLIENT_SECRET`

**Skip Message Example:**
```
OAuth provider credentials not configured. To run OAuth tests, configure at least one provider:
  Google OAuth:
    - OAUTH_GOOGLE_CLIENT_ID: Your Google OAuth client ID
    - OAUTH_GOOGLE_CLIENT_SECRET: Your Google OAuth client secret
  GitHub OAuth:
    - OAUTH_GITHUB_CLIENT_ID: Your GitHub OAuth client ID
    - OAUTH_GITHUB_CLIENT_SECRET: Your GitHub OAuth client secret
Example: export OAUTH_GOOGLE_CLIENT_ID=your-id OAUTH_GOOGLE_CLIENT_SECRET=your-secret
```

## Usage

### Configuring Test Requirements

When creating a TestRunner, specify which dependencies are required:

```go
config := &e2e.RunnerConfig{
    RequireSMTP:  true,  // Skip if SMTP credentials missing
    RequireOAuth: true,  // Skip if OAuth providers missing
    
    // Other configuration...
    ValidatePostgresTriggers: true,
    ValidateRedisConnection:  true,
    UseDBTransactions:        true,
    RedisNamespaceMode:       true,
}

runner := e2e.NewTestRunner(db, redis, baseURL, config)
```

### Test Execution Flow

The TestRunner automatically checks for missing dependencies before executing each test:

```go
// In runSingleTest()
if shouldSkip, skipReason := tr.ShouldSkipTest(ctx); shouldSkip {
    result.Status = TestStatusSkip
    result.SkipReason = skipReason
    tr.LogStructured("INFO", "Test skipped", map[string]interface{}{
        "test":   test.Name,
        "reason": skipReason,
    })
    return result
}
```

### Test Results

Skipped tests are reported with:
- **Status**: `TestStatusSkip`
- **SkipReason**: Detailed message with remediation instructions
- **Duration**: Time spent on skip detection (minimal)

## Implementation Details

### ShouldSkipTest Method

The core logic that determines if a test should be skipped:

```go
func (tr *TestRunner) ShouldSkipTest(ctx context.Context) (bool, string) {
    if tr.config.RequireSMTP {
        if skip, reason := tr.checkSMTPAvailable(); skip {
            return true, reason
        }
    }

    if tr.config.RequireOAuth {
        if skip, reason := tr.checkOAuthAvailable(); skip {
            return true, reason
        }
    }

    return false, ""
}
```

### SMTP Detection

```go
func (tr *TestRunner) checkSMTPAvailable() (bool, string) {
    smtpHost := getEnvVar("SMTP_HOST")
    smtpUser := getEnvVar("SMTP_USER")
    smtpPassword := getEnvVar("SMTP_PASSWORD")

    if smtpHost == "" || smtpUser == "" || smtpPassword == "" {
        return true, "SMTP credentials not configured. To run email tests, ..."
    }

    return false, ""
}
```

### OAuth Detection

```go
func (tr *TestRunner) checkOAuthAvailable() (bool, string) {
    googleClientID := getEnvVar("OAUTH_GOOGLE_CLIENT_ID")
    googleClientSecret := getEnvVar("OAUTH_GOOGLE_CLIENT_SECRET")
    githubClientID := getEnvVar("OAUTH_GITHUB_CLIENT_ID")
    githubClientSecret := getEnvVar("OAUTH_GITHUB_CLIENT_SECRET")

    // If at least one OAuth provider is configured, don't skip
    if (googleClientID != "" && googleClientSecret != "") ||
        (githubClientID != "" && githubClientSecret != "") {
        return false, ""
    }

    return true, "OAuth provider credentials not configured. To run OAuth tests, ..."
}
```

## Testing

Comprehensive test coverage ensures the smart skipping logic works correctly:

### Test Files
- `runner_skipping_test.go`: Unit tests for skip logic
- `runner_skip_demo_test.go`: Demonstration of skip messages

### Test Coverage
- ✅ SMTP credentials present → test runs
- ✅ SMTP credentials missing → test skipped with clear message
- ✅ Partial SMTP credentials → test skipped
- ✅ OAuth providers configured → test runs
- ✅ No OAuth providers → test skipped with clear message
- ✅ Partial OAuth credentials → test skipped
- ✅ Combined requirements → skips on first missing dependency
- ✅ No requirements → test always runs

### Running Tests

```bash
# Run all smart skip tests
go test -v ./internal/testing/e2e -run TestShouldSkipTest

# Demonstrate skip messages
go test -v ./internal/testing/e2e -run TestDemonstrateSkipMessages
```

## Benefits

1. **No False Failures**: Tests don't fail due to missing optional dependencies
2. **Clear Remediation**: Skip messages provide exact instructions to configure dependencies
3. **Flexible Testing**: Teams can run partial test suites based on available credentials
4. **CI/CD Friendly**: Different environments can have different credential availability
5. **Developer Experience**: Clear guidance reduces setup friction

## Example Scenarios

### Scenario 1: Local Development Without SMTP
Developer runs E2E tests locally without configuring SMTP:
- Email verification tests are **skipped** with clear message
- All other tests run normally
- Developer knows exactly how to enable email tests if needed

### Scenario 2: CI/CD With Limited Credentials
CI pipeline runs with database and Redis but no OAuth providers:
- OAuth flow tests are **skipped** with clear message
- Authentication, MFA, and other tests run normally
- Pipeline succeeds with partial test coverage

### Scenario 3: Full E2E Environment
QA environment has all credentials configured:
- All tests run normally
- No tests are skipped
- Full coverage validation

## Future Enhancements

Potential extensions to smart skipping logic:

1. **Additional Providers**: Support for more OAuth providers (Facebook, Twitter, etc.)
2. **Conditional Features**: Skip MFA tests if TOTP secrets are not configured
3. **External Service Detection**: Skip tests requiring external APIs when unavailable
4. **Configuration Files**: Support credential detection from config files
5. **Skip Reports**: Generate reports of skipped tests across test runs

## Related Documentation

- `internal/testing/e2e/runner.go`: TestRunner implementation
- `docs/E2E_TESTING.md`: E2E testing guide
- `.env.example`: Environment variable reference
- `AGENTS.md`: Testing environment setup

## Compliance

This implementation satisfies:
- **Requirement 1.3**: Graceful test skipping with clear messages
- **Design Section 1 (TestRunner)**: Smart skipping feature
- **Testing Standard**: Clear error messages and remediation guidance
