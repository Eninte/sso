# Handler Layer Coverage Gap Analysis

**Generated:** 2024-01-XX  
**Overall Handler Coverage:** 79.57% (740/930 statements)  
**Target Coverage:** 80.00%  
**Deficit:** 0.43% (190 uncovered statements)

## Executive Summary

The handler layer is close to the 80% coverage threshold, requiring approximately **28 additional test cases** to cover the remaining 190 uncovered statements. The analysis identified three primary categories of coverage gaps:

1. **Setup & Configuration Handlers** (14.3% - 56.1% coverage)
2. **Error Handling Paths** (35.7% - 75.0% coverage)
3. **MFA & Recovery Code Flows** (77.8% - 78.9% coverage)

---

## Critical Coverage Gaps (Priority: High)

### 1. Setup Connection Testing (`setup.go:295` - `testConnections`) - **14.3% Coverage**

**Function:** `testConnections()`

**Uncovered Paths:**
- Database connection failure handling with filtered credential logging
- Redis connection failure with timeout handling
- Redis connection skip when `redisAddr` is empty
- Context timeout scenarios for both DB and Redis

**Missing Test Scenarios:**
- Database connection timeout (>10s)
- Database connection failure with connection refused
- Redis connection timeout (>5s)
- Redis connection failure with authentication error
- Optional Redis configuration (empty `redisAddr`)

**Impact:** High - Setup process is critical for initial system configuration

**Remediation:**
```go
// Suggested test cases:
- TestSetupHandler_testConnections_DBTimeout
- TestSetupHandler_testConnections_DBConnectionRefused
- TestSetupHandler_testConnections_RedisTimeout
- TestSetupHandler_testConnections_RedisAuthFailure
- TestSetupHandler_testConnections_RedisOptional
```

---

### 2. OAuth Error Handling (`helpers.go:162` - `writeOAuthError`) - **35.7% Coverage**

**Function:** `writeOAuthError()`

**Uncovered Paths:**
- `service.ErrInvalidClient` error mapping
- `service.ErrInvalidRedirectURI` error mapping
- `service.ErrAccountLocked` error mapping
- `service.ErrAccountDisabled` error mapping
- `service.ErrInvalidToken` error mapping
- `service.ErrEmailNotVerified` error mapping
- Default internal error fallback

**Missing Test Scenarios:**
- OAuth flow with invalid client credentials
- OAuth flow with mismatched redirect URI
- OAuth flow with locked account
- OAuth flow with disabled account
- OAuth flow with expired/invalid token
- OAuth flow with unverified email
- OAuth flow with unexpected internal error

**Impact:** High - OAuth error handling is security-critical

**Remediation:**
```go
// Suggested test cases:
- TestWriteOAuthError_InvalidClient
- TestWriteOAuthError_InvalidRedirectURI
- TestWriteOAuthError_AccountLocked
- TestWriteOAuthError_AccountDisabled
- TestWriteOAuthError_InvalidToken
- TestWriteOAuthError_EmailNotVerified
- TestWriteOAuthError_InternalError
```

---

### 3. Setup Configuration Validation (`setup.go:215` - `validateSetupConfig`) - **52.9% Coverage**

**Function:** `validateSetupConfig()`

**Uncovered Paths:**
- Missing required configuration fields validation
- Invalid port number validation
- Invalid database name validation
- Invalid Redis DB number validation
- JWT key path validation
- SMTP configuration validation

**Missing Test Scenarios:**
- Missing `DB_HOST` field
- Invalid `DB_PORT` (non-numeric, out of range)
- Empty `DB_NAME` field
- Invalid `REDIS_DB` (negative, > 15)
- Missing JWT key paths
- Invalid SMTP port

**Impact:** Medium-High - Configuration errors can prevent system startup

**Remediation:**
```go
// Suggested test cases:
- TestSetupHandler_validateSetupConfig_MissingDBHost
- TestSetupHandler_validateSetupConfig_InvalidDBPort
- TestSetupHandler_validateSetupConfig_EmptyDBName
- TestSetupHandler_validateSetupConfig_InvalidRedisDB
- TestSetupHandler_validateSetupConfig_MissingJWTKeys
- TestSetupHandler_validateSetupConfig_InvalidSMTPConfig
```

---

### 4. Setup DSN Building (`setup.go:260` - `buildDSNs`) - **54.5% Coverage**

