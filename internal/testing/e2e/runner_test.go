package e2e

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestNewTestRunner verifies TestRunner initialization with default config.
func TestNewTestRunner(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)

	if runner == nil {
		t.Fatal("NewTestRunner returned nil")
	}

	if runner.baseURL != "http://localhost:9090" {
		t.Errorf("expected baseURL 'http://localhost:9090', got '%s'", runner.baseURL)
	}

	if runner.config == nil {
		t.Fatal("config should not be nil when nil is passed (should use defaults)")
	}

	if !runner.config.ValidatePostgresTriggers {
		t.Error("expected ValidatePostgresTriggers to be true by default")
	}

	if !runner.config.ValidateRedisConnection {
		t.Error("expected ValidateRedisConnection to be true by default")
	}

	if runner.httpClient == nil {
		t.Error("httpClient should be initialized")
	}
}

// TestNewTestRunnerWithCustomConfig verifies custom config is respected.
func TestNewTestRunnerWithCustomConfig(t *testing.T) {
	config := &RunnerConfig{
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
		UseDBTransactions:        false,
		RedisNamespaceMode:       false,
		CaptureStackTraces:       false,
		LogEnvironmentVars:       true,
	}

	runner := NewTestRunner(nil, nil, "http://test:8080", config)

	if runner.config.ValidatePostgresTriggers {
		t.Error("expected ValidatePostgresTriggers to be false")
	}

	if runner.config.LogEnvironmentVars != true {
		t.Error("expected LogEnvironmentVars to be true")
	}
}

// TestTestStatusConstants verifies status constants are defined correctly.
func TestTestStatusConstants(t *testing.T) {
	tests := []struct {
		status   TestStatus
		expected string
	}{
		{TestStatusPass, "PASS"},
		{TestStatusFail, "FAIL"},
		{TestStatusSkip, "SKIP"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("expected status '%s', got '%s'", tt.expected, string(tt.status))
		}
	}
}

// TestRunSingleTestPass verifies successful test execution.
func TestRunSingleTestPass(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", &RunnerConfig{
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
		UseDBTransactions:        false,
		RedisNamespaceMode:       false,
		CaptureStackTraces:       false,
		LogEnvironmentVars:       false,
	})

	test := Test{
		Name: "PassingTest",
		Run: func(ctx context.Context, tr *TestRunner) error {
			return nil // Test passes
		},
	}

	ctx := context.Background()
	result := runner.runSingleTest(ctx, test)

	if result.Status != TestStatusPass {
		t.Errorf("expected status PASS, got %s", result.Status)
	}

	if result.Name != "PassingTest" {
		t.Errorf("expected name 'PassingTest', got '%s'", result.Name)
	}

	// Duration may be 0 for very fast tests, just verify it's not negative
	if result.Duration < 0 {
		t.Error("expected duration to be non-negative")
	}

	if result.FailureLog != nil {
		t.Error("expected no failure log for passing test")
	}
}

// TestRunSingleTestFail verifies failed test execution and failure logging.
func TestRunSingleTestFail(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", &RunnerConfig{
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
		UseDBTransactions:        false,
		RedisNamespaceMode:       false,
		CaptureStackTraces:       true,
		LogEnvironmentVars:       false,
	})

	testErr := errors.New("test failure")
	test := Test{
		Name: "FailingTest",
		Run: func(ctx context.Context, tr *TestRunner) error {
			return testErr
		},
	}

	ctx := context.Background()
	result := runner.runSingleTest(ctx, test)

	if result.Status != TestStatusFail {
		t.Errorf("expected status FAIL, got %s", result.Status)
	}

	if result.FailureLog == nil {
		t.Fatal("expected failure log for failing test")
	}

	if result.FailureLog.ErrorMessage != "test failure" {
		t.Errorf("expected error message 'test failure', got '%s'", result.FailureLog.ErrorMessage)
	}

	if result.FailureLog.StackTrace == "" {
		t.Error("expected stack trace to be captured when CaptureStackTraces is true")
	}

	if result.FailureLog.Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}
}

// TestRunMultipleTests verifies multiple tests are executed in sequence.
func TestRunMultipleTests(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", &RunnerConfig{
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
		UseDBTransactions:        false,
		RedisNamespaceMode:       false,
		CaptureStackTraces:       false,
		LogEnvironmentVars:       false,
	})

	executionOrder := []string{}

	tests := []Test{
		{
			Name: "Test1",
			Run: func(ctx context.Context, tr *TestRunner) error {
				executionOrder = append(executionOrder, "Test1")
				return nil
			},
		},
		{
			Name: "Test2",
			Run: func(ctx context.Context, tr *TestRunner) error {
				executionOrder = append(executionOrder, "Test2")
				return errors.New("fail")
			},
		},
		{
			Name: "Test3",
			Run: func(ctx context.Context, tr *TestRunner) error {
				executionOrder = append(executionOrder, "Test3")
				return nil
			},
		},
	}

	ctx := context.Background()
	results := runner.Run(ctx, tests)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify execution order
	expectedOrder := []string{"Test1", "Test2", "Test3"}
	for i, name := range expectedOrder {
		if executionOrder[i] != name {
			t.Errorf("expected execution order[%d] to be '%s', got '%s'", i, name, executionOrder[i])
		}
	}

	// Verify results
	if results[0].Status != TestStatusPass {
		t.Errorf("Test1 should pass, got %s", results[0].Status)
	}

	if results[1].Status != TestStatusFail {
		t.Errorf("Test2 should fail, got %s", results[1].Status)
	}

	if results[2].Status != TestStatusPass {
		t.Errorf("Test3 should pass, got %s", results[2].Status)
	}
}

