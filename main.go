// main.go
package main

import (
	// Added for logger setup in tests, keep if helpers are needed elsewhere
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

	// Original dependencies
	gitignorelib "github.com/sabhiram/go-gitignore"
	pflag "github.com/spf13/pflag"
	// gocodewalker "github.com/boyter/gocodewalker" // Not used in this version yet
)

const Version = "0.2.3" // Keep original version for now

// --- File Info Struct ---
type FileInfo struct {
	Path     string
	Size     int64
	IsManual bool
}

// --- GitignoreMatcher Struct (Used by current generateConcatenatedCode) ---
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
		// Use original library for now
		matcher, err = gitignorelib.CompileIgnoreFile(gitignorePath)
		if err != nil {
			return nil, fmt.Errorf("error compiling gitignore file %s: %w", gitignorePath, err)
		}
		slog.Debug("Successfully compiled gitignore file.", "path", gitignorePath)
	}
	return &GitignoreMatcher{matcher: matcher, root: root}, nil
}

func (g *GitignoreMatcher) Match(relativePath string, isDir bool) bool {
	if g == nil || g.matcher == nil {
		return false
	}
	return g.matcher.MatchesPath(relativePath)
}

// --- TreeNode Struct (For Summary) ---
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
	// Define command-line flags using pflag (Original Set)
	pflag.StringVarP(&targetDirFlagValue, "directory", "d", ".", "Target directory to scan.")
	// Keep original defaults from prompt for now
	pflag.StringSliceVarP(&extensions, "extensions", "e", []string{"py", "json", "sh", "txt", "rst", "md", "go", "mod", "sum", "yaml", "yml"}, "Comma-separated file extensions.")
	pflag.StringSliceVarP(&manualFiles, "files", "f", []string{}, "Comma-separated specific file paths.")
	pflag.StringSliceVarP(&excludePatterns, "exclude", "x", []string{}, "Comma-separated glob patterns to exclude.")
	pflag.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore processing.")
	pflag.StringVar(&logLevelStr, "loglevel", "info", "Set logging verbosity (debug, info, warn, error).")
	pflag.StringVarP(&outputFile, "output", "o", "", "Output file path.")
	pflag.StringVarP(&configFileFlag, "config", "c", "", "Path to a custom configuration file.")
	pflag.BoolVarP(&versionFlag, "version", "v", false, "Print version and exit.")

	// Define original usage message
	pflag.Usage = func() {
		// Use original usage message structure if needed, or keep the updated one from before
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

	// --- Load Configuration (Uses original config struct temporarily) ---
	// NOTE: This will ignore the new fields until config.go is updated later
	appConfig, loadErr := loadConfig(configFileFlag) // loadConfig needs to exist (see below)
	if loadErr != nil {
		slog.Error("Failed to load configuration, using defaults.", "error", loadErr)
		appConfig = defaultConfig // Ensure defaultConfig is defined (see below)
	}

	// --- Argument Mode Validation (Use original logic or keep updated one) ---
	positionalArgs := pflag.Args()
	finalTargetDirectory := ""
	// ... (Use the argument validation logic from your working version or the one previously discussed) ...
	// Simplified for now:
	if len(positionalArgs) > 0 {
		finalTargetDirectory = positionalArgs[0]
	} else {
		finalTargetDirectory = targetDirFlagValue
	}
	if finalTargetDirectory == "" {
		finalTargetDirectory = "."
	}

	// --- Validate Final Target Directory ---
	absTargetDir, err := filepath.Abs(finalTargetDirectory)
	if err != nil {
		slog.Error("Could not determine absolute path for target directory.", "path", finalTargetDirectory, "error", err)
		fmt.Fprintf(os.Stderr, "Error: Invalid target directory path '%s': %v\n", finalTargetDirectory, err)
		os.Exit(1)
	}
	finalTargetDirectory = absTargetDir

	// Perform initial stat check here for early exit (part of NonExistentDir fix)
	dirInfo, err := os.Stat(finalTargetDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Error("Target directory does not exist.", "path", finalTargetDirectory)
			fmt.Fprintf(os.Stderr, "Error: Target directory '%s' not found.\n", finalTargetDirectory)
		} else {
			slog.Error("Error accessing target directory.", "path", finalTargetDirectory, "error", err)
			fmt.Fprintf(os.Stderr, "Error accessing target directory '%s': %v\n", finalTargetDirectory, err)
		}
		os.Exit(1) // Exit here, generateConcatenatedCode handles internal check too
	}
	if !dirInfo.IsDir() {
		slog.Error("Specified target path is not a directory.", "path", finalTargetDirectory)
		fmt.Fprintf(os.Stderr, "Error: Specified target path '%s' is not a directory.\n", finalTargetDirectory)
		os.Exit(1)
	}

	// --- Determine final settings (Using original flags/config structure) ---
	commentMarker := *appConfig.CommentMarker // Assume config.go defines these
	headerText := *appConfig.HeaderText
	finalUseGitignore := !noGitignore // Use original flag logic

	// Determine Extensions (Flag potentially overrides Config - check original config.go logic)
	finalExtensionsList := extensions                                                     // Simplification - assumes flag overrides default
	if !pflag.CommandLine.Changed("extensions") && len(appConfig.IncludeExtensions) > 0 { // Rough check for config override
		finalExtensionsList = appConfig.IncludeExtensions
	}
	finalExtensionsSet := processExtensions(finalExtensionsList) // Ensure processExtensions exists

	// Determine Exclude Patterns (Flag potentially overrides Config - check original config.go)
	finalExcludePatternsList := excludePatterns                                      // Simplification - assumes flag overrides default
	if !pflag.CommandLine.Changed("exclude") && len(appConfig.ExcludePatterns) > 0 { // Rough check
		finalExcludePatternsList = appConfig.ExcludePatterns
	}

	// --- Input Validation (Original basic checks) ---
	if len(finalExtensionsSet) == 0 && len(manualFiles) == 0 {
		slog.Error("Processing criteria missing. No extensions or manual files.")
		os.Exit(1)
	}

	// --- Generate Output ---
	concatenatedOutput, includedFiles, emptyFiles, errorFiles, totalSize, genErr := generateConcatenatedCode(
		finalTargetDirectory,
		finalExtensionsSet,
		manualFiles,
		finalExcludePatternsList,
		finalUseGitignore, // Pass the bool
		headerText,
		commentMarker,
	)

	// Handle error from generation (including NonExistentDir)
	if genErr != nil {
		slog.Error("Error during file processing.", "error", genErr)
		// Don't duplicate "not found" message if main already printed it
		if !(os.IsNotExist(genErr)) {
			fmt.Fprintf(os.Stderr, "Error during processing: %v\n", genErr)
		}
		os.Exit(1)
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

	// --- Print Summary ---
	printSummaryTree(includedFiles, emptyFiles, errorFiles, totalSize, finalTargetDirectory, summaryWriter) // Ensure printSummaryTree exists

	slog.Debug("Execution finished.")

	if len(errorFiles) > 0 { // Exit if file-specific errors occurred
		os.Exit(1)
	}
}

