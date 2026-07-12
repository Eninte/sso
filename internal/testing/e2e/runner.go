// Package e2e provides E2E test execution infrastructure for the SSO service.
// The TestRunner component stabilizes test execution through environment validation,
// test isolation mechanisms, and detailed failure logging.
package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	apperrors "github.com/example/sso/internal/errors"
)

// ============================================================================
// Core Types
// ============================================================================

// TestRunner orchestrates E2E test execution with environment validation,
// test isolation, and comprehensive failure reporting.
type TestRunner struct {
	db         *sql.DB
	redis      *redis.Client
	httpClient *http.Client
	baseURL    string
	config     *RunnerConfig

	// activeTx stores the active transaction for the current test
	activeTx *sql.Tx

	// testNamespace stores the Redis key namespace for the current test
	testNamespace string

	// testID stores a unique identifier for the current test, used for
	// pattern-based cleanup of data that escapes transaction boundaries
	testID string
}

// RunnerConfig holds configuration for the test runner.
type RunnerConfig struct {
	// Environment validation settings
	ValidatePostgresTriggers bool
	ValidateRedisConnection  bool

	// Smart skipping settings (for tests requiring specific dependencies)
	RequireSMTP  bool // Skip test if SMTP credentials are missing
	RequireOAuth bool // Skip test if OAuth provider configs are missing

	// Test isolation settings
	UseDBTransactions  bool
	RedisNamespaceMode bool

	// Logging settings
	CaptureStackTraces bool
	LogEnvironmentVars bool
}

// DefaultConfig returns a RunnerConfig with recommended defaults.
func DefaultConfig() *RunnerConfig {
	return &RunnerConfig{
		ValidatePostgresTriggers: true,
		ValidateRedisConnection:  true,
		UseDBTransactions:        true,
		RedisNamespaceMode:       true,
		CaptureStackTraces:       true,
		LogEnvironmentVars:       false, // False to avoid leaking secrets
	}
}

// ============================================================================
// Test Result Types
// ============================================================================

// TestStatus represents the execution status of a test.
type TestStatus string

const (
	// TestStatusPass indicates the test passed successfully.
	TestStatusPass TestStatus = "PASS"

	// TestStatusFail indicates the test failed.
	TestStatusFail TestStatus = "FAIL"

	// TestStatusSkip indicates the test was skipped (e.g., environment issue).
	TestStatusSkip TestStatus = "SKIP"
)

// TestResult captures the outcome of a single test execution.
type TestResult struct {
	Name       string
	Status     TestStatus
	Duration   time.Duration
	FailureLog *FailureLog
	SkipReason string
}

// FailureLog contains detailed debugging information for failed tests.
// Validates: Requirements 1.2
type FailureLog struct {
	// Request contains the full HTTP request dump (method, headers, body)
	Request *HTTPRequestCapture

	// Response contains the full HTTP response dump (status, headers, body)
	Response *HTTPResponseCapture

	// Environment contains relevant environment state at failure time
	Environment *EnvironmentState

	// StackTrace contains goroutine stack traces at failure time
	StackTrace string

	// ErrorMessage is the primary error message
	ErrorMessage string

	// Timestamp is when the failure occurred
	Timestamp time.Time
}

// HTTPRequestCapture contains full HTTP request details for debugging.
type HTTPRequestCapture struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	RawDump string            `json:"raw_dump"`
}

