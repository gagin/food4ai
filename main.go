// main.go
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog" // Import slog
	"os"
	"path/filepath"
	"sort"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

// --- Custom Flag Type ---
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ", ") }
func (s *stringSliceFlag) Set(value string) error {
	parts := strings.Split(value, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			*s = append(*s, trimmed)
		}
	}
	return nil
}

// --- Global Variables for Flags ---
// Config is loaded in main() after logger setup
var (
	targetDir       string
	extensions      stringSliceFlag // Holds values ONLY if provided via flag
	manualFiles     stringSliceFlag
	excludePatterns stringSliceFlag // Holds values ONLY if provided via flag
	noGitignore     bool
	logLevelStr     string // Flag to set log level as string
)

// --- Helper function to check if a flag was explicitly set ---
func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func init() {
	// Define flags here. Defaults shown are hardcoded, config is loaded later.
	flag.StringVar(&targetDir, "d", ".", "The target directory to scan recursively.")
	flag.StringVar(&targetDir, "directory", ".", "The target directory to scan recursively.")

	// Initialize flag variables as empty. They will ONLY be populated if the flag is used.
	// Help text shows hardcoded defaults. Actual defaults depend on config loaded in main.
	extensions = make(stringSliceFlag, 0)
	flag.Var(&extensions, "e", fmt.Sprintf("File extensions to include (repeatable, overrides config). Default depends on config (e.g., %v)", defaultConfig.IncludeExtensions))
	flag.Var(&extensions, "extensions", fmt.Sprintf("File extensions to include (repeatable, overrides config). Default depends on config (e.g., %v)", defaultConfig.IncludeExtensions))

	manualFiles = make(stringSliceFlag, 0)
	flag.Var(&manualFiles, "f", "Specific file paths to include manually (repeatable, bypasses ignores/excludes).")
	flag.Var(&manualFiles, "files", "Specific file paths to include manually (repeatable, bypasses ignores/excludes).")

	excludePatterns = make(stringSliceFlag, 0)
	flag.Var(&excludePatterns, "x", fmt.Sprintf("Glob patterns to exclude (relative to target dir, repeatable, overrides config). Default depends on config (e.g., %v)", defaultConfig.ExcludePatterns))
	flag.Var(&excludePatterns, "exclude", fmt.Sprintf("Glob patterns to exclude (relative to target dir, repeatable, overrides config). Default depends on config (e.g., %v)", defaultConfig.ExcludePatterns))

	// Default for noGitignore depends on config's default useGitignore.
	flag.BoolVar(&noGitignore, "no-gitignore", !*defaultConfig.UseGitignore, fmt.Sprintf("Disable .gitignore processing (overrides config). Config default for 'use_gitignore': %t", *defaultConfig.UseGitignore))

	// Log level flag
	flag.StringVar(&logLevelStr, "loglevel", "info", "Set logging verbosity (debug, info, warning, error)")

	// --- Usage Message ---
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "Concatenate specified file types and/or specific files into a single output.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Reads config from ~/.config/food4ai/config.toml if it exists.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Command-line flags for extensions (-e) and excludes (-x) override config values.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Prints concatenated output to stdout, logs to stderr.\n\n")
		flag.PrintDefaults()
	}
}

