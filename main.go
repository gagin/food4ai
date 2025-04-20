// main.go
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log/slog" // Import slog
	"os"
	"path/filepath"
	"strings"
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
func generateConcatenatedCode(dir string, exts map[string]struct{}, manualFiles, excludePatterns []string, useGitignore bool, header, marker string) (string, error) {
	var output strings.Builder
	output.WriteString(header + "\n\n")

	// Validate exclude patterns
	for _, pattern := range excludePatterns {
		_, err := filepath.Match(pattern, "")
		if err != nil {
			slog.Warn("Invalid exclude pattern", "pattern", pattern, "error", err)
			continue
		}
	}

	// Example file scanning
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if _, ok := exts[strings.ToLower(filepath.Ext(path))]; !ok {
			return nil
		}
		for _, pattern := range excludePatterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
				slog.Debug("Skipping excluded file", "path", path, "pattern", pattern)
				return nil
			}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("Error reading file", "path", path, "error", err)
			output.WriteString(fmt.Sprintf("%s %s\n# Error reading file: %v\n%s\n", marker, path, err, marker))
			return nil
		}
		output.WriteString(fmt.Sprintf("%s %s\n%s\n%s\n", marker, path, content, marker))
		return nil
	})
	if err != nil {
		return "", err
	}

	return output.String(), nil
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
