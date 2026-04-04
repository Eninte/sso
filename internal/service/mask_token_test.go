package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "空字符串",
			input: "",
			want:  "***",
		},
		{
			name:  "短于8字符",
			input: "short",
			want:  "***",
		},
		{
			name:  "等于8字符",
			input: "12345678",
			want:  "***",
		},
		{
			name:  "长于8字符",
			input: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9",
			want:  "eyJhbGci...",
		},
		{
			name:  "恰好9字符",
			input: "123456789",
			want:  "12345678...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskToken(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
