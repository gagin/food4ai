// cmd/codecat/walk.go
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath" // Keep for potential future use (e.g., sorting metadata)
	"strings"

	gocodewalker "github.com/boyter/gocodewalker"
	// No sabhiram import needed
)

// generateConcatenatedCode uses gocodewalker, configured by useGitignore bool.
func generateConcatenatedCode(
	dir string,
	exts map[string]struct{},
	manualFilePaths []string,
	excludePatterns []string, // Combined list from config + flag
	useGitignore bool, // Boolean controlling ignore processing
	header, marker string,
	noScan bool, // Flag to skip scanning
	// Future flags:
	// includeFileList bool,
	// includeEmptyFilesList bool,
) (
	output string,
	includedFiles []FileInfo,
	emptyFiles []string,
	errorFiles map[string]error,
	totalSize int64,
	returnedErr error,
) {

	var outputBuilder strings.Builder
	if header != "" {
		outputBuilder.WriteString(header + "\n\n")
	} else {
		outputBuilder.WriteString("\n")
	}

	includedFiles = make([]FileInfo, 0)
	emptyFiles = make([]string, 0)
	errorFiles = make(map[string]error)
	processedFiles := make(map[string]bool)
	totalSize = 0

	// Validate exclude patterns provided
	validExcludePatterns := make([]string, 0, len(excludePatterns))
	for _, pattern := range excludePatterns {
		if _, errMatch := filepath.Match(pattern, ""); errMatch != nil {
			slog.Warn("Invalid exclude pattern, ignoring.",
				"pattern", pattern, "error", errMatch)
		} else {
			validExcludePatterns = append(validExcludePatterns, pattern)
		}
	}
	excludePatterns = validExcludePatterns // Use the validated list

	// Process Manually Added Files FIRST
	if len(manualFilePaths) > 0 {
		slog.Debug("Processing manually specified files.", "count", len(manualFilePaths))
		for _, manualPath := range manualFilePaths {
			absManualPath, errAbs := filepath.Abs(manualPath)
			if errAbs != nil {
				slog.Warn("Could not get absolute path for manual file.",
					"path", manualPath, "error", errAbs)
				errorFiles[manualPath] = errAbs // Use original path as key
				continue
			}
			slog.Debug("Attempting to process manual file.", "path", absManualPath)

			fileInfo, errStat := os.Stat(absManualPath)
			if errStat != nil {
				logMsg := "Cannot stat manual file."
				if os.IsNotExist(errStat) {
					logMsg = "Manual file not found."
				}
				slog.Warn(logMsg, "path", absManualPath, "error", errStat)
				errorFiles[absManualPath] = errStat // Use absolute path as key
				continue
			}
			if fileInfo.IsDir() {
				slog.Warn("Manual path points to a directory, skipping.", "path", absManualPath)
				errorFiles[absManualPath] = fmt.Errorf("path is a directory")
				continue
			}

			content, errRead := os.ReadFile(absManualPath)
			if errRead != nil {
				slog.Warn("Error reading manual file content.",
					"path", absManualPath, "error", errRead)
				errorFiles[absManualPath] = errRead
				processedFiles[absManualPath] = true // Mark processed even on error
				continue
			}

			displayPath := absManualPath // Default to absolute
			relPath, errRel := filepath.Rel(dir, absManualPath)
			// Use relative path only if clearly inside target directory tree
			if errRel == nil && !strings.HasPrefix(filepath.ToSlash(relPath), "..") {
				displayPath = filepath.ToSlash(relPath)
			} else {
				displayPath = filepath.ToSlash(absManualPath)
			}

			if len(content) == 0 {
				slog.Info("Manual file is empty.", "path", absManualPath)
				emptyFiles = append(emptyFiles, displayPath)
				processedFiles[absManualPath] = true // Mark processed
				continue
			}

			slog.Debug("Adding manual file content.",
				"path", displayPath, "size", len(content))
			outputBuilder.WriteString(fmt.Sprintf("%s %s\n%s\n%s\n\n",
				marker, displayPath, string(content), marker))
			includedFiles = append(includedFiles, FileInfo{
				Path: displayPath, Size: fileInfo.Size(), IsManual: true})
			totalSize += fileInfo.Size()
			processedFiles[absManualPath] = true // Mark processed
		}
	}

	// --- Directory Scanning ---
	// Skip scan if noScan flag is true OR if no extensions were provided
	shouldScan := !noScan && len(exts) > 0

	if shouldScan {
		slog.Info("Starting file scan.", "directory", dir, "useGitignore", useGitignore)
		// Initial directory check remains necessary before starting walker
		dirInfo, statErr := os.Stat(dir)
		if statErr != nil {
			logMsg := "Cannot stat target directory before walk."
			if os.IsNotExist(statErr) {
				logMsg = "Target directory does not exist."
			}
			slog.Error(logMsg, "path", dir, "error", statErr)
			returnedErr = statErr
			return // Return error immediately
		}
		if !dirInfo.IsDir() {
			statErr = fmt.Errorf("target path '%s' is not a directory", dir)
			slog.Error("Target path is not a directory.", "path", dir)
			returnedErr = statErr
			return // Return error immediately
		}

		var walkErr error // For errors returned by walker Start() or during walk

		// Always use gocodewalker
		fileListQueue := make(chan *gocodewalker.File, 100)
		fileWalker := gocodewalker.NewFileWalker(dir, fileListQueue)

		// Configure walker based on useGitignore flag
		ignoreIgnores := !useGitignore
		fileWalker.IgnoreGitIgnore = ignoreIgnores
		fileWalker.IgnoreIgnoreFile = ignoreIgnores
		slog.Debug("Configured walker ignore flags",
			"IgnoreGitIgnore", fileWalker.IgnoreGitIgnore,
			"IgnoreIgnoreFile", fileWalker.IgnoreIgnoreFile)

		// Configure allowed extensions on walker
		allowedExtList := []string{}
		for extWithDot := range exts {
			allowedExtList = append(allowedExtList, strings.TrimPrefix(extWithDot, "."))
		}
		fileWalker.AllowListExtensions = allowedExtList
		slog.Debug("Set walker AllowListExtensions", "extensions", allowedExtList)

		// Prepare patterns for manual filtering (walker excludes not used)
		manualPathExcludes := []string{}
		manualDirExcludes := []string{}

		if useGitignore { // Add .git exclude only if respecting gitignores
			manualPathExcludes = append(manualPathExcludes, ".git/*")
			manualDirExcludes = append(manualDirExcludes, ".git")
		}

		for _, pattern := range excludePatterns {
			if strings.HasSuffix(pattern, "/") {
				manualDirExcludes = append(manualDirExcludes, strings.TrimSuffix(pattern, "/"))
			} else {
				manualPathExcludes = append(manualPathExcludes, pattern)
			}
		}
		slog.Debug("Manual path excludes (filepath.Match)", "patterns", manualPathExcludes)
		slog.Debug("Manual directory excludes (basename check)", "patterns", manualDirExcludes)

		// Set Error Handler
		var firstWalkError error
		walkerErrorHandler := func(e error) bool {
			slog.Warn("Error during walk.", "error", e)
			if firstWalkError == nil {
				firstWalkError = e
			}
			// Continue walking if possible, error will be reported at the end
			return true
		}
		fileWalker.SetErrorHandler(walkerErrorHandler)

		// Start the walker and check for immediate errors
		walkErr = fileWalker.Start()
		if walkErr != nil {
			slog.Error("Failed to start file walk.", "directory", dir, "error", walkErr)
			returnedErr = walkErr // Assign the error from Start()
			return                // Return immediately
		}

		// Launch goroutine to process results from the channel
		processingDone := make(chan struct{})
		go func() {
			defer close(processingDone) // Ensure channel is closed when done
			for f := range fileListQueue {
				absPath := f.Location

				// Check if already processed manually
				if processedFiles[absPath] {
					relPathForLog, _ := filepath.Rel(dir, absPath)
					slog.Debug("Walk: Skipping file already processed manually.",
						"path", filepath.ToSlash(relPathForLog))
					continue // Skip this file
				}

				// Calculate relative path for checks and output
				relPath, errRel := filepath.Rel(dir, absPath)
				if errRel != nil {
					slog.Warn("Could not get relative path during walk.",
						"path", absPath, "error", errRel)
					// Record error using absolute path if relative fails
					errorFiles[filepath.ToSlash(absPath)] = errRel
					processedFiles[absPath] = true // Mark to avoid reprocessing
					continue
				}
				relPath = filepath.ToSlash(relPath) // Use consistent slashes

				// Stat the file/dir yielded by walker
				fileInfo, statErr := os.Stat(absPath)
				if statErr != nil {
					slog.Warn("Could not stat path from walker.",
						"path", absPath, "error", statErr)
					errorFiles[relPath] = statErr // Use relative path for error key now
					processedFiles[absPath] = true
					continue
				}
				isDir := fileInfo.IsDir()

				// Apply manual exclude filters
				excluded := false
				// 1. Check dir/ patterns
				if isDir {
					for _, dirPatternBase := range manualDirExcludes {
						if fileInfo.Name() == dirPatternBase {
							slog.Debug("Walk: Skipping directory (manual dir/ check).",
								"path", relPath, "pattern", dirPatternBase+"/")
							excluded = true
							break
						}
					}
				} else { // Is file
					for _, dirPatternBase := range manualDirExcludes {
						if strings.HasPrefix(relPath, dirPatternBase+"/") {
							slog.Debug("Walk: Skipping file in excluded dir (manual dir/ check).",
								"path", relPath, "pattern", dirPatternBase+"/")
							excluded = true
							break
						}
					}
				}
				// 2. Check other patterns if not already excluded
				if !excluded {
					for _, pattern := range manualPathExcludes {
						matchRel, _ := filepath.Match(pattern, relPath)
						matchName := false
						// Match basename only for files unless pattern includes slash
						if !isDir && !strings.Contains(pattern, "/") {
							matchName, _ = filepath.Match(pattern, fileInfo.Name())
						}
						if matchRel || matchName {
							slog.Debug("Walk: Skipping entry (manual path check).",
								"path", relPath, "pattern", pattern)
							excluded = true
							break
						}
					}
				}

				if excluded {
					processedFiles[absPath] = true // Mark as excluded
					continue                       // Skip this item
				}

				// Skip directories after all checks are passed
				if isDir {
					processedFiles[absPath] = true // Mark dir as processed
					continue
				}

				// Extension check is redundant due to AllowListExtensions on walker

				// Read file content
				content, errRead := os.ReadFile(absPath)
				if errRead != nil {
					slog.Warn("Error reading file content.", "path", relPath, "error", errRead)
					errorFiles[relPath] = errRead
					processedFiles[absPath] = true // Mark processed even on error
					continue
				}

				// Handle empty file
				if len(content) == 0 {
					slog.Info("Found empty file during scan.", "path", relPath)
					emptyFiles = append(emptyFiles, relPath)
					processedFiles[absPath] = true
					continue
				}

				// Add content to output
				fileSize := fileInfo.Size()
				slog.Debug("Adding file content to output.", "path", relPath, "size", fileSize)
				outputBuilder.WriteString(fmt.Sprintf("%s %s\n%s\n%s\n\n",
					marker, relPath, string(content), marker))
				includedFiles = append(includedFiles, FileInfo{
					Path: relPath, Size: fileSize, IsManual: false})
				totalSize += fileSize
				processedFiles[absPath] = true // Mark as processed successfully
			}
		}() // End of processor goroutine

		<-processingDone // Wait for channel processing to complete

		// Determine final error state from walk
		finalWalkError := walkErr // Error from Start()
		if finalWalkError == nil && firstWalkError != nil {
			// Use error from handler if Start() was okay but traversal had issues
			finalWalkError = firstWalkError
			slog.Warn("Walk completed with non-critical errors.",
				"directory", dir, "first_error", finalWalkError)
		} else if finalWalkError != nil {
			// Error occurred during Start() itself
			slog.Error("Walk failed.", "directory", dir, "error", finalWalkError)
		}

		// Assign walk error to return value if no earlier error occurred
		if returnedErr == nil && finalWalkError != nil {
			returnedErr = fmt.Errorf("file walk operation failed: %w", finalWalkError)
		}
		// Log overall completion status
		if returnedErr == nil {
			slog.Info("File scan completed.")
		} else {
			slog.Error("File scan finished with errors.", "error", returnedErr)
		}

	} else if noScan {
		slog.Info("Skipping directory scan due to --no-scan flag.")
	} else if len(exts) == 0 {
		slog.Info("Skipping directory scan as no extensions were specified.")
	} // End if shouldScan

	// Append Metadata section (to be implemented later)
	// if includeFileList { ... }
	// if includeEmptyFilesList { ... }

	// Prepare final output string
	output = strings.TrimSuffix(outputBuilder.String(), "\n\n")
	if outputBuilder.Len() > 0 && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	// Handle edge cases for empty output or header-only output
	if header != "" && strings.TrimSpace(output) == header {
		output = header + "\n"
	} else if output == "\n" && header == "" {
		output = ""
	}

	return // Use named return values
}