// TestCheckRedisConnection verifies Redis connection validation.
func TestCheckRedisConnection(t *testing.T) {
	t.Run("nil redis client", func(t *testing.T) {
		runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)
		ctx := context.Background()

		err := runner.checkRedisConnection(ctx)
		if err == nil {
			t.Error("expected error for nil redis client")
		}
	})
}

// TestCheckPostgresTriggers verifies PostgreSQL trigger validation.
func TestCheckPostgresTriggers(t *testing.T) {
	t.Run("nil database connection", func(t *testing.T) {
		runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)
		ctx := context.Background()

		err := runner.checkPostgresTriggers(ctx)
		if err == nil {
			t.Error("expected error for nil database connection")
		}
	})
}

// TestCaptureStackTrace verifies stack trace capture with goroutine count header.
func TestCaptureStackTrace(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)

	stackTrace := runner.captureStackTrace()

	if stackTrace == "" {
		t.Error("expected non-empty stack trace")
	}

	// Stack trace should contain goroutine information and header
	if !strings.Contains(stackTrace, "Goroutine Stack Traces") {
		t.Error("expected stack trace to contain header with goroutine count")
	}

	// Stack trace should be substantial
	if len(stackTrace) < 100 {
		t.Error("stack trace seems too short")
	}
}

// TestCaptureEnvironmentVars verifies environment variable capture.
func TestCaptureEnvironmentVars(t *testing.T) {
	// Set some test environment variables
	os.Setenv("E2E_BASE_URL", "http://test:8080")
	os.Setenv("SERVER_ENV", "test")
	defer func() {
		os.Unsetenv("E2E_BASE_URL")
		os.Unsetenv("SERVER_ENV")
	}()

	runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)

	env := runner.captureEnvironmentVars()

	if env == nil {
		t.Fatal("expected non-nil environment map")
	}

	// Should contain captured environment variables
	if env["E2E_BASE_URL"] != "http://test:8080" {
		t.Errorf("expected E2E_BASE_URL to be 'http://test:8080', got '%s'", env["E2E_BASE_URL"])
	}

	if env["SERVER_ENV"] != "test" {
		t.Errorf("expected SERVER_ENV to be 'test', got '%s'", env["SERVER_ENV"])
	}
}

// TestCaptureEnvironmentState verifies comprehensive environment state capture.
func TestCaptureEnvironmentState(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)

	ctx := context.Background()
	state := runner.captureEnvironmentState(ctx)

	if state == nil {
		t.Fatal("expected non-nil environment state")
	}

	if state.EnvVars == nil {
		t.Error("expected EnvVars map to be initialized")
	}

	if state.GoroutineCount <= 0 {
		t.Error("expected positive goroutine count")
	}

	if state.Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}

	// DB connection should be checked (will be not connected with nil db)
	if state.DBConnection.IsConnected {
		t.Error("expected DB connection to be false with nil db")
	}

	if state.DBConnection.Error == "" {
		t.Error("expected DB connection error message")
	}

	// Redis connection should be checked (will be not connected with nil redis)
	if state.RedisConnection.IsConnected {
		t.Error("expected Redis connection to be false with nil redis")
	}

	if state.RedisConnection.Error == "" {
		t.Error("expected Redis connection error message")
	}
}

// TestCheckDBConnectionStatus verifies DB connection status checking.
func TestCheckDBConnectionStatus(t *testing.T) {
	t.Run("nil database", func(t *testing.T) {
		runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)
		ctx := context.Background()

		status := runner.checkDBConnectionStatus(ctx)

		if status.IsConnected {
			t.Error("expected IsConnected to be false for nil database")
		}

		if !strings.Contains(status.Error, "nil") {
			t.Errorf("expected error to mention 'nil', got: %s", status.Error)
		}

		if status.LastChecked.IsZero() {
			t.Error("expected LastChecked to be set")
		}
	})
}

// TestCheckRedisConnectionStatus verifies Redis connection status checking.
func TestCheckRedisConnectionStatus(t *testing.T) {
	t.Run("nil redis client", func(t *testing.T) {
		runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)
		ctx := context.Background()

		status := runner.checkRedisConnectionStatus(ctx)

		if status.IsConnected {
			t.Error("expected IsConnected to be false for nil redis")
		}

		if !strings.Contains(status.Error, "nil") {
			t.Errorf("expected error to mention 'nil', got: %s", status.Error)
		}

		if status.LastChecked.IsZero() {
			t.Error("expected LastChecked to be set")
		}
	})
}

// TestCaptureHTTPRequestResponse verifies HTTP request/response capture.
func TestCaptureHTTPRequestResponse(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)

	// Create test request
	reqBody := strings.NewReader(`{"username":"test","password":"secret"}`)
	req := httptest.NewRequest("POST", "http://localhost:9090/api/auth/login", reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token")

	// Create test response
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusOK)
	recorder.WriteString(`{"access_token":"xyz"}`)
	resp := recorder.Result()

	// Capture
	reqCapture, respCapture := runner.CaptureHTTPRequestResponse(req, resp)

	// Verify request capture
	if reqCapture == nil {
		t.Fatal("expected non-nil request capture")
	}

	if reqCapture.Method != "POST" {
		t.Errorf("expected method POST, got %s", reqCapture.Method)
	}

	if !strings.Contains(reqCapture.URL, "/api/auth/login") {
		t.Errorf("expected URL to contain /api/auth/login, got %s", reqCapture.URL)
	}

	if reqCapture.Headers["Content-Type"] != "application/json" {
		t.Errorf("expected Content-Type header, got %s", reqCapture.Headers["Content-Type"])
	}

	// Authorization header should be redacted
	if reqCapture.Headers["Authorization"] != "[REDACTED]" {
		t.Errorf("expected Authorization to be redacted, got %s", reqCapture.Headers["Authorization"])
	}

	if !strings.Contains(reqCapture.Body, "username") {
		t.Error("expected body to be captured")
	}

	// Verify response capture
	if respCapture == nil {
		t.Fatal("expected non-nil response capture")
	}

	if respCapture.StatusCode != http.StatusOK {
		t.Errorf("expected status code 200, got %d", respCapture.StatusCode)
	}

	if !strings.Contains(respCapture.Body, "access_token") {
		t.Error("expected body to contain access_token")
	}
}

