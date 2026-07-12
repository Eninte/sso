# Handler Layer Coverage Gap Checklist

**Current Coverage:** 79.57% (740/930 statements)  
**Target:** 80.00%  
**Remaining:** 190 uncovered statements (~28 test cases needed)

---

## Priority 1: Critical Gaps (High Priority)

### Setup & Configuration Handlers

- [ ] **setup.go:295 - testConnections** (14.3% coverage) - 5 tests
  - [ ] Test database connection timeout
  - [ ] Test database connection refused
  - [ ] Test Redis connection timeout
  - [ ] Test Redis authentication failure
  - [ ] Test optional Redis configuration (empty address)

- [ ] **helpers.go:162 - writeOAuthError** (35.7% coverage) - 7 tests
  - [ ] Test ErrInvalidClient error mapping
  - [ ] Test ErrInvalidRedirectURI error mapping
  - [ ] Test ErrAccountLocked error mapping
  - [ ] Test ErrAccountDisabled error mapping
  - [ ] Test ErrInvalidToken error mapping
  - [ ] Test ErrEmailNotVerified error mapping
  - [ ] Test default internal error fallback

- [ ] **setup.go:215 - validateSetupConfig** (52.9% coverage) - 6 tests
  - [ ] Test missing DB_HOST field
  - [ ] Test invalid DB_PORT (non-numeric, out of range)
  - [ ] Test empty DB_NAME field
  - [ ] Test invalid REDIS_DB (negative, > 15)
  - [ ] Test missing JWT key paths
  - [ ] Test invalid SMTP configuration

- [ ] **setup.go:260 - buildDSNs** (54.5% coverage) - 5 tests
  - [ ] Test DSN with SSL enabled
  - [ ] Test DSN with special characters in password
  - [ ] Test Redis DSN with authentication
  - [ ] Test Redis DSN without authentication
  - [ ] Test Redis DSN with custom DB number

- [ ] **setup_keys.go:21 - HandleSetupGenerateKeys** (54.8% coverage) - 4 tests
  - [ ] Test RSA key generation failure
  - [ ] Test file write permission denied
  - [ ] Test key directory does not exist
  - [ ] Test invalid key path

**Subtotal: 27 critical tests**

---

## Priority 2: MFA & Recovery Flows (Medium Priority)

### MFA Handlers

- [ ] **mfa.go:134 - HandleMFAStatus** (77.8% coverage) - 2 tests
  - [ ] Test MFA status when disabled
  - [ ] Test service error during status retrieval

- [ ] **mfa.go:156 - HandleGenerateRecoveryCodes** (78.9% coverage) - 3 tests
  - [ ] Test generation without MFA enabled
  - [ ] Test database error during generation
  - [ ] Test regeneration (replace existing codes)

- [ ] **mfa.go:193 - HandleVerifyRecoveryCode** (78.9% coverage) - 3 tests
  - [ ] Test invalid recovery code format
  - [ ] Test already-used recovery code
  - [ ] Test verification with no codes exist
  - [ ] Test service error during verification

- [ ] **mfa.go:231 - HandleGetRecoveryCodeStatus** (77.8% coverage) - 2 tests
  - [ ] Test status with no codes generated
  - [ ] Test status when all codes are used
  - [ ] Test service error during status check

**Subtotal: 10 MFA tests**

---

## Priority 3: Admin & Health Monitoring (Medium Priority)

### Admin Handlers

- [ ] **admin.go:278 - HandleSystemHealth** (60.0% coverage) - 3 tests
  - [ ] Test health check with database failure
  - [ ] Test health check with Redis failure
  - [ ] Test health check with both services unhealthy

- [ ] **admin.go:298 - HandleCleanup** (60.0% coverage) - 3 tests
  - [ ] Test cleanup of expired sessions
  - [ ] Test cleanup of expired tokens
  - [ ] Test cleanup of expired verification codes
  - [ ] Test service error during cleanup

- [ ] **userinfo.go:39 - Handle** (61.9% coverage) - 4 tests
  - [ ] Test with malformed access token
  - [ ] Test with expired access token
  - [ ] Test with valid token but user deleted
  - [ ] Test service error during user lookup

**Subtotal: 10 admin/monitoring tests**

---

## Priority 4: Optional Improvements (Low Priority)

### Additional Coverage (Optional)

- [ ] **authorize.go:31 - HandleAuthorize** (78.3% coverage) - 2 tests
- [ ] **captcha.go:28 - Handle** (75.0% coverage) - 1 test
- [ ] **user.go:76 - HandleForgotPassword** (76.5% coverage) - 2 tests
- [ ] **token.go:144 - HandleLogoutAll** (75.0% coverage) - 1 test
- [ ] **init.go:50 - HandleInitPage** (69.2% coverage) - 2 tests
- [ ] **init.go:78 - HandleSystemStatus** (63.6% coverage) - 2 tests

**Subtotal: 10 optional tests**

---

## Testing Approach

### 1. Table-Driven Tests

Use table-driven test pattern for multiple scenarios:

```go
func TestHandlerFunction(t *testing.T) {
    tests := []struct {
        name        string
        input       interface{}
        mockSetup   func(*mock.MockStore)
        wantErr     bool
        wantStatus  int
    }{
        // Test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### 2. Error Path Focus

Prioritize testing error handling branches:
- Service layer errors
- Database connection failures
- Validation failures
- Timeout scenarios

### 3. Mock Dependencies

Use `internal/store/mock` for Store layer:
```go
mockStore := mock.NewMockStore()
mockStore.On("MethodName", args).Return(result, error)
```

### 4. HTTP Test Utilities

Use `httptest` for HTTP handlers:
```go
req := httptest.NewRequest("POST", "/endpoint", body)
rr := httptest.NewRecorder()
handler.ServeHTTP(rr, req)
```

---

## Verification Steps

1. **Run Coverage Analysis:**
   ```bash
   go test -coverprofile=coverage.out ./internal/handler/...
   ```

2. **Check Threshold:**
   ```bash
   go run cmd/coverage-check/main.go -profile coverage.out -threshold 80.0 -verbose
   ```

3. **Identify Remaining Gaps:**
   ```bash
   go tool cover -func=coverage.out | grep "internal/handler" | grep -v "100.0%"
   ```

4. **Generate HTML Report:**
   ```bash
   go tool cover -html=coverage.out -o coverage.html
   ```

---

## Progress Tracking

### Coverage Milestones

- [ ] **Phase 1:** Setup handlers → Target: 75% overall (+5%)
- [ ] **Phase 2:** OAuth error handling → Target: 78% overall (+3%)
- [ ] **Phase 3:** MFA flows → Target: 79.5% overall (+1.5%)
- [ ] **Phase 4:** Admin handlers → Target: 80%+ overall (+0.5%)

### Completion Criteria

✅ **Task Complete When:**
- Overall handler coverage ≥ 80%
- All high-priority gaps addressed
- Tests follow table-driven pattern
- Error paths are covered
- Tests are documented in TESTING.md

---

## Resources

- **Analysis Report:** `docs/reports/handler_coverage_gaps_analysis.md`
- **Coverage Tool:** `cmd/coverage-check/main.go`
- **Test Examples:** `internal/handler/handler_test.go`
- **Mock Store:** `internal/store/mock/`
- **Testing Guide:** `docs/TESTING.md`

---

## Notes

- Focus on **error handling paths** - these are often uncovered
- Use **context timeouts** for connection tests
- Test **boundary values** for validation logic
- Ensure **cleanup** in test teardown
- Document **test patterns** for future reference