// HTTPResponseCapture contains full HTTP response details for debugging.
type HTTPResponseCapture struct {
	Status     string            `json:"status"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	RawDump    string            `json:"raw_dump"`
}

// EnvironmentState captures environment state at failure time.
type EnvironmentState struct {
	EnvVars         map[string]string `json:"env_vars"`
	DBConnection    ConnectionStatus  `json:"db_connection"`
	RedisConnection ConnectionStatus  `json:"redis_connection"`
	Timestamp       time.Time         `json:"timestamp"`
	GoroutineCount  int               `json:"goroutine_count"`
}

// ConnectionStatus represents the health status of a connection.
type ConnectionStatus struct {
	IsConnected bool      `json:"is_connected"`
	Error       string    `json:"error,omitempty"`
	LastChecked time.Time `json:"last_checked"`
}

// ============================================================================
// Test Definition
// ============================================================================

// Test represents a single E2E test to be executed.
type Test struct {
	Name string
	Run  func(ctx context.Context, tr *TestRunner) error
}

// ============================================================================
// Constructor
// ============================================================================

// NewTestRunner creates a new TestRunner instance with the provided dependencies.
func NewTestRunner(db *sql.DB, redis *redis.Client, baseURL string, config *RunnerConfig) *TestRunner {
	if config == nil {
		config = DefaultConfig()
	}

	return &TestRunner{
		db:    db,
		redis: redis,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
		config:  config,
	}
}

// ============================================================================
// Environment Validation
// ============================================================================

// ValidateEnvironment performs pre-flight checks to ensure the test environment
// is properly configured. This includes verifying PostgreSQL triggers for test
// user auto-verification and Redis connectivity.
//
// Returns an error if any critical environment requirements are not met.
func (tr *TestRunner) ValidateEnvironment(ctx context.Context) error {
	if tr.config.ValidatePostgresTriggers {
		if err := tr.checkPostgresTriggers(ctx); err != nil {
			return apperrors.Wrap(
				apperrors.ErrCodeInternal,
				"PostgreSQL trigger validation failed",
				http.StatusServiceUnavailable,
				err,
			)
		}
	}

	if tr.config.ValidateRedisConnection {
		if err := tr.checkRedisConnection(ctx); err != nil {
			return apperrors.Wrap(
				apperrors.ErrCodeInternal,
				"Redis connection validation failed",
				http.StatusServiceUnavailable,
				err,
			)
		}
	}

	return nil
}

// ShouldSkipTest determines if a test should be skipped based on missing
// environment dependencies. Returns true with a skip reason if the test
// should be skipped, otherwise returns false.
//
// This enables graceful test skipping when optional dependencies like SMTP
// or OAuth providers are not configured, preventing false test failures.
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

// checkPostgresTriggers verifies that the auto_verify_test_email trigger exists
// and is active on the users table. This trigger automatically verifies @example.com
// email addresses for test purposes.
func (tr *TestRunner) checkPostgresTriggers(ctx context.Context) error {
	if tr.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT COUNT(*)
		FROM pg_trigger
		WHERE tgname = 'trigger_auto_verify_test_email'
		  AND tgrelid = 'users'::regclass
		  AND tgenabled = 'O'
	`

	var count int
	err := tr.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to query triggers: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("trigger_auto_verify_test_email not found or not enabled on users table. Run 'make test-e2e-prepare' to set up test environment")
	}

	return nil
}

// checkRedisConnection verifies that Redis is accessible and responding.
func (tr *TestRunner) checkRedisConnection(ctx context.Context) error {
	if tr.redis == nil {
		return fmt.Errorf("redis client is nil")
	}

	pong, err := tr.redis.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}

	if pong != "PONG" {
		return fmt.Errorf("redis ping returned unexpected response: %s", pong)
	}

	return nil
}

// checkSMTPAvailable checks if SMTP credentials are configured in the environment.
// Returns (true, reason) if SMTP is not available and tests should be skipped.
//
// SMTP Configuration Requirements:
// - SMTP_HOST must be set and not empty
// - SMTP_USER must be set and not empty
// - SMTP_PASSWORD must be set and not empty
//
// If any of these are missing, email-related tests should be skipped gracefully
// rather than failing.
func (tr *TestRunner) checkSMTPAvailable() (bool, string) {
	// Import os package for environment variable access
	smtpHost := getEnvVar("SMTP_HOST")
	smtpUser := getEnvVar("SMTP_USER")
	smtpPassword := getEnvVar("SMTP_PASSWORD")

	if smtpHost == "" || smtpUser == "" || smtpPassword == "" {
		reason := "SMTP credentials not configured. To run email tests, set environment variables:\n" +
			"  - SMTP_HOST: SMTP server address (e.g., smtp.example.com)\n" +
			"  - SMTP_USER: SMTP username/email\n" +
			"  - SMTP_PASSWORD: SMTP password or app-specific password\n" +
			"Example: export SMTP_HOST=smtp.gmail.com SMTP_USER=test@example.com SMTP_PASSWORD=yourpass"
		return true, reason
	}

	return false, ""
}