// TestTruncateBody verifies body truncation works correctly.
func TestTruncateBody(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)

	t.Run("short body no truncation", func(t *testing.T) {
		body := "short body"
		truncated := runner.truncateBody(body, 100)

		if truncated != body {
			t.Error("short body should not be truncated")
		}
	})

	t.Run("long body truncated", func(t *testing.T) {
		body := strings.Repeat("a", 1000)
		truncated := runner.truncateBody(body, 100)

		if len(truncated) >= 1000 {
			t.Error("long body should be truncated")
		}

		if !strings.Contains(truncated, "TRUNCATED") {
			t.Error("truncated body should indicate truncation")
		}

		if !strings.Contains(truncated, "1000 bytes") {
			t.Error("truncated body should show original size")
		}
	})
}

// TestIsSensitiveHeader verifies sensitive header detection.
func TestIsSensitiveHeader(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)

	tests := []struct {
		header    string
		sensitive bool
	}{
		{"Authorization", true},
		{"authorization", true},
		{"Cookie", true},
		{"Set-Cookie", true},
		{"X-API-Key", true},
		{"X-Auth-Token", true},
		{"Content-Type", false},
		{"Accept", false},
		{"User-Agent", false},
	}

	for _, tt := range tests {
		result := runner.isSensitiveHeader(tt.header)
		if result != tt.sensitive {
			t.Errorf("isSensitiveHeader(%s) = %v, expected %v", tt.header, result, tt.sensitive)
		}
	}
}

// TestDoHTTPRequest verifies the convenience method for HTTP requests.
func TestDoHTTPRequest(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	runner := NewTestRunner(nil, nil, server.URL, nil)

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	resp, reqCapture, respCapture, err := runner.DoHTTPRequest(req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	defer resp.Body.Close()

	if reqCapture == nil {
		t.Fatal("expected non-nil request capture")
	}

	if respCapture == nil {
		t.Fatal("expected non-nil response capture")
	}

	if respCapture.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", respCapture.StatusCode)
	}

	// Body should still be readable
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "status") {
		t.Error("response body should still be readable")
	}
}

// TestLogStructured verifies structured logging format.
func TestLogStructured(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)

	// Just verify it doesn't panic
	runner.LogStructured("INFO", "test message", map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	})

	runner.LogStructured("ERROR", "error message", nil)
}

// TestLogTestFailure verifies test failure logging.
func TestLogTestFailure(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)

	failureLog := &FailureLog{
		ErrorMessage: "test failed",
		Timestamp:    time.Now(),
		Request: &HTTPRequestCapture{
			Method: "POST",
			URL:    "http://test/api/login",
		},
		Response: &HTTPResponseCapture{
			Status:     "500 Internal Server Error",
			StatusCode: 500,
		},
		Environment: &EnvironmentState{
			DBConnection: ConnectionStatus{
				IsConnected: false,
				Error:       "connection refused",
			},
			RedisConnection: ConnectionStatus{
				IsConnected: true,
			},
			GoroutineCount: 10,
		},
		StackTrace: "goroutine 1...",
	}

	// Just verify it doesn't panic
	runner.LogTestFailure("TestName", failureLog)
}

// TestGetters verifies accessor methods.
func TestGetters(t *testing.T) {
	mockDB := &sql.DB{}
	mockRedis := redis.NewClient(&redis.Options{})
	baseURL := "http://test:8080"

	runner := NewTestRunner(mockDB, mockRedis, baseURL, nil)

	if runner.GetDB() != mockDB {
		t.Error("GetDB returned unexpected value")
	}

	if runner.GetRedis() != mockRedis {
		t.Error("GetRedis returned unexpected value")
	}

	if runner.GetBaseURL() != baseURL {
		t.Errorf("GetBaseURL returned '%s', expected '%s'", runner.GetBaseURL(), baseURL)
	}

	if runner.GetHTTPClient() == nil {
		t.Error("GetHTTPClient returned nil")
	}

	if runner.GetHTTPClient().Timeout != 30*time.Second {
		t.Errorf("HTTP client timeout is %v, expected 30s", runner.GetHTTPClient().Timeout)
	}
}

// TestDefaultConfig verifies default configuration values.
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	tests := []struct {
		name     string
		got      bool
		expected bool
	}{
		{"ValidatePostgresTriggers", config.ValidatePostgresTriggers, true},
		{"ValidateRedisConnection", config.ValidateRedisConnection, true},
		{"UseDBTransactions", config.UseDBTransactions, true},
		{"RedisNamespaceMode", config.RedisNamespaceMode, true},
		{"CaptureStackTraces", config.CaptureStackTraces, true},
		{"LogEnvironmentVars", config.LogEnvironmentVars, false},
	}

	for _, tt := range tests {
		if tt.got != tt.expected {
			t.Errorf("%s: got %v, expected %v", tt.name, tt.got, tt.expected)
		}
	}
}