**Function:** `buildDSNs()`

**Uncovered Paths:**
- PostgreSQL DSN construction with SSL mode
- Redis DSN construction with password
- Redis DSN construction without password
- URL encoding of special characters in credentials

**Missing Test Scenarios:**
- DSN with SSL enabled (`DB_SSL_MODE=require`)
- DSN with special characters in password (e.g., `@`, `:`, `/`)
- Redis DSN with authentication
- Redis DSN without authentication
- Redis DSN with custom database number

**Impact:** Medium - DSN construction errors prevent database connection

**Remediation:**
```go
// Suggested test cases:
- TestSetupHandler_buildDSNs_WithSSL
- TestSetupHandler_buildDSNs_SpecialCharsInPassword
- TestSetupHandler_buildDSNs_RedisWithAuth
- TestSetupHandler_buildDSNs_RedisWithoutAuth
- TestSetupHandler_buildDSNs_RedisCustomDB
```

---

### 5. Setup Key Generation (`setup_keys.go:21` - `HandleSetupGenerateKeys`) - **54.8% Coverage**

**Function:** `HandleSetupGenerateKeys()`

**Uncovered Paths:**
- RSA key pair generation failure
- File write permission errors
- Directory creation failure
- Key file validation after creation

**Missing Test Scenarios:**
- RSA key generation with insufficient entropy
- Key file write with permission denied
- Key directory does not exist
- Invalid key path (e.g., `/root/keys`)

**Impact:** High - JWT keys are critical for authentication

**Remediation:**
```go
// Suggested test cases:
- TestSetupHandler_HandleSetupGenerateKeys_GenerationFailure
- TestSetupHandler_HandleSetupGenerateKeys_WritePermissionDenied
- TestSetupHandler_HandleSetupGenerateKeys_DirectoryNotExist
- TestSetupHandler_HandleSetupGenerateKeys_InvalidKeyPath
```

---

## Medium Priority Coverage Gaps

### 6. MFA Status Check (`mfa.go:134` - `HandleMFAStatus`) - **77.8% Coverage**

**Uncovered Paths:**
- MFA status check for user without MFA enabled
- Service layer error handling in status retrieval

**Missing Test Scenarios:**
- Get MFA status when MFA is disabled
- Service error during status retrieval

**Remediation:**
```go
// Suggested test cases:
- TestMFAHandler_HandleMFAStatus_MFADisabled
- TestMFAHandler_HandleMFAStatus_ServiceError
```

---

### 7. Recovery Code Generation (`mfa.go:156` - `HandleGenerateRecoveryCodes`) - **78.9% Coverage**

**Uncovered Paths:**
- Recovery code generation when MFA is not enabled
- Recovery code generation failure due to database error
- Recovery code regeneration (overwriting existing codes)

**Missing Test Scenarios:**
- Generate recovery codes without MFA enabled (should fail)
- Database error during code generation
- Regenerate recovery codes (replace existing)

**Remediation:**
```go
// Suggested test cases:
- TestMFAHandler_HandleGenerateRecoveryCodes_MFANotEnabled
- TestMFAHandler_HandleGenerateRecoveryCodes_DatabaseError
- TestMFAHandler_HandleGenerateRecoveryCodes_Regenerate
```

---

### 8. Recovery Code Verification (`mfa.go:193` - `HandleVerifyRecoveryCode`) - **78.9% Coverage**

**Uncovered Paths:**
- Invalid recovery code format
- Recovery code already used
- Recovery code not found (user has no codes)
- Service layer error during verification

**Missing Test Scenarios:**
- Verify with invalid code format
- Verify with already-used recovery code
- Verify when user has no recovery codes
- Database error during verification

**Remediation:**
```go
// Suggested test cases:
- TestMFAHandler_HandleVerifyRecoveryCode_InvalidFormat
- TestMFAHandler_HandleVerifyRecoveryCode_AlreadyUsed
- TestMFAHandler_HandleVerifyRecoveryCode_NoCodesExist
- TestMFAHandler_HandleVerifyRecoveryCode_ServiceError
```

---

### 9. Recovery Code Status (`mfa.go:231` - `HandleGetRecoveryCodeStatus`) - **77.8% Coverage**

**Uncovered Paths:**
- Get status when user has no recovery codes
- Get status when all codes are used
- Service error during status retrieval

