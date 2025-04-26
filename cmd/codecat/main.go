// cmd/codecat/main.go
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	pflag "github.com/spf13/pflag"
)

const Version = "0.4.0" // Incremented version for log level default change

var (
	targetDirFlagValue string
	extensions         []string
	manualFiles        []string
	excludePatterns    []string
	noGitignore        bool
	logLevelStr        string // Flag variable
	outputFile         string
	configFileFlag     string
	versionFlag        bool
	noScanFlag         bool
)

func init() {
	pflag.StringVarP(&targetDirFlagValue, "directory", "d", "",
		"Target directory/directories to scan (comma-separated, optional).")
	pflag.StringSliceVarP(&extensions, "extensions", "e", []string{},
		"Extensions to include (overrides config, comma-separated).")
	pflag.StringSliceVarP(&manualFiles, "files", "f", []string{},
		"Manual files to include (paths relative to CWD, comma-separated).")
	pflag.StringSliceVarP(&excludePatterns, "exclude", "x", []string{},
		"CWD-relative path glob patterns to exclude (adds to .codecat_exclude, comma-separated).")
	pflag.BoolVar(&noGitignore, "no-gitignore", false,
		"Disable .gitignore processing.")
	// Default log level changed to WARN
	pflag.StringVar(&logLevelStr, "loglevel", "warn",
		"Log level (debug, info, warn, error).")
	pflag.StringVarP(&outputFile, "output", "o", "",
		"Output file path (instead of stdout). Summary/Logs go to stderr/stdout respectively.")
	pflag.StringVarP(&configFileFlag, "config", "c", "",
		"Custom config file path.")
	pflag.BoolVarP(&versionFlag, "version", "v", false,
		"Print version and exit.")
	pflag.BoolVarP(&noScanFlag, "no-scan", "n", false,
		"Skip directory scanning. Requires -f flag.")

	pflag.Usage = func() {
		// Usage string formatting remains the same
		fmt.Fprintf(os.Stderr, `Usage: %s [target_directory] [flags]
   or: %s [flags]

Concatenate source code files relative to the Current Working Directory (CWD).

Modes:
1. Positional Argument: 'codecat <dir>' implies scanning ONLY <dir>. Cannot be used with -d.
2. Flags Only: Use '-d <dirs>' to specify scan directories (comma-separated).
   If -d is omitted and -n (no-scan) is NOT used, CWD ('.') is scanned by default.
   If -n is used, -d is ignored.

Exclusion Hierarchy:
1. Basename excludes from global config (%s).
2. CWD-relative excludes from '.codecat_exclude' in CWD.
3. CWD-relative excludes from '-x' flag.
4. .gitignore rules (if enabled).

Output:
- Code to stdout (default) or -o <file>.
- Summary/Logs to stderr (default) or stdout (if -o is used).

Flags:
`, os.Args[0], os.Args[0], filepath.Join("~", ".config", "codecat", "config.toml"))
		pflag.PrintDefaults()
	}
}

// parseCommaSeparatedSlice remains the same
func parseCommaSeparatedSlice(flagValues []string) []string {
	if flagValues == nil {
		return []string{}
	}
	result := []string{}
	for _, val := range flagValues {
		parts := strings.Split(val, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
	}
	return result
}

// loadProjectExcludes remains the same
func loadProjectExcludes(cwd string) []string {
	excludeFilePath := filepath.Join(cwd, ".codecat_exclude")
	patterns := []string{}

	file, err := os.Open(excludeFilePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			slog.Debug("No .codecat_exclude file found in CWD.", "path", excludeFilePath)
		} else {
			slog.Warn("Error opening .codecat_exclude file, ignoring.",
				"path", excludeFilePath, "error", err)
		}
		return patterns
	}
	defer file.Close()

	// Log at INFO level as it's a significant action if the file exists
	slog.Info("Loading project-specific excludes.", "path", excludeFilePath)
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, errMatch := filepath.Match(line, "a/b"); errMatch != nil {
			slog.Warn("Invalid pattern in .codecat_exclude, skipping.",
				"path", excludeFilePath, "line", lineNumber, "pattern", line, "error", errMatch)
			continue
		}
		patterns = append(patterns, line)
	}

	if err := scanner.Err(); err != nil {
		slog.Warn("Error reading .codecat_exclude file, using patterns read so far.",
			"path", excludeFilePath, "error", err)
	}

	slog.Debug("Loaded project exclude patterns", "patterns", patterns)
	return patterns
}

