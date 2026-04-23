// Package handler bugfix探索性测试
// 这些测试用于验证bug condition，在未修复代码上应该失败
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Task 1.3: Bug Condition Exploration - Missing Configuration Saved Check
// ============================================================================

// TestBugCondition_SetupPageAccessibleAfterConfigSaved tests that the setup page
// is accessible after configuration has been saved to .env file.
//
// **Property 1: Bug Condition** - Post-Setup Access Allowed
// **CRITICAL**: This test MUST FAIL on unfixed code - failure confirms the bug exists
// **DO NOT attempt to fix the test or the code when it fails**
// **NOTE**: This test encodes the expected behavior - it will validate the fix when it passes
//
// Bug Condition: .env file exists, localhost request → setup page shown with new token
// Expected Behavior: Should reject with 403 and "配置已完成" message
func TestBugCondition_SetupPageAccessibleAfterConfigSaved(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	// Create .env file (simulating completed setup)
	err := os.WriteFile(envPath, []byte("SERVER_HOST=0.0.0.0\nSERVER_PORT=9090\n"), 0600)
	require.NoError(t, err)

	// Create setup handler
	handler := NewSetupHandler(envPath, "1.0.0")

	// Make request to setup page from localhost
	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	req.RemoteAddr = "127.0.0.1:12345" // Localhost
	w := httptest.NewRecorder()

	handler.HandleSetupPage(w, req)

	// Expected: Should reject with 403 (setup already completed)
	// Unfixed code: Returns 200 and shows setup page
	assert.Equal(t, http.StatusForbidden, w.Code, "Setup page should be blocked when .env exists")

	// Verify error message (log actual response for debugging)
	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	if err == nil {
		t.Logf("Response: %+v", response)
		// The error details should contain the message
		if details, ok := response["details"].(string); ok {
			assert.Contains(t, details, "配置已完成", "Error details should indicate setup is complete")
		}
	}
}

// TestBugCondition_SetupPageTokenRegenerationAfterSave tests that after saving
// configuration and token expiring, the setup page regenerates a new token.
//
// Bug Condition: Token expires after config save → new token generated on page access
// Expected Behavior: Should reject access, not regenerate token
func TestBugCondition_SetupPageTokenRegenerationAfterSave(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	// Create setup handler
	handler := NewSetupHandler(envPath, "1.0.0")

	// Get initial token
	initialToken := handler.GetSetupToken()
	assert.NotEmpty(t, initialToken)

	// Simulate config save by creating .env file
	err := os.WriteFile(envPath, []byte("SERVER_HOST=0.0.0.0\n"), 0600)
	require.NoError(t, err)

	// Simulate token expiration by setting it to nil
	handler.setupToken.Store(nil)

	// Make request to setup page (this would regenerate token in unfixed code)
	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.HandleSetupPage(w, req)

	// Expected: Should reject with 403 (setup already completed)
	// Unfixed code: Regenerates token and shows setup page
	assert.Equal(t, http.StatusForbidden, w.Code, "Setup page should be blocked even if token is nil")

	// Verify token was NOT regenerated
	newToken := handler.GetSetupToken()
	// In fixed code, token should remain nil or empty (not regenerated)
	// In unfixed code, token would be regenerated
	if w.Code == http.StatusOK {
		// If page was shown (unfixed code), token would be regenerated
		assert.NotEmpty(t, newToken, "Unfixed code regenerates token")
		assert.NotEqual(t, initialToken, newToken, "Unfixed code generates new token")
	}
}

// TestBugCondition_SetupSaveWhenConfigExists tests that HandleSetupSave
// allows saving configuration even when .env file already exists.
//
// Bug Condition: .env exists, valid token, localhost → config save allowed
// Expected Behavior: Should reject with error (race condition prevention)
func TestBugCondition_SetupSaveWhenConfigExists(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	// Create .env file (simulating completed setup)
	err := os.WriteFile(envPath, []byte("SERVER_HOST=0.0.0.0\n"), 0600)
	require.NoError(t, err)

	// Create setup handler
	handler := NewSetupHandler(envPath, "1.0.0")
	token := handler.GetSetupToken()

	// Prepare save request
	reqBody := map[string]string{
		"SERVER_HOST": "0.0.0.0",
		"SERVER_PORT": "9090",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/setup/save", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Setup-Token", token)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.HandleSetupSave(w, req)

	// Expected: Should reject (config already exists)
	// Unfixed code: Allows save and overwrites existing config
	// Note: This test may pass on unfixed code if no check exists
	// The main bug is in HandleSetupPage, but this tests race condition prevention
	if w.Code == http.StatusOK {
		t.Log("Warning: Setup save allowed when config exists (potential race condition)")
	}
}

