package e2e

import (
	"context"
	"testing"
)

// TestTestRunner_smartSkipping demonstrates how to use smart test skipping for tests
// that require optional environment dependencies like SMTP or OAuth.
//
// Validates: Requirements 1.3
func TestTestRunner_smartSkipping(t *testing.T) {
	// Example 1: Test requiring SMTP credentials
	// This test will be skipped if SMTP_HOST, SMTP_USER, or SMTP_PASSWORD are not configured

	smtpConfig := &RunnerConfig{
		RequireSMTP:              true, // Enable SMTP dependency check
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
		UseDBTransactions:        false,
		RedisNamespaceMode:       false,
		CaptureStackTraces:       false,
	}

	smtpRunner := NewTestRunner(nil, nil, "http://localhost:9090", smtpConfig)
	ctx := context.Background()

	emailTest := Test{
		Name: "Email Verification Flow",
		Run: func(ctx context.Context, tr *TestRunner) error {
			// This test would send actual emails
			// It will be skipped if SMTP is not configured
			return nil
		},
	}

	smtpResults := smtpRunner.Run(ctx, []Test{emailTest})

	// Check result - if SMTP not configured, test will be skipped
	if smtpResults[0].Status == TestStatusSkip {
		t.Logf("Test skipped: %s", smtpResults[0].SkipReason)
	}

	// Example 2: Test requiring OAuth provider credentials
	// This test will be skipped if no OAuth providers are configured

	oauthConfig := &RunnerConfig{
		RequireOAuth:             true, // Enable OAuth dependency check
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
		UseDBTransactions:        false,
		RedisNamespaceMode:       false,
		CaptureStackTraces:       false,
	}

	oauthRunner := NewTestRunner(nil, nil, "http://localhost:9090", oauthConfig)

	oauthTest := Test{
		Name: "Google OAuth Login Flow",
		Run: func(ctx context.Context, tr *TestRunner) error {
			// This test would test OAuth login
			// It will be skipped if OAuth providers are not configured
			return nil
		},
	}

	oauthResults := oauthRunner.Run(ctx, []Test{oauthTest})

	// Check result - if OAuth not configured, test will be skipped
	if oauthResults[0].Status == TestStatusSkip {
		t.Logf("Test skipped: %s", oauthResults[0].SkipReason)
	}
}

// TestExample_SMTPSkipping is a unit test demonstrating SMTP skip behavior.
// Validates: Requirements 1.3
func TestExample_SMTPSkipping(t *testing.T) {
	config := &RunnerConfig{
		RequireSMTP:              true,
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
		UseDBTransactions:        false,
		RedisNamespaceMode:       false,
	}

	runner := NewTestRunner(nil, nil, "http://localhost:9090", config)
	ctx := context.Background()

	// This will check if SMTP is configured
	shouldSkip, reason := runner.ShouldSkipTest(ctx)

	if shouldSkip {
		t.Logf("Test would be skipped: %s", reason)
	} else {
		t.Log("SMTP is configured, test would run")
	}
}

// TestExample_OAuthSkipping is a unit test demonstrating OAuth skip behavior.
// Validates: Requirements 1.3
func TestExample_OAuthSkipping(t *testing.T) {
	config := &RunnerConfig{
		RequireOAuth:             true,
		ValidatePostgresTriggers: false,
		ValidateRedisConnection:  false,
		UseDBTransactions:        false,
		RedisNamespaceMode:       false,
	}

	runner := NewTestRunner(nil, nil, "http://localhost:9090", config)
	ctx := context.Background()

	// This will check if OAuth providers are configured
	shouldSkip, reason := runner.ShouldSkipTest(ctx)

	if shouldSkip {
		t.Logf("Test would be skipped: %s", reason)
	} else {
		t.Log("OAuth is configured, test would run")
	}
}
