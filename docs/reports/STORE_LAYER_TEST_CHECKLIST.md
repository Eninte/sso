# Store Layer Testing Checklist

**Purpose:** Systematic checklist for achieving 80%+ coverage in `internal/store/postgres/`

## Current Status

- ✅ Overall: 79.71% (Target: 80%+)
- ❌ mfa_recovery.go: 63.10% (Need: +17%)
- ❌ verification.go: 55.56% (Need: +25%)
- ❌ token.go: 78.67% (Need: +2%)

## Priority 1: Critical Security Operations

### MFA Recovery Code Operations (mfa_recovery.go)

#### Uncovered Database Error Handling

- [ ] **StoreMFARecoveryCodes - Transaction Failures**
  - [ ] BeginTx failure (connection timeout)
  - [ ] DELETE old codes failure (table lock)
  - [ ] INSERT new code failure (constraint violation)
  - [ ] Commit failure (network interruption)
  - [ ] Rollback verification

- [ ] **VerifyAndUseMFARecoveryCode - Atomic Operations**
  - [ ] Concurrent code verification (race condition test)
  - [ ] Already-used code verification (replay attack prevention)
  - [ ] ExecContext failure (database timeout)
  - [ ] RowsAffected() error handling

- [ ] **DisableMFAAndClearRecoveryCodes - Transaction Consistency**
  - [ ] BeginTx failure
  - [ ] User UPDATE failure mid-transaction
  - [ ] Recovery code DELETE failure mid-transaction
  - [ ] Commit failure (data consistency check)
  - [ ] Rollback behavior verification
  - [ ] Atomicity test (no partial updates)

- [ ] **GetUnusedMFARecoveryCodes - Query Errors**
  - [ ] QueryContext failure (connection error)
  - [ ] Rows.Scan() error (type mismatch)
  - [ ] Rows.Err() error (cursor error)

- [ ] **HMAC Operations**
  - [ ] hashRecoveryCode with no HMAC key set
  - [ ] getMFARecoveryHMACKey concurrent access
  - [ ] SetMFARecoveryHMACKey during active verification

#### Edge Cases

- [ ] Store 0 recovery codes (empty array)
- [ ] Store 100 recovery codes (stress test)
- [ ] Verify code with NULL user_id
- [ ] Verify code with empty string
- [ ] Verify code with 1000-character string

---

### Authorization Code Operations (token.go)

#### Uncovered Database Error Handling

- [ ] **UpdateAuthorizationCode - TOCTOU Prevention**
  - [ ] Concurrent code redemption (10 goroutines)
  - [ ] Already-used code update attempt
  - [ ] ExecContext failure (timeout)
  - [ ] RowsAffected() error handling
  - [ ] Verify ErrAuthorizationCodeUsed returned correctly

- [ ] **GetAuthorizationCode - Query Errors**
  - [ ] Scan error (NULL in NOT NULL field)
  - [ ] Connection timeout
  - [ ] Invalid code format (SQL injection attempt)

#### Edge Cases

- [ ] Authorization code exactly at expiration time
- [ ] Code with empty scopes array
- [ ] Code with 50 scopes (array size stress)
- [ ] Code with NULL CodeChallenge (non-PKCE flow)

---

### Reset Token Operations (verification.go)

#### Uncovered Database Error Handling

- [ ] **MarkResetTokenUsed - Atomic Update**
  - [ ] Concurrent token usage (race condition)
  - [ ] Already-used token marking
  - [ ] ExecContext failure
  - [ ] RowsAffected() error
  - [ ] Verify ErrNotFound when token missing

- [ ] **StoreResetToken - Validation**
  - [ ] Invalid table name (SQL injection prevention)
  - [ ] ExecContext failure on DELETE old token
  - [ ] ExecContext failure on INSERT new token

- [ ] **GetResetToken - Query Errors**
  - [ ] Scan error (NULL in used_at field)
  - [ ] ErrNotFound handling
  - [ ] Connection timeout

#### Edge Cases

- [ ] Token expiration in past (negative duration)
- [ ] Token expiration 10 years future (overflow check)
- [ ] User with multiple reset tokens (should only keep latest)

---

## Priority 2: Batch Operations & Cleanup

### Token Cleanup (token.go)

#### Uncovered Database Error Handling

- [ ] **CleanupExpired - Context Handling**
  - [ ] Context cancellation mid-cleanup
  - [ ] Context timeout during batch
  - [ ] Multiple table cleanup with partial failure

- [ ] **cleanupExpiredBatch - Batch Processing**
  - [ ] ExecContext failure mid-batch
  - [ ] RowsAffected() error
  - [ ] Invalid table name (security check)
  - [ ] Primary key column lookup failure

#### Edge Cases

- [ ] Cleanup with 0 expired records
- [ ] Cleanup with exactly CleanupBatchSize records
- [ ] Cleanup with 10,000+ expired records (multiple batches)
- [ ] Cleanup interrupted at 50% completion

---

## Priority 3: User Operations

### User CRUD (user.go)

#### Uncovered Database Error Handling

- [ ] **GetUser / GetUserByEmail / GetUserByID**
  - [ ] Scan error (NULL in NOT NULL field)
  - [ ] Connection timeout
  - [ ] Invalid UUID format

- [ ] **Update - Constraint Violations**
  - [ ] Duplicate email (unique constraint)
  - [ ] ExecContext failure
  - [ ] RowsAffected = 0 (user not found)

- [ ] **Delete - Cascade Handling**
  - [ ] Foreign key constraint violation (active sessions)
  - [ ] ExecContext failure

