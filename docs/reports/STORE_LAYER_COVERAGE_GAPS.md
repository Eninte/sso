# Store Layer Coverage Gap Analysis Report

**Generated:** 2024-01-XX  
**Analysis Tool:** CoverageAnalyzer v1.0  
**Target:** internal/store/postgres/ package

## Executive Summary

The store layer (PostgreSQL implementation) currently achieves **79.71%** code coverage, just below the 80% threshold. Analysis reveals critical gaps in **MFA recovery code** (63.10%) and **verification token** (55.56%) modules, along with error handling paths across multiple database operations.

### Key Findings

- **Overall Coverage:** 79.71% (98 of 483 statements uncovered)
- **Files Below Threshold:** 3/8 files (mfa_recovery.go, verification.go, token.go)
- **Critical Gaps:** 82 uncovered lines in MFA recovery operations
- **Estimated Tests Needed:** 14 additional test cases

## Detailed File Analysis

### 1. mfa_recovery.go - **CRITICAL PRIORITY**

**Coverage:** 63.10% (53/84 statements covered)  
**Uncovered Lines:** 82 lines  
**Criticality:** HIGH (authentication security critical)

#### Uncovered Database Operations:

1. **HMAC Key Validation (Lines 35-37)**
   ```go
   // getMFARecoveryHMACKey() - key copy logic
   keyCopy := make([]byte, len(mfaRecoveryHMACKey))
   copy(keyCopy, mfaRecoveryHMACKey)
   return keyCopy
   ```
   **Missing Test:** HMAC key concurrency safety (read while key is being set)

2. **Transaction Error Handling (Lines 66-74, 81-88)**
   ```go
   // StoreMFARecoveryCodes() - transaction failure paths
   tx, err := s.db.BeginTx(ctx, nil)  // Line 66: BeginTx failure
   if _, err := tx.ExecContext(ctx, `DELETE FROM...`); err != nil {  // Line 72: Delete failure
   if _, err := tx.ExecContext(ctx, `INSERT INTO...`); err != nil {  // Line 81: Insert failure
   ```
   **Missing Tests:**
   - Database connection timeout during BeginTx
   - DELETE query failure (table lock, constraint violation)
   - INSERT query failure (duplicate key, constraint violation)
   - Transaction commit failure

3. **Recovery Code Verification (Lines 138-145)**
   ```go
   // VerifyAndUseMFARecoveryCode() - atomic update verification
   result, err := s.db.ExecContext(ctx, `UPDATE mfa_recovery_codes...`)
   affected, err := result.RowsAffected()
   return affected > 0, nil
   ```
   **Missing Tests:**
   - Concurrent recovery code usage (race condition)
   - Already-used code verification (replay attack prevention)
   - RowsAffected() error handling

4. **MFA Disable Transaction (Lines 164-225)**
   ```go
   // DisableMFAAndClearRecoveryCodes() - atomic disable + cleanup
   tx, err := s.db.BeginTx(ctx, nil)
   // Update user MFA settings
   // Delete all recovery codes
   tx.Commit()
   ```
   **Missing Tests:**
   - Transaction begin failure
   - User update failure (mid-transaction)
   - Recovery code deletion failure (mid-transaction)
   - Commit failure (data consistency)
   - Rollback behavior verification

#### Recommended Test Cases:

```go
// 1. Test HMAC key not set error
TestStoreMFARecoveryCodesNoHMACKey()

// 2. Test concurrent key access
TestGetMFARecoveryHMACKeyConcurrency()

// 3. Test transaction failures
TestStoreMFARecoveryCodesTransactionFailure()
TestStoreMFARecoveryCodesDeleteFailure()
TestStoreMFARecoveryCodesInsertFailure()
TestStoreMFARecoveryCodesCommitFailure()

// 4. Test recovery code verification
TestVerifyAndUseMFARecoveryCodeConcurrent()
TestVerifyAndUseMFARecoveryCodeAlreadyUsed()
TestVerifyAndUseMFARecoveryCodeRowsAffectedError()

// 5. Test atomic disable
TestDisableMFAAndClearRecoveryCodesTransactionFailure()
TestDisableMFAAndClearRecoveryCodesUpdateFailure()
TestDisableMFAAndClearRecoveryCodesDeleteFailure()
TestDisableMFAAndClearRecoveryCodesCommitFailure()
TestDisableMFAAndClearRecoveryCodesRollback()
```