// TestFailureLogStructure verifies FailureLog contains all required fields.
func TestFailureLogStructure(t *testing.T) {
	log := &FailureLog{
		Request: &HTTPRequestCapture{
			Method: "POST",
			URL:    "http://test/api/login",
		},
		Response: &HTTPResponseCapture{
			Status:     "200 OK",
			StatusCode: 200,
		},
		Environment: &EnvironmentState{
			EnvVars: map[string]string{
				"KEY": "value",
			},
		},
		StackTrace:   "goroutine stack...",
		ErrorMessage: "test error",
		Timestamp:    time.Now(),
	}

	if log.ErrorMessage != "test error" {
		t.Errorf("ErrorMessage = '%s', expected 'test error'", log.ErrorMessage)
	}

	if log.Request.Method != "POST" {
		t.Error("Request not preserved")
	}

	if log.Response.StatusCode != 200 {
		t.Error("Response not preserved")
	}

	if log.Environment.EnvVars["KEY"] != "value" {
		t.Error("Environment map not preserved")
	}

	if log.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

// TestTestResultStructure verifies TestResult contains all required fields.
func TestTestResultStructure(t *testing.T) {
	result := TestResult{
		Name:     "TestName",
		Status:   TestStatusPass,
		Duration: 100 * time.Millisecond,
		FailureLog: &FailureLog{
			ErrorMessage: "error",
		},
		SkipReason: "skipped",
	}

	if result.Name != "TestName" {
		t.Errorf("Name = '%s', expected 'TestName'", result.Name)
	}

	if result.Status != TestStatusPass {
		t.Errorf("Status = '%s', expected 'PASS'", result.Status)
	}

	if result.Duration != 100*time.Millisecond {
		t.Errorf("Duration = %v, expected 100ms", result.Duration)
	}

	if result.FailureLog == nil {
		t.Error("FailureLog should not be nil")
	}

	if result.SkipReason != "skipped" {
		t.Errorf("SkipReason = '%s', expected 'skipped'", result.SkipReason)
	}
}

// ============================================================================
// Test Isolation and Cleanup Tests
// ============================================================================

// TestIsolateAndCleanup verifies test isolation and cleanup mechanisms.
func TestIsolateAndCleanup(t *testing.T) {
	t.Run("transaction isolation without transaction support", func(t *testing.T) {
		runner := NewTestRunner(nil, nil, "http://localhost:9090", &RunnerConfig{
			UseDBTransactions:  false,
			RedisNamespaceMode: false,
		})

		test := Test{Name: "TestNoIsolation"}
		ctx := context.Background()

		err := runner.IsolateTest(ctx, test)
		if err != nil {
			t.Errorf("IsolateTest should not error when isolation disabled: %v", err)
		}

		err = runner.CleanupTest(ctx, test)
		if err != nil {
			t.Errorf("CleanupTest should not error when isolation disabled: %v", err)
		}
	})

	t.Run("redis namespace generation", func(t *testing.T) {
		// Use miniredis to avoid depending on a local Redis server
		mr := miniredis.RunT(t)
		mockRedis := redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
		})
		defer mockRedis.Close()

		runner := NewTestRunner(nil, mockRedis, "http://localhost:9090", &RunnerConfig{
			UseDBTransactions:  false,
			RedisNamespaceMode: true,
		})

		test := Test{Name: "Test With Spaces"}
		ctx := context.Background()

		err := runner.IsolateTest(ctx, test)
		if err != nil {
			t.Errorf("IsolateTest should not error: %v", err)
		}

		// Verify namespace was set
		if runner.testNamespace == "" {
			t.Error("expected testNamespace to be set")
		}

		// Verify namespace contains sanitized test name
		if !strings.Contains(runner.testNamespace, "Test-With-Spaces") &&
			!strings.Contains(runner.testNamespace, "TestWithSpaces") {
			t.Errorf("expected namespace to contain sanitized test name, got: %s", runner.testNamespace)
		}

		// Verify namespace has timestamp component
		if !strings.Contains(runner.testNamespace, "test:") {
			t.Errorf("expected namespace to start with 'test:', got: %s", runner.testNamespace)
		}

		err = runner.CleanupTest(ctx, test)
		if err != nil {
			t.Errorf("CleanupTest should not error: %v", err)
		}

		// Verify namespace was cleared
		if runner.testNamespace != "" {
			t.Error("expected testNamespace to be cleared after cleanup")
		}
	})
}

