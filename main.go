// main.go
package main

import (
	// Keep for helpers if needed

	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	// Dependencies
	gocodewalker "github.com/boyter/gocodewalker"   // Use new walker
	gitignorelib "github.com/sabhiram/go-gitignore" // Still needed for NewGitignoreMatcher (though not used in walk)
	pflag "github.com/spf13/pflag"
)

const Version = "0.2.3" // Keep original version for now

// --- Structs ---
type FileInfo struct {
	Path     string
	Size     int64
	IsManual bool
}

// Required by generateConcatenatedCode signature, but logic not used when using gocodewalker
type GitignoreMatcher struct {
	matcher gitignorelib.IgnoreParser
	root    string
}

func NewGitignoreMatcher(root string) (*GitignoreMatcher, error) {
	// This function might still be called by tests or older code paths, keep stub
	slog.Debug("NewGitignoreMatcher called (likely legacy path/test)", "root", root)
	return nil, nil // Return nil as it's not used for the main walk now
}

func (g *GitignoreMatcher) Match(relativePath string, isDir bool) bool {
	// Keep stub
	return false
}

type TreeNode struct {
	Name     string
	Children map[string]*TreeNode
	FileInfo *FileInfo
}

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
	pflag.StringVarP(&targetDirFlagValue, "directory", "d", ".", "Target directory to scan.")
	// Defaults may come from config later, empty for now allows config to set them
	pflag.StringSliceVarP(&extensions, "extensions", "e", []string{}, "Comma-separated file extensions (overrides config).")
	pflag.StringSliceVarP(&manualFiles, "files", "f", []string{}, "Comma-separated specific file paths.")
	pflag.StringSliceVarP(&excludePatterns, "exclude", "x", []string{}, "Comma-separated glob patterns to exclude (adds to config).")
	pflag.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore processing.")
	pflag.StringVar(&logLevelStr, "loglevel", "info", "Set logging verbosity (debug, info, warn, error).")
	pflag.StringVarP(&outputFile, "output", "o", "", "Output file path.")
	pflag.StringVarP(&configFileFlag, "config", "c", "", "Path to a custom configuration file.")
	pflag.BoolVarP(&versionFlag, "version", "v", false, "Print version and exit.")

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

	// Load Configuration
	appConfig, loadErr := loadConfig(configFileFlag) // Assumes config.go exists
	if loadErr != nil {
		slog.Error("Failed to load configuration, using defaults.", "error", loadErr)
		appConfig = defaultConfig // Assumes defaultConfig is defined in config.go
	}

	// Argument Mode Validation (Using previously discussed logic)
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
		fmt.Fprintf(os.Stderr, "Refusing execution: Multiple positional arguments provided: %v.\nUse either a single directory argument OR flags.\n", positionalArgs)
		os.Exit(1)
	} else if len(positionalArgs) == 1 {
		if conflictingFlagSet {
			fmt.Fprintf(os.Stderr, "Refusing execution: Cannot mix positional argument '%s' with flag '--%s'.\nUse flags exclusively for complex commands.\n", positionalArgs[0], firstConflict)
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
		slog.Error("Could not determine absolute path for target directory.", "path", finalTargetDirectory, "error", err)
		fmt.Fprintf(os.Stderr, "Error: Invalid target directory path '%s': %v\n", finalTargetDirectory, err)
		os.Exit(1)
	}
	finalTargetDirectory = absTargetDir

	// Initial Stat Check (Part of NonExistentDir fix)
	// generateConcatenatedCode will also check, but this provides earlier user feedback.
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

	// Determine final settings
	commentMarker := *appConfig.CommentMarker
	headerText := *appConfig.HeaderText
	finalUseGitignore := !noGitignore // Determine based on original flag

	// Determine Extensions (Flag overrides Config)
	finalExtensionsList := appConfig.IncludeExtensions // Start with config/default
	if pflag.CommandLine.Changed("extensions") {
		slog.Debug("Using extensions from command line flag (overrides config).", "extensions", extensions)
		finalExtensionsList = extensions
	} else {
		slog.Debug("Using extensions from config/default.", "extensions", appConfig.IncludeExtensions)
	}
	finalExtensionsSet := processExtensions(finalExtensionsList)
	slog.Debug("Final extension set prepared", "set_keys", mapsKeys(finalExtensionsSet))

	// Determine Exclude Patterns (Flag ADDS to Config) - Corrected Logic
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

	// Generate Output
	concatenatedOutput, includedFiles, emptyFiles, errorFiles, totalSize, genErr := generateConcatenatedCode(
		finalTargetDirectory,
		finalExtensionsSet,
		manualFiles,
		finalExcludePatternsList,
		finalUseGitignore, // Pass bool for now
		headerText,
		commentMarker,
	)

	// Handle error from generation
	if genErr != nil {
		slog.Error("Error during file processing.", "error", genErr)
		// Avoid duplicate "not found" message
		if !(os.IsNotExist(genErr)) {
			fmt.Fprintf(os.Stderr, "Error during processing: %v\n", genErr)
		}
		// We should exit here regardless of manual files, as the scan itself failed if attempted
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

	// Write Concatenated Code
	if concatenatedOutput != "" {
		_, errWrite := io.WriteString(codeWriter, concatenatedOutput)
		if errWrite != nil {
			slog.Error("Failed to write concatenated code.", "error", errWrite)
			fmt.Fprintf(os.Stderr, "Error: Failed to write output: %v\n", errWrite)
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

	// Print Summary
	printSummaryTree(includedFiles, emptyFiles, errorFiles, totalSize, finalTargetDirectory, summaryWriter)

	slog.Debug("Execution finished.")

	if len(errorFiles) > 0 {
		os.Exit(1)
	}
}

// --- generateConcatenatedCode (Integrated with gocodewalker) ---
func generateConcatenatedCode(
	dir string, // Expect absolute path
	exts map[string]struct{},
	manualFilePaths []string,
	excludePatterns []string,
	useGitignore bool, // Controls walker ignore settings now
	header, marker string,
) (output string, includedFiles []FileInfo, emptyFiles []string, errorFiles map[string]error, totalSize int64, returnedErr error) {

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

	// Validate exclude patterns
	validExcludePatterns := make([]string, 0, len(excludePatterns))
	for _, pattern := range excludePatterns {
		if _, errMatch := filepath.Match(pattern, ""); errMatch != nil {
			slog.Warn("Invalid exclude pattern, ignoring.", "pattern", pattern, "error", errMatch)
		} else {
			validExcludePatterns = append(validExcludePatterns, pattern)
		}
	}
	excludePatterns = validExcludePatterns

	// Process Manually Added Files FIRST
	if len(manualFilePaths) > 0 {
		slog.Debug("Processing manually specified files.", "count", len(manualFilePaths))
		for _, manualPath := range manualFilePaths {
			absManualPath, errAbs := filepath.Abs(manualPath)
			if errAbs != nil {
				slog.Warn("Could not get absolute path for manual file.", "path", manualPath, "error", errAbs)
				errorFiles[manualPath] = errAbs
				continue
			}
			slog.Debug("Attempting to process manual file.", "path", absManualPath)
			fileInfo, errStat := os.Stat(absManualPath)
			if errStat != nil {
				if os.IsNotExist(errStat) {
					slog.Warn("Manual file not found.", "path", absManualPath)
				} else {
					slog.Warn("Cannot stat manual file.", "path", absManualPath, "error", errStat)
				}
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

	// --- Directory Scanning ---
	shouldScan := len(exts) > 0

	if shouldScan {
		slog.Info("Starting file scan.", "directory", dir, "useGitignore", useGitignore)

		// Initial directory check
		dirInfo, statErr := os.Stat(dir)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				slog.Error("Target directory does not exist.", "path", dir)
			} else {
				slog.Error("Cannot stat target directory before walk.", "path", dir, "error", statErr)
			}
			returnedErr = statErr
			return
		}
		if !dirInfo.IsDir() {
			statErr = fmt.Errorf("target path '%s' is not a directory", dir)
			slog.Error("Target path is not a directory.", "path", dir)
			returnedErr = statErr
			return
		}

		// --- Use gocodewalker for scanning ---
		fileListQueue := make(chan *gocodewalker.File, 100)
		fileWalker := gocodewalker.NewFileWalker(dir, fileListQueue)

		// Configure Ignores based on useGitignore flag
		// If true, we want walker to process ignores (flag should be false)
		// If false, we want walker to ignore ignores (flag should be true)
		fileWalker.IgnoreGitIgnore = !useGitignore
		fileWalker.IgnoreIgnoreFile = !useGitignore
		slog.Debug("Configured walker ignore flags", "IgnoreGitIgnore", fileWalker.IgnoreGitIgnore, "IgnoreIgnoreFile", fileWalker.IgnoreIgnoreFile)

		// Configure Extensions
		allowedExtList := []string{}
		for extWithDot := range exts {
			allowedExtList = append(allowedExtList, strings.TrimPrefix(extWithDot, "."))
		}
		fileWalker.AllowListExtensions = allowedExtList
		slog.Debug("Set walker AllowListExtensions", "extensions", allowedExtList)

		// Configure Excludes
		locationExcludePatterns := []string{}
		manualDirExcludePatterns := []string{}
		// Add .git exclude implicitly if not ignoring gitignore files
		if useGitignore {
			locationExcludePatterns = append(locationExcludePatterns, ".git")   // Exclude .git dir itself
			locationExcludePatterns = append(locationExcludePatterns, ".git/*") // Exclude contents
		}
		for _, p := range excludePatterns {
			if strings.HasSuffix(p, "/") {
				dirPatternBase := strings.TrimSuffix(p, "/")
				manualDirExcludePatterns = append(manualDirExcludePatterns, dirPatternBase)
				locationExcludePatterns = append(locationExcludePatterns, dirPatternBase+"/*")
				locationExcludePatterns = append(locationExcludePatterns, dirPatternBase)
			} else {
				locationExcludePatterns = append(locationExcludePatterns, p)
			}
		}
		fileWalker.LocationExcludePattern = locationExcludePatterns
		slog.Debug("Set walker LocationExcludePattern", "patterns", locationExcludePatterns)
		slog.Debug("Manual directory exclude patterns", "patterns", manualDirExcludePatterns)

		// Set Error Handler
		var firstWalkError error
		walkerErrorHandler := func(e error) bool {
			slog.Warn("Error during walk.", "error", e) // Path info might be limited
			if firstWalkError == nil {
				firstWalkError = e
			}
			return true // Continue walking if possible
		}
		fileWalker.SetErrorHandler(walkerErrorHandler)

		// Start and check initial error
		walkErr := fileWalker.Start()
		if walkErr != nil {
			slog.Error("Failed to start file walk.", "directory", dir, "error", walkErr)
			returnedErr = walkErr
			return // Return immediately
		}

		// If Start() succeeded, launch processor goroutine
		processingDone := make(chan struct{})
		go func() {
			defer close(processingDone)
			for f := range fileListQueue {
				absPath := f.Location
				if processedFiles[absPath] {
					continue
				}

				relPath, errRel := filepath.Rel(dir, absPath)
				if errRel != nil {
					slog.Warn("Could not get relative path during walk.", "path", absPath, "error", errRel)
					errorFiles[filepath.ToSlash(absPath)] = errRel
					continue
				}
				relPath = filepath.ToSlash(relPath)

				// Manual check for dir/ patterns
				isDirExcluded := false
				for _, dirPattern := range manualDirExcludePatterns {
					if strings.HasPrefix(relPath, dirPattern+"/") {
						slog.Debug("Walk: Skipping file within manually excluded dir.", "path", relPath, "pattern", dirPattern+"/")
						isDirExcluded = true
						break
					}
				}
				if isDirExcluded {
					continue
				}

				// Stat to check if dir and get size/mode
				fileInfo, statErr := os.Stat(absPath)
				if statErr != nil {
					slog.Warn("Could not stat path from walker.", "path", absPath, "error", statErr)
					errorFiles[relPath] = statErr // Use relPath as key for error map
					continue
				}
				if fileInfo.IsDir() {
					continue
				} // Skip directories yielded by walker

				// Extension check is redundant because we set AllowListExtensions
				// Read File Content
				content, errRead := os.ReadFile(absPath)
				if errRead != nil {
					slog.Warn("Error reading file content.", "path", relPath, "error", errRead)
					errorFiles[relPath] = errRead
					processedFiles[absPath] = true
					continue
				}

				// Handle empty file
				if len(content) == 0 {
					slog.Info("Found empty file during scan.", "path", relPath)
					emptyFiles = append(emptyFiles, relPath)
					processedFiles[absPath] = true
					continue
				}

				// Add content
				fileSize := fileInfo.Size()
				slog.Debug("Adding file content to output.", "path", relPath, "size", fileSize)
				outputBuilder.WriteString(fmt.Sprintf("%s %s\n%s\n%s\n\n", marker, relPath, string(content), marker))
				includedFiles = append(includedFiles, FileInfo{Path: relPath, Size: fileSize, IsManual: false})
				totalSize += fileSize
				processedFiles[absPath] = true
			}
		}()

		<-processingDone // Wait for processor

		// Check if handler stored an error
		if firstWalkError != nil && walkErr == nil {
			walkErr = firstWalkError // Prioritize error from Start, but use handler error if Start was ok
		}

		// Check final walkErr after processing is done
		if walkErr != nil {
			slog.Error("File walk completed with errors.", "directory", dir, "error", walkErr)
			if returnedErr == nil {
				returnedErr = walkErr
			}
		} else {
			slog.Info("File scan completed.")
		}

	} // end if shouldScan

	// --- Append Metadata (Not implemented yet) ---

	// --- Prepare final output string ---
	output = strings.TrimSuffix(outputBuilder.String(), "\n\n")
	if outputBuilder.Len() > 0 && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	if header != "" && strings.TrimSpace(output) == header {
		output = header + "\n"
	} else if output == "\n" && header == "" {
		output = ""
	}

	return // Use named return values
}

// --- Helper Functions ---

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

// mapsKeys Helper to get map keys for logging set contents
func mapsKeys[M ~map[K]V, K comparable, V any](m M) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	sort.Slice(r, func(i, j int) bool {
		ki := fmt.Sprint(r[i])
		kj := fmt.Sprint(r[j])
		return ki < kj
	})
	return r
}

// formatBytes formats bytes into human-readable string.
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

// buildTree builds the file tree structure for the summary.
func buildTree(files []FileInfo) *TreeNode {
	root := &TreeNode{Name: ".", Children: make(map[string]*TreeNode)}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
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
				if childNode.FileInfo == nil {
					childNode.FileInfo = file
				} else {
					slog.Warn("Tree building conflict: node already has FileInfo", "nodeName", childNode.Name)
				}
			}
			currentNode = childNode
		}
	}
	return root
}