func main() {
	startTime := time.Now()
	pflag.Parse()

	if versionFlag {
		fmt.Printf("codecat version %s\n", Version)
		os.Exit(0)
	}

	// --- Setup Logging ---
	var logLevel slog.Level
	// Update the default level in the error message
	if err := logLevel.UnmarshalText([]byte(logLevelStr)); err != nil {
		slog.Error("Invalid log level specified, using 'warn'.",
			"input", logLevelStr, "error", err)
		logLevel = slog.LevelWarn // Default to WARN if parsing fails
	}
	logOpts := &slog.HandlerOptions{Level: logLevel, AddSource: logLevel <= slog.LevelDebug}
	logOutput := os.Stderr
	if outputFile != "" {
		logOutput = os.Stdout
	}
	handler := slog.NewTextHandler(logOutput, logOpts)
	slog.SetDefault(slog.New(handler))
	slog.Debug("Logging setup complete.", "level", logLevel.String())

	// --- Get CWD ---
	cwd, errCwd := os.Getwd()
	if errCwd != nil {
		slog.Error("Failed to get current working directory. Cannot proceed.", "error", errCwd)
		fmt.Fprintf(os.Stderr, "Fatal Error: Could not determine current working directory: %v\n", errCwd)
		os.Exit(1)
	}
	slog.Debug("Current working directory determined.", "cwd", cwd)

	// --- Load Configuration ---
	appConfig, loadErr := loadConfig(configFileFlag)
	if loadErr != nil {
		slog.Error("Fatal error loading configuration.", "error", loadErr)
		fmt.Fprintf(os.Stderr, "Fatal Error loading configuration: %v\n", loadErr)
		os.Exit(1)
	}

	// --- Determine Scan Directories ---
	scanDirs := []string{}
	positionalArgs := pflag.Args()
	targetDirFlagProvided := pflag.CommandLine.Changed("directory")

	if len(positionalArgs) > 1 {
		slog.Error("Too many positional arguments.", "args", positionalArgs)
		fmt.Fprintf(os.Stderr,
			"Error: Expected at most one positional argument (target directory), got %d: %v\n",
			len(positionalArgs), positionalArgs)
		pflag.Usage()
		os.Exit(1)
	}

	if len(positionalArgs) == 1 {
		if targetDirFlagProvided {
			slog.Error("Cannot use both positional argument and -d flag.",
				"positional", positionalArgs[0], "flag", targetDirFlagValue)
			fmt.Fprintf(os.Stderr,
				"Error: Cannot specify a target directory via positional argument ('%s') and the -d flag ('%s') simultaneously.\n",
				positionalArgs[0], targetDirFlagValue)
			pflag.Usage()
			os.Exit(1)
		}
		scanDirs = []string{positionalArgs[0]}
		slog.Debug("Using scan directory from positional argument.", "dir", scanDirs[0])
	} else if targetDirFlagProvided {
		scanDirs = parseCommaSeparatedSlice([]string{targetDirFlagValue})
		slog.Debug("Using scan directories from -d flag.", "dirs", scanDirs)
	} else if !noScanFlag {
		scanDirs = []string{"."}
		slog.Debug("Defaulting to scan CWD.", "dir", scanDirs[0])
	} else {
		slog.Debug("No scan directories specified and --no-scan is active.")
	}

	absScanDirs := make([]string, 0, len(scanDirs))
	for _, dir := range scanDirs {
		absDir := filepath.Join(cwd, dir)
		if !filepath.IsAbs(dir) {
			// absDir calculated above is correct
		} else {
			absDir = dir // It was already absolute
		}
		absScanDirs = append(absScanDirs, filepath.Clean(absDir))
	}
	scanDirs = absScanDirs
	if len(scanDirs) > 0 {
		slog.Debug("Resolved absolute scan directories.", "dirs", scanDirs)
	}

	// --- Process Flags and Config Values ---
	finalNoScan := noScanFlag
	finalManualFiles := parseCommaSeparatedSlice(manualFiles)
	if len(finalManualFiles) > 0 {
		slog.Debug("Using manual files.", "files", finalManualFiles)
	}
	finalFlagExcludes := parseCommaSeparatedSlice(excludePatterns)
	if len(finalFlagExcludes) > 0 {
		slog.Debug("Using command-line CWD-relative excludes.", "patterns", finalFlagExcludes)
	}
	projectExcludes := loadProjectExcludes(cwd)
	basenameExcludes := appConfig.ExcludeBasenames

	finalUseGitignore := *appConfig.UseGitignore
	if pflag.CommandLine.Changed("no-gitignore") {
		finalUseGitignore = !noGitignore
		slog.Debug("Overriding gitignore setting via flag.", "use_gitignore", finalUseGitignore)
	} else {
		slog.Debug("Using gitignore setting from config/default.", "use_gitignore", finalUseGitignore)
	}

	finalExtensionsList := appConfig.IncludeExtensions
	if pflag.CommandLine.Changed("extensions") {
		finalExtensionsList = parseCommaSeparatedSlice(extensions)
		slog.Debug("Overriding extensions via flag.", "extensions", finalExtensionsList)
	} else {
		slog.Debug("Using extensions from config/default.", "extensions", finalExtensionsList)
	}
	finalExtensionsSet := processExtensions(finalExtensionsList)
	slog.Debug("Final extension set prepared.", "set_keys", mapsKeys(finalExtensionsSet))

	commentMarker := *appConfig.CommentMarker
	headerText := *appConfig.HeaderText

	// --- Input Validation ---
	if finalNoScan && len(finalManualFiles) == 0 {
		slog.Error("Processing criteria missing. --no-scan used and no manual files (-f) provided.")
		fmt.Fprintln(os.Stderr, "Error: --no-scan flag requires specifying files to include with -f.")
		os.Exit(1)
	}
	if !finalNoScan && len(finalExtensionsSet) == 0 && len(finalManualFiles) == 0 && len(scanDirs) > 0 {
		slog.Error(
			"Processing criteria missing. Scan requested but no extensions/manual files given.")
		fmt.Fprintln(os.Stderr,
			"Error: No file extensions specified (config or -e) and no manual files (-f) given, but a scan was requested.")
		os.Exit(1)
	}

	// --- Generate Output ---
	// Log start at INFO level as it's a key operation beginning
	slog.Info("Starting code concatenation process.")
	concatenatedOutput, includedFiles, emptyFiles, errorFiles, totalSize, genErr := generateConcatenatedCode(
		cwd,
		scanDirs,
		finalExtensionsSet,
		finalManualFiles,
		basenameExcludes,
		projectExcludes,
		finalFlagExcludes,
		finalUseGitignore,
		headerText, commentMarker,
		finalNoScan,
	)

	// --- Error Handling After Generation ---
	exitCode := 0
	if genErr != nil {
		// generateConcatenatedCode logs specifics
		slog.Error("Error(s) reported during file processing.", "error", genErr)
		exitCode = 1
	}
	if len(errorFiles) > 0 && exitCode == 0 {
		exitCode = 1
		// Log at WARN level as processing finished but with issues
		slog.Warn("Individual file errors were encountered during processing.")
	}

	// --- Determine Output Target ---
	var codeWriter io.Writer
	var summaryWriter io.Writer = logOutput
	var outputFileHandle *os.File
	if outputFile != "" {
		var errCreate error
		outputFileHandle, errCreate = os.Create(outputFile)
		if errCreate != nil {
			slog.Error("Failed to create output file, writing to stdout instead.",
				"path", outputFile, "error", errCreate)
			fmt.Fprintf(os.Stderr, "Error creating output file '%s': %v\n", outputFile, errCreate)
			fmt.Fprintln(os.Stderr, "Writing code output to standard output.")
			codeWriter = os.Stdout
			if exitCode == 0 {
				exitCode = 1
			}
		} else {
			codeWriter = outputFileHandle
			// Log at INFO level as it's a key successful action
			slog.Info("Writing concatenated code to file.", "path", outputFile)
		}
	} else {
		codeWriter = os.Stdout
		// Log at INFO level as it's a key successful action
		slog.Info("Writing concatenated code to stdout.")
	}

	// --- Write Concatenated Code ---
	if concatenatedOutput != "" {
		_, errWrite := io.WriteString(codeWriter, concatenatedOutput)
		if errWrite != nil {
			slog.Error("Failed to write concatenated code output.", "error", errWrite)
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", errWrite)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	} else if exitCode == 0 && len(includedFiles) == 0 {
		// Log at WARN level as it's potentially unexpected but not an error
		slog.Warn("No content generated. Output is empty.")
	}

	if outputFileHandle != nil {
		errClose := outputFileHandle.Close()
		if errClose != nil {
			slog.Error("Failed to close output file.", "path", outputFile, "error", errClose)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}

	// --- Print Summary ---
	printSummaryTree(includedFiles, emptyFiles, errorFiles, totalSize, cwd, summaryWriter)

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	// Log at INFO level as it's the final status
	slog.Info("Execution finished.", "duration", duration.String())

	os.Exit(exitCode)
}
