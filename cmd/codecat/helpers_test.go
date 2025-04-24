// cmd/codecat/helpers_test.go
package main

import (
	"testing"

	// Use testify for assertions as the original test likely did
	"github.com/stretchr/testify/assert"
)

// TestProcessExtensions moved from walk_test.go
func TestProcessExtensions(t *testing.T) {
	testCases := []struct {
		name     string
		input    []string
		expected map[string]struct{}
	}{
		{
			name:     "Empty input",
			input:    []string{},
			expected: map[string]struct{}{},
		},
		{
			name:     "Basic extensions",
			input:    []string{"py", "txt", "json"},
			expected: map[string]struct{}{".py": {}, ".txt": {}, ".json": {}},
		},
		{
			name:     "With leading dots",
			input:    []string{".py", "txt", ".json"},
			expected: map[string]struct{}{".py": {}, ".txt": {}, ".json": {}},
		},
		{
			name:     "Mixed case",
			input:    []string{"Py", ".TXT", "jSoN"},
			expected: map[string]struct{}{".py": {}, ".txt": {}, ".json": {}},
		},
		{
			name:     "With whitespace",
			input:    []string{" py ", " .txt"},
			expected: map[string]struct{}{".py": {}, ".txt": {}},
		},
		{
			name:     "With empty strings",
			input:    []string{"py", "", " ", ".txt"},
			expected: map[string]struct{}{".py": {}, ".txt": {}},
		},
		{
			name:     "Comma separated string",
			input:    []string{"go, mod, sum", ".yaml, .yml"},
			expected: map[string]struct{}{".go": {}, ".mod": {}, ".sum": {}, ".yaml": {}, ".yml": {}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := processExtensions(tc.input)
			assert.Equal(t, tc.expected, actual) // Using testify assertion
		})
	}
}

// TODO: Add tests for mapsKeys function if needed
// func TestMapsKeys(t *testing.T) { ... }

// TODO: Add tests for formatBytes function
// func TestFormatBytes(t *testing.T) { ... }