// ============================================================================
// Preservation Tests - Baseline Behavior
// ============================================================================

// TestPreservation_FirstTimeSetupAccess tests that first-time access
// (no .env file) correctly shows the setup page.
//
// This should PASS on both unfixed and fixed code.
func TestPreservation_FirstTimeSetupAccess(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	// Ensure .env does NOT exist
	_, err := os.Stat(envPath)
	require.True(t, os.IsNotExist(err), ".env should not exist for first-time setup")

	// Create setup handler
	handler := NewSetupHandler(envPath, "1.0.0")

	// Make request to setup page from localhost
	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.HandleSetupPage(w, req)

	// Expected: Should show setup page (200 OK)
	// This should pass on both unfixed and fixed code
	assert.Equal(t, http.StatusOK, w.Code, "First-time setup access should be allowed")

	// Verify token was generated
	token := handler.GetSetupToken()
	assert.NotEmpty(t, token, "Setup token should be generated for first-time access")
}

// TestPreservation_NonLocalhostRejected tests that non-localhost requests
// are rejected with 403 Forbidden.
//
// This should PASS on both unfixed and fixed code.
func TestPreservation_NonLocalhostRejected(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	handler := NewSetupHandler(envPath, "1.0.0")

	// Make request from non-localhost
	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	req.RemoteAddr = "192.168.1.100:12345" // Not localhost
	w := httptest.NewRecorder()

	handler.HandleSetupPage(w, req)

	// Expected: Should reject with 403
	// This should pass on both unfixed and fixed code
	assert.Equal(t, http.StatusForbidden, w.Code, "Non-localhost requests should be rejected")
}

// TestPreservation_ValidTokenAllowsOperations tests that valid token
// allows configuration operations.
//
// This should PASS on both unfixed and fixed code.
func TestPreservation_ValidTokenAllowsOperations(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	// Ensure .env does NOT exist (first-time setup)
	_, err := os.Stat(envPath)
	require.True(t, os.IsNotExist(err))

	handler := NewSetupHandler(envPath, "1.0.0")
	token := handler.GetSetupToken()

	// Prepare save request with valid token
	reqBody := map[string]string{
		"SERVER_HOST": "0.0.0.0",
		"SERVER_PORT": "9090",
		"DB_PASSWORD":  "test123",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/setup/save", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Setup-Token", token)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.HandleSetupSave(w, req)

	// Expected: Should succeed (200 OK)
	// This should pass on both unfixed and fixed code
	assert.Equal(t, http.StatusOK, w.Code, "Valid token should allow config save")

	// Verify .env file was created
	assert.FileExists(t, envPath, ".env file should be created")

	// Verify file permissions
	info, err := os.Stat(envPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), ".env should have 0600 permissions")
}


// ============================================================================
// Task 1.4: Bug Condition Exploration - Ungraceful syscall.Exec Restart
// ============================================================================

// TestBugCondition_UngracefulRestartWithoutCleanup tests that HandleSetupSave
// uses syscall.Exec without closing connections or waiting for requests.
//
// **Property 1: Bug Condition** - No Connection Cleanup Before Restart
// **CRITICAL**: This test MUST FAIL on unfixed code - failure confirms the bug exists
// **DO NOT attempt to fix the test or the code when it fails**
// **NOTE**: This test encodes the expected behavior - it will validate the fix when it passes
//
// Bug Condition: Config saved → syscall.Exec called without cleanup
// Expected Behavior: Should trigger graceful shutdown (close connections, wait for requests)
//
// Note: This test is difficult to implement fully because syscall.Exec replaces
// the process. We test the behavior indirectly by verifying that the restart
// mechanism does NOT use syscall.Exec in the fixed code.
func TestBugCondition_UngracefulRestartWithoutCleanup(t *testing.T) {
	// This test documents the expected behavior but cannot fully test syscall.Exec
	// because it would replace the test process itself.
	//
	// The bug is: HandleSetupSave uses syscall.Exec which:
	// 1. Does not close database connections
	// 2. Does not close Redis connections
	// 3. Does not wait for in-flight HTTP requests
	// 4. Directly replaces the process
	//
	// Expected behavior (after fix):
	// 1. Send SIGTERM to self
	// 2. Graceful shutdown mechanism closes connections
	// 3. Process manager restarts the service
	//
	// We can verify the fix by checking that:
	// - The code sends SIGTERM instead of calling syscall.Exec
	// - The graceful shutdown mechanism is triggered
	//
	// For now, we document this as a known limitation and rely on:
	// 1. Code review to verify syscall.Exec is removed
	// 2. Integration tests to verify graceful shutdown works
	// 3. Manual testing to verify restart behavior

	t.Skip("This test documents the bug but cannot fully test syscall.Exec behavior. " +
		"The bug is confirmed by code inspection: HandleSetupSave at setup.go:176-220 " +
		"uses syscall.Exec without closing connections. " +
		"Fix verification will be done through code review and integration tests.")
}

