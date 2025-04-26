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

// generateConcatenatedCode walks directories, processes files, and generates the output.
func generateConcatenatedCode(
	cwd string,
	scanDirs []string,
	exts map[string]struct{},
	manualFilePaths []string,
	excludeBasenames []string,
	projectExcludePatterns []string,
	flagExcludePatterns []string,
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
	slog.Debug("generateConcatenatedCode received extensions map", "exts_keys", mapsKeys(exts))

	var outputBuilder strings.Builder
	if header != "" {
		outputBuilder.WriteString(header)
	}

	includedFiles = make([]FileInfo, 0)
	emptyFiles = make([]string, 0)
	errorFiles = make(map[string]error)
	processedAbsPaths := make(map[string]bool)
	totalSize = 0

	// --- Pre-validate and Combine Exclude Patterns ---
	validBasenameExcludes := make([]string, 0, len(excludeBasenames))
	for _, pattern := range excludeBasenames {
		if _, errMatch := filepath.Match(pattern, "a"); errMatch != nil {
			slog.Warn("Invalid global exclude basename pattern syntax, ignoring.",
				"pattern", pattern, "error", errMatch)
		} else {
			validBasenameExcludes = append(validBasenameExcludes, pattern)
		}
	}
	slog.Debug("Using validated basename exclude patterns", "patterns", validBasenameExcludes)

	cwdRelativeExcludePatterns := []string{}
	combinedCwdExcludes := append([]string{}, projectExcludePatterns...)
	combinedCwdExcludes = append(combinedCwdExcludes, flagExcludePatterns...)
	for _, pattern := range combinedCwdExcludes {
		source := tern(contains(flagExcludePatterns, pattern), "flag", "project")
		if _, errMatch := filepath.Match(pattern, "a"); errMatch != nil {
			slog.Warn("Invalid CWD-relative exclude pattern syntax, ignoring.",
				"pattern", pattern, "source", source, "error", errMatch)
			continue
		}
		cwdRelativeExcludePatterns = append(cwdRelativeExcludePatterns, pattern)
	}
	slog.Debug("Using combined CWD-relative exclude patterns", "patterns", cwdRelativeExcludePatterns)

	// --- Process Manually Specified Files (-f) ---
	// Call the function from manual_files.go - REMOVE exclude patterns from call
	processManualFiles(
		cwd,
		manualFilePaths,
		// No longer pass exclude lists here
		marker,
		&outputBuilder,
		processedAbsPaths,
		&includedFiles,
		&emptyFiles,
		errorFiles,
		&totalSize,
	)

	// --- Perform Directory Scan ---
	shouldScan := !noScan && len(scanDirs) > 0
	if shouldScan {
		// Create Excluder using the validated/combined patterns for the scan phase
		excluder := NewDefaultExcluder(validBasenameExcludes, cwdRelativeExcludePatterns)

		if len(exts) == 0 && len(manualFilePaths) == 0 {
			slog.Warn("Scanning requested, but no extensions/manual files provided. Scan will find nothing.")
		}
		slog.Info("Starting file scan.", "scanDirs", scanDirs, "useGitignore", useGitignore)

		for _, scanDir := range scanDirs {
			slog.Debug("Scanning directory", "path", scanDir)
			// Directory validation...
			dirInfo, statErr := os.Stat(scanDir)
			if statErr != nil {
				logMsg := tern(os.IsNotExist(statErr), "Target scan directory does not exist.", "Cannot stat target scan directory.")
				slog.Error(logMsg, "path", scanDir, "error", statErr)
				relScanDir, relErr := filepath.Rel(cwd, scanDir)
				relScanDir = tern(relErr == nil, filepath.ToSlash(relScanDir), filepath.ToSlash(scanDir))
				errorFiles[relScanDir+"/"] = statErr
				if returnedErr == nil {
					returnedErr = fmt.Errorf("scan directory '%s' error: %w", scanDir, statErr)
				}
				continue
			}
			if !dirInfo.IsDir() {
				errMsg := fmt.Errorf("target scan path '%s' is not a directory", scanDir)
				slog.Error(errMsg.Error(), "path", scanDir)
				relScanDir, relErr := filepath.Rel(cwd, scanDir)
				relScanDir = tern(relErr == nil, filepath.ToSlash(relScanDir), filepath.ToSlash(scanDir))
				errorFiles[relScanDir] = errMsg
				if returnedErr == nil {
					returnedErr = errMsg
				}
				continue
			}

			// Setup walker...
			fileListQueue := make(chan *gocodewalker.File, 100)
			absScanDir, errAbsScan := filepath.Abs(scanDir)
			if errAbsScan != nil {
				slog.Error("Could not get absolute path for scan directory, gitignore might be affected.", "path", scanDir, "error", errAbsScan)
				absScanDir = scanDir
			}
			fileWalker := gocodewalker.NewFileWalker(absScanDir, fileListQueue)
			fileWalker.IgnoreGitIgnore = !useGitignore
			fileWalker.IgnoreIgnoreFile = !useGitignore
			walkerExts := []string{}
			for extWithDot := range exts {
				if extWithDot != "" {
					walkerExts = append(walkerExts, strings.TrimPrefix(extWithDot, "."))
				}
			}
			if len(walkerExts) > 0 {
				fileWalker.AllowListExtensions = walkerExts
				slog.Debug("Set walker AllowListExtensions", "extensions", walkerExts)
			} else {
				fileWalker.AllowListExtensions = nil
				slog.Debug("No specific extensions provided; walker allows all.")
			}
			var walkErr error
			var firstWalkError error
			processingDone := make(chan struct{})
			go func() { // Start walker
				defer close(processingDone)
				walkerErrorHandler := func(e error) bool {
					slog.Warn("Error reported by file walker.", "scanDir", absScanDir, "error", e)
					if firstWalkError == nil {
						firstWalkError = e
					}
					return true
				}
				fileWalker.SetErrorHandler(walkerErrorHandler)
				walkErr = fileWalker.Start()
				if walkErr != nil {
					slog.Error("File walker failed for directory.", "scanDir", absScanDir, "error", walkErr)
				}
			}()
			// Process walker results
			for f := range fileListQueue {
				absPath := f.Location
				baseName := filepath.Base(absPath)
				relPathCwd, errRel := filepath.Rel(cwd, absPath)
				relPathCwd = tern(errRel == nil, filepath.ToSlash(relPathCwd), filepath.ToSlash(absPath))
				if errRel != nil {
					slog.Warn("Could not get relative path for scanned item, using absolute.", "absolutePath", absPath, "cwd", cwd, "error", errRel)
				}
				slog.Debug("Processing item from walker", "absPath", absPath, "relPathCwd", relPathCwd, "baseName", baseName)
				if processedAbsPaths[absPath] {
					slog.Debug("Walk: Skipping item already processed manually.", "path", relPathCwd)
					continue
				}
				fileInfo, statErr := os.Stat(absPath)
				if statErr != nil {
					slog.Warn("Could not stat path from walker.", "path", relPathCwd, "error", statErr)
					errorFiles[relPathCwd] = statErr
					processedAbsPaths[absPath] = true
					continue
				}
				isDir := fileInfo.IsDir()
				pathInfo := PathInfo{AbsPath: absPath, RelPathCwd: relPathCwd, BaseName: baseName, IsDir: isDir}
				excluded, reason, pattern := excluder.IsExcluded(pathInfo) // Use excluder here
				if excluded {
					logMsg := tern(isDir, "Excluding directory and its contents.", "Excluding file.")
					slog.Log(nil, slog.LevelDebug, logMsg, "path", relPathCwd, "reason", reason, "pattern", pattern)
					processedAbsPaths[absPath] = true
					continue
				}
				if isDir {
					slog.Debug("Walk: Processing directory (not excluded).", "path", relPathCwd)
					processedAbsPaths[absPath] = true
					continue
				}
				currentExt := strings.ToLower(filepath.Ext(baseName))
				_, extAllowed := exts[currentExt]
				slog.Debug("Checking file extension.", "path", relPathCwd, "ext", currentExt, "allowed", extAllowed)
				if len(exts) > 0 && !extAllowed {
					slog.Debug("Walk: Skipping file with non-matching extension.", "path", relPathCwd, "ext", currentExt)
					processedAbsPaths[absPath] = true
					continue
				}
				content, errRead := os.ReadFile(absPath)
				if errRead != nil {
					slog.Warn("Error reading file content.", "path", relPathCwd, "error", errRead)
					errorFiles[relPathCwd] = errRead
					processedAbsPaths[absPath] = true
					continue
				}
				if len(content) == 0 {
					slog.Debug("Found empty file during scan.", "path", relPathCwd)
					emptyFiles = append(emptyFiles, relPathCwd)
					processedAbsPaths[absPath] = true
					continue
				}
				fileSize := fileInfo.Size()
				appendFileContent(&outputBuilder, marker, relPathCwd, content)
				includedFiles = append(includedFiles, FileInfo{Path: relPathCwd, Size: fileSize, IsManual: false})
				totalSize += fileSize
				processedAbsPaths[absPath] = true
			}
			<-processingDone
			finalWalkError := walkErr
			if finalWalkError == nil && firstWalkError != nil {
				finalWalkError = firstWalkError
				slog.Warn("Walk completed with non-critical errors.", "scanDir", absScanDir, "first_error", finalWalkError)
			} else if finalWalkError != nil {
				slog.Error("Walk failed for directory.", "scanDir", absScanDir, "error", finalWalkError)
			}
			if returnedErr == nil && finalWalkError != nil {
				returnedErr = fmt.Errorf("file walk operation failed for '%s': %w", scanDir, finalWalkError)
			}
		}
		if returnedErr == nil {
			slog.Info("File scan completed.")
		} else {
			slog.Error("File scan finished with errors.", "first_error", returnedErr)
		}
	} else if noScan {
		slog.Info("Skipping directory scan due to --no-scan flag.")
	} else if len(scanDirs) == 0 {
		slog.Info("Skipping directory scan as no scan directories were provided or determined.")
	}

	// --- Finalize ---
	output = outputBuilder.String() // Directly get string
	return
}