// checkOAuthAvailable checks if OAuth provider configurations are present.
// Returns (true, reason) if OAuth providers are not configured and tests should be skipped.
//
// OAuth Configuration Requirements (at least one provider must be configured):
// - Google: OAUTH_GOOGLE_CLIENT_ID and OAUTH_GOOGLE_CLIENT_SECRET
// - GitHub: OAUTH_GITHUB_CLIENT_ID and OAUTH_GITHUB_CLIENT_SECRET
// - Other providers follow similar patterns
//
// If no OAuth providers are configured, OAuth-related tests should be skipped.
func (tr *TestRunner) checkOAuthAvailable() (bool, string) {
	// Check for common OAuth provider configurations
	googleClientID := getEnvVar("OAUTH_GOOGLE_CLIENT_ID")
	googleClientSecret := getEnvVar("OAUTH_GOOGLE_CLIENT_SECRET")

	githubClientID := getEnvVar("OAUTH_GITHUB_CLIENT_ID")
	githubClientSecret := getEnvVar("OAUTH_GITHUB_CLIENT_SECRET")

	// If at least one OAuth provider is configured, don't skip
	if (googleClientID != "" && googleClientSecret != "") ||
		(githubClientID != "" && githubClientSecret != "") {
		return false, ""
	}

	reason := "OAuth provider credentials not configured. To run OAuth tests, configure at least one provider:\n" +
		"  Google OAuth:\n" +
		"    - OAUTH_GOOGLE_CLIENT_ID: Your Google OAuth client ID\n" +
		"    - OAUTH_GOOGLE_CLIENT_SECRET: Your Google OAuth client secret\n" +
		"  GitHub OAuth:\n" +
		"    - OAUTH_GITHUB_CLIENT_ID: Your GitHub OAuth client ID\n" +
		"    - OAUTH_GITHUB_CLIENT_SECRET: Your GitHub OAuth client secret\n" +
		"Example: export OAUTH_GOOGLE_CLIENT_ID=your-id OAUTH_GOOGLE_CLIENT_SECRET=your-secret"
	return true, reason
}

// ============================================================================
// Test Execution
// ============================================================================

// Run executes a collection of tests and returns their results.
// Tests are executed sequentially with proper isolation and cleanup between each test.
func (tr *TestRunner) Run(ctx context.Context, tests []Test) []TestResult {
	results := make([]TestResult, 0, len(tests))

	for _, test := range tests {
		result := tr.runSingleTest(ctx, test)
		results = append(results, result)
	}

	return results
}

// runSingleTest executes a single test with isolation and captures detailed results.
func (tr *TestRunner) runSingleTest(ctx context.Context, test Test) TestResult {
	result := TestResult{
		Name: test.Name,
	}

	tr.LogStructured("INFO", "Starting test", map[string]interface{}{
		"test": test.Name,
	})

	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	// Check if test should be skipped due to missing dependencies
	// Validates: Requirements 1.3
	if shouldSkip, skipReason := tr.ShouldSkipTest(ctx); shouldSkip {
		result.Status = TestStatusSkip
		result.SkipReason = skipReason
		tr.LogStructured("INFO", "Test skipped", map[string]interface{}{
			"test":   test.Name,
			"reason": skipReason,
		})
		return result
	}

	// Set up test isolation
	if err := tr.IsolateTest(ctx, test); err != nil {
		result.Status = TestStatusSkip
		result.SkipReason = fmt.Sprintf("Failed to isolate test: %v", err)
		tr.LogStructured("WARN", "Test skipped", map[string]interface{}{
			"test":   test.Name,
			"reason": result.SkipReason,
		})
		return result
	}

	// Execute the test
	err := test.Run(ctx, tr)

	// Clean up test isolation
	cleanupErr := tr.CleanupTest(ctx, test)
	if cleanupErr != nil {
		tr.LogStructured("WARN", "Test cleanup failed", map[string]interface{}{
			"test":  test.Name,
			"error": cleanupErr.Error(),
		})
	}

	// Process test result
	if err != nil {
		result.Status = TestStatusFail
		result.FailureLog = tr.captureFailureLog(ctx, err)
		tr.LogTestFailure(test.Name, result.FailureLog)
	} else {
		result.Status = TestStatusPass
		tr.LogStructured("INFO", "Test passed", map[string]interface{}{
			"test":     test.Name,
			"duration": result.Duration.String(),
		})
	}

	return result
}