**Missing Test Scenarios:**
- Get status with no recovery codes generated
- Get status when all 10 codes are used
- Service error during status check

**Remediation:**
```go
// Suggested test cases:
- TestMFAHandler_HandleGetRecoveryCodeStatus_NoCodesGenerated
- TestMFAHandler_HandleGetRecoveryCodeStatus_AllCodesUsed
- TestMFAHandler_HandleGetRecoveryCodeStatus_ServiceError
```

---

### 10. System Health Check (`admin.go:278` - `HandleSystemHealth`) - **60.0% Coverage**

**Uncovered Paths:**
- Database connection health check failure
- Redis connection health check failure
- Partial system degradation (DB healthy, Redis unhealthy)

**Missing Test Scenarios:**
- Health check with database connection failure
- Health check with Redis connection failure
- Health check with both services unhealthy

**Remediation:**
```go
// Suggested test cases:
- TestAdminHandler_HandleSystemHealth_DBUnhealthy
- TestAdminHandler_HandleSystemHealth_RedisUnhealthy
- TestAdminHandler_HandleSystemHealth_BothUnhealthy
```

---

### 11. System Cleanup (`admin.go:298` - `HandleCleanup`) - **60.0% Coverage**

**Uncovered Paths:**
- Cleanup of expired sessions
- Cleanup of expired tokens
- Cleanup of expired verification codes
- Service error during cleanup

**Missing Test Scenarios:**
- Cleanup with expired sessions present
- Cleanup with expired tokens present
- Cleanup with expired verification codes present
- Service error during cleanup operation

**Remediation:**
```go
// Suggested test cases:
- TestAdminHandler_HandleCleanup_ExpiredSessions
- TestAdminHandler_HandleCleanup_ExpiredTokens
- TestAdminHandler_HandleCleanup_ExpiredVerificationCodes
- TestAdminHandler_HandleCleanup_ServiceError
```

---

### 12. UserInfo Endpoint (`userinfo.go:39` - `Handle`) - **61.9% Coverage**

**Uncovered Paths:**
- Invalid access token in Authorization header
- Expired access token
- User not found (token valid but user deleted)
- Service error during user info retrieval

**Missing Test Scenarios:**
- Request with malformed access token
- Request with expired access token
- Request with valid token but user deleted
- Service error during user lookup

**Remediation:**
```go
// Suggested test cases:
- TestUserInfoHandler_Handle_MalformedToken
- TestUserInfoHandler_Handle_ExpiredToken
- TestUserInfoHandler_Handle_UserDeleted
- TestUserInfoHandler_Handle_ServiceError
```

---

## Low Priority Coverage Gaps

### 13. Authorization Endpoint (`authorize.go:31` - `HandleAuthorize`) - **78.3% Coverage**

**Uncovered Paths:**
- Missing required OAuth parameters
- Invalid response_type
- Invalid scope

### 14. Captcha Handler (`captcha.go:28` - `Handle`) - **75.0% Coverage**

**Uncovered Paths:**
- Captcha generation failure
- Captcha storage failure in Redis

### 15. Forgot Password (`user.go:76` - `HandleForgotPassword`) - **76.5% Coverage**

**Uncovered Paths:**
- Email not found in system
- Email service unavailable
- Rate limiting exceeded

### 16. Logout All Sessions (`token.go:144` - `HandleLogoutAll`) - **75.0% Coverage**

**Uncovered Paths:**
- Token revocation failure
- Session cleanup failure

---

## Remediation Plan Summary

### Immediate Actions (To reach 80% coverage)

1. **Add Setup Handler Tests** (Priority 1)
   - Focus on `testConnections` (14.3% coverage)
   - Focus on `validateSetupConfig` (52.9% coverage)
   - Focus on `buildDSNs` (54.5% coverage)
   - Focus on `HandleSetupGenerateKeys` (54.8% coverage)
   - **Estimated tests needed:** 12-15 test cases

2. **Add OAuth Error Handling Tests** (Priority 2)
   - Focus on `writeOAuthError` (35.7% coverage)
   - Test all error mapping branches
   - **Estimated tests needed:** 7 test cases

3. **Add MFA Tests** (Priority 3)
   - Focus on recovery code flows
   - Focus on MFA status checks
   - **Estimated tests needed:** 8-10 test cases

