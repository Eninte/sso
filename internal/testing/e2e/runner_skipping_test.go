package e2e

import (
	"context"
	"os"
	"testing"
)

// TestShouldSkipTest_SMTP tests smart test skipping for SMTP requirements
func TestShouldSkipTest_SMTP(t *testing.T) {
	tests := []struct {
		name         string
		smtpHost     string
		smtpUser     string
		smtpPassword string
		requireSMTP  bool
		expectSkip   bool
	}{
		{
			name:         "all SMTP credentials present - should not skip",
			smtpHost:     "smtp.example.com",
			smtpUser:     "user@example.com",
			smtpPassword: "password",
			requireSMTP:  true,
			expectSkip:   false,
		},
		{
			name:         "missing SMTP_HOST - should skip",
			smtpHost:     "",
			smtpUser:     "user@example.com",
			smtpPassword: "password",
			requireSMTP:  true,
			expectSkip:   true,
		},
		{
			name:         "missing SMTP_USER - should skip",
			smtpHost:     "smtp.example.com",
			smtpUser:     "",
			smtpPassword: "password",
			requireSMTP:  true,
			expectSkip:   true,
		},
		{
			name:         "missing SMTP_PASSWORD - should skip",
			smtpHost:     "smtp.example.com",
			smtpUser:     "user@example.com",
			smtpPassword: "",
			requireSMTP:  true,
			expectSkip:   true,
		},
		{
			name:         "SMTP not required - should not skip even without credentials",
			smtpHost:     "",
			smtpUser:     "",
			smtpPassword: "",
			requireSMTP:  false,
			expectSkip:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Backup and restore environment
			originalHost := os.Getenv("SMTP_HOST")
			originalUser := os.Getenv("SMTP_USER")
			originalPassword := os.Getenv("SMTP_PASSWORD")
			defer func() {
				os.Setenv("SMTP_HOST", originalHost)
				os.Setenv("SMTP_USER", originalUser)
				os.Setenv("SMTP_PASSWORD", originalPassword)
			}()

			// Set test environment
			os.Setenv("SMTP_HOST", tt.smtpHost)
			os.Setenv("SMTP_USER", tt.smtpUser)
			os.Setenv("SMTP_PASSWORD", tt.smtpPassword)

			// Create test runner
			config := &RunnerConfig{
				RequireSMTP: tt.requireSMTP,
			}
			tr := &TestRunner{
				config: config,
			}

			// Execute
			ctx := context.Background()
			shouldSkip, reason := tr.ShouldSkipTest(ctx)

			// Verify
			if shouldSkip != tt.expectSkip {
				t.Errorf("ShouldSkipTest() shouldSkip = %v, want %v", shouldSkip, tt.expectSkip)
			}

			if tt.expectSkip && reason == "" {
				t.Error("Expected skip reason but got empty string")
			}

			if !tt.expectSkip && reason != "" {
				t.Errorf("Expected no skip reason but got: %s", reason)
			}

			// Verify skip reason contains remediation instructions
			if tt.expectSkip {
				if !containsString(reason, "SMTP_HOST") {
					t.Error("Skip reason should mention SMTP_HOST")
				}
				if !containsString(reason, "SMTP_USER") {
					t.Error("Skip reason should mention SMTP_USER")
				}
				if !containsString(reason, "SMTP_PASSWORD") {
					t.Error("Skip reason should mention SMTP_PASSWORD")
				}
				if !containsString(reason, "Example:") {
					t.Error("Skip reason should contain example command")
				}
			}
		})
	}
}