// ============================================================================
// Test Isolation
// ============================================================================

// IsolateTest sets up isolation mechanisms for a test execution.
// This includes starting a database transaction and setting up Redis namespacing.
func (tr *TestRunner) IsolateTest(ctx context.Context, test Test) error {
	// Generate unique test identifier for pattern-based cleanup
	tr.testID = fmt.Sprintf("e2e_%d_%s", time.Now().UnixNano(), sanitizeTestName(test.Name))

	// Database transaction isolation
	if tr.config.UseDBTransactions && tr.db != nil {
		tx, err := tr.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		tr.activeTx = tx
	}

	// Redis namespace isolation
	// Generate unique namespace for this test using timestamp + test name
	if tr.config.RedisNamespaceMode && tr.redis != nil {
		tr.testNamespace = fmt.Sprintf("test:%d:%s", time.Now().UnixNano(), sanitizeTestName(test.Name))
	}

	return nil
}

// CleanupTest performs cleanup after test execution.
// This includes rolling back database transactions, clearing Redis test data,
// and pattern-based cleanup for data that may have escaped transaction boundaries
// (e.g., through HTTP requests handled by the server's own DB connections).
func (tr *TestRunner) CleanupTest(ctx context.Context, test Test) error {
	var cleanupErrors []error

	// Rollback database transaction if active
	if tr.config.UseDBTransactions && tr.activeTx != nil {
		if err := tr.activeTx.Rollback(); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("transaction rollback failed: %w", err))
		}
		tr.activeTx = nil
	}

	// Pattern-based cleanup: removes test data that escaped the transaction
	// boundary (e.g., HTTP requests executed by the server's own DB connections).
	if tr.testID != "" && tr.db != nil {
		isolation := NewIsolationHelper(tr.db, tr.redis)
		if err := isolation.CleanupTestDataByPattern(ctx, "%"+tr.testID+"%"); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("pattern cleanup failed: %w", err))
		}
	}
	// Always clear testID after cleanup attempt
	tr.testID = ""

	// Clean up Redis test keys with namespace prefix
	if tr.config.RedisNamespaceMode && tr.redis != nil && tr.testNamespace != "" {
		if err := tr.cleanupRedisKeys(ctx); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("redis cleanup failed: %w", err))
		}
		tr.testNamespace = ""
	}

	// Return combined error if any cleanup failed
	if len(cleanupErrors) > 0 {
		return fmt.Errorf("cleanup errors: %v", cleanupErrors)
	}

	return nil
}

// cleanupRedisKeys deletes all Redis keys matching the test namespace pattern.
func (tr *TestRunner) cleanupRedisKeys(ctx context.Context) error {
	pattern := tr.testNamespace + ":*"

	// Use SCAN to find all keys with the test namespace prefix
	iter := tr.redis.Scan(ctx, 0, pattern, 0).Iterator()
	keysToDelete := []string{}

	for iter.Next(ctx) {
		keysToDelete = append(keysToDelete, iter.Val())
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to scan redis keys: %w", err)
	}

	// Delete all found keys
	if len(keysToDelete) > 0 {
		if err := tr.redis.Del(ctx, keysToDelete...).Err(); err != nil {
			return fmt.Errorf("failed to delete redis keys: %w", err)
		}
	}

	return nil
}

// sanitizeTestName converts test name to a safe Redis key component.
func sanitizeTestName(name string) string {
	// Replace spaces and special characters with hyphens
	safe := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			safe += string(r)
		} else {
			safe += "-"
		}
	}
	return safe
}