// TestSanitizeTestName verifies test name sanitization for Redis keys.
func TestSanitizeTestName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "TestSimple",
			expected: "TestSimple",
		},
		{
			name:     "name with spaces",
			input:    "Test With Spaces",
			expected: "Test-With-Spaces",
		},
		{
			name:     "name with special characters",
			input:    "Test@#$%^&*()Name",
			expected: "Test---------Name",
		},
		{
			name:     "name with mixed case and numbers",
			input:    "Test123ABC",
			expected: "Test123ABC",
		},
		{
			name:     "name with unicode",
			input:    "Test中文Name",
			expected: "Test--Name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeTestName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeTestName(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGenerateTestID verifies that GenerateTestID produces identifiers
// compatible with the format used by TestRunner.IsolateTest.
func TestGenerateTestID(t *testing.T) {
	testID := GenerateTestID("TestMyExample")

	// Should start with "e2e_"
	if len(testID) <= 4 || testID[:4] != "e2e_" {
		t.Errorf("GenerateTestID should start with 'e2e_', got: %s", testID)
	}

	// Should contain sanitized test name
	if !strings.Contains(testID, "TestMyExample") {
		t.Errorf("GenerateTestID should contain sanitized test name, got: %s", testID)
	}

	// Format: e2e_<unix_nano>_<sanitized_name>
	parts := strings.SplitN(testID, "_", 3)
	if len(parts) != 3 {
		t.Errorf("GenerateTestID should have 3 parts separated by '_', got: %s", testID)
	} else {
		if parts[0] != "e2e" {
			t.Errorf("first part should be 'e2e', got: %s", parts[0])
		}
		if parts[2] != "TestMyExample" {
			t.Errorf("third part should be sanitized test name, got: %s", parts[2])
		}
	}

	// Two calls should produce valid IDs
	id1 := GenerateTestID("TestName")
	id2 := GenerateTestID("TestName")
	if !strings.HasPrefix(id1, "e2e_") {
		t.Errorf("id1 should start with 'e2e_', got: %s", id1)
	}
	if !strings.HasPrefix(id2, "e2e_") {
		t.Errorf("id2 should start with 'e2e_', got: %s", id2)
	}
}

// TestActiveTxLifecycle verifies transaction lifecycle management.
func TestActiveTxLifecycle(t *testing.T) {
	t.Run("transaction stored and cleared", func(t *testing.T) {
		// Create a mock database
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		runner := NewTestRunner(db, nil, "http://localhost:9090", &RunnerConfig{
			UseDBTransactions:  true,
			RedisNamespaceMode: false,
		})

		test := Test{Name: "TransactionTest"}
		ctx := context.Background()

		// Expect transaction begin
		mock.ExpectBegin()

		err = runner.IsolateTest(ctx, test)
		if err != nil {
			t.Fatalf("IsolateTest failed: %v", err)
		}

		// Verify transaction is stored
		if runner.activeTx == nil {
			t.Error("expected activeTx to be set after IsolateTest")
		}

		// Expect transaction rollback
		mock.ExpectRollback()

		// Expect pattern-based cleanup queries for each table
		// New flow: first collect user UUIDs, then delete audit logs by UUID, then remaining tables
		testIDPattern := "%" + runner.testID + "%"

		// Phase 1: collect user IDs matching the pattern
		userRows := sqlmock.NewRows([]string{"id"})
		mock.ExpectQuery(`SELECT id::text FROM users WHERE email LIKE \$1 OR id::text LIKE \$1`).
			WithArgs(testIDPattern).
			WillReturnRows(userRows)

		// Phase 2: audit logs skipped (no matching users)

		// Phase 3: remaining tables
		mock.ExpectExec("DELETE FROM verification_tokens").WithArgs(testIDPattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM reset_tokens").WithArgs(testIDPattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM authorization_codes").WithArgs(testIDPattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM tokens").WithArgs(testIDPattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM oauth_clients").WithArgs(testIDPattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM users").WithArgs(testIDPattern).WillReturnResult(sqlmock.NewResult(0, 0))

		err = runner.CleanupTest(ctx, test)
		if err != nil {
			t.Fatalf("CleanupTest failed: %v", err)
		}

		// Verify transaction is cleared
		if runner.activeTx != nil {
			t.Error("expected activeTx to be cleared after CleanupTest")
		}

		// Verify all expectations met
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sqlmock expectations: %v", err)
		}
	})
}

// ============================================================================
// Smart Skipping Tests
// ============================================================================

// TestShouldSkipTest_SMTPMissing verifies test is skipped when SMTP is required but not configured.
// Validates: Requirements 1.3
func TestShouldSkipTest_SMTPMissing(t *testing.T) {
	// Clear SMTP environment variables
	oldHost := os.Getenv("SMTP_HOST")
	oldUser := os.Getenv("SMTP_USER")
	oldPassword := os.Getenv("SMTP_PASSWORD")
	defer func() {
		os.Setenv("SMTP_HOST", oldHost)
		os.Setenv("SMTP_USER", oldUser)
		os.Setenv("SMTP_PASSWORD", oldPassword)
	}()

	os.Unsetenv("SMTP_HOST")
	os.Unsetenv("SMTP_USER")
	os.Unsetenv("SMTP_PASSWORD")

	config := &RunnerConfig{
		RequireSMTP:              true,
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
	}

	runner := NewTestRunner(nil, nil, "http://localhost:9090", config)
	ctx := context.Background()

	shouldSkip, reason := runner.ShouldSkipTest(ctx)

	if !shouldSkip {
		t.Error("expected test to be skipped when SMTP is required but not configured")
	}

	if reason == "" {
		t.Error("expected skip reason to be provided")
	}

	if !strings.Contains(reason, "SMTP") {
		t.Errorf("expected reason to mention SMTP, got: %s", reason)
	}

	if !strings.Contains(reason, "SMTP_HOST") {
		t.Errorf("expected reason to mention SMTP_HOST, got: %s", reason)
	}
}

// TestShouldSkipTest_SMTPConfigured verifies test is not skipped when SMTP is configured.
// Validates: Requirements 1.3
func TestShouldSkipTest_SMTPConfigured(t *testing.T) {
	// Set SMTP environment variables
	oldHost := os.Getenv("SMTP_HOST")
	oldUser := os.Getenv("SMTP_USER")
	oldPassword := os.Getenv("SMTP_PASSWORD")
	defer func() {
		os.Setenv("SMTP_HOST", oldHost)
		os.Setenv("SMTP_USER", oldUser)
		os.Setenv("SMTP_PASSWORD", oldPassword)
	}()

	os.Setenv("SMTP_HOST", "smtp.example.com")
	os.Setenv("SMTP_USER", "test@example.com")
	os.Setenv("SMTP_PASSWORD", "testpass")

	config := &RunnerConfig{
		RequireSMTP:              true,
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
	}

	runner := NewTestRunner(nil, nil, "http://localhost:9090", config)
	ctx := context.Background()

	shouldSkip, reason := runner.ShouldSkipTest(ctx)

	if shouldSkip {
		t.Errorf("expected test not to be skipped when SMTP is configured, got reason: %s", reason)
	}

	if reason != "" {
		t.Errorf("expected no skip reason, got: %s", reason)
	}
}

// TestShouldSkipTest_OAuthMissing verifies test is skipped when OAuth is required but not configured.
// Validates: Requirements 1.3
func TestShouldSkipTest_OAuthMissing(t *testing.T) {
	// Clear OAuth environment variables
	oldGoogleID := os.Getenv("OAUTH_GOOGLE_CLIENT_ID")
	oldGoogleSecret := os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET")
	oldGithubID := os.Getenv("OAUTH_GITHUB_CLIENT_ID")
	oldGithubSecret := os.Getenv("OAUTH_GITHUB_CLIENT_SECRET")
	defer func() {
		os.Setenv("OAUTH_GOOGLE_CLIENT_ID", oldGoogleID)
		os.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", oldGoogleSecret)
		os.Setenv("OAUTH_GITHUB_CLIENT_ID", oldGithubID)
		os.Setenv("OAUTH_GITHUB_CLIENT_SECRET", oldGithubSecret)
	}()

	os.Unsetenv("OAUTH_GOOGLE_CLIENT_ID")
	os.Unsetenv("OAUTH_GOOGLE_CLIENT_SECRET")
	os.Unsetenv("OAUTH_GITHUB_CLIENT_ID")
	os.Unsetenv("OAUTH_GITHUB_CLIENT_SECRET")

	config := &RunnerConfig{
		RequireOAuth:             true,
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
	}

	runner := NewTestRunner(nil, nil, "http://localhost:9090", config)
	ctx := context.Background()

	shouldSkip, reason := runner.ShouldSkipTest(ctx)

	if !shouldSkip {
		t.Error("expected test to be skipped when OAuth is required but not configured")
	}

	if reason == "" {
		t.Error("expected skip reason to be provided")
	}

	if !strings.Contains(reason, "OAuth") {
		t.Errorf("expected reason to mention OAuth, got: %s", reason)
	}

	if !strings.Contains(reason, "OAUTH_GOOGLE_CLIENT_ID") {
		t.Errorf("expected reason to mention OAUTH_GOOGLE_CLIENT_ID, got: %s", reason)
	}
}

// TestShouldSkipTest_OAuthConfiguredGoogle verifies test is not skipped when Google OAuth is configured.
// Validates: Requirements 1.3
func TestShouldSkipTest_OAuthConfiguredGoogle(t *testing.T) {
	// Set Google OAuth environment variables
	oldGoogleID := os.Getenv("OAUTH_GOOGLE_CLIENT_ID")
	oldGoogleSecret := os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET")
	defer func() {
		os.Setenv("OAUTH_GOOGLE_CLIENT_ID", oldGoogleID)
		os.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", oldGoogleSecret)
	}()

	os.Setenv("OAUTH_GOOGLE_CLIENT_ID", "test-google-id")
	os.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", "test-google-secret")

	config := &RunnerConfig{
		RequireOAuth:             true,
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
	}

	runner := NewTestRunner(nil, nil, "http://localhost:9090", config)
	ctx := context.Background()

	shouldSkip, reason := runner.ShouldSkipTest(ctx)

	if shouldSkip {
		t.Errorf("expected test not to be skipped when Google OAuth is configured, got reason: %s", reason)
	}

	if reason != "" {
		t.Errorf("expected no skip reason, got: %s", reason)
	}
}

// TestShouldSkipTest_OAuthConfiguredGithub verifies test is not skipped when GitHub OAuth is configured.
// Validates: Requirements 1.3
func TestShouldSkipTest_OAuthConfiguredGithub(t *testing.T) {
	// Set GitHub OAuth environment variables
	oldGithubID := os.Getenv("OAUTH_GITHUB_CLIENT_ID")
	oldGithubSecret := os.Getenv("OAUTH_GITHUB_CLIENT_SECRET")
	defer func() {
		os.Setenv("OAUTH_GITHUB_CLIENT_ID", oldGithubID)
		os.Setenv("OAUTH_GITHUB_CLIENT_SECRET", oldGithubSecret)
	}()

	os.Setenv("OAUTH_GITHUB_CLIENT_ID", "test-github-id")
	os.Setenv("OAUTH_GITHUB_CLIENT_SECRET", "test-github-secret")

	config := &RunnerConfig{
		RequireOAuth:             true,
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
	}

	runner := NewTestRunner(nil, nil, "http://localhost:9090", config)
	ctx := context.Background()

	shouldSkip, reason := runner.ShouldSkipTest(ctx)

	if shouldSkip {
		t.Errorf("expected test not to be skipped when GitHub OAuth is configured, got reason: %s", reason)
	}

	if reason != "" {
		t.Errorf("expected no skip reason, got: %s", reason)
	}
}

// TestShouldSkipTest_BothSMTPAndOAuthRequired verifies multiple dependency checks work together.
// Validates: Requirements 1.3
func TestShouldSkipTest_BothSMTPAndOAuthRequired(t *testing.T) {
	// Clear all environment variables
	oldHost := os.Getenv("SMTP_HOST")
	oldUser := os.Getenv("SMTP_USER")
	oldPassword := os.Getenv("SMTP_PASSWORD")
	oldGoogleID := os.Getenv("OAUTH_GOOGLE_CLIENT_ID")
	oldGoogleSecret := os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET")
	defer func() {
		os.Setenv("SMTP_HOST", oldHost)
		os.Setenv("SMTP_USER", oldUser)
		os.Setenv("SMTP_PASSWORD", oldPassword)
		os.Setenv("OAUTH_GOOGLE_CLIENT_ID", oldGoogleID)
		os.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", oldGoogleSecret)
	}()

	os.Unsetenv("SMTP_HOST")
	os.Unsetenv("SMTP_USER")
	os.Unsetenv("SMTP_PASSWORD")
	os.Unsetenv("OAUTH_GOOGLE_CLIENT_ID")
	os.Unsetenv("OAUTH_GOOGLE_CLIENT_SECRET")

	config := &RunnerConfig{
		RequireSMTP:              true,
		RequireOAuth:             true,
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
	}

	runner := NewTestRunner(nil, nil, "http://localhost:9090", config)
	ctx := context.Background()

	shouldSkip, reason := runner.ShouldSkipTest(ctx)

	if !shouldSkip {
		t.Error("expected test to be skipped when both SMTP and OAuth are required but not configured")
	}

	// Should fail on first check (SMTP)
	if !strings.Contains(reason, "SMTP") {
		t.Errorf("expected reason to mention SMTP, got: %s", reason)
	}
}

// TestShouldSkipTest_NoRequirements verifies test is not skipped when no dependencies are required.
// Validates: Requirements 1.3
func TestShouldSkipTest_NoRequirements(t *testing.T) {
	config := &RunnerConfig{
		RequireSMTP:              false,
		RequireOAuth:             false,
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
	}

	runner := NewTestRunner(nil, nil, "http://localhost:9090", config)
	ctx := context.Background()

	shouldSkip, reason := runner.ShouldSkipTest(ctx)

	if shouldSkip {
		t.Errorf("expected test not to be skipped when no dependencies are required, got reason: %s", reason)
	}

	if reason != "" {
		t.Errorf("expected no skip reason, got: %s", reason)
	}
}

// TestRunSingleTest_SkipsDueToSMTPMissing verifies that a test is skipped when SMTP is required but missing.
// Validates: Requirements 1.3
func TestRunSingleTest_SkipsDueToSMTPMissing(t *testing.T) {
	// Clear SMTP environment variables
	oldHost := os.Getenv("SMTP_HOST")
	oldUser := os.Getenv("SMTP_USER")
	oldPassword := os.Getenv("SMTP_PASSWORD")
	defer func() {
		os.Setenv("SMTP_HOST", oldHost)
		os.Setenv("SMTP_USER", oldUser)
		os.Setenv("SMTP_PASSWORD", oldPassword)
	}()

	os.Unsetenv("SMTP_HOST")
	os.Unsetenv("SMTP_USER")
	os.Unsetenv("SMTP_PASSWORD")

	config := &RunnerConfig{
		RequireSMTP:              true,
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
		UseDBTransactions:        false,
		RedisNamespaceMode:       false,
		CaptureStackTraces:       false,
	}

	runner := NewTestRunner(nil, nil, "http://localhost:9090", config)
	ctx := context.Background()

	testExecuted := false
	test := Test{
		Name: "SMTP Dependent Test",
		Run: func(ctx context.Context, tr *TestRunner) error {
			testExecuted = true
			return nil
		},
	}

	result := runner.runSingleTest(ctx, test)

	if result.Status != TestStatusSkip {
		t.Errorf("expected test to be skipped, got status: %s", result.Status)
	}

	if result.SkipReason == "" {
		t.Error("expected skip reason to be set")
	}

	if !strings.Contains(result.SkipReason, "SMTP") {
		t.Errorf("expected skip reason to mention SMTP, got: %s", result.SkipReason)
	}

	if testExecuted {
		t.Error("expected test not to be executed when skipped")
	}
}

// TestCheckSMTPAvailable_PartialConfig verifies skip when only some SMTP vars are set.
// Validates: Requirements 1.3
func TestCheckSMTPAvailable_PartialConfig(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		user     string
		password string
		wantSkip bool
	}{
		{
			name:     "All configured",
			host:     "smtp.example.com",
			user:     "test@example.com",
			password: "testpass",
			wantSkip: false,
		},
		{
			name:     "Missing host",
			host:     "",
			user:     "test@example.com",
			password: "testpass",
			wantSkip: true,
		},
		{
			name:     "Missing user",
			host:     "smtp.example.com",
			user:     "",
			password: "testpass",
			wantSkip: true,
		},
		{
			name:     "Missing password",
			host:     "smtp.example.com",
			user:     "test@example.com",
			password: "",
			wantSkip: true,
		},
		{
			name:     "All missing",
			host:     "",
			user:     "",
			password: "",
			wantSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save old values
			oldHost := os.Getenv("SMTP_HOST")
			oldUser := os.Getenv("SMTP_USER")
			oldPassword := os.Getenv("SMTP_PASSWORD")
			defer func() {
				os.Setenv("SMTP_HOST", oldHost)
				os.Setenv("SMTP_USER", oldUser)
				os.Setenv("SMTP_PASSWORD", oldPassword)
			}()

			// Set test values
			if tt.host != "" {
				os.Setenv("SMTP_HOST", tt.host)
			} else {
				os.Unsetenv("SMTP_HOST")
			}
			if tt.user != "" {
				os.Setenv("SMTP_USER", tt.user)
			} else {
				os.Unsetenv("SMTP_USER")
			}
			if tt.password != "" {
				os.Setenv("SMTP_PASSWORD", tt.password)
			} else {
				os.Unsetenv("SMTP_PASSWORD")
			}

			runner := NewTestRunner(nil, nil, "http://localhost:9090", nil)
			shouldSkip, reason := runner.checkSMTPAvailable()

			if shouldSkip != tt.wantSkip {
				t.Errorf("checkSMTPAvailable() shouldSkip = %v, want %v (reason: %s)", shouldSkip, tt.wantSkip, reason)
			}

			if tt.wantSkip && reason == "" {
				t.Error("expected skip reason when SMTP should be skipped")
			}
		})
	}
}