// --- generateConcatenatedCode (With NonExistentDir Fix) ---
func generateConcatenatedCode(
	dir string, // Expect absolute path
	exts map[string]struct{},
	manualFilePaths []string,
	excludePatterns []string,
	useGitignore bool, // Original boolean parameter
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

	// --- Directory Scanning ---
	shouldScan := len(exts) > 0 // Scan only if extensions are specified

	if shouldScan {
		slog.Info("Starting file scan.", "directory", dir)

		// *** ADDED: Check initial directory access right before walking ***
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

		// Load Gitignore (Original Logic)
		var gitignore *GitignoreMatcher
		var gitignoreErr error
		if useGitignore {
			gitignore, gitignoreErr = NewGitignoreMatcher(dir)
			if gitignoreErr != nil {
				slog.Warn("Could not initialize gitignore processor. Gitignore files will not be processed.", "directory", dir, "error", gitignoreErr)
				gitignore = nil
			} else if gitignore != nil && gitignore.matcher != nil {
				slog.Debug("Initialized gitignore processor.", "directory", dir)
			}
		}

		// Perform the Walk using filepath.WalkDir
		walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErrIn error) error {
			// --- Start of Original WalkDirFunc Logic ---
			if walkErrIn != nil {
				relPathErr, _ := filepath.Rel(dir, path)
				displayPathErr := filepath.ToSlash(relPathErr)
				if relPathErr == "" || strings.Contains(displayPathErr, "..") {
					displayPathErr = filepath.ToSlash(path)
				}
				slog.Warn("Error accessing path during walk, skipping entry.", "path", displayPathErr, "error", walkErrIn)
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

			slog.Debug("Walk: Processing entry", "path", relPath, "is_dir", d.IsDir())

			if relPath == "." {
				return nil
			}

			if d.IsDir() {
				if d.Name() == ".git" {
					slog.Debug("Walk: Skipping .git directory.", "path", relPath)
					return fs.SkipDir
				}
				if useGitignore && gitignore != nil && gitignore.Match(relPath, true) {
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
				return nil
			}

			// File Processing
			if processedFiles[absPath] {
				slog.Debug("Walk: Skipping file already processed manually.", "path", relPath)
				return nil
			}

			fileExt := strings.ToLower(filepath.Ext(path))
			if _, ok := exts[fileExt]; !ok {
				slog.Debug("Walk: Skipping file - extension not in included set", "path", relPath, "extension", fileExt)
				return nil
			}
			slog.Debug("Walk: Extension matched", "path", relPath, "extension", fileExt)

			if useGitignore && gitignore != nil && gitignore.Match(relPath, false) {
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

			slog.Debug("Adding file content to output.", "path", relPath, "size", fileSize)
			outputBuilder.WriteString(fmt.Sprintf("%s %s\n%s\n%s\n\n", marker, relPath, string(content), marker))
			includedFiles = append(includedFiles, FileInfo{Path: relPath, Size: fileSize, IsManual: false})
			totalSize += fileSize
			processedFiles[absPath] = true
			return nil
			// --- End of Original WalkDirFunc Logic ---
		})

		// Check the error returned by the WalkDir function itself
		if walkErr != nil {
			slog.Error("File walk initiation or execution failed.", "directory", dir, "error", walkErr)
			returnedErr = walkErr
			return // Exit the function
		} else {
			slog.Info("File scan completed.")
		}

	} // end if shouldScan

	// --- Append Metadata (Not implemented in this version) ---

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
	// Use %.1f for KiB and higher, %d for Bytes
	if exp == 0 {
		return fmt.Sprintf("%d B", b)
	}
	// Check if the value has no fractional part after formatting to 1 decimal place
	if val == float64(int64(val)) {
		return fmt.Sprintf("%d %ciB", int64(val), unitPrefix)
	}
	return fmt.Sprintf("%.1f %ciB", val, unitPrefix)
}

// buildTree builds the file tree structure for the summary.
func buildTree(files []FileInfo) *TreeNode {
	root := &TreeNode{Name: ".", Children: make(map[string]*TreeNode)}
	// Sort files by path for consistent tree structure
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	for i := range files {
		file := &files[i] // Use pointer to avoid copying struct in loop
		normalizedPath := filepath.ToSlash(file.Path)
		parts := strings.Split(normalizedPath, "/")
		currentNode := root
		for j, part := range parts {
			if part == "" { // Handle potential empty parts from //
				continue
			}
			isLastPart := (j == len(parts)-1)

			childNode, exists := currentNode.Children[part]
			if !exists {
				childNode = &TreeNode{Name: part, Children: make(map[string]*TreeNode)}
				currentNode.Children[part] = childNode
			}

			// If it's the last part, it represents the file itself
			if isLastPart {
				// Only assign FileInfo if it's not already assigned (prevents overwriting dir node with file info if names clash?)
				// Although directory entries shouldn't be in the includedFiles list typically.
				if childNode.FileInfo == nil {
					childNode.FileInfo = file
				} else {
					// This case might indicate an issue if a directory name matches a file name at the same level.
					slog.Warn("Tree building conflict: node already has FileInfo", "nodeName", childNode.Name)
				}
			}
			currentNode = childNode // Move down the tree
		}
	}
	return root
}

// printTreeRecursive recursively prints the file tree.
func printTreeRecursive(writer io.Writer, node *TreeNode, indent string, isLast bool) {
	// Skip printing the root node itself if called directly on root
	if node.Name == "." {
		// Process children directly without printing root indicator
		childNames := make([]string, 0, len(node.Children))
		for name := range node.Children {
			childNames = append(childNames, name)
		}
		sort.Strings(childNames) // Sort root children
		for i, name := range childNames {
			printTreeRecursive(writer, node.Children[name], indent, i == len(childNames)-1)
		}
		return
	}

	// Print current node
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

	// Prepare indent for children
	childIndent := indent
	if isLast {
		childIndent += "    "
	} else {
		childIndent += "│   "
	}

	// Sort and recurse on children
	childNames := make([]string, 0, len(node.Children))
	for name := range node.Children {
		childNames = append(childNames, name)
	}
	sort.Strings(childNames) // Sort children at each level

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
		// Determine a display name for the root of the tree
		treeRootName := filepath.Base(targetDir)
		// Handle cases where Base might return "." or "/"
		if treeRootName == "." || treeRootName == string(filepath.Separator) {
			// Use absolute path if Base gives something generic
			if abs, err := filepath.Abs(targetDir); err == nil {
				treeRootName = abs
			} else {
				treeRootName = targetDir // Fallback to original targetDir if Abs fails
			}
		}

		fmt.Fprintf(outputWriter, "Included %d files (%s total) from '%s':\n", len(includedFiles), formatBytes(totalSize), treeRootName)
		// Build and print the tree structure
		fileTree := buildTree(includedFiles)
		// Don't print the base directory name again if printTreeRecursive handles children directly
		// fmt.Fprintf(outputWriter, "%s/\n", treeRootName) // Optional: Print root dir name explicitly
		printTreeRecursive(outputWriter, fileTree, "", true) // Start recursion on root node

	} else {
		fmt.Fprintln(outputWriter, "No files included in the output.")
	}

	// Print empty files list
	if len(emptyFiles) > 0 {
		fmt.Fprintf(outputWriter, "\nEmpty files found (%d):\n", len(emptyFiles))
		sort.Strings(emptyFiles) // Sort for consistent output
		for _, p := range emptyFiles {
			// Use a simple list format
			fmt.Fprintf(outputWriter, "- %s\n", p)
		}
	}

	// Print errors encountered
	if len(errorFiles) > 0 {
		fmt.Fprintf(outputWriter, "\nErrors encountered (%d):\n", len(errorFiles))
		errorPaths := make([]string, 0, len(errorFiles))
		for p := range errorFiles {
			errorPaths = append(errorPaths, p)
		}
		sort.Strings(errorPaths) // Sort errors by path
		for _, p := range errorPaths {
			fmt.Fprintf(outputWriter, "- %s: %v\n", p, errorFiles[p])
		}
	}
	fmt.Fprintln(outputWriter, "---------------")
}
