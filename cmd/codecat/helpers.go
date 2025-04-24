// cmd/codecat/helpers.go
package main

import (
	"fmt"
	"sort"
	"strings"
)

// processExtensions processes a list of extension strings into a set for quick lookup.
func processExtensions(extList []string) map[string]struct{} {
	processed := make(map[string]struct{})
	for _, ext := range extList {
		parts := strings.Split(ext, ",")
		for _, part := range parts {
			cleaned := strings.TrimSpace(strings.ToLower(part))
			if cleaned == "" {
				continue
			}
			if !strings.HasPrefix(cleaned, ".") {
				cleaned = "." + cleaned
			}
			processed[cleaned] = struct{}{}
		}
	}
	return processed
}

// mapsKeys Helper to get map keys for logging set contents
func mapsKeys[M ~map[K]V, K comparable, V any](m M) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	sort.Slice(r, func(i, j int) bool {
		ki := fmt.Sprint(r[i])
		kj := fmt.Sprint(r[j])
		return ki < kj
	})
	return r
}

// formatBytes formats bytes into human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	val := float64(b) / float64(div)
	unitPrefix := "KMGTPE"[exp]
	if val == float64(int64(val)) {
		return fmt.Sprintf("%d %ciB", int64(val), unitPrefix)
	}
	return fmt.Sprintf("%.1f %ciB", val, unitPrefix)
}

// globToRegex converts simple globs (*, ?) to regex for filename matching.
// Anchors the regex to match the whole string.
func globToRegex(glob string) string {
	var regex strings.Builder
	regex.WriteString("^") // Anchor start

	for i := 0; i < len(glob); i++ {
		char := glob[i]
		switch char {
		case '*':
			regex.WriteString(".*") // Match zero or more characters
		case '?':
			regex.WriteString(".") // Match any single character
		case '.', '[', ']', '{', '}', '(', ')', '+', '^', '$', '|', '\\':
			// Escape regex metacharacters
			regex.WriteByte('\\')
			regex.WriteByte(char)
		default:
			regex.WriteByte(char) // Append literal character
		}
	}

	regex.WriteString("$") // Anchor end
	return regex.String()
}