- [ ] **UnlockAccount**
  - [ ] ExecContext failure
  - [ ] User not found

#### Edge Cases

- [ ] Email with 320 characters (max RFC 5321 length)
- [ ] Email with Unicode characters (emoji@example.com)
- [ ] Password hash with NULL (should be rejected)

---

## Priority 4: Supporting Operations

### Client Operations (client.go)

#### Uncovered Error Handling

- [ ] **GetClient**
  - [ ] Scan error (NULL in redirect_uris array)
  - [ ] ErrNotFound handling

#### Edge Cases

- [ ] Client with 0 redirect URIs
- [ ] Client with 100 redirect URIs

---

### Key Operations (key.go)

#### Uncovered Error Handling

- [ ] **StorePublicKey / GetPublicKey / DeletePublicKey**
  - [ ] ExecContext failures
  - [ ] Scan errors
  - [ ] ErrNotFound handling

- [ ] **Rotation Operations**
  - [ ] StorePrivateKey failure
  - [ ] GetActivePrivateKey scan error
  - [ ] RotateKeys transaction failure

#### Edge Cases

- [ ] Key with 10,000-character PEM data
- [ ] Multiple active keys (should only have 1)

---

### Audit Operations (audit.go)

#### Uncovered Error Handling

- [ ] **CreateAuditLog**
  - [ ] ExecContext failure (disk full)
  - [ ] Constraint violation

- [ ] **GetAuditLogs**
  - [ ] Query with invalid filter parameters
  - [ ] Pagination edge cases (offset > total rows)

- [ ] **GetAuditLogsByUser**
  - [ ] User with 10,000+ audit logs
  - [ ] QueryContext failure

#### Edge Cases

- [ ] Audit log with 10MB metadata JSON
- [ ] Filter with NULL start_time
- [ ] Limit = 0 (should return empty)

---

### Connection Pool (postgres.go)

#### Uncovered Error Handling

- [ ] **NewStore**
  - [ ] Invalid connection string
  - [ ] Connection timeout
  - [ ] Database not accessible

- [ ] **Close**
  - [ ] Close error handling

- [ ] **withTimeout**
  - [ ] Already cancelled context
  - [ ] Timeout during query execution

#### Edge Cases

- [ ] MaxOpenConns = 1 (connection exhaustion)
- [ ] ConnMaxLifetime = 0 (unlimited lifetime)

---

## Testing Strategy

### 1. Use sqlmock for Error Injection

```go
import "github.com/DATA-DOG/go-sqlmock"

func TestStoreMFARecoveryCodesTransactionFailure(t *testing.T) {
    db, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer db.Close()

    store := &Store{db: db}

    // Inject transaction failure
    mock.ExpectBegin().WillReturnError(sql.ErrConnDone)

    err = store.StoreMFARecoveryCodes(ctx, userID, codes)
    require.Error(t, err)
    require.Contains(t, err.Error(), "begin transaction failed")
}
```

### 2. Use Goroutines for Concurrency Tests

```go
func TestVerifyAndUseMFARecoveryCodeConcurrent(t *testing.T) {
    var wg sync.WaitGroup
    successCount := atomic.Int32{}

    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            valid, err := store.VerifyAndUseMFARecoveryCode(ctx, userID, code)
            if err == nil && valid {
                successCount.Add(1)
            }
        }()
    }
    wg.Wait()

    // Only ONE goroutine should succeed
    require.Equal(t, int32(1), successCount.Load())
}
```

### 3. Use testcontainers for Integration Tests (Optional)

```go
import "github.com/testcontainers/testcontainers-go/modules/postgres"

func setupTestDB(t *testing.T) *Store {
    ctx := context.Background()
    container, _ := postgres.RunContainer(ctx)
    connStr, _ := container.ConnectionString(ctx)
    return NewStore(connStr)
}
```

### 4. Run with Race Detector

```bash
go test -race -count=100 ./internal/store/postgres/...
```

---

## Verification Checklist

After implementing tests, verify:

- [ ] Run coverage analysis: `go test -coverprofile=coverage_store.out ./internal/store/postgres/...`
- [ ] Check overall coverage ≥ 80%
- [ ] Check no file below 75%
- [ ] Run race detector: `go test -race ./internal/store/postgres/...`
- [ ] Run linters: `make lint`
- [ ] Check all tests pass: `go test -v ./internal/store/postgres/...`

---

## Quick Reference: Test File Mapping

| Source File        | Test File              | Priority | Tests Needed |
|--------------------|------------------------|----------|--------------|
| mfa_recovery.go    | mfa_recovery_test.go   | P0       | 12           |
| token.go           | token_test.go          | P0       | 10           |
| verification.go    | verification_test.go   | P1       | 8            |
| user.go            | user_test.go           | P2       | 5            |
| key.go             | key_test.go            | P2       | 5            |
| audit.go           | audit_test.go          | P2       | 3            |
| client.go          | client_test.go         | P2       | 2            |
| postgres.go        | postgres_test.go       | P2       | 3            |

**Total:** 48 tests to achieve comprehensive coverage

**Minimum for 80%:** 14 tests (focus on P0 items)

---

## Success Metrics

### Phase 1 Complete When:
- ✅ Overall coverage ≥ 80%
- ✅ All P0 security operations tested
- ✅ All race conditions verified with `-race`
- ✅ All SQL injection prevention validated

### Phase 2 Complete When:
- ✅ Overall coverage ≥ 90%
- ✅ All error paths tested
- ✅ All edge cases covered
- ✅ Documentation updated with test patterns

---

**Last Updated:** 2024-01-XX  
**Owner:** Quality Engineering Team
