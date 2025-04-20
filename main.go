package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitignorelib "github.com/sabhiram/go-gitignore"
	pflag "github.com/spf13/pflag"
)

const Version = 0.2.1" // major.minor.patch

// --- File Info Struct ---
type FileInfo struct {
	Path     string
	Size     int64
	IsManual bool
}

// --- Global Variables for Flags ---
var (
	targetDirFlagValue string
	extensions         []string
	manualFiles        []string
	excludePatterns    []string
	noGitignore        bool
	logLevelStr        string
	outputFile         string
	configFileFlag     string
	versionFlag        bool // Added for -v/--version
)

func init() {
	// Define command-line flags using pflag
	pflag.StringVarP(&targetDirFlagValue, "directory", "d", ".", "Target directory to scan (use this OR a positional argument, not both).")
	pflag.StringSliceVarP(&extensions, "extensions", "e", defaultConfig.IncludeExtensions, fmt.Sprintf("Comma-separated file extensions to include (requires flags mode). Default: %v", defaultConfig.IncludeExtensions))
	pflag.StringSliceVarP(&manualFiles, "files", "f", []string{}, "Comma-separated specific file paths to include manually (requires flags mode).")
	pflag.StringSliceVarP(&excludePatterns, "exclude", "x", defaultConfig.ExcludePatterns, fmt.Sprintf("Comma-separated glob patterns to exclude (requires flags mode). Default: %v", defaultConfig.ExcludePatterns))
	pflag.BoolVar(&noGitignore, "no-gitignore", !*defaultConfig.UseGitignore, fmt.Sprintf("Disable .gitignore processing (requires flags mode). Config default: %t", *defaultConfig.UseGitignore))
	pflag.StringVar(&logLevelStr, "loglevel", "info", "Set logging verbosity (debug, info, warning, error).")
	pflag.StringVarP(&outputFile, "output", "o", "", "Output file path (writes code to file, summary to stdout).")
	pflag.StringVarP(&configFileFlag, "config", "c", "", "Path to a custom configuration file (overrides default ~/.config/codecat/config.toml).")
	pflag.BoolVarP(&versionFlag, "version", "v", false, "Print version and exit.") // Added for -v/--version

	// Define custom usage message using pflag's Usage variable
	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: %s [target_directory]
   or: %s [flags]

Concatenate source code files into a single output stream or file.

Mode 1: Provide a single [target_directory] positional argument.
        Scans this directory using default or config file settings.
        Cannot be combined with flags like -d, -e, -f, -x, -o, -c, --config, --no-gitignore.

Mode 2: Use flags only (no positional arguments).
        Use -d to specify directory (defaults to '.'), -e, -f, -x, -o, -c, --config etc.

Output:
  - Default: Code to stdout, Summary/Logs to stderr.
  - With -o <file>: Code to <file>, Summary/Logs to stdout.

Flags:
`, os.Args[0], os.Args[0])
		pflag.PrintDefaults()
	}
}

// --- Main Execution ---
func main() {
	// Current time: Sunday, April 20, 2025 at 12:35:54 AM PDT
	_ = time.Now()

	pflag.Parse()

	// Handle --version/-v flag
	if versionFlag {
		fmt.Printf("codecat version %s\n", Version)
		os.Exit(0)
	}

	// --- Setup Logging (Always to Stderr) ---
	var logLevel slog.Level
	if err := logLevel.UnmarshalText([]byte(logLevelStr)); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level %q specified, defaulting to 'info'. Use debug, info, warn, or error.\n", logLevelStr)
		logLevel = slog.LevelInfo
	}
	logOpts := &slog.HandlerOptions{Level: logLevel, AddSource: logLevel <= slog.LevelDebug}
	handler := slog.NewTextHandler(os.Stderr, logOpts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// --- Load Configuration ---
	configFlagPassed := pflag.CommandLine.Changed("config")
	appConfig, loadErr := loadConfig(configFileFlag)
	if loadErr != nil {
		slog.Error("Failed to load configuration.", "error", loadErr)
		if configFlagPassed {
			fmt.Fprintf(os.Stderr, "Error: Could not load specified configuration file '%s': %v\n", configFileFlag, loadErr)
			os.Exit(1)
		} else {
			slog.Warn("Proceeding with default settings due to config load issue.")
		}
	}
	// *** DEBUG: Log loaded extensions ***
	slog.Debug("Loaded config extensions check", "config_path_flag", configFileFlag, "loaded_extensions", appConfig.IncludeExtensions)

	// --- Argument Mode Validation ---
	positionalArgs := pflag.Args()
	finalTargetDirectory := ""

	var conflictingFlagSet bool = false
	var firstConflict string = ""
	metaFlags := map[string]struct{}{
		"help":     {},
		"loglevel": {},
	}

	pflag.Visit(func(f *pflag.Flag) {
		if _, isMeta := metaFlags[f.Name]; !isMeta {
			conflictingFlagSet = true
			if firstConflict == "" {
				firstConflict = f.Name
			}
		}
	})

	if len(positionalArgs) > 1 {
		refusalMsg := fmt.Sprintf("Refusing execution: Multiple positional arguments provided: %v.\nUse either a single directory argument OR flags, not both. Run with --help for usage details.", positionalArgs)
		fmt.Fprintln(os.Stderr, refusalMsg)
		os.Exit(1)
	} else if len(positionalArgs) == 1 {
		if conflictingFlagSet {
			refusalMsg := fmt.Sprintf("Refusing execution: Cannot mix positional argument '%s' with flag '--%s'.\nPlease use flags exclusively for complex commands. Run with --help for usage details.", positionalArgs[0], firstConflict)
			fmt.Fprintln(os.Stderr, refusalMsg)
			os.Exit(1)
		} else {
			finalTargetDirectory = positionalArgs[0]
			if finalTargetDirectory == "" {
				finalTargetDirectory = "."
			}
			slog.Debug("Using target directory from positional argument.", "path", finalTargetDirectory)
		}
	} else {
		finalTargetDirectory = targetDirFlagValue
		slog.Debug("Using flags mode. Target directory from -d or default.", "path", finalTargetDirectory)
	}

	// --- Validate Final Target Directory ---
	if finalTargetDirectory == "" {
		slog.Error("Internal configuration error: Target directory is empty.")
		os.Exit(1)
	}
	absTargetDir, err := filepath.Abs(finalTargetDirectory)
	if err != nil {
		slog.Error("Could not determine absolute path for target directory.", "path", finalTargetDirectory, "error", err)
		fmt.Fprintf(os.Stderr, "Error determining absolute path for '%s': %v\n", finalTargetDirectory, err)
		os.Exit(1)
	}
	finalTargetDirectory = absTargetDir

	dirInfo, err := os.Stat(finalTargetDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Error("Target directory does not exist.", "path", finalTargetDirectory)
			fmt.Fprintf(os.Stderr, "Error: Target directory '%s' not found.\n", finalTargetDirectory)
		} else {
			slog.Error("Error accessing target directory.", "path", finalTargetDirectory, "error", err)
			fmt.Fprintf(os.Stderr, "Error accessing target directory '%s': %v\n", finalTargetDirectory, err)
		}
		os.Exit(1)
	}
	if !dirInfo.IsDir() {
		slog.Error("Specified target path is not a directory.", "path", finalTargetDirectory)
		fmt.Fprintf(os.Stderr, "Error: Specified target path '%s' is not a directory.\n", finalTargetDirectory)
		os.Exit(1)
	}

	// --- Determine final settings (extensions, excludes, gitignore) ---
	commentMarker := *appConfig.CommentMarker
	headerText := *appConfig.HeaderText

	var finalExtensionsList []string
	if pflag.CommandLine.Changed("extensions") {
		slog.Debug("Using extensions from command line flag.", "extensions", extensions)
		finalExtensionsList = extensions
	} else if len(appConfig.IncludeExtensions) > 0 {
		slog.Debug("Using extensions from loaded config.", "config_extensions", appConfig.IncludeExtensions)
		finalExtensionsList = appConfig.IncludeExtensions
	} else {
		slog.Debug("No extensions specified via flag or config. Using hardcoded default.", "default_extensions", defaultConfig.IncludeExtensions)
		finalExtensionsList = defaultConfig.IncludeExtensions
	}
	finalExtensionsSet := processExtensions(finalExtensionsList)
	// *** DEBUG: Log final extension set being used ***
	debugExtensions := mapsKeys(finalExtensionsSet) // Get keys for logging
	slog.Debug("Final extension set prepared", "set_keys", debugExtensions)

	var finalExcludePatternsList []string
	if pflag.CommandLine.Changed("exclude") {
		slog.Debug("Using exclude patterns from command line flag.", "patterns", excludePatterns)
		finalExcludePatternsList = excludePatterns
	} else {
		slog.Debug("Using exclude patterns from loaded config.", "patterns", appConfig.ExcludePatterns)
		finalExcludePatternsList = appConfig.ExcludePatterns
		if finalExcludePatternsList == nil {
			finalExcludePatternsList = []string{}
		}
	}
	// *** DEBUG: Log final exclude patterns ***
	slog.Debug("Final exclude patterns", "patterns", finalExcludePatternsList)

	var finalUseGitignore bool
	if pflag.CommandLine.Changed("no-gitignore") {
		finalUseGitignore = !noGitignore
		slog.Debug("Using gitignore setting from command line flag.", "use_gitignore", finalUseGitignore)
	} else {
		finalUseGitignore = *appConfig.UseGitignore
		slog.Debug("Using gitignore setting from loaded config.", "use_gitignore", finalUseGitignore)
	}
	// *** DEBUG: Log final gitignore setting ***
	slog.Debug("Final useGitignore setting", "value", finalUseGitignore)

	// --- Input Validation ---
	if len(finalExtensionsSet) == 0 && len(manualFiles) == 0 {
		slog.Error("Processing criteria missing. No file extensions specified and no manual files provided. Nothing to process.")
		os.Exit(1)
	}

	// --- Generate Output ---
	concatenatedOutput, includedFiles, emptyFiles, errorFiles, totalSize, genErr := generateConcatenatedCode(
		finalTargetDirectory,
		finalExtensionsSet, // Pass the final set
		manualFiles,
		finalExcludePatternsList, // Pass the final list
		finalUseGitignore,        // Pass the final bool
		headerText,               // Pass loaded/default value
		commentMarker,            // Pass loaded/default value
	)
	if genErr != nil {
		slog.Error("Error during file processing, results may be incomplete.", "error", genErr)
	}

	// --- Determine Output Target and Summary Writer ---
	var codeWriter io.Writer
	var summaryWriter io.Writer
	var outputFileHandle *os.File

	if outputFile != "" {
		file, errCreate := os.Create(outputFile)
		if errCreate != nil {
			slog.Error("Failed to create output file.", "path", outputFile, "error", errCreate)
			fmt.Fprintf(os.Stderr, "Error: Failed to create output file '%s': %v\n", outputFile, errCreate)
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

	// --- Write Concatenated Code ---
	if concatenatedOutput != "" {
		_, errWrite := io.WriteString(codeWriter, concatenatedOutput)
		if errWrite != nil {
			slog.Error("Failed to write concatenated code.", "error", errWrite)
			fmt.Fprintf(os.Stderr, "Error: Failed to write output: %v\n", errWrite)
			if outputFileHandle != nil {
				outputFileHandle.Close()
			}
			os.Exit(1)
		}
	} else if genErr == nil && len(includedFiles) == 0 {
		slog.Warn("No content generated. Output is empty (no matching files found or files were empty/unreadable).")
	}

	if outputFileHandle != nil {
		errClose := outputFileHandle.Close()
		if errClose != nil {
			slog.Error("Failed to close output file.", "path", outputFile, "error", errClose)
		}
	}

	// --- Print Summary ---
	printSummaryTree(includedFiles, emptyFiles, errorFiles, totalSize, finalTargetDirectory, summaryWriter)

	slog.Debug("Execution finished.")

	if genErr != nil || len(errorFiles) > 0 {
		os.Exit(1)
	}
}

// --- Helper Functions & Structs ---

// processExtensions processes a list of extension strings into a set for quick lookup.
func processExtensions(extList []string) map[string]struct{} {
	processed := make(map[string]struct{})
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
			processed[cleaned] = struct{}{}
		}
	}
	return processed
}

// generateConcatenatedCode implementation - ADDING DEBUG LOGS INSIDE WALK
func generateConcatenatedCode(
	dir string,
	exts map[string]struct{}, // This is the final set passed from main
	manualFilePaths []string,
	excludePatterns []string,
	useGitignore bool,
	header, marker string,
) (output string, includedFiles []FileInfo, emptyFiles []string, errorFiles map[string]error, totalSize int64, err error) {

	var outputBuilder strings.Builder
	if header != "" {
		outputBuilder.WriteString(header + "\n\n")
	} else {
		outputBuilder.WriteString("\n")
	}

	includedFiles = make([]FileInfo, 0)
	emptyFiles = make([]string, 0)
	errorFiles = make(map[string]error)

	validExcludePatterns := make([]string, 0, len(excludePatterns))
	for _, pattern := range excludePatterns {
		_, errMatch := filepath.Match(pattern, "")
		if errMatch != nil {
			slog.Warn("Invalid exclude pattern, it will be ignored.", "pattern", pattern, "error", errMatch)
		} else {
			validExcludePatterns = append(validExcludePatterns, pattern)
		}
	}
	excludePatterns = validExcludePatterns

	slog.Info("Starting file scan.", "directory", dir)

	processedFiles := make(map[string]bool)
	totalSize = 0

	// --- Process Manually Added Files ---
	// (Manual file processing logic remains the same - already has debug logs)
	if len(manualFilePaths) > 0 {
		slog.Debug("Processing manually specified files.", "count", len(manualFilePaths))
		for _, manualPath := range manualFilePaths {
			absManualPath, errAbs := filepath.Abs(manualPath)
			if errAbs != nil {
				slog.Warn("Could not get absolute path for manual file, skipping.", "path", manualPath, "error", errAbs)
				errorFiles[manualPath] = errAbs
				continue
			}
			slog.Debug("Attempting to process manual file.", "path", absManualPath)

			fileInfo, errStat := os.Stat(absManualPath)
			if errStat != nil {
				if os.IsNotExist(errStat) {
					slog.Warn("Manual file not found, skipping.", "path", absManualPath)
				} else {
					slog.Warn("Cannot stat manual file, skipping.", "path", absManualPath, "error", errStat)
				}
				errorFiles[absManualPath] = errStat
				continue
			}

			if fileInfo.IsDir() {
				slog.Warn("Manual path points to a directory, skipping.", "path", absManualPath)
				errorFiles[absManualPath] = fmt.Errorf("path is a directory")
				continue
			}

			content, errRead := os.ReadFile(absManualPath)
			if errRead != nil {
				slog.Warn("Error reading manual file content.", "path", absManualPath, "error", errRead)
				errorFiles[absManualPath] = errRead
				processedFiles[absManualPath] = true
				continue
			}

			displayPath := absManualPath
			relPath, errRel := filepath.Rel(dir, absManualPath)
			if errRel == nil && !strings.Contains(filepath.ToSlash(relPath), "..") {
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

			slog.Debug("Adding manual file content.", "path", displayPath, "size", len(content))
			outputBuilder.WriteString(fmt.Sprintf("%s %s\n%s\n%s\n\n", marker, displayPath, string(content), marker))
			includedFiles = append(includedFiles, FileInfo{Path: displayPath, Size: fileInfo.Size(), IsManual: true})
			totalSize += fileInfo.Size()
			processedFiles[absManualPath] = true
		}
	}

	var gitignore *GitignoreMatcher

	if useGitignore && len(exts) > 0 {
		var errGitignore error
		gitignore, errGitignore = NewGitignoreMatcher(dir)
		if errGitignore != nil {
			slog.Warn("Could not initialize gitignore processor. Gitignore files will not be processed.", "directory", dir, "error", errGitignore)
			gitignore = nil
		} else if gitignore.matcher != nil {
			slog.Debug("Initialized gitignore processor.", "directory", dir)
		}
	}

	filesFoundInWalk := 0
	if len(exts) > 0 {
		slog.Debug("Starting directory walk.", "directory", dir)
		walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErrIn error) error {
			if walkErrIn != nil {
				relPathErr, _ := filepath.Rel(dir, path)
				displayPathErr := filepath.ToSlash(relPathErr)
				if relPathErr == "" || strings.Contains(displayPathErr, "..") {
					displayPathErr = filepath.ToSlash(path)
				}
				slog.Warn("Error accessing path during walk, skipping.", "path", displayPathErr, "error", walkErrIn)
				errorFiles[displayPathErr] = walkErrIn
				if d != nil && d.IsDir() && errors.Is(walkErrIn, fs.ErrPermission) {
					return fs.SkipDir
				}
				return nil
			}

			absPath := path
			if !filepath.IsAbs(path) {
				absPath = filepath.Join(dir, path)
			}
			absPath = filepath.Clean(absPath)

			relPath, errRel := filepath.Rel(dir, path)
			if errRel != nil {
				slog.Warn("Could not determine relative path, skipping.", "base", dir, "target", path, "error", errRel)
				errorFiles[filepath.ToSlash(path)] = errRel
				return nil
			}
			relPath = filepath.ToSlash(relPath)

			// *** DEBUG: Log entry being processed ***
			slog.Debug("Walk: Processing entry", "path", relPath, "is_dir", d.IsDir())

			if d.IsDir() {
				if relPath == "." {
					return nil
				}

				if d.Name() == ".git" {
					slog.Debug("Walk: Skipping .git directory.", "path", relPath)
					return fs.SkipDir
				}
				if gitignore != nil && gitignore.Match(relPath, true) {
					slog.Debug("Walk: Skipping directory due to gitignore.", "path", relPath)
					return fs.SkipDir
				}
				for _, pattern := range excludePatterns {
					matchRel, _ := filepath.Match(pattern, relPath)
					matchName, _ := filepath.Match(pattern, d.Name())
					if matchRel || matchName {
						slog.Debug("Walk: Skipping directory due to exclude pattern.", "path", relPath, "pattern", pattern)
						return fs.SkipDir
					}
				}
				slog.Debug("Walk: Entering directory", "path", relPath)
				return nil // Continue into directory
			}

			// --- File Processing ---

			if processedFiles[absPath] {
				slog.Debug("Walk: Skipping file already processed manually.", "path", relPath)
				return nil
			}

			fileExt := strings.ToLower(filepath.Ext(path))
			// *** DEBUG: Log extension check ***
			slog.Debug("Walk: Checking file extension", "path", relPath, "extension", fileExt)
			if _, ok := exts[fileExt]; !ok {
				// *** DEBUG: Log skip reason ***
				slog.Debug("Walk: Skipping file - extension not in included set", "path", relPath, "extension", fileExt, "include_set", mapsKeys(exts))
				return nil // Skip file if extension doesn't match
			}
			// *** DEBUG: Log match ***
			slog.Debug("Walk: Extension matched", "path", relPath, "extension", fileExt)

			if gitignore != nil && gitignore.Match(relPath, false) {
				slog.Debug("Walk: Skipping file due to gitignore.", "path", relPath)
				return nil
			}

			for _, pattern := range excludePatterns {
				matchRel, _ := filepath.Match(pattern, relPath)
				matchName, _ := filepath.Match(pattern, d.Name())
				if matchRel || matchName {
					slog.Debug("Walk: Skipping file due to exclude pattern.", "path", relPath, "pattern", pattern)
					return nil
				}
			}

			// --- Read File Content ---
			slog.Debug("Walk: Reading file content", "path", relPath)
			content, errRead := os.ReadFile(path)
			if errRead != nil {
				slog.Warn("Error reading file content.", "path", relPath, "error", errRead)
				errorFiles[relPath] = errRead
				processedFiles[absPath] = true
				return nil
			}

			fileInfo, _ := d.Info()
			fileSize := int64(0)
			if fileInfo != nil {
				fileSize = fileInfo.Size()
			}

			if len(content) == 0 {
				slog.Info("Found empty file during scan.", "path", relPath)
				emptyFiles = append(emptyFiles, relPath)
				processedFiles[absPath] = true
				return nil
			}

			slog.Debug("Adding file content to output.", "path", relPath, "size", len(content))
			outputBuilder.WriteString(fmt.Sprintf("%s %s\n%s\n%s\n\n", marker, relPath, string(content), marker))
			includedFiles = append(includedFiles, FileInfo{Path: relPath, Size: fileSize, IsManual: false})
			totalSize += fileSize
			processedFiles[absPath] = true
			filesFoundInWalk++
			return nil
		})

		if walkErr != nil {
			slog.Error("File walk finished with error.", "directory", dir, "error", walkErr)
			err = fmt.Errorf("error walking directory %s: %w", dir, walkErr)
		}
	}

	slog.Info("File scan completed.")

	output = strings.TrimSuffix(outputBuilder.String(), "\n\n")
	if outputBuilder.Len() > 0 && output == "" {
		output = "\n"
	}
	if header == "" && output == "\n" {
		output = ""
	} else if output != "" && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	return
}

// --- Gitignore Implementation ---
// (GitignoreMatcher struct, NewGitignoreMatcher, Match methods remain the same)
type GitignoreMatcher struct {
	matcher gitignorelib.IgnoreParser
	root    string
}

func NewGitignoreMatcher(root string) (*GitignoreMatcher, error) {
	gitignorePath := filepath.Join(root, ".gitignore")
	var matcher gitignorelib.IgnoreParser
	var err error
	if _, statErr := os.Stat(gitignorePath); os.IsNotExist(statErr) {
		slog.Debug("No .gitignore file found at root.", "directory", root)
		matcher = nil
	} else if statErr != nil {
		return nil, fmt.Errorf("error stating .gitignore file %s: %w", gitignorePath, statErr)
	} else {
		matcher, err = gitignorelib.CompileIgnoreFile(gitignorePath)
		if err != nil {
			return nil, fmt.Errorf("error compiling gitignore file %s: %w", gitignorePath, err)
		}
		slog.Debug("Successfully compiled gitignore file.", "path", gitignorePath)
	}
	return &GitignoreMatcher{matcher: matcher, root: root}, nil
}
func (g *GitignoreMatcher) Match(relativePath string, isDir bool) bool {
	if g.matcher == nil {
		return false
	}
	return g.matcher.MatchesPath(relativePath)
}

// --- Tree Node for Summary ---
// (TreeNode struct remains the same)
type TreeNode struct {
	Name     string
	Children map[string]*TreeNode
	FileInfo *FileInfo
}

// --- Tree/Summary Helper Functions ---
// (formatBytes, buildTree, printTreeRecursive, printSummaryTree remain the same)
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
func buildTree(files []FileInfo) *TreeNode {
	root := &TreeNode{Name: ".", Children: make(map[string]*TreeNode)}
	for i := range files {
		file := &files[i]
		normalizedPath := filepath.ToSlash(file.Path)
		parts := strings.Split(normalizedPath, "/")
		currentNode := root
		for j, part := range parts {
			if part == "" {
				continue
			}
			isLastPart := (j == len(parts)-1)
			childNode, exists := currentNode.Children[part]
			if !exists {
				childNode = &TreeNode{Name: part, Children: make(map[string]*TreeNode)}
				currentNode.Children[part] = childNode
			}
			if isLastPart {
				childNode.FileInfo = file
			}
			currentNode = childNode
		}
	}
	return root
}
func printTreeRecursive(writer io.Writer, node *TreeNode, indent string, isLast bool) {
	fileInfoStr := ""
	manualMarker := ""
	if node.FileInfo != nil {
		fileInfoStr = fmt.Sprintf(" (%s)", formatBytes(node.FileInfo.Size))
		if node.FileInfo.IsManual {
			manualMarker = " [M]"
		}
	}
	if node.Name == "." { /* Handled by caller */
	} else {
		connector := "├── "
		if isLast {
			connector = "└── "
		}
		fmt.Fprintf(writer, "%s%s%s%s%s\n", indent, connector, node.Name, manualMarker, fileInfoStr)
		if isLast {
			indent += "    "
		} else {
			indent += "│   "
		}
	}
	childNames := make([]string, 0, len(node.Children))
	for name := range node.Children {
		childNames = append(childNames, name)
	}
	sort.Strings(childNames)
	for i, name := range childNames {
		printTreeRecursive(writer, node.Children[name], indent, i == len(childNames)-1)
	}
}
func printSummaryTree(
	includedFiles []FileInfo, emptyFiles []string, errorFiles map[string]error,
	totalSize int64, targetDir string, outputWriter io.Writer,
) {
	fmt.Fprintln(outputWriter, "\n--- Summary ---")
	if len(includedFiles) > 0 {
		treeRootName := filepath.Base(targetDir)
		if treeRootName == "." || treeRootName == string(filepath.Separator) {
			if abs, err := filepath.Abs(targetDir); err == nil {
				treeRootName = abs
			} else {
				treeRootName = targetDir
			}
		}
		fmt.Fprintf(outputWriter, "Included %d files (%s total) from '%s':\n", len(includedFiles), formatBytes(totalSize), treeRootName)
		sort.Slice(includedFiles, func(i, j int) bool { return includedFiles[i].Path < includedFiles[j].Path })
		fileTree := buildTree(includedFiles)
		fmt.Fprintf(outputWriter, "%s/\n", treeRootName)
		rootChildNames := make([]string, 0, len(fileTree.Children))
		for name := range fileTree.Children {
			rootChildNames = append(rootChildNames, name)
		}
		sort.Strings(rootChildNames)
		for i, name := range rootChildNames {
			printTreeRecursive(outputWriter, fileTree.Children[name], "", i == len(rootChildNames)-1)
		}
	} else {
		fmt.Fprintln(outputWriter, "No files included in the output.")
	}
	if len(emptyFiles) > 0 {
		fmt.Fprintf(outputWriter, "\nEmpty files found (%d):\n", len(emptyFiles))
		sort.Strings(emptyFiles)
		for _, p := range emptyFiles {
			fmt.Fprintf(outputWriter, "- %s\n", p)
		}
	}
	if len(errorFiles) > 0 {
		fmt.Fprintf(outputWriter, "\nErrors encountered (%d):\n", len(errorFiles))
		errorPaths := make([]string, 0, len(errorFiles))
		for p := range errorFiles {
			errorPaths = append(errorPaths, p)
		}
		sort.Strings(errorPaths)
		for _, p := range errorPaths {
			fmt.Fprintf(outputWriter, "- %s: %v\n", p, errorFiles[p])
		}
	}
	fmt.Fprintln(outputWriter, "---------------")
}

// Helper to get map keys for logging set contents
func mapsKeys[M ~map[K]V, K comparable, V any](m M) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	sort.Slice(r, func(i, j int) bool { // Sort for consistent log output
		// Assuming K is string or comparable type convertible to string
		return fmt.Sprint(r[i]) < fmt.Sprint(r[j])
	})
	return r
}
