package cmd

import (
	"testing"
	
	"github.com/urfave/cli/v3"
)

func TestNormalizeTokens(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "single token",
			input:    []string{"token1"},
			expected: []string{"token1"},
		},
		{
			name:     "multiple tokens in separate strings",
			input:    []string{"token1", "token2", "token3"},
			expected: []string{"token1", "token2", "token3"},
		},
		{
			name:     "comma separated tokens in single string",
			input:    []string{"token1,token2,token3"},
			expected: []string{"token1", "token2", "token3"},
		},
		{
			name:     "mixed format",
			input:    []string{"token1", "token2,token3", "token4"},
			expected: []string{"token1", "token2", "token3", "token4"},
		},
		{
			name:     "tokens with spaces",
			input:    []string{" token1 ", "token2 , token3", " token4"},
			expected: []string{"token1", "token2", "token3", "token4"},
		},
		{
			name:     "empty strings and spaces",
			input:    []string{"", " ", "token1,,token2", " , , "},
			expected: []string{"token1", "token2"},
		},
		{
			name:     "only empty tokens",
			input:    []string{"", " ", ",,,", "   "},
			expected: []string{},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "complex mixed case",
			input:    []string{"token1,token2", "", "  token3  ", "token4,  ,token5", "  "},
			expected: []string{"token1", "token2", "token3", "token4", "token5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeTokens(tt.input)
			
			// Check length
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d tokens, got %d", len(tt.expected), len(result))
				t.Errorf("Expected: %v", tt.expected)
				t.Errorf("Got: %v", result)
				return
			}

			// Check each token
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected token at index %d to be '%s', got '%s'", i, expected, result[i])
				}
			}
		})
	}
}

func TestNormalizeTokensEdgeCases(t *testing.T) {
	// Test edge cases separately for clarity
	
	t.Run("single comma", func(t *testing.T) {
		result := normalizeTokens([]string{","})
		if len(result) != 0 {
			t.Errorf("Expected empty result for single comma, got %v", result)
		}
	})

	t.Run("multiple commas", func(t *testing.T) {
		result := normalizeTokens([]string{",,,,"})
		if len(result) != 0 {
			t.Errorf("Expected empty result for multiple commas, got %v", result)
		}
	})

	t.Run("commas with spaces", func(t *testing.T) {
		result := normalizeTokens([]string{" , , , "})
		if len(result) != 0 {
			t.Errorf("Expected empty result for commas with spaces, got %v", result)
		}
	})

	t.Run("real world example", func(t *testing.T) {
		input := []string{"token1,token2", "token3", "token4,token5,token6"}
		expected := []string{"token1", "token2", "token3", "token4", "token5", "token6"}
		result := normalizeTokens(input)
		
		if len(result) != len(expected) {
			t.Errorf("Expected %d tokens, got %d", len(expected), len(result))
		}
		
		for i, exp := range expected {
			if i >= len(result) || result[i] != exp {
				t.Errorf("Expected token %d to be '%s', got '%s'", i, exp, result[i])
			}
		}
	})
}

func TestNormalizeTokensPreservesOrder(t *testing.T) {
	// Test that the order of tokens is preserved
	input := []string{"first", "second,third", "fourth"}
	expected := []string{"first", "second", "third", "fourth"}
	result := normalizeTokens(input)

	for i, exp := range expected {
		if i >= len(result) || result[i] != exp {
			t.Errorf("Token order not preserved. Expected position %d to be '%s', got '%s'", i, exp, result[i])
		}
	}
}

func TestNormalizeTokensDoesNotDuplicate(t *testing.T) {
	// Test that the function doesn't remove duplicates (it shouldn't)
	input := []string{"token1", "token1", "token2,token1"}
	result := normalizeTokens(input)

	// Should have 4 tokens (duplicates preserved): token1, token1, token2, token1
	if len(result) != 4 {
		t.Errorf("Expected 4 tokens (with duplicates), got %d", len(result))
	}

	expected := []string{"token1", "token1", "token2", "token1"}
	if len(result) != len(expected) {
		t.Errorf("Expected %d tokens, got %d", len(expected), len(result))
	}
}

// Test the structure and existence of CLI commands
func TestCommandStructure(t *testing.T) {
	// Test that all expected commands exist and have the right names
	if Server.Name != "server" {
		t.Errorf("Expected Server command name to be 'server', got '%s'", Server.Name)
	}

	if Client.Name != "client" {
		t.Errorf("Expected Client command name to be 'client', got '%s'", Client.Name)
	}

	if Host.Name != "host" {
		t.Errorf("Expected Host command name to be 'host', got '%s'", Host.Name)
	}
}

func TestServerCommandFlags(t *testing.T) {
	// Test that server command has expected flags
	expectedFlags := map[string]bool{
		"listen": false,
		"tokens": false,
	}

	for _, flag := range Server.Flags {
		switch f := flag.(type) {
		case *cli.StringFlag:
			if f.Name == "listen" {
				expectedFlags["listen"] = true
			}
		case *cli.StringSliceFlag:
			if f.Name == "tokens" {
				expectedFlags["tokens"] = true
			}
		}
	}

	for flagName, found := range expectedFlags {
		if !found {
			t.Errorf("Expected server command to have '%s' flag", flagName)
		}
	}
}

func TestClientCommandFlags(t *testing.T) {
	// Test that client command has expected flags
	expectedFlags := map[string]bool{
		"id":                 false,
		"host-id":           false,
		"signaling-address": false,
		"local-address":     false,
		"protocol":          false,
		"stun-addresses":    false,
		"token":             false,
		"timeout":           false,
	}

	for _, flag := range Client.Flags {
		switch f := flag.(type) {
		case *cli.StringFlag:
			if _, exists := expectedFlags[f.Name]; exists {
				expectedFlags[f.Name] = true
			}
		case *cli.StringSliceFlag:
			if _, exists := expectedFlags[f.Name]; exists {
				expectedFlags[f.Name] = true
			}
		case *cli.Int16Flag:
			if _, exists := expectedFlags[f.Name]; exists {
				expectedFlags[f.Name] = true
			}
		}
	}

	for flagName, found := range expectedFlags {
		if !found {
			t.Errorf("Expected client command to have '%s' flag", flagName)
		}
	}
}

func TestHostCommandFlags(t *testing.T) {
	// Test that host command has expected flags
	expectedFlags := map[string]bool{
		"id":                 false,
		"signaling-address": false,
		"local-address":     false,
		"protocol":          false,
		"stun-addresses":    false,
		"token":             false,
	}

	for _, flag := range Host.Flags {
		switch f := flag.(type) {
		case *cli.StringFlag:
			if _, exists := expectedFlags[f.Name]; exists {
				expectedFlags[f.Name] = true
			}
		case *cli.StringSliceFlag:
			if _, exists := expectedFlags[f.Name]; exists {
				expectedFlags[f.Name] = true
			}
		}
	}

	for flagName, found := range expectedFlags {
		if !found {
			t.Errorf("Expected host command to have '%s' flag", flagName)
		}
	}
}

