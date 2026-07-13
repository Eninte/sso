// Package postgres PostgreSQL error mapping tests.
package postgres

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestIsUniqueViolation(t *testing.T) {
	uniqueViolation := &pgconn.PgError{Code: "23505"}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "direct", err: uniqueViolation, expected: true},
		{name: "wrapped", err: fmt.Errorf("create user: %w", uniqueViolation), expected: true},
		{name: "other postgres error", err: &pgconn.PgError{Code: "23503"}, expected: false},
		{name: "non postgres error", err: fmt.Errorf("connection failed"), expected: false},
		{name: "nil", err: nil, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isUniqueViolation(tt.err))
		})
	}
}