// processExtensions (Keep as is)
func processExtensions(extList []string) map[string]struct{} {
	processed := make(map[string]struct{})
	for _, ext := range extList {
		ext = strings.TrimSpace(strings.ToLower(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		processed[ext] = struct{}{}
	}
	return processed
}

// generateConcatenatedCode now uses slog for logging
func generateConcatenatedCode(
	targetDir string,
	extensionsToUse map[string]struct{},
	manualFiles []string,
	excludePatternsToUse []string,
	useGitignore bool,
	headerText string,
	commentStart string,
) (string, error) {

	commentEnd := commentStart
	outputParts := []string{headerText + "\n"}
	filesToProcess := make(map[string]struct{}) // Absolute paths
	var emptyFilePaths []string                 // Store paths of empty files

	cwd, _ := os.Getwd()
	cwdAbs, _ := filepath.Abs(cwd)
	targetDirAbs, err := filepath.Abs(targetDir)
	if err != nil {
		slog.Warn("Could not get absolute path for target directory, proceeding with relative path.", "target", targetDir, "error", err)
		targetDirAbs = targetDir // Use original path
	}

	// --- 1. Process Manually Specified Files ---
	if len(manualFiles) > 0 {
		slog.Info("Processing manually specified files (bypassing filters).")
		for _, fileStr := range manualFiles {
			absPath, err := filepath.Abs(fileStr)
			if err != nil {
				slog.Warn("Error getting absolute path for manual file, skipping.", "file", fileStr, "error", err)
				continue
			}
			fileInfo, err := os.Stat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					slog.Warn("Manual file not found, skipping.", "file", fileStr)
				} else {
					slog.Warn("Error stating manual file, skipping.", "file", fileStr, "error", err)
				}
				continue
			}
			if fileInfo.IsDir() {
				slog.Warn("Manual path is a directory, skipping.", "path", fileStr)
				continue
			}
			if _, exists := filesToProcess[absPath]; exists {
				slog.Debug("Skipping duplicate manual file.", "file", fileStr)
			} else {
				slog.Debug("Adding manual file.", "file", fileStr, "absolute_path", absPath)
				filesToProcess[absPath] = struct{}{}
			}
		}
	}

	// --- 2. Process Directory Scan ---
	foundInScan := 0
	skippedByIgnore := 0
	skippedByExclude := 0
	var ignorer *gitignore.GitIgnore

	// Load gitignore if enabled
	if useGitignore {
		gitignorePath := filepath.Join(targetDir, ".gitignore")
		if _, err := os.Stat(gitignorePath); err == nil {
			ignorer, err = gitignore.CompileIgnoreFile(gitignorePath)
			if err != nil {
				slog.Warn("Error compiling gitignore file, proceeding without gitignore rules.", "path", gitignorePath, "error", err)
				ignorer = nil // Ensure ignorer is nil on error
			} else {
				slog.Info("Applying .gitignore rules.", "path", gitignorePath)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("Could not stat gitignore file.", "path", gitignorePath, "error", err)
		} else {
			slog.Info("No .gitignore file found in target directory.")
		}
	}

	dirInfo, err := os.Stat(targetDir)
	if err == nil && dirInfo.IsDir() {
		slog.Info("Scanning directory.", "path", targetDirAbs)
		if len(extensionsToUse) > 0 {
			extKeys := make([]string, 0, len(extensionsToUse))
			for k := range extensionsToUse {
				extKeys = append(extKeys, k)
			}
			sort.Strings(extKeys)
			slog.Debug("Looking for file extensions.", "extensions", extKeys)
		} else {
			slog.Info("No extensions specified for directory scan (only processing manual files).")
		}
		slog.Debug("Applying .gitignore rules status.", "enabled", useGitignore && ignorer != nil)
		if len(excludePatternsToUse) > 0 {
			slog.Debug("Applying exclude patterns.", "patterns", excludePatternsToUse)
		} else {
			slog.Debug("No exclude patterns specified.")
		}

		if len(extensionsToUse) > 0 { // Only walk if there are extensions to find
			walkErr := filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, walkErrIn error) error {
				if walkErrIn != nil {
					slog.Warn("Error accessing path during scan, skipping entry.", "path", path, "error", walkErrIn)
					if d != nil && d.IsDir() {
						return fs.SkipDir // Try to skip the whole dir if possible
					}
					return nil // Skip just this entry
				}

				// --- Get Relative Path (best effort) ---
				relPath, relErr := filepath.Rel(targetDirAbs, path)
				relPathSlash := filepath.ToSlash(relPath) // Convert to slashes for matching
				if relErr != nil {
					slog.Debug("Could not get relative path, using base name for matching.", "path", path, "error", relErr)
					relPathSlash = filepath.Base(path) // Fallback for matching
				}

				// --- Filter Directories ---
				if d.IsDir() {
					// Check gitignore
					if ignorer != nil && ignorer.MatchesPath(relPathSlash) {
						slog.Debug("Skipping ignored directory (gitignore).", "path", path)
						return fs.SkipDir
					}
					// Check exclude patterns (match full relative path or just name)
					for _, pattern := range excludePatternsToUse {
						matchRel, _ := filepath.Match(pattern, relPathSlash)
						matchBase, _ := filepath.Match(pattern, d.Name())
						if matchRel || matchBase {
							slog.Debug("Skipping excluded directory (pattern).", "path", path, "pattern", pattern)
							return fs.SkipDir
						}
					}
					return nil // Directory is not ignored/excluded, continue descent
				}

				// --- Filter Files ---
				// 1. Check Extension
				ext := strings.ToLower(filepath.Ext(path))
				if _, shouldInclude := extensionsToUse[ext]; !shouldInclude {
					return nil // Skip file with non-matching extension
				}

				// 2. Resolve Absolute Path (needed for uniqueness check)
				absPath, absErr := filepath.Abs(path)
				if absErr != nil {
					slog.Warn("Could not get absolute path for file, skipping.", "path", path, "error", absErr)
					return nil
				}

				// 3. Check if already added manually
				if _, exists := filesToProcess[absPath]; exists {
					return nil // Already included via -f
				}

				// 4. Check .gitignore
				if ignorer != nil && ignorer.MatchesPath(relPathSlash) {
					slog.Debug("Skipping ignored file (gitignore).", "path", path)
					skippedByIgnore++
					return nil
				}

				// 5. Check Explicit Excludes (match full relative path or just basename)
				excludeMatch := false
				for _, pattern := range excludePatternsToUse {
					matchRel, _ := filepath.Match(pattern, relPathSlash)
					matchBase, _ := filepath.Match(pattern, filepath.Base(path))
					if matchRel || matchBase {
						slog.Debug("Skipping excluded file (pattern).", "path", path, "pattern", pattern)
						excludeMatch = true
						break
					}
				}
				if excludeMatch {
					skippedByExclude++
					return nil
				}

				// --- Add File ---
				filesToProcess[absPath] = struct{}{}
				foundInScan++
				return nil
			}) // End WalkDir func

			if walkErr != nil {
				// Log the error that stopped the walk (if any)
				slog.Error("Directory scan terminated unexpectedly.", "error", walkErr)
			}
		} else {
			slog.Info("Skipping directory walk as no extensions were specified for inclusion.")
		}

		// Log scan summary
		slog.Info("Directory scan complete.", "found", foundInScan, "skipped_gitignore", skippedByIgnore, "skipped_exclude", skippedByExclude)

	} else if err != nil { // Error stating target dir
		errMsg := fmt.Sprintf("Target directory '%s' not found or is not accessible.", targetDir)
		slog.Error(errMsg, "error", err)
		// Only return error if NO manual files were provided either
		if len(manualFiles) == 0 {
			return "", errors.New(errMsg)
		} else {
			slog.Warn("Proceeding with only manually specified files due to directory error.")
		}
	} else if !dirInfo.IsDir() { // Target exists but is not a dir
		errMsg := fmt.Sprintf("Target '%s' is not a directory.", targetDir)
		slog.Error(errMsg)
		if len(manualFiles) == 0 {
			return "", errors.New(errMsg)
		} else {
			slog.Warn("Proceeding with only manually specified files as target is not a directory.")
		}
	} else {
		// This case might happen if the directory is valid but no extensions were provided for scanning.
		slog.Info("Target is a directory, but no extensions provided for scanning.")
	}

	// --- 3. Sort and Concatenate ---
	if len(filesToProcess) == 0 {
		slog.Warn("No files found to process (either from scan or manual input).")
		return headerText + "\n", nil // Return just the header
	}

	allFilesSorted := make([]string, 0, len(filesToProcess))
	for absPath := range filesToProcess {
		allFilesSorted = append(allFilesSorted, absPath)
	}
	sort.Strings(allFilesSorted) // Sort by absolute path for consistent order

	totalFiles := len(allFilesSorted)
	processedCount := 0
	slog.Info(fmt.Sprintf("Processing %d unique files...", totalFiles))

	for i, absPath := range allFilesSorted {
		// Determine display path (relative to CWD if possible, POSIX style)
		displayPath := absPath
		if cwdAbs != "" {
			relPath, err := filepath.Rel(cwdAbs, absPath)
			if err == nil {
				displayPath = relPath
			}
		}
		displayPathPosix := filepath.ToSlash(displayPath)

		slog.Debug("Processing file.", "index", i+1, "total", totalFiles, "path", displayPathPosix)

		contentBytes, err := os.ReadFile(absPath)
		if err != nil {
			// Include header/footer for files that failed to read, log warning
			errorMsg := fmt.Sprintf("# Error reading file %s: %v", displayPathPosix, err)
			outputParts = append(outputParts, fmt.Sprintf("\n%s %s\n%s\n%s\n",
				commentStart, displayPathPosix, errorMsg, commentEnd))
			slog.Warn("Error reading file content, skipping content.", "file", displayPathPosix, "error", err)
			continue // Go to next file
		}

		// Handle Empty vs Non-Empty
		if len(contentBytes) == 0 {
			emptyFilePaths = append(emptyFilePaths, displayPathPosix)
			slog.Debug("Found empty file.", "file", displayPathPosix)
		} else {
			// Append non-empty file content with markers
			outputParts = append(outputParts, fmt.Sprintf("\n%s %s\n", commentStart, displayPathPosix))
			outputParts = append(outputParts, string(contentBytes))
			outputParts = append(outputParts, fmt.Sprintf("\n%s\n", commentEnd))
			processedCount++
		}
	}

	// Append the list of empty files, if any
	if len(emptyFilePaths) > 0 {
		slog.Info(fmt.Sprintf("Found %d empty files.", len(emptyFilePaths)))
		sort.Strings(emptyFilePaths) // Sort empty file paths for consistent output
		outputParts = append(outputParts, "\nEmpty files:\n")
		for _, emptyPath := range emptyFilePaths {
			outputParts = append(outputParts, fmt.Sprintf("\t%s\n", emptyPath))
		}
	}

	slog.Info("Finished processing.", "non_empty_files", processedCount, "empty_files", len(emptyFilePaths))
	return strings.Join(outputParts, ""), nil
}