// ============================================================================
// Failure Logging
// ============================================================================

// captureFailureLog creates a comprehensive failure log for debugging.
// Captures full HTTP request/response data, environment state including
// DB/Redis connection status, and goroutine stack traces.
// Validates: Requirements 1.2
func (tr *TestRunner) captureFailureLog(ctx context.Context, err error) *FailureLog {
	log := &FailureLog{
		ErrorMessage: err.Error(),
		Timestamp:    time.Now(),
	}

	// Capture stack traces with all goroutines
	if tr.config.CaptureStackTraces {
		log.StackTrace = tr.captureStackTrace()
	}

	// Capture comprehensive environment state
	log.Environment = tr.captureEnvironmentState(ctx)

	return log
}

// CaptureHTTPRequestResponse captures full HTTP request and response data
// for inclusion in failure logs. This should be called by test code when
// HTTP requests are made.
// Validates: Requirements 1.2
func (tr *TestRunner) CaptureHTTPRequestResponse(req *http.Request, resp *http.Response) (*HTTPRequestCapture, *HTTPResponseCapture) {
	var reqCapture *HTTPRequestCapture
	var respCapture *HTTPResponseCapture

	// Capture request
	if req != nil {
		reqCapture = &HTTPRequestCapture{
			Method:  req.Method,
			URL:     req.URL.String(),
			Headers: make(map[string]string),
		}

		// Copy headers (excluding sensitive ones)
		for key, values := range req.Header {
			if !tr.isSensitiveHeader(key) {
				reqCapture.Headers[key] = strings.Join(values, ", ")
			} else {
				reqCapture.Headers[key] = "[REDACTED]"
			}
		}

		// Capture request body if present
		if req.Body != nil {
			bodyBytes, err := io.ReadAll(req.Body)
			if err == nil {
				reqCapture.Body = tr.truncateBody(string(bodyBytes), 10240) // 10KB limit
				// Restore body for actual request
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}
		}

		// Capture raw dump
		dumpBytes, err := httputil.DumpRequestOut(req, true)
		if err == nil {
			reqCapture.RawDump = tr.truncateBody(string(dumpBytes), 10240)
		}
	}

	// Capture response
	if resp != nil {
		respCapture = &HTTPResponseCapture{
			Status:     resp.Status,
			StatusCode: resp.StatusCode,
			Headers:    make(map[string]string),
		}

		// Copy headers
		for key, values := range resp.Header {
			respCapture.Headers[key] = strings.Join(values, ", ")
		}

		// Capture response body
		if resp.Body != nil {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err == nil {
				respCapture.Body = tr.truncateBody(string(bodyBytes), 10240) // 10KB limit
				// Restore body for further reading
				resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}
		}

		// Capture raw dump
		dumpBytes, err := httputil.DumpResponse(resp, true)
		if err == nil {
			respCapture.RawDump = tr.truncateBody(string(dumpBytes), 10240)
		}
	}

	return reqCapture, respCapture
}

// captureStackTrace captures goroutine stack traces for all goroutines.
// Uses runtime.Stack with allGoroutines=true to capture complete stack state.
// Validates: Requirements 1.2
func (tr *TestRunner) captureStackTrace() string {
	// Allocate 64KB buffer for stack traces (should be sufficient for most cases)
	buf := make([]byte, 65536)
	n := runtime.Stack(buf, true) // true = all goroutines

	// Add header with goroutine count
	goroutineCount := runtime.NumGoroutine()
	header := fmt.Sprintf("=== Goroutine Stack Traces (Total: %d) ===\n\n", goroutineCount)

	return header + string(buf[:n])
}

// captureEnvironmentState captures comprehensive environment state including
// environment variables, DB connection status, Redis connection status.
// Validates: Requirements 1.2
func (tr *TestRunner) captureEnvironmentState(ctx context.Context) *EnvironmentState {
	state := &EnvironmentState{
		EnvVars:        make(map[string]string),
		Timestamp:      time.Now(),
		GoroutineCount: runtime.NumGoroutine(),
	}

	// Capture environment variables (safe ones only, or all if configured)
	state.EnvVars = tr.captureEnvironmentVars()

	// Check DB connection status
	state.DBConnection = tr.checkDBConnectionStatus(ctx)

	// Check Redis connection status
	state.RedisConnection = tr.checkRedisConnectionStatus(ctx)

	return state
}