// ============================================================================
// GetTestID Tests
// ============================================================================

// TestGetTestID_ReturnsEmptyBeforeIsolation verifies GetTestID returns empty before IsolateTest.
func TestGetTestID_ReturnsEmptyBeforeIsolation(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", &RunnerConfig{
		UseDBTransactions:  false,
		RedisNamespaceMode: false,
	})

	if id := runner.GetTestID(); id != "" {
		t.Errorf("expected empty testID before isolation, got '%s'", id)
	}
}

// TestGetTestID_ReturnsNonEmptyAfterIsolation verifies GetTestID returns a value after IsolateTest.
func TestGetTestID_ReturnsNonEmptyAfterIsolation(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", &RunnerConfig{
		UseDBTransactions:  false,
		RedisNamespaceMode: false,
	})

	test := Test{Name: "GetTestIDTest"}
	ctx := context.Background()

	err := runner.IsolateTest(ctx, test)
	if err != nil {
		t.Fatalf("IsolateTest failed: %v", err)
	}

	testID := runner.GetTestID()
	if testID == "" {
		t.Error("expected non-empty testID after IsolateTest")
	}

	// Verify testID contains sanitized test name
	if !strings.Contains(testID, "GetTestIDTest") {
		t.Errorf("expected testID to contain test name, got '%s'", testID)
	}

	// Verify testID starts with e2e_ prefix
	if !strings.HasPrefix(testID, "e2e_") {
		t.Errorf("expected testID to start with 'e2e_', got '%s'", testID)
	}

	// Cleanup
	runner.CleanupTest(ctx, test)
}

