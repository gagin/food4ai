// cmd/codecat/manual_files.go
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// processManualFiles handles the inclusion of files explicitly specified via the -f flag.
// It bypasses ALL exclusion rules (basename, CWD-relative, gitignore).
// It modifies the provided maps and slices directly.
func processManualFiles(
	cwd string,
	manualFilePaths []string,
	// --- Exclude patterns are no longer needed here ---
	// basenameExcludes []string,
	// cwdRelativeExcludePatterns []string,
	marker string,
	outputBuilder *strings.Builder,
	processedAbsPaths map[string]bool, // Keep track of processed files
	includedFiles *[]FileInfo, // Pointer to modify the slice
	emptyFiles *[]string, // Pointer to modify the slice
	errorFiles map[string]error, // Modify directly
	totalSize *int64, // Pointer to modify total size
) {
	if len(manualFilePaths) == 0 {
		return // Nothing to do
	}

	slog.Debug("Processing manually specified files (-f overrides excludes).", "count", len(manualFilePaths))
	for _, manualPathRaw := range manualFilePaths {
		// Resolve paths relative to CWD
		absManualPath := filepath.Join(cwd, manualPathRaw)
		if !filepath.IsAbs(manualPathRaw) {
			// Keep absManualPath as calculated above
		} else {
			absManualPath = manualPathRaw // It was already absolute
		}
		absManualPath = filepath.Clean(absManualPath)

		relPathCwd, errRel := filepath.Rel(cwd, absManualPath)
		if errRel != nil {
			slog.Warn("Could not get relative path for manual file, using absolute.",
				"absolutePath", absManualPath, "cwd", cwd, "error", errRel)
			relPathCwd = filepath.ToSlash(absManualPath) // Use absolute if relative fails
		} else {
			relPathCwd = filepath.ToSlash(relPathCwd) // Ensure slash format for consistency
		}

		// Skip duplicates
		if processedAbsPaths[absManualPath] {
			slog.Debug("Skipping duplicate manual file.", "path", relPathCwd)
			continue
		}

		slog.Debug("Attempting to process manual file.", "raw", manualPathRaw,
			"absolute", absManualPath, "relativeToCwd", relPathCwd)

		// Stat the file
		fileInfo, errStat := os.Stat(absManualPath)
		if errStat != nil {
			logMsg := "Cannot stat manual file."
			if os.IsNotExist(errStat) {
				logMsg = "Manual file not found."
			}
			slog.Warn(logMsg, "path", relPathCwd, "absolute", absManualPath, "error", errStat)
			errorFiles[relPathCwd] = errStat        // Record error
			processedAbsPaths[absManualPath] = true // Mark as processed even on error
			continue
		}

		// Skip directories specified via -f
		if fileInfo.IsDir() {
			slog.Warn("Manual path points to a directory, skipping.", "path", relPathCwd)
			errorFiles[relPathCwd] = fmt.Errorf("path is a directory")
			processedAbsPaths[absManualPath] = true
			continue
		}

		// --- NO EXCLUSION CHECKS for -f files ---
		slog.Debug("Including manual file (bypassing excludes).", "path", relPathCwd)

		// Read file content
		content, errRead := os.ReadFile(absManualPath)
		if errRead != nil {
			slog.Warn("Error reading manual file content.", "path", relPathCwd, "error", errRead)
			errorFiles[relPathCwd] = errRead
			processedAbsPaths[absManualPath] = true
			continue
		}

		// Handle empty files
		if len(content) == 0 {
			slog.Debug("Manual file is empty.", "path", relPathCwd)
			*emptyFiles = append(*emptyFiles, relPathCwd) // Append to slice via pointer
			processedAbsPaths[absManualPath] = true
			continue
		}

		// Use the helper function (now in helpers.go) to append content
		appendFileContent(outputBuilder, marker, relPathCwd, content)

		// Append to slices/maps via pointers or direct map access
		*includedFiles = append(*includedFiles, FileInfo{
			Path: relPathCwd, Size: fileInfo.Size(), IsManual: true})
		*totalSize += fileInfo.Size()           // Add to total size via pointer
		processedAbsPaths[absManualPath] = true // Mark as processed
	}
}
