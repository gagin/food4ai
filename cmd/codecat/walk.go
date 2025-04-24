// cmd/codecat/walk.go
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	gocodewalker "github.com/boyter/gocodewalker"
)

// finalizeOutput prepares the final output string from the builder.
func finalizeOutput(builder *strings.Builder, header string) string {
	output := strings.TrimSuffix(builder.String(), "\n\n")
	if builder.Len() > 0 && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	if header != "" && strings.TrimSpace(output) == header {
		output = header + "\n"
	} else if output == "\n" && header == "" {
		output = ""
	}
	return output
}

// handleDirValidationError checks for directory validation errors and handles early return.
func handleDirValidationError(
	checkErr error,
	isNotDir bool,
	dirPath string,
	builder *strings.Builder,
	header string,
	includedFiles []FileInfo,
	emptyFiles []string,
	output *string,
	returnedErr *error,
) bool {
	var actualErr error
	if checkErr != nil {
		logMsg := "Cannot stat target directory before walk."
		if os.IsNotExist(checkErr) {
			logMsg = "Target directory does not exist."
		}
		slog.Error(logMsg, "path", dirPath, "error", checkErr)
		actualErr = checkErr
	} else if isNotDir {
		actualErr = fmt.Errorf("target path '%s' is not a directory", dirPath)
		slog.Error("Target path is not a directory.", "path", dirPath)
	}

	if actualErr != nil {
		*returnedErr = actualErr
		if len(includedFiles) > 0 || len(emptyFiles) > 0 {
			*output = finalizeOutput(builder, header)
		} else {
			*output = ""
		}
		return true
	}
	return false
}