// TestGetTestID_ReturnsConsistentValue verifies GetTestID returns the same value on repeated calls.
func TestGetTestID_ReturnsConsistentValue(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", &RunnerConfig{
		UseDBTransactions:  false,
		RedisNamespaceMode: false,
	})

	test := Test{Name: "ConsistentIDTest"}
	ctx := context.Background()

	err := runner.IsolateTest(ctx, test)
	if err != nil {
		t.Fatalf("IsolateTest failed: %v", err)
	}

	id1 := runner.GetTestID()
	id2 := runner.GetTestID()

	if id1 != id2 {
		t.Errorf("expected consistent testID, got '%s' then '%s'", id1, id2)
	}

	// Cleanup
	runner.CleanupTest(ctx, test)
}

// TestGetTestID_ClearedAfterCleanup verifies GetTestID returns empty after CleanupTest.
func TestGetTestID_ClearedAfterCleanup(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", &RunnerConfig{
		UseDBTransactions:  false,
		RedisNamespaceMode: false,
	})

	test := Test{Name: "CleanupIDTest"}
	ctx := context.Background()

	err := runner.IsolateTest(ctx, test)
	if err != nil {
		t.Fatalf("IsolateTest failed: %v", err)
	}

	if id := runner.GetTestID(); id == "" {
		t.Fatal("expected non-empty testID after IsolateTest")
	}

	err = runner.CleanupTest(ctx, test)
	if err != nil {
		t.Fatalf("CleanupTest failed: %v", err)
	}

	if id := runner.GetTestID(); id != "" {
		t.Errorf("expected empty testID after CleanupTest, got '%s'", id)
	}
}

