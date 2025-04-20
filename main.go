// main.go
// Defines the main entry point and CLI argument parsing for food4ai using pflag.
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
	"time" // Added for timestamp comment

	gitignorelib "github.com/sabhiram/go-gitignore"
	pflag "github.com/spf13/pflag" // Use pflag library
)

// --- File Info Struct ---
type FileInfo struct {
	Path     string // Relative path from targetDir (or absolute if manual and outside)
	Size     int64
	IsManual bool
}

// --- Global Variables for Flags ---
var (
	targetDirFlagValue string
	extensions         []string // Use []string with pflag
	manualFiles        []string // Use []string with pflag
	excludePatterns    []string // Use []string with pflag
	noGitignore        bool
	logLevelStr        string
	outputFile         string
	configFileFlag     string
)

// Helper function to check if a flag was explicitly set on the command line
// Needed because pflag.Visit visits all flags, not just changed ones like standard flag.Visit
// pflag.CommandLine.Changed(name) is the preferred way now.
// func isFlagPassed(name string) bool { // No longer strictly needed if using Changed()
// 	 return pflag.CommandLine.Changed(name)
// }

func init() {
	// Define command-line flags using pflag
	// StringVarP defines long name, short name (single char), default value, usage
	pflag.StringVarP(&targetDirFlagValue, "directory", "d", ".", "Target directory to scan (use this OR a positional argument, not both).")
	// StringSliceVarP defines slice flags, handles comma-separation and repetition
	pflag.StringSliceVarP(&extensions, "extensions", "e", defaultConfig.IncludeExtensions, fmt.Sprintf("Comma-separated file extensions to include (requires flags mode). Default: %v", defaultConfig.IncludeExtensions))
	pflag.StringSliceVarP(&manualFiles, "files", "f", []string{}, "Comma-separated specific file paths to include manually (requires flags mode).")
	pflag.StringSliceVarP(&excludePatterns, "exclude", "x", defaultConfig.ExcludePatterns, fmt.Sprintf("Comma-separated glob patterns to exclude (requires flags mode). Default: %v", defaultConfig.ExcludePatterns))
	// BoolVar usually doesn't have a short version unless standard; use BoolVarP if needed
	pflag.BoolVar(&noGitignore, "no-gitignore", !*defaultConfig.UseGitignore, fmt.Sprintf("Disable .gitignore processing (requires flags mode). Config default: %t", *defaultConfig.UseGitignore))
	pflag.StringVar(&logLevelStr, "loglevel", "info", "Set logging verbosity (debug, info, warning, error).")
	pflag.StringVarP(&outputFile, "output", "o", "", "Output file path (writes code to file, summary to stdout).")
	// Use StringVarP to add the -c short option for config
	pflag.StringVarP(&configFileFlag, "config", "c", "", "Path to a custom configuration file (overrides default ~/.config/food4ai/config.toml).")

	// Define custom usage message using pflag's Usage variable
	pflag.Usage = func() {
		// Write custom header directly to stderr (pflag default output)
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
`, os.Args[0], os.Args[0]) // Use os.Args[0] for program name

		// Use pflag's default printer for flag descriptions
		pflag.PrintDefaults()
	}
}

// --- Main Execution ---
func main() {
	// Add a timestamp comment for context
	// Current time: Saturday, April 19, 2025 at 11:43:51 PM PDT
	_ = time.Now()

	// Parse flags using pflag
	pflag.Parse()

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
	// Use pflag.CommandLine.Changed(name) to see if flag was set from command line
	configFlagPassed := pflag.CommandLine.Changed("config")
	appConfig, loadErr := loadConfig(configFileFlag) // Pass the value from the --config / -c flag
	if loadErr != nil {
		slog.Error("Failed to load configuration.", "error", loadErr)
		if configFlagPassed {
			// If a specific config file was requested and failed, exit.
			fmt.Fprintf(os.Stderr, "Error: Could not load specified configuration file '%s': %v\n", configFileFlag, loadErr)
			os.Exit(1)
		} else {
			// If default config loading failed, warn but proceed with hardcoded defaults.
			slog.Warn("Proceeding with default settings due to config load issue.")
		}
	}

	// --- Argument Mode Validation ---
	positionalArgs := pflag.Args() // Use pflag.Args() to get non-flag arguments
	finalTargetDirectory := ""

	var conflictingFlagSet bool = false
	var firstConflict string = ""
	// Define flags that are NOT considered operational/conflicting for ambiguity checks
	metaFlags := map[string]struct{}{
		"help":     {}, // pflag automatically adds --help
		"loglevel": {},
		// Add "version" here if implemented
	}

	// Iterate over flags *actually set* on the command line using pflag.Visit
	pflag.Visit(func(f *pflag.Flag) {
		if _, isMeta := metaFlags[f.Name]; !isMeta {
			// If a flag was set and it's not in our metaFlags list, it conflicts with positional args
			conflictingFlagSet = true
			if firstConflict == "" {
				firstConflict = f.Name // Record the first conflicting flag found
			}
		}
	})

	if len(positionalArgs) > 1 {
		// Case A: Multiple positional args -> Refusal Message
		refusalMsg := fmt.Sprintf("Refusing execution: Multiple positional arguments provided: %v.\nUse either a single directory argument OR flags, not both. Run with --help for usage details.", positionalArgs)
		fmt.Fprintln(os.Stderr, refusalMsg)
		os.Exit(1)
	} else if len(positionalArgs) == 1 {
		// Case B: Single positional arg
		if conflictingFlagSet {
			// Refusal Message: Mixed positional arg with an operational flag
			// Include the specific conflicting flag name found
			refusalMsg := fmt.Sprintf("Refusing execution: Cannot mix positional argument '%s' with flag '--%s'.\nPlease use flags exclusively for complex commands. Run with --help for usage details.", positionalArgs[0], firstConflict)
			fmt.Fprintln(os.Stderr, refusalMsg)
			os.Exit(1)
		} else {
			// Valid: Use positional arg as target directory
			finalTargetDirectory = positionalArgs[0]
			if finalTargetDirectory == "" {
				finalTargetDirectory = "."
			}
			slog.Debug("Using target directory from positional argument.", "path", finalTargetDirectory)
		}
	} else {
		// Case C: No positional args -> Flags mode
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
		slog.Debug("Using extensions from command line.", "extensions", extensions)
		finalExtensionsList = extensions
	} else if len(appConfig.IncludeExtensions) > 0 {
		slog.Debug("Using extensions from config.", "extensions", appConfig.IncludeExtensions)
		finalExtensionsList = appConfig.IncludeExtensions
	} else {
		slog.Debug("No extensions specified via command line or config. Using hardcoded default.", "extensions", defaultConfig.IncludeExtensions)
		finalExtensionsList = defaultConfig.IncludeExtensions
	}
	finalExtensionsSet := processExtensions(finalExtensionsList)

	var finalExcludePatternsList []string
	if pflag.CommandLine.Changed("exclude") {
		slog.Debug("Using exclude patterns from command line.", "patterns", excludePatterns)
		finalExcludePatternsList = excludePatterns
	} else {
		slog.Debug("Using exclude patterns from config.", "patterns", appConfig.ExcludePatterns)
		finalExcludePatternsList = appConfig.ExcludePatterns
		if finalExcludePatternsList == nil {
			finalExcludePatternsList = []string{}
		}
	}

	var finalUseGitignore bool
	if pflag.CommandLine.Changed("no-gitignore") {
		finalUseGitignore = !noGitignore // Value of noGitignore is true if flag is present
		slog.Debug("Using gitignore setting from command line.", "use_gitignore", finalUseGitignore)
	} else {
		finalUseGitignore = *appConfig.UseGitignore
		slog.Debug("Using gitignore setting from config.", "use_gitignore", finalUseGitignore)
	}

	// --- Input Validation ---
	if len(finalExtensionsSet) == 0 && len(manualFiles) == 0 {
		slog.Error("Processing criteria missing. No file extensions specified and no manual files provided. Nothing to process.")
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
		os.Exit(1) // Exit with error status if issues occurred
	}
}

// --- Helper Functions & Structs ---

// processExtensions processes a list of extension strings into a set for quick lookup.
func processExtensions(extList []string) map[string]struct{} {
	processed := make(map[string]struct{})
	for _, ext := range extList {
		// Handle potential multiple extensions in one string if comma used with repetition
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

// generateConcatenatedCode implementation remains the same.
func generateConcatenatedCode(
	dir string,
	exts map[string]struct{},
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

			if d.IsDir() {
				if relPath == "." {
					return nil
				}

				if d.Name() == ".git" {
					slog.Debug("Skipping .git directory.", "path", relPath)
					return fs.SkipDir
				}
				if gitignore != nil && gitignore.Match(relPath, true) {
					slog.Debug("Skipping directory due to gitignore.", "path", relPath)
					return fs.SkipDir
				}
				for _, pattern := range excludePatterns {
					matchRel, _ := filepath.Match(pattern, relPath)
					matchName, _ := filepath.Match(pattern, d.Name())
					if matchRel || matchName {
						slog.Debug("Skipping directory due to exclude pattern.", "path", relPath, "pattern", pattern)
						return fs.SkipDir
					}
				}
				return nil
			}

			if processedFiles[absPath] {
				slog.Debug("Skipping file already processed manually.", "path", relPath)
				return nil
			}

			fileExt := strings.ToLower(filepath.Ext(path))
			if _, ok := exts[fileExt]; !ok {
				return nil
			}

			if gitignore != nil && gitignore.Match(relPath, false) {
				slog.Debug("Skipping file due to gitignore.", "path", relPath)
				return nil
			}

			for _, pattern := range excludePatterns {
				matchRel, _ := filepath.Match(pattern, relPath)
				matchName, _ := filepath.Match(pattern, d.Name())
				if matchRel || matchName {
					slog.Debug("Skipping file due to exclude pattern.", "path", relPath, "pattern", pattern)
					return nil
				}
			}

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

			slog.Debug("Adding file content.", "path", relPath, "size", len(content))
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
type TreeNode struct {
	Name     string
	Children map[string]*TreeNode
	FileInfo *FileInfo
}

// --- Tree/Summary Helper Functions ---
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

	if node.Name == "." {
		// Root placeholder node, handled by caller
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
	includedFiles []FileInfo,
	emptyFiles []string,
	errorFiles map[string]error,
	totalSize int64,
	targetDir string,
	outputWriter io.Writer,
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

		sort.Slice(includedFiles, func(i, j int) bool {
			return includedFiles[i].Path < includedFiles[j].Path
		})
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