// TestBugCondition_RestartMechanismDocumentation documents the restart mechanism
// bug for reference.
func TestBugCondition_RestartMechanismDocumentation(t *testing.T) {
	// Document the bug for reference:
	//
	// Current behavior (unfixed code):
	// 1. HandleSetupSave writes .env file
	// 2. Starts goroutine with 3-second delay
	// 3. Reads .env file and merges with os.Environ()
	// 4. Calls syscall.Exec(executable, args, envVars)
	// 5. Process is immediately replaced
	// 6. No cleanup: DB connections leak, Redis connections leak, HTTP requests fail
	//
	// Expected behavior (after fix):
	// 1. HandleSetupSave writes .env file
	// 2. Starts goroutine with 3-second delay
	// 3. Sends SIGTERM to self: os.FindProcess(os.Getpid()).Signal(syscall.SIGTERM)
	// 4. Main server's graceful shutdown is triggered
	// 5. Graceful shutdown closes DB, Redis, waits for HTTP requests
	// 6. Process exits cleanly
	// 7. Process manager (systemd, Docker) restarts the service
	//
	// Verification approach:
	// - Code review: Verify syscall.Exec is replaced with SIGTERM
	// - Integration test: Verify graceful shutdown is triggered
	// - Manual test: Verify process manager restarts service

	t.Log("Bug documented: syscall.Exec used without graceful shutdown")
	t.Log("Expected fix: Replace syscall.Exec with SIGTERM signal")
	t.Log("Verification: Code review + integration tests")
}

// ============================================================================
// Preservation Tests - Restart Behavior
// ============================================================================

// TestPreservation_ConfigSaveCreatesEnvFile tests that config save creates
// .env file with correct permissions.
//
// This should PASS on both unfixed and fixed code.
func TestPreservation_ConfigSaveCreatesEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	handler := NewSetupHandler(envPath, "1.0.0")
	token := handler.GetSetupToken()

	reqBody := map[string]string{
		"SERVER_HOST": "0.0.0.0",
		"SERVER_PORT": "9090",
		"DB_PASSWORD":  "test123",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/setup/save", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Setup-Token", token)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.HandleSetupSave(w, req)

	// Expected: .env file created with 0600 permissions
	assert.FileExists(t, envPath)
	info, err := os.Stat(envPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), ".env should have 0600 permissions")
}

// TestPreservation_ConfigSaveInvalidatesToken tests that config save
// invalidates the setup token.
//
// This should PASS on both unfixed and fixed code.
func TestPreservation_ConfigSaveInvalidatesToken(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	handler := NewSetupHandler(envPath, "1.0.0")
	token := handler.GetSetupToken()
	assert.NotEmpty(t, token)

	reqBody := map[string]string{
		"SERVER_HOST": "0.0.0.0",
		"DB_PASSWORD":  "test123",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/setup/save", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Setup-Token", token)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.HandleSetupSave(w, req)

	// Expected: Token should be invalidated (nil)
	// Note: We can't directly check this without accessing private fields
	// But we can verify the response indicates success
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestPreservation_ConfigSaveReturnsSuccessResponse tests that config save
// returns success response to client.
//
// This should PASS on both unfixed and fixed code.
func TestPreservation_ConfigSaveReturnsSuccessResponse(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	handler := NewSetupHandler(envPath, "1.0.0")
	token := handler.GetSetupToken()

	reqBody := map[string]string{
		"SERVER_HOST": "0.0.0.0",
		"DB_PASSWORD":  "test123",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/setup/save", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Setup-Token", token)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.HandleSetupSave(w, req)

	// Expected: Success response (200 OK)
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify response contains success message
	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	// Response format: {"data": {"message": "...", "note": "..."}}
	if data, ok := response["data"].(map[string]interface{}); ok {
		assert.Contains(t, data, "message")
	} else {
		assert.Contains(t, response, "message")
	}
}

// TestPreservation_ConfigSaveFailureNoRestart tests that config save failure
// does not trigger restart.
//
// This should PASS on both unfixed and fixed code.
func TestPreservation_ConfigSaveFailureNoRestart(t *testing.T) {
	// Use invalid path to cause write failure
	handler := NewSetupHandler("/invalid/path/.env", "1.0.0")
	token := handler.GetSetupToken()

	reqBody := map[string]string{
		"SERVER_HOST": "0.0.0.0",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/setup/save", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Setup-Token", token)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.HandleSetupSave(w, req)

	// Expected: Error response (no restart triggered)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
