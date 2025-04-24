// cmd/codecat/main.go
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	pflag "github.com/spf13/pflag"
	// Other imports might be needed depending on final logic
)

const Version = "0.2.2" // Keep original version for now

// --- Global Variables for Flags (Original Set) ---
var (
	targetDirFlagValue string
	extensions         []string
	manualFiles        []string
	excludePatterns    []string
	noGitignore        bool // Keep original flag for now
	logLevelStr        string
	outputFile         string
	configFileFlag     string
	versionFlag        bool
)

func init() {
	// Define command-line flags using pflag (Original Set)
	pflag.StringVarP(&targetDirFlagValue, "directory", "d", ".", "Target directory to scan.")
	pflag.StringSliceVarP(&extensions, "extensions", "e", []string{}, "Comma-separated file extensions (overrides config).")
	pflag.StringSliceVarP(&manualFiles, "files", "f", []string{}, "Comma-separated specific file paths.")
	pflag.StringSliceVarP(&excludePatterns, "exclude", "x", []string{}, "Comma-separated glob patterns to exclude (adds to config).")
	pflag.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore processing.") // Will be replaced later
	pflag.StringVar(&logLevelStr, "loglevel", "info", "Set logging verbosity (debug, info, warn, error).")
	pflag.StringVarP(&outputFile, "output", "o", "", "Output file path.")
	pflag.StringVarP(&configFileFlag, "config", "c", "", "Path to a custom configuration file.")
	pflag.BoolVarP(&versionFlag, "version", "v", false, "Print version and exit.")

	// Define original usage message
	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: %s [target_directory]
   or: %s [flags]

Concatenate source code files.

Flags:
`, os.Args[0], os.Args[0])
		pflag.PrintDefaults()
	}
}

// --- Main Execution ---
func main() {
	_ = time.Now()
	pflag.Parse()

	if versionFlag {
		fmt.Printf("codecat version %s\n", Version)
		os.Exit(0)
	}

	// Setup Logging
	var logLevel slog.Level
	if err := logLevel.UnmarshalText([]byte(logLevelStr)); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level %q, defaulting to 'info'.\n", logLevelStr)
		logLevel = slog.LevelInfo
	}
	logOpts := &slog.HandlerOptions{Level: logLevel, AddSource: logLevel <= slog.LevelDebug}
	handler := slog.NewTextHandler(os.Stderr, logOpts)
	slog.SetDefault(slog.New(handler))

	// Load Configuration (Function defined in config.go)
	appConfig, loadErr := loadConfig(configFileFlag)
	if loadErr != nil {
		slog.Error("Failed to load configuration, using defaults.", "error", loadErr)
		appConfig = defaultConfig // Defined in config.go
	}

	// Argument Mode Validation
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
		fmt.Fprintf(os.Stderr, "Refusing execution: Multiple positional arguments provided: %v.\n", positionalArgs)
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

	// Validate Final Target Directory
	absTargetDir, err := filepath.Abs(finalTargetDirectory)
	if err != nil {
		slog.Error("Could not determine absolute path.", "path", finalTargetDirectory, "error", err)
		fmt.Fprintf(os.Stderr, "Error: Invalid target directory path '%s': %v\n", finalTargetDirectory, err)
		os.Exit(1)
	}
	finalTargetDirectory = absTargetDir

	// Initial Stat Check for early user feedback
	dirInfo, err := os.Stat(finalTargetDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: Target directory '%s' not found.\n", finalTargetDirectory)
		} else {
			fmt.Fprintf(os.Stderr, "Error accessing target directory '%s': %v\n", finalTargetDirectory, err)
		}
		os.Exit(1)
	}
	if !dirInfo.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: Specified target path '%s' is not a directory.\n", finalTargetDirectory)
		os.Exit(1)
	}

	// Determine final settings
	// Ensure pointers from config have defaults if necessary (using appConfig)
	commentMarker := ""
	if appConfig.CommentMarker != nil {
		commentMarker = *appConfig.CommentMarker
	} else {
		commentMarker = *defaultConfig.CommentMarker
	}
	headerText := ""
	if appConfig.HeaderText != nil {
		headerText = *appConfig.HeaderText
	} else {
		headerText = *defaultConfig.HeaderText
	}
	finalUseGitignore := false // Default to false if nil
	if appConfig.UseGitignore != nil {
		finalUseGitignore = *appConfig.UseGitignore
	} else {
		finalUseGitignore = *defaultConfig.UseGitignore
	}
	// Apply command-line override
	if pflag.CommandLine.Changed("no-gitignore") {
		finalUseGitignore = !noGitignore
	}
	slog.Debug("Final useGitignore setting", "value", finalUseGitignore)

	// Determine Extensions (Flag overrides Config)
	finalExtensionsList := appConfig.IncludeExtensions
	if pflag.CommandLine.Changed("extensions") {
		slog.Debug("Using extensions from command line flag (overrides config).", "extensions", extensions)
		finalExtensionsList = extensions
	} else {
		slog.Debug("Using extensions from config/default.", "extensions", appConfig.IncludeExtensions)
	}
	finalExtensionsSet := processExtensions(finalExtensionsList)                         // Defined in helpers.go
	slog.Debug("Final extension set prepared", "set_keys", mapsKeys(finalExtensionsSet)) // Defined in helpers.go

	// Determine Exclude Patterns (Flag ADDS to Config)
	finalExcludePatternsList := appConfig.ExcludePatterns
	if finalExcludePatternsList == nil {
		finalExcludePatternsList = []string{}
	}
	slog.Debug("Exclude patterns from config/default.", "patterns", finalExcludePatternsList)
	if pflag.CommandLine.Changed("exclude") {
		flagExcludes := excludePatterns
		slog.Debug("Adding exclude patterns from command line flag.", "patterns", flagExcludes)
		finalExcludePatternsList = append(finalExcludePatternsList, flagExcludes...)
	}
	slog.Debug("Final combined exclude patterns", "patterns", finalExcludePatternsList)

	// Input Validation
	if len(finalExtensionsSet) == 0 && len(manualFiles) == 0 {
		slog.Error("Processing criteria missing. No extensions or manual files.")
		fmt.Fprintln(os.Stderr, "Error: No file extensions specified (use -e or config) and no manual files given (-f).")
		os.Exit(1)
	}

	// --- Generate Output (Call function defined in walk.go) ---
	concatenatedOutput, includedFiles, emptyFiles, errorFiles, totalSize, genErr := generateConcatenatedCode(
		finalTargetDirectory,
		finalExtensionsSet,
		manualFiles,
		finalExcludePatternsList,
		finalUseGitignore,
		headerText,
		commentMarker,
		// Add future flags here:
		// "recursive", // example mode
		// false, // includeFileList
		// false, // includeEmptyFilesList
		// false, // noScan
	)

	// Handle error from generation
	if genErr != nil {
		slog.Error("Error during file processing.", "error", genErr)
		if !(os.IsNotExist(genErr)) { // Avoid duplicate message if main already checked
			fmt.Fprintf(os.Stderr, "Error during processing: %v\n", genErr)
		}
		os.Exit(1)
	}

	// Determine Output Target and Summary Writer
	var codeWriter io.Writer
	var summaryWriter io.Writer
	var outputFileHandle *os.File
	if outputFile != "" {
		file, errCreate := os.Create(outputFile)
		if errCreate != nil {
			slog.Error("Failed to create output file.", "path", outputFile, "error", errCreate)
			fmt.Fprintf(os.Stderr, "Error creating output file '%s': %v\n", outputFile, errCreate)
			os.Exit(1)
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
	if concatenatedOutput != "" {
		_, errWrite := io.WriteString(codeWriter, concatenatedOutput)
		if errWrite != nil {
			slog.Error("Failed to write concatenated code.", "error", errWrite)
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", errWrite)
			if outputFileHandle != nil {
				_ = outputFileHandle.Close()
			}
			os.Exit(1)
		}
	} else if genErr == nil && len(includedFiles) == 0 && len(manualFiles) == 0 {
		slog.Warn("No content generated. Output is empty.")
	}

	if outputFileHandle != nil {
		errClose := outputFileHandle.Close()
		if errClose != nil {
			slog.Error("Failed to close output file.", "path", outputFile, "error", errClose)
		}
	}

	// Print Summary (Function defined in summary.go)
	printSummaryTree(includedFiles, emptyFiles, errorFiles, totalSize, finalTargetDirectory, summaryWriter)

	slog.Debug("Execution finished.")

	if len(errorFiles) > 0 {
		os.Exit(1)
	}
}
