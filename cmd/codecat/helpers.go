// cmd/codecat/helpers.go
package main

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
)

// --- Other helper functions remain the same ---
func processExtensions(extList []string) map[string]struct{} {
	processed := make(map[string]struct{})
	slog.Debug("Processing extensions list", "input_list", extList)
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
			if cleaned == "." {
				slog.Warn("Ignoring invalid extension format '.' - use specific filenames with -f for extensionless files.",
					"input_part", part)
				continue
			}
			processed[cleaned] = struct{}{}
		}
	}
	slog.Debug("Finished processing extensions", "processed_keys", mapsKeys(processed))
	return processed
}
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
func matchesGlob(target string, patterns []string) (bool, string) {
	for _, pattern := range patterns {
		match, _ := filepath.Match(pattern, target)
		if match {
			return true, pattern
		}
	}
	return false, ""
}
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
func appendFileContent(builder *strings.Builder, marker, relPathCwd string, content []byte) {
	slog.Debug("Adding file content to output.", "path", relPathCwd, "size", len(content))
	builder.WriteString(fmt.Sprintf("%s %s\n%s%s\n",
		marker, relPathCwd, string(content), marker))
}
func tern[T any](condition bool, trueVal, falseVal T) T {
	if condition {
		return trueVal
	}
	return falseVal
}