// TestGetTestID_UniquePerTest verifies different tests get different testIDs.
func TestGetTestID_UniquePerTest(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", &RunnerConfig{
		UseDBTransactions:  false,
		RedisNamespaceMode: false,
	})

	ctx := context.Background()

	test1 := Test{Name: "Test1"}
	runner.IsolateTest(ctx, test1)
	id1 := runner.GetTestID()
	runner.CleanupTest(ctx, test1)

	test2 := Test{Name: "Test2"}
	runner.IsolateTest(ctx, test2)
	id2 := runner.GetTestID()
	runner.CleanupTest(ctx, test2)

	if id1 == id2 {
		t.Errorf("expected unique testIDs for different tests, both got '%s'", id1)
	}
}

// TestGetTestID_UsableInTestData verifies testID can be embedded in test data patterns.
func TestGetTestID_UsableInTestData(t *testing.T) {
	runner := NewTestRunner(nil, nil, "http://localhost:9090", &RunnerConfig{
		UseDBTransactions:  false,
		RedisNamespaceMode: false,
	})

	test := Test{Name: "UsableIDTest"}
	ctx := context.Background()

	err := runner.IsolateTest(ctx, test)
	if err != nil {
		t.Fatalf("IsolateTest failed: %v", err)
	}

	testID := runner.GetTestID()

	// Verify testID can be used to construct email-like test data
	email := fmt.Sprintf("user-%s@example.com", testID)
	if !strings.Contains(email, testID) {
		t.Errorf("expected email to contain testID, got '%s'", email)
	}

	// Verify testID can be used in client_id-like test data
	clientID := fmt.Sprintf("client-%s", testID)
	if !strings.Contains(clientID, testID) {
		t.Errorf("expected clientID to contain testID, got '%s'", clientID)
	}

	// Cleanup
	runner.CleanupTest(ctx, test)
}