// generateConcatenatedCode uses gocodewalker, configured by useGitignore bool.
func generateConcatenatedCode(
	dir string,
	exts map[string]struct{},
	manualFilePaths []string,
	excludePatterns []string,
	useGitignore bool,
	header, marker string,
	noScan bool,
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
	output = "" // Initialize named return

	validExcludePatterns := make([]string, 0, len(excludePatterns))
	for _, pattern := range excludePatterns {
		if _, errMatch := filepath.Match(pattern, ""); errMatch != nil {
			slog.Warn("Invalid exclude pattern, ignoring.",
				"pattern", pattern, "error", errMatch)
		} else {
			validExcludePatterns = append(validExcludePatterns, pattern)
		}
	}
	excludePatterns = validExcludePatterns

	if len(manualFilePaths) > 0 {
		slog.Debug("Processing manually specified files.", "count", len(manualFilePaths))
		for _, manualPath := range manualFilePaths {
			absManualPath, errAbs := filepath.Abs(manualPath)
			if errAbs != nil {
				slog.Warn("Could not get absolute path for manual file.",
					"path", manualPath, "error", errAbs)
				errorFiles[manualPath] = errAbs
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
				errorFiles[absManualPath] = errStat
				continue
			}
			if fileInfo.IsDir() {
				slog.Warn("Manual path points to a directory.", "path", absManualPath)
				errorFiles[absManualPath] = fmt.Errorf("path is a directory")
				continue
			}
			content, errRead := os.ReadFile(absManualPath)
			if errRead != nil {
				slog.Warn("Error reading manual file content.",
					"path", absManualPath, "error", errRead)
				errorFiles[absManualPath] = errRead
				processedFiles[absManualPath] = true
				continue
			}
			displayPath := absManualPath
			relPath, errRel := filepath.Rel(dir, absManualPath)
			if errRel == nil && !strings.HasPrefix(filepath.ToSlash(relPath), "..") {
				displayPath = filepath.ToSlash(relPath)
			} else {
				displayPath = filepath.ToSlash(absManualPath)
			}
			if len(content) == 0 {
				slog.Info("Manual file is empty.", "path", absManualPath)
				emptyFiles = append(emptyFiles, displayPath)
				processedFiles[absManualPath] = true
				continue
			}
			slog.Debug("Adding manual file content.",
				"path", displayPath, "size", len(content))
			outputBuilder.WriteString(fmt.Sprintf("%s %s\n%s\n%s\n\n",
				marker, displayPath, string(content), marker))
			includedFiles = append(includedFiles, FileInfo{
				Path: displayPath, Size: fileInfo.Size(), IsManual: true})
			totalSize += fileInfo.Size()
			processedFiles[absManualPath] = true
		}
	}

	shouldScan := !noScan && len(exts) > 0

	if shouldScan {
		slog.Info("Starting file scan.", "directory", dir, "useGitignore", useGitignore)

		dirInfo, statErr := os.Stat(dir)
		if handleDirValidationError(statErr, false, dir, &outputBuilder, header, includedFiles, emptyFiles, &output, &returnedErr) {
			return
		}
		if handleDirValidationError(nil, !dirInfo.IsDir(), dir, &outputBuilder, header, includedFiles, emptyFiles, &output, &returnedErr) {
			return
		}

		var walkErr error

		fileListQueue := make(chan *gocodewalker.File, 100)
		fileWalker := gocodewalker.NewFileWalker(dir, fileListQueue)

		ignoreIgnores := !useGitignore
		fileWalker.IgnoreGitIgnore = ignoreIgnores
		fileWalker.IgnoreIgnoreFile = ignoreIgnores
		slog.Debug("Configured walker ignore flags",
			"IgnoreGitIgnore", fileWalker.IgnoreGitIgnore,
			"IgnoreIgnoreFile", fileWalker.IgnoreIgnoreFile)

		allowedExtList := []string{}
		for extWithDot := range exts {
			allowedExtList = append(allowedExtList, strings.TrimPrefix(extWithDot, "."))
		}
		fileWalker.AllowListExtensions = allowedExtList
		slog.Debug("Set walker AllowListExtensions", "extensions", allowedExtList)

		manualPathExcludes := []string{}
		manualDirExcludes := []string{}

		if useGitignore {
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

		var firstWalkError error
		walkerErrorHandler := func(e error) bool {
			slog.Warn("Error during walk.", "error", e)
			if firstWalkError == nil {
				firstWalkError = e
			}
			return true
		}
		fileWalker.SetErrorHandler(walkerErrorHandler)

		walkErr = fileWalker.Start()
		if walkErr != nil {
			slog.Error("Failed to start file walk.", "directory", dir, "error", walkErr)
			returnedErr = walkErr
			if len(includedFiles) > 0 || len(emptyFiles) > 0 {
				output = finalizeOutput(&outputBuilder, header)
			} else {
				output = ""
			}
			return
		}

		processingDone := make(chan struct{})
		go func() {
			defer close(processingDone)
			for f := range fileListQueue {
				absPath := f.Location
				if processedFiles[absPath] {
					relPathForLog, _ := filepath.Rel(dir, absPath)
					slog.Debug("Walk: Skipping file already processed manually.",
						"path", filepath.ToSlash(relPathForLog))
					continue
				}

				relPath, errRel := filepath.Rel(dir, absPath)
				if errRel != nil {
					slog.Warn("Could not get relative path during walk.",
						"path", absPath, "error", errRel)
					errorFiles[filepath.ToSlash(absPath)] = errRel
					processedFiles[absPath] = true
					continue
				}
				relPath = filepath.ToSlash(relPath)

				fileInfo, statErr := os.Stat(absPath)
				if statErr != nil {
					slog.Warn("Could not stat path from walker.",
						"path", absPath, "error", statErr)
					errorFiles[relPath] = statErr
					processedFiles[absPath] = true
					continue
				}
				isDir := fileInfo.IsDir()

				excluded := false
				if isDir {
					for _, dirPatternBase := range manualDirExcludes {
						if fileInfo.Name() == dirPatternBase {
							slog.Debug("Walk: Skipping directory (manual dir/ check).",
								"path", relPath, "pattern", dirPatternBase+"/")
							excluded = true
							break
						}
					}
				} else {
					for _, dirPatternBase := range manualDirExcludes {
						if strings.HasPrefix(relPath, dirPatternBase+"/") {
							slog.Debug("Walk: Skipping file in excluded dir (manual dir/ check).",
								"path", relPath, "pattern", dirPatternBase+"/")
							excluded = true
							break
						}
					}
				}

				if !excluded {
					for _, pattern := range manualPathExcludes {
						matchRel, _ := filepath.Match(pattern, relPath)
						matchName := false
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
					processedFiles[absPath] = true
					continue
				}

				if isDir {
					processedFiles[absPath] = true
					continue
				}

				content, errRead := os.ReadFile(absPath)
				if errRead != nil {
					slog.Warn("Error reading file content.", "path", relPath, "error", errRead)
					errorFiles[relPath] = errRead
					processedFiles[absPath] = true
					continue
				}

				if len(content) == 0 {
					slog.Info("Found empty file during scan.", "path", relPath)
					emptyFiles = append(emptyFiles, relPath)
					processedFiles[absPath] = true
					continue
				}

				fileSize := fileInfo.Size()
				slog.Debug("Adding file content to output.", "path", relPath, "size", fileSize)
				outputBuilder.WriteString(fmt.Sprintf("%s %s\n%s\n%s\n\n",
					marker, relPath, string(content), marker))
				includedFiles = append(includedFiles, FileInfo{
					Path: relPath, Size: fileSize, IsManual: false})
				totalSize += fileSize
				processedFiles[absPath] = true
			}
		}()

		<-processingDone

		finalWalkError := walkErr
		if finalWalkError == nil && firstWalkError != nil {
			finalWalkError = firstWalkError
			slog.Warn("Walk completed with non-critical errors.",
				"directory", dir, "first_error", finalWalkError)
		} else if finalWalkError != nil {
			slog.Error("Walk failed.", "directory", dir, "error", finalWalkError)
		}

		if returnedErr == nil && finalWalkError != nil {
			returnedErr = fmt.Errorf("file walk operation failed: %w", finalWalkError)
		}
		if returnedErr == nil {
			slog.Info("File scan completed.")
		} else {
			slog.Error("File scan finished with errors.", "error", returnedErr)
		}

	} else if noScan {
		slog.Info("Skipping directory scan due to --no-scan flag.")
	} else if len(exts) == 0 {
		slog.Info("Skipping directory scan as no extensions were specified.")
	}

	// Append Metadata section (to be implemented later)

	if returnedErr == nil {
		output = finalizeOutput(&outputBuilder, header)
	} else if output == "" && (len(includedFiles) > 0 || len(emptyFiles) > 0) {
		// Ensure partial output is set if an error occurred after manual processing
		output = finalizeOutput(&outputBuilder, header)
	}

	return
}