// captureEnvironmentVars captures relevant environment variables.
// If LogEnvironmentVars is enabled, captures all safe variables.
// Otherwise, captures only essential test-related variables.
// Validates: Requirements 1.2
func (tr *TestRunner) captureEnvironmentVars() map[string]string {
	env := make(map[string]string)

	// List of safe environment variables to always capture
	safeVars := []string{
		"E2E_BASE_URL",
		"SERVER_ENV",
		"RATE_LIMIT_REQUESTS",
		"DB_SSL_MODE",
		"REDIS_ADDR",
		"GO_ENV",
		"CI",
		"GITHUB_ACTIONS",
	}

	for _, key := range safeVars {
		if value := os.Getenv(key); value != "" {
			env[key] = value
		}
	}

	// If full logging enabled, capture additional variables
	if tr.config.LogEnvironmentVars {
		additionalVars := []string{
			"DB_HOST",
			"DB_PORT",
			"DB_NAME",
			"DB_USER",
			"REDIS_DB",
			"JWT_ACCESS_EXPIRY",
			"JWT_REFRESH_EXPIRY",
			"BCRYPT_COST",
		}

		for _, key := range additionalVars {
			if value := os.Getenv(key); value != "" {
				// Redact sensitive values
				if strings.Contains(strings.ToLower(key), "password") ||
					strings.Contains(strings.ToLower(key), "secret") ||
					strings.Contains(strings.ToLower(key), "key") {
					env[key] = "[REDACTED]"
				} else {
					env[key] = value
				}
			}
		}
	}

	return env
}

// checkDBConnectionStatus checks the database connection health.
// Validates: Requirements 1.2
func (tr *TestRunner) checkDBConnectionStatus(ctx context.Context) ConnectionStatus {
	status := ConnectionStatus{
		LastChecked: time.Now(),
	}

	if tr.db == nil {
		status.IsConnected = false
		status.Error = "database connection is nil"
		return status
	}

	// Ping with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := tr.db.PingContext(pingCtx); err != nil {
		status.IsConnected = false
		status.Error = fmt.Sprintf("ping failed: %v", err)
		return status
	}

	status.IsConnected = true
	return status
}

// checkRedisConnectionStatus checks the Redis connection health.
// Validates: Requirements 1.2
func (tr *TestRunner) checkRedisConnectionStatus(ctx context.Context) ConnectionStatus {
	status := ConnectionStatus{
		LastChecked: time.Now(),
	}

	if tr.redis == nil {
		status.IsConnected = false
		status.Error = "redis client is nil"
		return status
	}

	// Ping with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if _, err := tr.redis.Ping(pingCtx).Result(); err != nil {
		status.IsConnected = false
		status.Error = fmt.Sprintf("ping failed: %v", err)
		return status
	}

	status.IsConnected = true
	return status
}

// isSensitiveHeader checks if a header name contains sensitive data.
func (tr *TestRunner) isSensitiveHeader(headerName string) bool {
	lower := strings.ToLower(headerName)
	sensitiveHeaders := []string{
		"authorization",
		"cookie",
		"set-cookie",
		"x-api-key",
		"x-auth-token",
	}

	for _, sensitive := range sensitiveHeaders {
		if strings.Contains(lower, sensitive) {
			return true
		}
	}

	return false
}

// truncateBody truncates a body string to the specified maximum length.
func (tr *TestRunner) truncateBody(body string, maxLen int) string {
	if len(body) <= maxLen {
		return body
	}
	return body[:maxLen] + fmt.Sprintf("\n... [TRUNCATED: %d bytes total]", len(body))
}

// ============================================================================
// HTTP Client Access
// ============================================================================

// GetHTTPClient returns the HTTP client for making requests to the SSO service.
func (tr *TestRunner) GetHTTPClient() *http.Client {
	return tr.httpClient
}