4. **Add Admin Handler Tests** (Priority 4)
   - Focus on `HandleSystemHealth` (60.0% coverage)
   - Focus on `HandleCleanup` (60.0% coverage)
   - **Estimated tests needed:** 4-6 test cases

### Testing Guidelines

**Table-Driven Test Pattern:**
```go
func TestSetupHandler_testConnections(t *testing.T) {
	tests := []struct {
		name          string
		dbDSN         string
		redisAddr     string
		redisPassword string
		redisDB       int
		mockSetup     func(*mock.MockStore)
		wantErr       bool
		errContains   string
	}{
		{
			name:      "database connection timeout",
			dbDSN:     "postgres://...",
			wantErr:   true,
			errContains: "connection timeout",
		},
		{
			name:      "redis connection failure",
			dbDSN:     "postgres://...",
			redisAddr: "localhost:6379",
			wantErr:   true,
			errContains: "redis",
		},
		// ... more test cases
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test implementation
		})
	}
}
```

**Error Path Testing:**
```go
// Test each error branch in writeOAuthError
func TestWriteOAuthError_AllBranches(t *testing.T) {
	errorCases := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "invalid client",
			err:            service.ErrInvalidClient,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_CLIENT",
		},
		// ... test all error types
	}
	// Test implementation
}
```

---

## Detailed File-Level Coverage

| File | Function | Coverage | Priority | Estimated Tests |
|------|----------|----------|----------|-----------------|
| setup.go | testConnections | 14.3% | High | 5 |
| helpers.go | writeOAuthError | 35.7% | High | 7 |
| setup.go | validateSetupConfig | 52.9% | High | 6 |
| setup.go | buildDSNs | 54.5% | High | 5 |
| setup_keys.go | HandleSetupGenerateKeys | 54.8% | High | 4 |
| setup.go | HandleSetupSave | 56.1% | Medium | 3 |
| admin.go | HandleSystemHealth | 60.0% | Medium | 3 |
| admin.go | HandleCleanup | 60.0% | Medium | 3 |
| userinfo.go | Handle | 61.9% | Medium | 4 |
| init.go | HandleSystemStatus | 63.6% | Medium | 2 |
| init.go | HandleInitPage | 69.2% | Low | 2 |
| setup.go | validateSetupToken | 71.4% | Low | 2 |
| setup_deps.go | testDBConnection | 71.4% | Low | 2 |
| init.go | HandleCreateAdmin | 72.2% | Low | 2 |
| captcha.go | Handle | 75.0% | Low | 1 |
| helpers.go | handleDecodeJSONError | 75.0% | Low | 1 |
| setup.go | GetSetupToken | 75.0% | Low | 1 |
| token.go | HandleLogoutAll | 75.0% | Low | 1 |
| userinfo.go | NewUserInfoHandler | 75.0% | Low | 1 |
| social.go | HandleCallback | 76.2% | Low | 2 |
| user.go | HandleForgotPassword | 76.5% | Low | 2 |
| mfa.go | HandleMFAStatus | 77.8% | Medium | 2 |
| mfa.go | HandleGetRecoveryCodeStatus | 77.8% | Medium | 2 |
| authorize.go | HandleAuthorize | 78.3% | Low | 2 |
| mfa.go | HandleGenerateRecoveryCodes | 78.9% | Medium | 3 |
| mfa.go | HandleVerifyRecoveryCode | 78.9% | Medium | 3 |

**Total Estimated Tests Needed:** 28 test cases

---

## Next Steps

1. **Execute Task 6.2:** Write table-driven tests for handler layer gaps
   - Start with high-priority setup handlers
   - Focus on error handling paths
   - Use existing test patterns from `handler_test.go`

2. **Verify Coverage Improvement:**
   - Run `go test -coverprofile=coverage.out ./internal/handler/...`
   - Run `go run cmd/coverage-check/main.go -profile coverage.out -threshold 80.0`
   - Ensure overall coverage reaches 80%+

3. **Document Test Patterns:**
   - Update `docs/TESTING.md` with new test examples
   - Document table-driven test structure for handlers
   - Document error path testing patterns

---

## References

- **Requirements:** Section 2.3 (Coverage gap identification)
- **Design:** Section 2 (CoverageAnalyzer Component)
- **Coverage Profile:** `coverage.out`
- **Test Framework:** Go testing package with table-driven tests
- **Mocking:** `internal/store/mock` for Store layer dependencies
