# Service Layer Test Gaps - Quick Reference

## Summary
- **Current Coverage**: 81.68% (1061/1299 statements)
- **Target Coverage**: 80% (already met)
- **Uncovered Critical Paths**: 35 functions/branches
- **Priority Tests Needed**: 15 (Immediate), 12 (Short-term), 8 (Long-term)

---

## Immediate Priority Tests (Compliance & Security)

### 1. Admin Audit Logging (4 tests - 2 hours)
**Files**: `internal/service/audit.go`, `internal/service/admin.go`

```go
// ✗ 0% coverage - audit.go:372-408
□ TestAuditService_LogUserDisabled
□ TestAuditService_LogUserEnabled  
□ TestAuditService_LogUserDeleted
□ TestAuditService_LogSystemCleanup
```

**Why**: Critical compliance gap - admin operations not audited

### 2. AdminService.DeleteUser (6 tests - 3 hours)
**File**: `internal/service/admin.go:231-257`

```go
// ✗ 27.3% coverage
□ TestAdminService_DeleteUser_FullFlow
□ TestAdminService_DeleteUser_TokenRevocationFails
□ TestAdminService_DeleteUser_CacheInvalidationFails
□ TestAdminService_DeleteUser_CannotDeleteLastAdmin
□ TestAdminService_DeleteUser_WithAuditLogging
□ TestAdminService_DeleteUser_MultipleTokensRevoked
```

**Why**: User deletion is critical; token/cache/audit failures untested

### 3. Social Login State Cleanup (5 tests - 3 hours)
**File**: `internal/service/social.go:157-181`

```go
// ✗ 38.5% coverage
□ TestSocialLoginService_cleanupExpiredStates_TickerCleanup
□ TestSocialLoginService_cleanupExpiredStates_InvalidData
□ TestSocialLoginService_cleanupExpiredStates_ExpiredStateRemoval
□ TestSocialLoginService_cleanupExpiredStates_GracefulShutdown
□ TestSocialLoginService_cleanupExpiredStates_ConcurrentAccess
```

**Why**: Memory leak + replay attack vector

---

## Short-Term Priority Tests

### 4. Admin User Management (4 tests - 2 hours)
**File**: `internal/service/admin.go:164-228`

```go
// ✗ 68.8% coverage each
□ TestAdminService_DisableUser_AlreadyDisabled
□ TestAdminService_DisableUser_WithCacheInvalidation
□ TestAdminService_EnableUser_AlreadyEnabled
□ TestAdminService_EnableUser_WithCacheInvalidation
```

### 5. Email SSL Path (3 tests - 2 hours)
**File**: `internal/service/email.go:143-201`

```go
// ✗ 0% coverage
□ TestEmailService_sendEmailSSL_Success
□ TestEmailService_sendEmailSSL_ConnectionFailure
□ TestEmailService_sendEmailSSL_AuthenticationFailure
```

### 6. OAuth Client Caching (5 tests - 2 hours)
**File**: `internal/service/oauth.go:101-131`

```go
// ✗ 46.2% coverage
□ TestOAuthService_getClient_CacheHit
□ TestOAuthService_getClient_CacheMissStoreError
□ TestOAuthService_getClient_CacheWriteFailure
□ TestOAuthService_getClient_NilCacheUsesStore
□ TestOAuthService_getClient_ConcurrentRequests
```

---

## Long-Term Priority Tests

### 7. Auth Service Options (2 tests - 1 hour)
**File**: `internal/service/auth.go:96-108`

```go
// ✗ 0-50% coverage
□ TestAuthService_WithLoginRateLimit
□ TestAuthService_WithMetrics
```

### 8. Auth Login Edge Cases (3 tests - 2 hours)
**File**: `internal/service/auth_login.go:153-194`

```go
// ✗ 45% coverage
□ TestAuthService_LoginWithAudit_RateLimited
□ TestAuthService_LoginWithAudit_MFARequiredNotProvided
□ TestAuthService_LoginWithAudit_AccountLockedWithAudit
```

### 9. Admin Cleanup (3 tests - 2 hours)
**File**: `internal/service/admin.go:284-313`

```go
// ✗ 60% coverage
□ TestAdminService_CleanupExpired_TokensAndCodes
□ TestAdminService_CleanupExpired_WithAuditLogging
□ TestAdminService_CleanupExpired_MultipleFailures
```

---

## Coverage Impact Projection

| Phase | Tests | Hours | Current | Target | Gain |
|-------|-------|-------|---------|--------|------|
| Immediate (1-3) | 15 | 8h | 81.68% | 85.2% | +3.52% |
| Short-term (4-6) | 12 | 6h | 85.2% | 88.0% | +2.80% |
| Long-term (7-9) | 8 | 5h | 88.0% | 89.5% | +1.50% |
| **Total** | **35** | **19h** | **81.68%** | **89.5%** | **+7.82%** |

---

## Quick Implementation Guide

### Test Structure Template
```go
func TestService_Method_Scenario(t *testing.T) {
    tests := []struct {
        name    string
        setup   func() dependencies
        input   inputType
        want    expectedType
        wantErr bool
        verify  func(t *testing.T, deps dependencies)
    }{
        // Test cases here
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // 1. Setup
            deps := tt.setup()
            svc := NewService(deps)
            
            // 2. Execute
            got, err := svc.Method(context.Background(), tt.input)
            
            // 3. Verify
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.want, got)
            }
            if tt.verify != nil {
                tt.verify(t, deps)
            }
        })
    }
}
```

### Mock Pattern for Admin Tests
```go
type mockAuditService struct {
    logs     []string
    warnings []string
    logErr   error
}

func (m *mockAuditService) LogUserDeleted(ctx context.Context, userID, ip string) {
    if m.logErr != nil {
        m.warnings = append(m.warnings, "audit failed")
        return
    }
    m.logs = append(m.logs, fmt.Sprintf("user.deleted:%s:%s", userID, ip))
}
```

---

## Uncovered Business Logic Rules

### Admin Service
- [ ] Cannot delete last admin user
- [ ] Admin cannot disable their own account
- [ ] Cleanup respects configurable retention period (7/30/90 days)
- [ ] Client IP required for security-sensitive operations

### Social Login Service  
- [ ] OAuth state expires after 5 minutes
- [ ] Duplicate state detection prevents replay
- [ ] Cleanup runs every 1 minute in background

### Email Service
- [ ] Port 465 uses direct SSL/TLS
- [ ] Port 587 uses STARTTLS upgrade
- [ ] Port 25 uses STARTTLS if available

---

## Files Requiring Attention

| File | Functions | Coverage | Priority |
|------|-----------|----------|----------|
| `admin.go` | 8 functions | 40-92% | 🔴 High |
| `audit.go` | 4 functions | 0% | 🔴 High |
| `social.go` | 1 function | 38.5% | 🔴 High |
| `email.go` | 2 functions | 0-66% | 🟡 Medium |
| `oauth.go` | 3 functions | 46-60% | 🟡 Medium |
| `auth.go` | 2 functions | 0-50% | 🟡 Medium |

---

## Next Steps

1. **Before next task (6.4)**: Review this list with team
2. **Sprint Planning**: Allocate 8 hours for immediate priority tests
3. **CI/CD**: Add coverage reporting to PR checks
4. **Documentation**: Update `TESTING.md` with new test patterns

---

**Generated**: 2026-07-12  
**Profile**: coverage_service.out  
**Total Statements**: 1299  
**Covered**: 1061 (81.68%)  
**Uncovered**: 238 (18.32%)
