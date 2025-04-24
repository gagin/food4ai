// cmd/codecat/main.go
package main

import (
	"errors" // Import errors for IsNotExist check
	"fmt"
	"io"
	"io/fs" // Import fs for ErrNotExist check
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	pflag "github.com/spf13/pflag"
)

const Version = "0.3.0"

var (
	targetDirFlagValue string
	extensions         []string
	manualFiles        []string
	excludePatterns    []string
	noGitignore        bool
	logLevelStr        string
	outputFile         string
	configFileFlag     string
	versionFlag        bool
	noScanFlag         bool // Now used
)

func init() {
	pflag.StringVarP(&targetDirFlagValue, "directory", "d", ".", "Target directory.")
	pflag.StringSliceVarP(&extensions, "extensions", "e", []string{}, "Extensions (overrides config).")
	pflag.StringSliceVarP(&manualFiles, "files", "f", []string{}, "Manual files.")
	pflag.StringSliceVarP(&excludePatterns, "exclude", "x", []string{}, "Exclude patterns (adds to config).")
	pflag.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore processing.")
	pflag.StringVar(&logLevelStr, "loglevel", "info", "Log level (debug, info, warn, error).")
	pflag.StringVarP(&outputFile, "output", "o", "", "Output file path.")
	pflag.StringVarP(&configFileFlag, "config", "c", "", "Custom config file.")
	pflag.BoolVarP(&versionFlag, "version", "v", false, "Print version.")
	pflag.BoolVarP(&noScanFlag, "no-scan", "n", false, "Skip scan, use -f files only.")

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: %s [target_directory]
   or: %s [flags]

Concatenate source code files.

Mode 1: Positional argument [target_directory]. Uses config settings. Conflicts with most flags.
Mode 2: Flags only. Use -d for directory, etc.

Output:
  Default: Code to stdout, Summary/Logs to stderr.
  With -o <file>: Code to <file>, Summary/Logs to stdout.

Flags:
`, os.Args[0], os.Args[0])
		pflag.PrintDefaults()
	}
}

func main() {
	_ = time.Now()
	pflag.Parse()

	if versionFlag {
		fmt.Printf("codecat version %s\n", Version)
		os.Exit(0)
	}

	var logLevel slog.Level
	if err := logLevel.UnmarshalText([]byte(logLevelStr)); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level %q, defaulting to 'info'.\n", logLevelStr)
		logLevel = slog.LevelInfo
	}
	logOpts := &slog.HandlerOptions{Level: logLevel, AddSource: logLevel <= slog.LevelDebug}
	handler := slog.NewTextHandler(os.Stderr, logOpts)
	slog.SetDefault(slog.New(handler))

	appConfig, loadErr := loadConfig(configFileFlag)
	if loadErr != nil {
		slog.Error("Failed to load configuration, using defaults.", "error", loadErr)
		appConfig = defaultConfig
	}

	positionalArgs := pflag.Args()
	finalTargetDirectory := ""
	var conflictingFlagSet bool = false
	var firstConflict string = ""
	metaFlags := map[string]struct{}{"help": {}, "loglevel": {}, "version": {}, "config": {}}
	pflag.Visit(func(f *pflag.Flag) {
		if _, isMeta := metaFlags[f.Name]; !isMeta {
			conflictingFlagSet = true
			if firstConflict == "" {
				firstConflict = f.Name
			}
		}
	})
	if len(positionalArgs) > 1 {
		fmt.Fprintf(os.Stderr, "Refusing execution: Multiple positional arguments: %v.\n", positionalArgs)
		os.Exit(1)
	} else if len(positionalArgs) == 1 {
		if conflictingFlagSet {
			fmt.Fprintf(os.Stderr, "Refusing execution: Cannot mix positional argument '%s' with flag '--%s'.\n", positionalArgs[0], firstConflict)
			os.Exit(1)
		}
		finalTargetDirectory = positionalArgs[0]
		if finalTargetDirectory == "" {
			finalTargetDirectory = "."
		}
		slog.Debug("Using target directory from positional argument.", "path", finalTargetDirectory)
	} else {
		finalTargetDirectory = targetDirFlagValue
		slog.Debug("Using flags mode. Target directory from -d or default.", "path", finalTargetDirectory)
	}

	absTargetDir, err := filepath.Abs(finalTargetDirectory)
	if err != nil {
		slog.Error("Could not determine absolute path.", "path", finalTargetDirectory, "error", err)
		fmt.Fprintf(os.Stderr, "Error: Invalid target directory path '%s': %v\n", finalTargetDirectory, err)
		os.Exit(1)
	}
	finalTargetDirectory = absTargetDir

	// Initial Stat Check - only exit if scan is intended OR no manual files provided
	finalNoScan := noScanFlag // Read flag value
	dirInfo, err := os.Stat(finalTargetDirectory)
	if err != nil {
		logMsg := "Error accessing target directory."
		errMsg := fmt.Sprintf("Error accessing target directory '%s': %v\n", finalTargetDirectory, err)
		if os.IsNotExist(err) {
			logMsg = "Target directory does not exist."
			errMsg = fmt.Sprintf("Error: Target directory '%s' not found.\n", finalTargetDirectory)
		}
		// Only exit if scan is enabled OR if no manual files were given
		if !finalNoScan || len(manualFiles) == 0 {
			slog.Error(logMsg, "path", finalTargetDirectory, "error", err)
			fmt.Fprint(os.Stderr, errMsg)
			os.Exit(1)
		} else {
			// Log warning but allow proceeding for manual files with --no-scan
			slog.Warn(logMsg+", proceeding for manual files (--no-scan active).",
				"path", finalTargetDirectory, "error", err)
		}
	} else if dirInfo != nil && !dirInfo.IsDir() { // Check dirInfo not nil before IsDir
		// Only exit if scan is enabled OR if no manual files were given
		if !finalNoScan || len(manualFiles) == 0 {
			slog.Error("Specified target path is not a directory.", "path", finalTargetDirectory)
			fmt.Fprintf(os.Stderr, "Error: Specified target path '%s' is not a directory.\n", finalTargetDirectory)
			os.Exit(1)
		} else {
			// Log warning but allow proceeding for manual files with --no-scan
			slog.Warn("Target path is not a directory, proceeding for manual files (--no-scan active).",
				"path", finalTargetDirectory)
		}
	}

	commentMarker := *appConfig.CommentMarker
	headerText := *appConfig.HeaderText

	finalUseGitignore := *appConfig.UseGitignore
	if pflag.CommandLine.Changed("no-gitignore") {
		finalUseGitignore = !noGitignore
		slog.Debug("Using gitignore setting from flag.", "use_gitignore", finalUseGitignore)
	} else {
		slog.Debug("Using gitignore setting from config/default.", "use_gitignore", finalUseGitignore)
	}

	finalExtensionsList := appConfig.IncludeExtensions
	if pflag.CommandLine.Changed("extensions") {
		slog.Debug("Using extensions from flag (overrides config).", "extensions", extensions)
		finalExtensionsList = extensions
	} else {
		slog.Debug("Using extensions from config/default.", "extensions", appConfig.IncludeExtensions)
	}
	finalExtensionsSet := processExtensions(finalExtensionsList)
	slog.Debug("Final extension set prepared", "set_keys", mapsKeys(finalExtensionsSet))

	finalExcludePatternsList := appConfig.ExcludePatterns
	if finalExcludePatternsList == nil {
		finalExcludePatternsList = []string{}
	}
	slog.Debug("Exclude patterns from config/default.", "patterns", finalExcludePatternsList)
	if pflag.CommandLine.Changed("exclude") {
		flagExcludes := excludePatterns
		slog.Debug("Adding exclude patterns from flag.", "patterns", flagExcludes)
		finalExcludePatternsList = append(finalExcludePatternsList, flagExcludes...)
	}
	slog.Debug("Final combined exclude patterns", "patterns", finalExcludePatternsList)

	// Input Validation
	if finalNoScan && len(manualFiles) == 0 {
		slog.Error("Processing criteria missing. --no-scan used and no manual files (-f) provided.")
		fmt.Fprintln(os.Stderr, "Error: --no-scan flag used, but no files specified with -f.")
		os.Exit(1)
	}
	if !finalNoScan && len(finalExtensionsSet) == 0 && len(manualFiles) == 0 {
		slog.Error("Processing criteria missing. No extensions or manual files.")
		fmt.Fprintln(os.Stderr, "Error: No file extensions specified and no manual files given.")
		os.Exit(1)
	}

	// --- Generate Output ---
	concatenatedOutput, includedFiles, emptyFiles, errorFiles, totalSize, genErr := generateConcatenatedCode(
		finalTargetDirectory,
		finalExtensionsSet,
		manualFiles,
		finalExcludePatternsList,
		finalUseGitignore,
		headerText,
		commentMarker,
		finalNoScan, // Pass the finalNoScan flag
		// Pass future flags here
	)

	// --- Updated Error Handling ---
	exitCode := 0           // Default success
	proceedToOutput := true // Assume we can print output unless critical error

	if genErr != nil {
		slog.Error("Error reported during file processing.", "error", genErr)

		// Check if it's the specific directory not found/access error
		isDirNotExistErr := errors.Is(genErr, fs.ErrNotExist) && strings.Contains(genErr.Error(), finalTargetDirectory)
		isDirNotDirErr := strings.Contains(genErr.Error(), "is not a directory") // Check for this error too

		// Should we stop completely? Only if scan was intended OR if it's an unexpected error type.
		if !finalNoScan && (isDirNotExistErr || isDirNotDirErr) {
			// Scan was intended, but directory was bad - main() already printed error, set exit code
			slog.Warn("Scan skipped due to inaccessible target directory.")
			exitCode = 1
			proceedToOutput = false // Don't try to write potentially empty/incomplete output
		} else if !(isDirNotExistErr || isDirNotDirErr) {
			// If it's a DIFFERENT error (e.g., walk failed mid-way), print it and exit.
			fmt.Fprintf(os.Stderr, "Error during processing: %v\n", genErr)
			exitCode = 1
			proceedToOutput = false // Don't write potentially incomplete output
		} else if finalNoScan && (isDirNotExistErr || isDirNotDirErr) {
			// If --no-scan was used, directory error is acceptable, log warning but proceed
			slog.Warn("Ignoring target directory access error due to --no-scan.", "error", genErr)
			// proceedToOutput remains true
		}
	}
	// Also mark for exit if file-specific errors occurred, but allow output
	if len(errorFiles) > 0 {
		if exitCode == 0 { // Don't override a critical exit code
			exitCode = 1
		}
	}

	// Determine Output Target and Summary Writer (No change needed)
	var codeWriter io.Writer
	var summaryWriter io.Writer
	var outputFileHandle *os.File
	if outputFile != "" {
		file, errCreate := os.Create(outputFile)
		if errCreate != nil {
			slog.Error("Failed to create output file.", "path", outputFile, "error", errCreate)
			fmt.Fprintf(os.Stderr, "Error creating output file '%s': %v\n", outputFile, errCreate)
			os.Exit(1) // Exit immediately on output file error
		}
		outputFileHandle = file
		codeWriter = file
		summaryWriter = os.Stdout
		slog.Info("Writing concatenated code to file.", "path", outputFile)
	} else {
		codeWriter = os.Stdout
		summaryWriter = os.Stderr
		slog.Info("Writing concatenated code to stdout.")
	}

	// Write Concatenated Code
	// Only write if we determined it's okay to proceed
	if proceedToOutput {
		if concatenatedOutput != "" {
			_, errWrite := io.WriteString(codeWriter, concatenatedOutput)
			if errWrite != nil {
				slog.Error("Failed to write concatenated code.", "error", errWrite)
				fmt.Fprintf(os.Stderr, "Error writing output: %v\n", errWrite)
				if outputFileHandle != nil {
					_ = outputFileHandle.Close()
				}
				os.Exit(1) // Exit immediately on write error
			}
		} else if genErr == nil && exitCode == 0 && len(includedFiles) == 0 && len(manualFiles) == 0 {
			// Only log empty warning if no errors occurred and no files were processed/specified
			slog.Warn("No content generated. Output is empty.")
		}
	} else {
		slog.Warn("Skipping output writing due to processing errors.")
	}

	if outputFileHandle != nil {
		errClose := outputFileHandle.Close()
		if errClose != nil {
			slog.Error("Failed to close output file.", "path", outputFile, "error", errClose)
			// Don't override previous exit code if closing fails
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}

	// Print Summary (No change needed)
	printSummaryTree(includedFiles, emptyFiles, errorFiles, totalSize, finalTargetDirectory, summaryWriter)

	slog.Debug("Execution finished.")

	// Exit with stored code
	os.Exit(exitCode)
}