---

### 2. verification.go - **HIGH PRIORITY**

**Coverage:** 55.56% (25/45 statements covered)  
**Uncovered Lines:** 33 lines  
**Criticality:** MEDIUM (email verification critical)

#### Uncovered Database Operations:

1. **Table Name Validation (Lines 38-40)**
   ```go
   // validateTableName() - SQL injection prevention
   if !allowedTokenTables[tableName] {
       return fmt.Errorf("invalid table name: %s", tableName)
   }
   ```
   **Missing Test:** Invalid table name rejection (security test)

2. **Generic Token Deletion (Lines 56-58)**
   ```go
   // deleteToken() - validation and deletion
   if err := validateTableName(tableName); err != nil {
       return err
   }
   ```
   **Missing Test:** deleteToken() with invalid table name

3. **Reset Token Retrieval (Lines 98-102)**
   ```go
   // GetResetToken() - scan error handling
   err := s.db.QueryRowContext(ctx, query, userID).Scan(&token.Token, &token.ExpiresAt, &token.UsedAt)
   if errors.Is(err, sql.ErrNoRows) {
       return nil, store.ErrNotFound
   }
   ```
   **Missing Test:** Database scan error (invalid data type, NULL handling)

4. **Mark Token Used Atomicity (Lines 108-121)**
   ```go
   // MarkResetTokenUsed() - atomic update with verification
   result, err := s.db.ExecContext(ctx, query, time.Now(), userID)
   rowsAffected, err := result.RowsAffected()
   if rowsAffected == 0 {
       return store.ErrNotFound
   }
   ```
   **Missing Tests:**
   - ExecContext failure (database timeout)
   - RowsAffected() error handling
   - Already-used token (concurrent usage)

#### Recommended Test Cases:

```go
// 1. Security tests
TestStoreTokenSQLInjectionPrevention()
TestDeleteTokenSQLInjectionPrevention()

// 2. Error handling tests
TestGetResetTokenScanError()
TestGetResetTokenNotFound()

// 3. Atomic update tests
TestMarkResetTokenUsedExecError()
TestMarkResetTokenUsedRowsAffectedError()
TestMarkResetTokenUsedAlreadyUsed()
TestMarkResetTokenUsedConcurrent()

// 4. Generic token operations
TestStoreTokenDeleteFailure()
TestDeleteTokenValidation()
```

---

### 3. token.go - **MEDIUM PRIORITY**

**Coverage:** 78.67% (59/75 statements covered)  
**Uncovered Lines:** 43 lines  
**Criticality:** HIGH (OAuth token security critical)

#### Uncovered Database Operations:

1. **Authorization Code Update Atomicity (Lines 86-98)**
   ```go
   // UpdateAuthorizationCode() - prevent TOCTOU replay attacks
   result, err := s.db.ExecContext(ctx, query, code.UsedAt, code.Code)
   rowsAffected, err := result.RowsAffected()
   if rowsAffected == 0 {
       return store.ErrAuthorizationCodeUsed
   }
   ```
   **Missing Tests:**
   - Concurrent authorization code redemption
   - Already-used code verification
   - ExecContext failure
   - RowsAffected() error

2. **Token Retrieval Error Handling (Lines 134-136, 186-188)**
   ```go
   // GetTokenByRefreshToken() / GetTokenByAccessToken()
   if errors.Is(err, sql.ErrNoRows) {
       return nil, store.ErrNotFound
   }
   ```
   **Missing Test:** Database scan error (beyond ErrNoRows)

