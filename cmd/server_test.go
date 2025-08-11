package cmd

import (
	"reflect"
	"testing"
)

func TestNormalizeTokens(t *testing.T) {
	testCases := []struct {
		name     string
		in       []string
		expected []string
	}{
		{
			name:     "single token",
			in:       []string{"token1"},
			expected: []string{"token1"},
		},
		{
			name:     "multiple tokens in one string",
			in:       []string{"token1,token2,token3"},
			expected: []string{"token1", "token2", "token3"},
		},
		{
			name:     "multiple tokens in multiple strings",
			in:       []string{"token1,token2", "token3"},
			expected: []string{"token1", "token2", "token3"},
		},
		{
			name:     "with spaces",
			in:       []string{" token1 , token2 ", "token3"},
			expected: []string{"token1", "token2", "token3"},
		},
		{
			name:     "empty parts",
			in:       []string{"token1,,token2"},
			expected: []string{"token1", "token2"},
		},
		{
			name:     "empty input",
			in:       []string{},
			expected: nil,
		},
		{
			name:     "input with empty string",
			in:       []string{""},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out := normalizeTokens(tc.in)
			if !reflect.DeepEqual(out, tc.expected) {
				t.Errorf("expected %v, got %v", tc.expected, out)
			}
		})
	}
}