func main() {
	flag.Parse() // Parse flags defined in init()

	// --- Setup Logging ---
	var logLevel slog.Level
	switch strings.ToLower(logLevelStr) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		fmt.Fprintf(os.Stderr, "Invalid log level %q, defaulting to 'info'\n", logLevelStr)
		logLevel = slog.LevelInfo
	}

	logOpts := &slog.HandlerOptions{
		Level: logLevel,
	}
	// Using TextHandler for CLI readability, JSONHandler is better for machine parsing
	handler := slog.NewTextHandler(os.Stderr, logOpts)
	logger := slog.New(handler)
	slog.SetDefault(logger) // Make this logger the default for the application

	// --- Load Configuration (now uses slog) ---
	appConfig, loadErr := loadConfig()
	if loadErr != nil {
		// Error logged within loadConfig, potentially exit or proceed with defaults
		slog.Error("Failed to load configuration, proceeding with potential issues.", "error", loadErr)
		// Depending on severity, might exit here:
		// os.Exit(1)
	}

	// --- Determine final settings based on flags vs config ---
	commentMarker := *appConfig.CommentMarker
	headerText := *appConfig.HeaderText

	// Extensions
	var finalExtensionsList []string
	extensionsFlagProvided := isFlagPassed("e") || isFlagPassed("extensions")
	if extensionsFlagProvided {
		slog.Debug("Using extensions from command line.", "extensions", []string(extensions))
		finalExtensionsList = extensions
	} else {
		slog.Debug("Using extensions from config.", "extensions", appConfig.IncludeExtensions)
		finalExtensionsList = appConfig.IncludeExtensions
	}
	finalExtensionsSet := processExtensions(finalExtensionsList)

	// Exclude Patterns
	var finalExcludePatternsList []string
	excludeFlagProvided := isFlagPassed("x") || isFlagPassed("exclude")
	if excludeFlagProvided {
		slog.Debug("Using exclude patterns from command line.", "patterns", []string(excludePatterns))
		finalExcludePatternsList = excludePatterns
	} else {
		slog.Debug("Using exclude patterns from config.", "patterns", appConfig.ExcludePatterns)
		finalExcludePatternsList = appConfig.ExcludePatterns
	}

	// Gitignore Flag (--no-gitignore) overrides config
	var finalUseGitignore bool
	if isFlagPassed("no-gitignore") {
		finalUseGitignore = !noGitignore // Value comes directly from flag
		slog.Debug("Using gitignore setting from command line.", "use_gitignore", finalUseGitignore)
	} else {
		finalUseGitignore = *appConfig.UseGitignore // Value comes from config
		slog.Debug("Using gitignore setting from config.", "use_gitignore", finalUseGitignore)
	}

	// --- Input Validation ---
	if len(finalExtensionsSet) == 0 && len(manualFiles) == 0 {
		slog.Error("No file extensions specified (via config or flags) and no manual files provided (-f). Nothing to process.")
		fmt.Fprintf(os.Stderr, "\n")
		flag.Usage() // Show usage on error
		os.Exit(1)
	} else if len(finalExtensionsSet) == 0 && len(manualFiles) > 0 {
		slog.Info("No extensions specified for directory scan. Only manually specified files (-f) will be processed.")
	}

	// --- Generate Output ---
	finalOutput, err := generateConcatenatedCode(
		targetDir,
		finalExtensionsSet,
		manualFiles,
		finalExcludePatternsList,
		finalUseGitignore,
		headerText,
		commentMarker,
	)
	if err != nil {
		// Specific error should have been logged within generateConcatenatedCode
		slog.Error("Failed to generate concatenated code.") // General error message
		os.Exit(1)
	}

	// --- Print Output ---
	expectedHeader := headerText + "\n"
	if finalOutput != "" && finalOutput != expectedHeader {
		fmt.Print(finalOutput) // Print to stdout
	} else if finalOutput == expectedHeader {
		slog.Warn("No files were found or included after filtering. Output contains only the header. No output written to stdout.")
	} else {
		slog.Warn("Final output is unexpectedly empty. No output written to stdout.")
	}
}