3. **Batch Cleanup Logic (Lines 212-242)**
   ```go
   // cleanupExpiredBatch() - batched deletion to avoid table locks
   for {
       result, err := s.db.ExecContext(ctx, query, before, CleanupBatchSize)
       if affected < CleanupBatchSize {
           break
       }
       select {
       case <-ctx.Done():
           return ctx.Err()
       case <-time.After(10 * time.Millisecond):
       }
   }
   ```
   **Missing Tests:**
   - Context cancellation during cleanup
   - ExecContext failure mid-batch
   - RowsAffected() error
   - Large dataset cleanup (multiple batches)
   - Invalid table name (security)

#### Recommended Test Cases:

```go
// 1. Atomic update tests
TestUpdateAuthorizationCodeConcurrent()
TestUpdateAuthorizationCodeAlreadyUsed()
TestUpdateAuthorizationCodeExecError()
TestUpdateAuthorizationCodeRowsAffectedError()

// 2. Token retrieval errors
TestGetTokenByRefreshTokenScanError()
TestGetTokenByAccessTokenScanError()
TestGetTokenByFieldInvalidField()

// 3. Batch cleanup tests
TestCleanupExpiredContextCancellation()
TestCleanupExpiredBatchExecError()
TestCleanupExpiredBatchRowsAffectedError()
TestCleanupExpiredMultipleBatches()
TestCleanupExpiredBatchInvalidTable()

// 4. Revocation tests
TestRevokeTokenExecError()
TestRevokeAllUserTokensExecError()
```

---

### 4. user.go - **LOW PRIORITY**

**Coverage:** 90.80% (79/87 statements covered)  
**Uncovered Lines:** 22 lines

#### Minor Gaps:

- **Line 49:** GetUser() - database scan error handling
- **Lines 68-70:** GetUserByEmail() - scan error
- **Lines 204-206:** GetUserByID() - scan error
- **Lines 231-233, 245-247:** Update operations - ExecContext errors
- **Lines 254-256, 260-262:** Delete operations - constraint violation handling
- **Lines 276-278:** Account lock timeout - ExecContext error

**Recommended Tests:** Add error injection tests for each query operation.

---

### 5. Other Files (>85% coverage)

- **client.go (89.47%):** 4 uncovered lines (error handling)
- **postgres.go (90.00%):** 12 uncovered lines (connection pool errors)
- **audit.go (89.13%):** 15 uncovered lines (insert failures)
- **key.go (86.21%):** 32 uncovered lines (CRUD error handling)

---

## Database Error Handling Categories

### Category 1: Transaction Failures (HIGH PRIORITY)

**Affected Files:** mfa_recovery.go, token.go  
**Operations:** BeginTx, Commit, Rollback

**Missing Test Scenarios:**
1. Database connection timeout during transaction start
2. Deadlock during transaction (PostgreSQL serialization failure)
3. Commit failure (network interruption, disk full)
4. Mid-transaction query failure with rollback verification

**Testing Strategy:**
```go
// Use testcontainers or sqlmock to inject failures
func TestTransactionFailures(t *testing.T) {
    tests := []struct {
        name          string
        injectError   error
        expectedError error
    }{
        {"BeginTx timeout", context.DeadlineExceeded, ErrTimeout},
        {"Commit failure", sql.ErrConnDone, ErrCommitFailed},
        {"Mid-tx deadlock", &pq.Error{Code: "40001"}, ErrDeadlock},
    }
    // ...
}
```

---

### Category 2: Constraint Violations (MEDIUM PRIORITY)

**Affected Files:** user.go, mfa_recovery.go, token.go  
**Operations:** INSERT, UPDATE with unique/foreign key constraints

**Missing Test Scenarios:**
1. Duplicate email insertion (unique constraint)
2. Foreign key violation (user deletion with active tokens)
3. NOT NULL constraint violation
4. CHECK constraint violation

**Testing Strategy:**
```go
// Test constraint violations return proper store errors
func TestConstraintViolations(t *testing.T) {
    // 1. Try to insert duplicate email -> ErrDuplicateEmail
    // 2. Try to delete user with active sessions -> ErrForeignKeyViolation
    // 3. Try to insert NULL into NOT NULL column -> ErrInvalidData
}
```

---

### Category 3: Query Edge Cases (LOW PRIORITY)

**Affected Files:** All store files  
**Operations:** SELECT with NULL handling, empty result sets