// GetBaseURL returns the base URL of the SSO service under test.
func (tr *TestRunner) GetBaseURL() string {
	return tr.baseURL
}

// GetDB returns the database connection for direct database operations if needed.
func (tr *TestRunner) GetDB() *sql.DB {
	return tr.db
}

// GetRedis returns the Redis client for direct Redis operations if needed.
func (tr *TestRunner) GetRedis() *redis.Client {
	return tr.redis
}

// GetTestID returns the unique test identifier for the current test execution.
// This identifier is generated during IsolateTest and can be embedded in test
// data (e.g., email addresses, client IDs) so that CleanupTest's pattern-based
// cleanup can find and remove all data created by the test.
//
// Example usage in a test:
//
//	func TestExample(t *testing.T) {
//	    runner.Run(ctx, []Test{{
//	        Name: "MyTest",
//	        Run: func(ctx context.Context, tr *TestRunner) error {
//	            testID := tr.GetTestID()
//	            email := fmt.Sprintf("user-%s@example.com", testID)
//	            // ... create user with this email ...
//	        },
//	    }})
//	}
func (tr *TestRunner) GetTestID() string {
	return tr.testID
}

// GetTx returns the active database transaction for the current test.
// Returns nil if DB transactions are disabled or no test is active.
// Use this to execute direct database operations within the test transaction,
// ensuring they are rolled back during cleanup.
func (tr *TestRunner) GetTx() *sql.Tx {
	return tr.activeTx
}

// ============================================================================
// Helper Methods for Test Integration
// ============================================================================

// DoHTTPRequest performs an HTTP request with automatic capture of request/response
// data for failure logging. This is a convenience method for test code.
func (tr *TestRunner) DoHTTPRequest(req *http.Request) (*http.Response, *HTTPRequestCapture, *HTTPResponseCapture, error) {
	resp, err := tr.httpClient.Do(req)
	reqCapture, respCapture := tr.CaptureHTTPRequestResponse(req, resp)
	return resp, reqCapture, respCapture, err
}

// LogStructured logs a structured message with timestamp.
// This provides consistent logging format across all E2E tests.
// Validates: Requirements 1.2
func (tr *TestRunner) LogStructured(level, message string, fields map[string]interface{}) {
	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z07:00")

	fmt.Printf("[%s] [%s] %s", timestamp, level, message)

	if len(fields) > 0 {
		fmt.Print(" |")
		for key, value := range fields {
			fmt.Printf(" %s=%v", key, value)
		}
	}

	fmt.Println()
}

// LogTestFailure logs a test failure with full details in structured format.
// Validates: Requirements 1.2
func (tr *TestRunner) LogTestFailure(testName string, failureLog *FailureLog) {
	tr.LogStructured("ERROR", fmt.Sprintf("Test failed: %s", testName), map[string]interface{}{
		"test":      testName,
		"error":     failureLog.ErrorMessage,
		"timestamp": failureLog.Timestamp.Format(time.RFC3339),
	})

	// Log detailed failure information
	if failureLog.Request != nil {
		tr.LogStructured("DEBUG", "Request details", map[string]interface{}{
			"method": failureLog.Request.Method,
			"url":    failureLog.Request.URL,
		})
	}

	if failureLog.Response != nil {
		tr.LogStructured("DEBUG", "Response details", map[string]interface{}{
			"status": failureLog.Response.Status,
			"code":   failureLog.Response.StatusCode,
		})
	}

	if failureLog.Environment != nil {
		tr.LogStructured("DEBUG", "Environment state", map[string]interface{}{
			"db_connected":    failureLog.Environment.DBConnection.IsConnected,
			"redis_connected": failureLog.Environment.RedisConnection.IsConnected,
			"goroutines":      failureLog.Environment.GoroutineCount,
		})
	}

	// Log stack trace if available
	if failureLog.StackTrace != "" {
		fmt.Print("\n=== Stack Trace ===\n")
		fmt.Print(failureLog.StackTrace)
		fmt.Println("===================")
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

// getEnvVar retrieves an environment variable value.
// This is a helper function for environment validation.
func getEnvVar(key string) string {
	return os.Getenv(key)
}
