// Package postgres PostgreSQL array adapter tests.
package postgres

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanTextArray(t *testing.T) {
	tests := []struct {
		name     string
		source   any
		expected []string
	}{
		{name: "null", source: nil, expected: nil},
		{name: "empty", source: `{}`, expected: []string{}},
		{
			name:     "special characters",
			source:   `{"openid","scope,with,commas","scope\"quote","scope\\backslash",""}`,
			expected: []string{"openid", "scope,with,commas", `scope"quote`, `scope\backslash`, ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var destination []string
			err := scanTextArray(&destination).Scan(tt.source)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, destination)
		})
	}

	t.Run("malformed", func(t *testing.T) {
		var destination []string
		err := scanTextArray(&destination).Scan(`{"unterminated"`)
		assert.Error(t, err)
	})
}

func TestScanTextArrayConcurrent(t *testing.T) {
	const workers = 32

	var waitGroup sync.WaitGroup
	waitGroup.Add(workers)
	for range workers {
		go func() {
			defer waitGroup.Done()

			var destination []string
			err := scanTextArray(&destination).Scan(`{"openid","profile"}`)
			assert.NoError(t, err)
			assert.Equal(t, []string{"openid", "profile"}, destination)
		}()
	}
	waitGroup.Wait()
}