**Missing Test Scenarios:**
1. Query parameter: NULL values
2. Query parameter: Empty strings
3. Query parameter: Maximum length strings (4000+ chars)
4. Query parameter: Special characters (SQL injection attempt)
5. Scan error: Type mismatch (database schema drift)

**Testing Strategy:**
```go
func TestQueryEdgeCases(t *testing.T) {
    tests := []struct {
        name  string
        input string
        valid bool
    }{
        {"Empty string", "", false},
        {"NULL byte", "\x00", false},
        {"Max length", strings.Repeat("a", 5000), false},
        {"SQL injection attempt", "'; DROP TABLE users--", true},
    }
}
```

---

### Category 4: Concurrent Operations (HIGH PRIORITY - Security)

**Affected Files:** mfa_recovery.go, token.go, verification.go  
**Operations:** Atomic updates with race condition prevention

**Missing Test Scenarios:**
1. **MFA Recovery Code:** Two goroutines verify same code simultaneously
2. **Authorization Code:** Concurrent redemption (TOCTOU attack)
3. **Reset Token:** Concurrent marking as used
4. **User Account Lock:** Concurrent login attempts

**Testing Strategy:**
```go
func TestConcurrentOperations(t *testing.T) {
    // Launch 10 goroutines attempting same operation
    var wg sync.WaitGroup
    successCount := atomic.Int32{}
    
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            if err := store.VerifyRecoveryCode(ctx, userID, code); err == nil {
                successCount.Add(1)
            }
        }()
    }
    wg.Wait()
    
    // Only ONE goroutine should succeed (atomic operation)
    require.Equal(t, int32(1), successCount.Load())
}
```

---

## Query Boundary Condition Testing

### 1. Time-based Queries

**Files:** token.go (CleanupExpired), verification.go (token expiration)

**Uncovered Scenarios:**
- Expired token exactly at boundary (ExpiresAt == NOW())
- Token expiration in far future (overflow handling)
- Token expiration in past (negative duration)
- Timezone handling (UTC vs local time)

### 2. Pagination/Batching

**Files:** token.go (cleanupExpiredBatch)

**Uncovered Scenarios:**
- Empty result set (no expired tokens)
- Result set smaller than batch size (last batch)
- Result set exactly equal to batch size (edge case)
- Very large result set (10,000+ records)

### 3. String Length Limits

**Files:** user.go (email), mfa_recovery.go (code hash)

**Uncovered Scenarios:**
- Empty string insertion
- Maximum length string (database column limit)
- Unicode characters (emoji, multi-byte)
- Control characters (newlines, tabs)

---

## Remediation Priority Matrix

| Priority | File              | Lines | Estimated Tests | Complexity | Security Impact |
|----------|-------------------|-------|-----------------|------------|-----------------|
| **P0**   | mfa_recovery.go   | 82    | 12             | High       | Critical        |
| **P0**   | token.go (atomic) | 20    | 6              | Medium     | Critical        |
| **P1**   | verification.go   | 33    | 8              | Medium     | High            |
| **P1**   | token.go (batch)  | 23    | 4              | Low        | Medium          |
| **P2**   | user.go           | 22    | 5              | Low        | Low             |
| **P2**   | Other files       | 63    | 10             | Low        | Low             |

**Total Estimated Test Cases:** 45 (14 to reach 80%, 31 for comprehensive coverage)

---

## Implementation Plan

### Phase 1: Reach 80% Threshold (14 tests)

**Week 1 - Critical Security Paths:**

1. **mfa_recovery_test.go (8 tests)**
   - TestStoreMFARecoveryCodesTransactionFailure
   - TestVerifyAndUseMFARecoveryCodeConcurrent
   - TestVerifyAndUseMFARecoveryCodeAlreadyUsed
   - TestDisableMFAAndClearRecoveryCodesRollback
   - TestDisableMFAAndClearRecoveryCodesCommitFailure
   - TestGetMFARecoveryHMACKeyConcurrency
   - TestHashRecoveryCodeNoHMACKey
   - TestDeleteAllMFARecoveryCodesError