// TestShouldSkipTest_OAuth tests smart test skipping for OAuth requirements
func TestShouldSkipTest_OAuth(t *testing.T) {
	tests := []struct {
		name               string
		googleClientID     string
		googleClientSecret string
		githubClientID     string
		githubClientSecret string
		requireOAuth       bool
		expectSkip         bool
	}{
		{
			name:               "Google OAuth configured - should not skip",
			googleClientID:     "google-client-id",
			googleClientSecret: "google-client-secret",
			githubClientID:     "",
			githubClientSecret: "",
			requireOAuth:       true,
			expectSkip:         false,
		},
		{
			name:               "GitHub OAuth configured - should not skip",
			googleClientID:     "",
			googleClientSecret: "",
			githubClientID:     "github-client-id",
			githubClientSecret: "github-client-secret",
			requireOAuth:       true,
			expectSkip:         false,
		},
		{
			name:               "Both OAuth providers configured - should not skip",
			googleClientID:     "google-client-id",
			googleClientSecret: "google-client-secret",
			githubClientID:     "github-client-id",
			githubClientSecret: "github-client-secret",
			requireOAuth:       true,
			expectSkip:         false,
		},
		{
			name:               "No OAuth providers configured - should skip",
			googleClientID:     "",
			googleClientSecret: "",
			githubClientID:     "",
			githubClientSecret: "",
			requireOAuth:       true,
			expectSkip:         true,
		},
		{
			name:               "Partial Google OAuth (missing secret) - should skip",
			googleClientID:     "google-client-id",
			googleClientSecret: "",
			githubClientID:     "",
			githubClientSecret: "",
			requireOAuth:       true,
			expectSkip:         true,
		},
		{
			name:               "Partial GitHub OAuth (missing secret) - should skip",
			googleClientID:     "",
			googleClientSecret: "",
			githubClientID:     "github-client-id",
			githubClientSecret: "",
			requireOAuth:       true,
			expectSkip:         true,
		},
		{
			name:               "OAuth not required - should not skip even without credentials",
			googleClientID:     "",
			googleClientSecret: "",
			githubClientID:     "",
			githubClientSecret: "",
			requireOAuth:       false,
			expectSkip:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Backup and restore environment
			originalGoogleID := os.Getenv("OAUTH_GOOGLE_CLIENT_ID")
			originalGoogleSecret := os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET")
			originalGithubID := os.Getenv("OAUTH_GITHUB_CLIENT_ID")
			originalGithubSecret := os.Getenv("OAUTH_GITHUB_CLIENT_SECRET")
			defer func() {
				os.Setenv("OAUTH_GOOGLE_CLIENT_ID", originalGoogleID)
				os.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", originalGoogleSecret)
				os.Setenv("OAUTH_GITHUB_CLIENT_ID", originalGithubID)
				os.Setenv("OAUTH_GITHUB_CLIENT_SECRET", originalGithubSecret)
			}()

			// Set test environment
			os.Setenv("OAUTH_GOOGLE_CLIENT_ID", tt.googleClientID)
			os.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", tt.googleClientSecret)
			os.Setenv("OAUTH_GITHUB_CLIENT_ID", tt.githubClientID)
			os.Setenv("OAUTH_GITHUB_CLIENT_SECRET", tt.githubClientSecret)

			// Create test runner
			config := &RunnerConfig{
				RequireOAuth: tt.requireOAuth,
			}
			tr := &TestRunner{
				config: config,
			}

			// Execute
			ctx := context.Background()
			shouldSkip, reason := tr.ShouldSkipTest(ctx)

			// Verify
			if shouldSkip != tt.expectSkip {
				t.Errorf("ShouldSkipTest() shouldSkip = %v, want %v", shouldSkip, tt.expectSkip)
			}

			if tt.expectSkip && reason == "" {
				t.Error("Expected skip reason but got empty string")
			}

			if !tt.expectSkip && reason != "" {
				t.Errorf("Expected no skip reason but got: %s", reason)
			}

			// Verify skip reason contains remediation instructions
			if tt.expectSkip {
				if !containsString(reason, "OAUTH_GOOGLE_CLIENT_ID") {
					t.Error("Skip reason should mention OAUTH_GOOGLE_CLIENT_ID")
				}
				if !containsString(reason, "OAUTH_GOOGLE_CLIENT_SECRET") {
					t.Error("Skip reason should mention OAUTH_GOOGLE_CLIENT_SECRET")
				}
				if !containsString(reason, "OAUTH_GITHUB_CLIENT_ID") {
					t.Error("Skip reason should mention OAUTH_GITHUB_CLIENT_ID")
				}
				if !containsString(reason, "OAUTH_GITHUB_CLIENT_SECRET") {
					t.Error("Skip reason should mention OAUTH_GITHUB_CLIENT_SECRET")
				}
				if !containsString(reason, "Example:") {
					t.Error("Skip reason should contain example command")
				}
			}
		})
	}
}

// TestShouldSkipTest_Combined tests combined SMTP and OAuth requirements
func TestShouldSkipTest_Combined(t *testing.T) {
	// Backup environment
	originalSMTPHost := os.Getenv("SMTP_HOST")
	originalSMTPUser := os.Getenv("SMTP_USER")
	originalSMTPPassword := os.Getenv("SMTP_PASSWORD")
	originalGoogleID := os.Getenv("OAUTH_GOOGLE_CLIENT_ID")
	originalGoogleSecret := os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET")
	defer func() {
		os.Setenv("SMTP_HOST", originalSMTPHost)
		os.Setenv("SMTP_USER", originalSMTPUser)
		os.Setenv("SMTP_PASSWORD", originalSMTPPassword)
		os.Setenv("OAUTH_GOOGLE_CLIENT_ID", originalGoogleID)
		os.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", originalGoogleSecret)
	}()

	// Configure environment - missing SMTP but have OAuth
	os.Setenv("SMTP_HOST", "")
	os.Setenv("SMTP_USER", "")
	os.Setenv("SMTP_PASSWORD", "")
	os.Setenv("OAUTH_GOOGLE_CLIENT_ID", "google-id")
	os.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", "google-secret")

	// Test: requires both SMTP and OAuth
	config := &RunnerConfig{
		RequireSMTP:  true,
		RequireOAuth: true,
	}
	tr := &TestRunner{
		config: config,
	}

	ctx := context.Background()
	shouldSkip, reason := tr.ShouldSkipTest(ctx)

	// Should skip because SMTP is missing (checked first)
	if !shouldSkip {
		t.Error("Should skip when SMTP is missing and required")
	}

	if !containsString(reason, "SMTP") {
		t.Error("Skip reason should mention SMTP when SMTP is the first missing requirement")
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
