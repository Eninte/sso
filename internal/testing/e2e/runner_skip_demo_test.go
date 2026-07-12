package e2e

import (
	"context"
	"os"
	"testing"
)

// TestDemonstrateSkipMessages demonstrates the clear skip messages with remediation instructions
// This test shows what users will see when tests are skipped due to missing dependencies
func TestDemonstrateSkipMessages(t *testing.T) {
	// Backup environment
	originalSMTPHost := os.Getenv("SMTP_HOST")
	originalSMTPUser := os.Getenv("SMTP_USER")
	originalSMTPPassword := os.Getenv("SMTP_PASSWORD")
	originalGoogleID := os.Getenv("OAUTH_GOOGLE_CLIENT_ID")
	originalGoogleSecret := os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET")
	originalGithubID := os.Getenv("OAUTH_GITHUB_CLIENT_ID")
	originalGithubSecret := os.Getenv("OAUTH_GITHUB_CLIENT_SECRET")
	defer func() {
		os.Setenv("SMTP_HOST", originalSMTPHost)
		os.Setenv("SMTP_USER", originalSMTPUser)
		os.Setenv("SMTP_PASSWORD", originalSMTPPassword)
		os.Setenv("OAUTH_GOOGLE_CLIENT_ID", originalGoogleID)
		os.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", originalGoogleSecret)
		os.Setenv("OAUTH_GITHUB_CLIENT_ID", originalGithubID)
		os.Setenv("OAUTH_GITHUB_CLIENT_SECRET", originalGithubSecret)
	}()

	t.Run("SMTP Missing Skip Message", func(t *testing.T) {
		// Clear SMTP credentials
		os.Setenv("SMTP_HOST", "")
		os.Setenv("SMTP_USER", "")
		os.Setenv("SMTP_PASSWORD", "")

		config := &RunnerConfig{
			RequireSMTP: true,
		}
		tr := &TestRunner{
			config: config,
		}

		ctx := context.Background()
		shouldSkip, reason := tr.ShouldSkipTest(ctx)

		if !shouldSkip {
			t.Fatal("Expected test to be skipped when SMTP credentials are missing")
		}

		t.Logf("\n=== SMTP SKIP MESSAGE ===\n%s\n=== END ===\n", reason)

		// Validate message contains all required information
		requiredParts := []string{
			"SMTP credentials not configured",
			"SMTP_HOST",
			"SMTP_USER",
			"SMTP_PASSWORD",
			"Example:",
		}

		for _, part := range requiredParts {
			if !containsString(reason, part) {
				t.Errorf("Skip message missing required part: %s", part)
			}
		}
	})

	t.Run("OAuth Missing Skip Message", func(t *testing.T) {
		// Clear OAuth credentials
		os.Setenv("OAUTH_GOOGLE_CLIENT_ID", "")
		os.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", "")
		os.Setenv("OAUTH_GITHUB_CLIENT_ID", "")
		os.Setenv("OAUTH_GITHUB_CLIENT_SECRET", "")

		config := &RunnerConfig{
			RequireOAuth: true,
		}
		tr := &TestRunner{
			config: config,
		}

		ctx := context.Background()
		shouldSkip, reason := tr.ShouldSkipTest(ctx)

		if !shouldSkip {
			t.Fatal("Expected test to be skipped when OAuth credentials are missing")
		}

		t.Logf("\n=== OAUTH SKIP MESSAGE ===\n%s\n=== END ===\n", reason)

		// Validate message contains all required information
		requiredParts := []string{
			"OAuth provider credentials not configured",
			"OAUTH_GOOGLE_CLIENT_ID",
			"OAUTH_GOOGLE_CLIENT_SECRET",
			"OAUTH_GITHUB_CLIENT_ID",
			"OAUTH_GITHUB_CLIENT_SECRET",
			"Example:",
		}

		for _, part := range requiredParts {
			if !containsString(reason, part) {
				t.Errorf("Skip message missing required part: %s", part)
			}
		}
	})

	t.Run("No Skip When All Configured", func(t *testing.T) {
		// Set all credentials
		os.Setenv("SMTP_HOST", "smtp.example.com")
		os.Setenv("SMTP_USER", "user@example.com")
		os.Setenv("SMTP_PASSWORD", "password")
		os.Setenv("OAUTH_GOOGLE_CLIENT_ID", "google-id")
		os.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", "google-secret")

		config := &RunnerConfig{
			RequireSMTP:  true,
			RequireOAuth: true,
		}
		tr := &TestRunner{
			config: config,
		}

		ctx := context.Background()
		shouldSkip, reason := tr.ShouldSkipTest(ctx)

		if shouldSkip {
			t.Errorf("Expected test NOT to be skipped when all credentials are configured, but got skip with reason: %s", reason)
		}

		if reason != "" {
			t.Errorf("Expected empty reason when not skipping, but got: %s", reason)
		}

		t.Log("✓ Test runs normally when all required dependencies are configured")
	})
}