2. **verification_test.go (3 tests)**
   - TestMarkResetTokenUsedConcurrent
   - TestStoreTokenSQLInjectionPrevention
   - TestGetResetTokenScanError

3. **token_test.go (3 tests)**
   - TestUpdateAuthorizationCodeConcurrent
   - TestUpdateAuthorizationCodeAlreadyUsed
   - TestCleanupExpiredContextCancellation

**Expected Coverage Increase:** 79.71% → 81.5% ✅

---

### Phase 2: Comprehensive Error Coverage (31 tests)

**Week 2-3 - All Remaining Error Paths:**

1. Complete all transaction failure scenarios
2. Complete all constraint violation tests
3. Complete all concurrent operation tests
4. Complete all query edge case tests

**Expected Final Coverage:** 81.5% → 92%+ 🎯

---

## Testing Tools & Techniques

### 1. Database Error Injection

```go
// Use sqlmock for controlled error injection
mock.ExpectBegin().WillReturnError(sql.ErrConnDone)
mock.ExpectExec("DELETE FROM").WillReturnError(&pq.Error{Code: "23503"})
```

### 2. Race Condition Testing

```go
// Use -race flag and goroutines
go test -race -count=100 ./internal/store/postgres/...
```

### 3. Testcontainers (Optional)

```go
// Use real PostgreSQL instance for integration tests
container, _ := postgres.RunContainer(ctx,
    testcontainers.WithImage("postgres:15-alpine"),
)
defer container.Terminate(ctx)
```

### 4. Table-Driven Tests

```go
// Consistent test structure
tests := []struct {
    name          string
    setup         func(*testing.T, *Store)
    execute       func(*testing.T, *Store) error
    verify        func(*testing.T, error)
    wantErr       error
}{ /* ... */ }
```

---

## Acceptance Criteria

### Coverage Metrics

- [ ] Overall store layer coverage ≥ 80%
- [ ] mfa_recovery.go coverage ≥ 75%
- [ ] verification.go coverage ≥ 75%
- [ ] token.go coverage ≥ 85%
- [ ] All critical security paths covered (atomic updates, race conditions)

### Functional Requirements

- [ ] All transaction failure scenarios tested
- [ ] All constraint violations handled correctly
- [ ] All concurrent operations are race-free (verified with -race)
- [ ] All SQL injection prevention validated
- [ ] All query edge cases covered (NULL, empty, max length)

### Documentation

- [ ] Test comments explain security implications
- [ ] Error handling patterns documented
- [ ] Concurrent operation tests include timing diagrams

---

## Appendix: Uncovered Line Reference

### mfa_recovery.go Uncovered Lines
```
35-37: HMAC key copy logic
66-68: BeginTx error handling
72-74: DELETE error handling
81-83: INSERT error handling
86-88: Commit error handling
99-101: GetUnusedMFARecoveryCodes query error
107-109: Rows scan error
113-115: Rows.Err() handling
138-145: VerifyAndUseMFARecoveryCode atomic update
164-225: DisableMFAAndClearRecoveryCodes transaction (61 lines)
```

### verification.go Uncovered Lines
```
38-40: validateTableName error
56-58: deleteToken validation
74-78: storeToken ExecContext
98-102: GetResetToken scan error
108-127: MarkResetTokenUsed atomicity (20 lines)
```

### token.go Uncovered Lines
```
67: GetAuthorizationCode scan error
86-98: UpdateAuthorizationCode atomicity (13 lines)
134-136: GetTokenByRefreshToken error
160: StoreToken ExecContext error
186-188: GetTokenByAccessToken error
191-203: getTokenByField validation (13 lines)
212-242: cleanupExpiredBatch full logic (31 lines)
252-255: CleanupExpired error wrapping
```

---

## Contact & Questions

For questions about this analysis or test implementation guidance, contact the quality engineering team or refer to:

- **Testing Guide:** `docs/TESTING.md`
- **Store Layer Docs:** `docs/DATABASE_SCHEMA.md`
- **Error Handling:** `internal/errors/README.md`

---

**End of Report**