// printTreeRecursive recursively prints the file tree.
func printTreeRecursive(writer io.Writer, node *TreeNode, indent string, isLast bool) {
	if node.Name == "." {
		childNames := make([]string, 0, len(node.Children))
		for name := range node.Children {
			childNames = append(childNames, name)
		}
		sort.Strings(childNames)
		for i, name := range childNames {
			printTreeRecursive(writer, node.Children[name], indent, i == len(childNames)-1)
		}
		return
	}
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	fileInfoStr := ""
	manualMarker := ""
	if node.FileInfo != nil {
		fileInfoStr = fmt.Sprintf(" (%s)", formatBytes(node.FileInfo.Size))
		if node.FileInfo.IsManual {
			manualMarker = " [M]"
		}
	}
	fmt.Fprintf(writer, "%s%s%s%s%s\n", indent, connector, node.Name, manualMarker, fileInfoStr)
	childIndent := indent
	if isLast {
		childIndent += "    "
	} else {
		childIndent += "│   "
	}
	childNames := make([]string, 0, len(node.Children))
	for name := range node.Children {
		childNames = append(childNames, name)
	}
	sort.Strings(childNames)
	for i, name := range childNames {
		printTreeRecursive(writer, node.Children[name], childIndent, i == len(childNames)-1)
	}
}

// printSummaryTree prints the final summary output.
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
		fileTree := buildTree(includedFiles)
		printTreeRecursive(outputWriter, fileTree, "", true)
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
