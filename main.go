// main.go
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sabhiram/go-gitignore"
)

// --- Custom Flag Type (Keep as is) ---
type stringSliceFlag []string

// ... (String() and Set() methods remain the same) ...
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

// --- Global Variables for Flags & Config ---
var (
	targetDir       string
	extensions      stringSliceFlag
	manualFiles     stringSliceFlag
	excludePatterns stringSliceFlag
	noGitignore     bool

	appConfig Config

	// Derived/Processed values - These will be set in init() after config load
	commentMarker string
	headerText    string // Store the final header text
)

func init() {
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	var loadErr error
	appConfig, loadErr = loadConfig() // Load config first
	if loadErr != nil {
		// Logged in loadConfig, continue with defaults embedded in appConfig
		log.Printf("Warning: Configuration loading failed: %v. Proceeding with default settings.", loadErr)
	}

	// --- Set Derived Values from Config ---
	// Use loaded config value or the hardcoded default if loading failed/value unset
	commentMarker = *appConfig.CommentMarker
	headerText = *appConfig.HeaderText // Get header text from config
	initialUseGitignore := *appConfig.UseGitignore

	// --- Define Flags (Defaults based on potentially loaded config) ---
	flag.StringVar(&targetDir, "d", ".", "The target directory to scan recursively.")
	flag.StringVar(&targetDir, "directory", ".", "The target directory to scan recursively.")

	extensions = make(stringSliceFlag, 0)
	for _, ext := range appConfig.IncludeExtensions {
		extensions.Set(ext)
	}
	flag.Var(&extensions, "e", fmt.Sprintf("File extensions to include (repeatable). Default: %v", appConfig.IncludeExtensions))
	flag.Var(&extensions, "extensions", fmt.Sprintf("File extensions to include (repeatable). Default: %v", appConfig.IncludeExtensions))

	manualFiles = make(stringSliceFlag, 0)
	flag.Var(&manualFiles, "f", "Specific file paths to include manually (repeatable, bypasses ignores/excludes).")
	flag.Var(&manualFiles, "files", "Specific file paths to include manually (repeatable, bypasses ignores/excludes).")

	excludePatterns = make(stringSliceFlag, 0)
	for _, pattern := range appConfig.ExcludePatterns {
		excludePatterns.Set(pattern)
	}
	flag.Var(&excludePatterns, "x", fmt.Sprintf("Glob patterns to exclude (relative to target dir, repeatable). Default: %v", appConfig.ExcludePatterns))
	flag.Var(&excludePatterns, "exclude", fmt.Sprintf("Glob patterns to exclude (relative to target dir, repeatable). Default: %v", appConfig.ExcludePatterns))

	flag.BoolVar(&noGitignore, "no-gitignore", !initialUseGitignore, fmt.Sprintf("Disable .gitignore processing. Default based on config: %t", !initialUseGitignore))

	// --- Usage Message ---
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "Concatenate specified file types and/or specific files into a single output.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Reads config from ~/.config/food4ai/config.toml if it exists.\n")
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

// generateConcatenatedCode finds and concatenates files, applying filters.
func generateConcatenatedCode(
	targetDir string,
	extensionsToUse map[string]struct{},
	manualFiles []string,
	excludePatterns []string,
	useGitignore bool,
	// Pass determined values from config/flags
	headerText string,
	commentStart string,
) (string, error) {

	commentEnd := commentStart // Assuming symmetric markers
	// Start with the configured header text
	outputParts := []string{headerText + "\n"}
	filesToProcess := make(map[string]struct{}) // Absolute paths
	var emptyFilePaths []string                 // Store paths of empty files

	cwd, _ := os.Getwd()
	cwdAbs, _ := filepath.Abs(cwd)
	targetDirAbs, err := filepath.Abs(targetDir)
	if err != nil {
		log.Printf("Warning: Could not get absolute path for target directory '%s': %v", targetDir, err)
		targetDirAbs = targetDir
	}

	// --- 1. Process Manually Specified Files (Bypass Filters) ---
	// ... (Manual file processing logic remains the same) ...
	if len(manualFiles) > 0 {
		log.Println("Processing manually specified files (these bypass filters):")
		for _, fileStr := range manualFiles {
			absPath, err := filepath.Abs(fileStr)
			if err != nil {
				log.Printf("  Warning: Error getting absolute path for '%s': %v", fileStr, err)
				continue
			}
			fileInfo, err := os.Stat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					log.Printf("  Warning: Manual file not found: %s", fileStr)
				} else {
					log.Printf("  Warning: Error stating manual file '%s': %v", fileStr, err)
				}
				continue
			}
			if fileInfo.IsDir() {
				log.Printf("  Warning: Manual path is a directory: %s", fileStr)
				continue
			}
			if _, exists := filesToProcess[absPath]; exists {
				log.Printf("  Skipping duplicate: %s", fileStr)
			} else {
				log.Printf("  Adding: %s (resolved to %s)", fileStr, absPath)
				filesToProcess[absPath] = struct{}{}
			}
		}
		log.Println("--------------------")
	}

	// --- 2. Process Directory Scan (Apply Filters) ---
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
				log.Printf("Warning: Error compiling %s: %v. Proceeding without gitignore rules.", gitignorePath, err)
				ignorer = nil
			} else {
				log.Printf("Info: Applying .gitignore rules from %s", gitignorePath)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Printf("Warning: Could not stat %s: %v.", gitignorePath, err)
		} else {
			log.Println("Info: No .gitignore found in target directory.")
		}
	}

	dirInfo, err := os.Stat(targetDir)
	if err == nil && dirInfo.IsDir() {
		// Log scan parameters...
		log.Printf("Scanning directory: %s...", targetDirAbs)
		extKeys := make([]string, 0, len(extensionsToUse))
		for k := range extensionsToUse {
			extKeys = append(extKeys, k)
		}
		sort.Strings(extKeys)
		log.Printf("Looking for files with extensions: %s", strings.Join(extKeys, ", "))
		log.Printf("Applying .gitignore rules: %t", useGitignore && ignorer != nil)
		if len(excludePatterns) > 0 {
			log.Printf("Applying exclude patterns: %v", excludePatterns)
		}

		// Walk the directory
		walkErr := filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, walkErrIn error) error {
			if walkErrIn != nil {
				log.Printf("  Warning: Error accessing path %q: %v", path, walkErrIn)
				// Try to skip directory if possible, otherwise just skip the entry
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil // Skip this entry but continue walk
			}

			// --- Filter Directories ---
			if d.IsDir() {
				relPath, relErr := filepath.Rel(targetDirAbs, path)
				relPathSlash := ""
				if relErr == nil {
					relPathSlash = filepath.ToSlash(relPath)
				}

				// Check gitignore for directories
				if ignorer != nil && relErr == nil && ignorer.MatchesPath(relPathSlash) {
					log.Printf("  Skipping ignored directory (gitignore): %s", path)
					return fs.SkipDir
				}
				// Check exclude patterns for directories
				if relErr == nil { // Only match excludes if relative path worked
					for _, pattern := range excludePatterns {
						match, _ := filepath.Match(pattern, relPathSlash)
						if match {
							log.Printf("  Skipping excluded directory (pattern '%s'): %s", pattern, path)
							return fs.SkipDir
						}
					}
				} else {
					// Fallback: Check exclude pattern against directory name if relative fails?
					for _, pattern := range excludePatterns {
						match, _ := filepath.Match(pattern, d.Name()) // Match against the dir name itself
						if match {
							log.Printf("  Skipping excluded directory (pattern '%s', matched name): %s", pattern, path)
							return fs.SkipDir
						}
					}
				}

				return nil // It's a directory, but not ignored/excluded, continue descending
			}

			// --- Filter Files ---
			// 1. Check Extension
			ext := strings.ToLower(filepath.Ext(path))
			if _, shouldInclude := extensionsToUse[ext]; !shouldInclude {
				return nil // Skip file with wrong extension
			}

			absPath, absErr := filepath.Abs(path)
			if absErr != nil {
				log.Printf("  Warning: Could not get absolute path for '%s': %v", path, absErr)
				return nil // Skip if can't resolve
			}

			// 2. Check if already added manually
			if _, exists := filesToProcess[absPath]; exists {
				return nil // Already included via -f
			}

			relPath, relErr := filepath.Rel(targetDirAbs, absPath)
			relPathSlash := ""
			if relErr == nil {
				relPathSlash = filepath.ToSlash(relPath)
			} else {
				log.Printf("  Warning: Could not get relative path for '%s' against '%s': %v. Filters might not apply correctly.", absPath, targetDirAbs, relErr)
			}

			// 3. Check .gitignore (if enabled and relative path is valid)
			if ignorer != nil && relErr == nil && ignorer.MatchesPath(relPathSlash) {
				log.Printf("  Skipping ignored file (gitignore): %s", path)
				skippedByIgnore++
				return nil
			}

			// 4. Check Explicit Excludes
			excludeMatch := false
			if relErr == nil { // Prefer relative path matching
				for _, pattern := range excludePatterns {
					match, _ := filepath.Match(pattern, relPathSlash)
					if match {
						log.Printf("  Skipping excluded file (pattern '%s', matched relative): %s", pattern, path)
						excludeMatch = true
						break
					}
				}
			} else { // Fallback to matching basename
				baseName := filepath.Base(path)
				for _, pattern := range excludePatterns {
					match, _ := filepath.Match(pattern, baseName)
					if match {
						log.Printf("  Skipping excluded file (pattern '%s', matched basename): %s", pattern, path)
						excludeMatch = true
						break
					}
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
			log.Printf("Error during directory scan: %v", walkErr)
		}

		// Log scan summary...
		log.Printf("Found %d files via directory scan.", foundInScan)
		if skippedByIgnore > 0 {
			log.Printf("Skipped %d files/dirs due to .gitignore rules.", skippedByIgnore)
		}
		if skippedByExclude > 0 {
			log.Printf("Skipped %d files/dirs due to exclude patterns.", skippedByExclude)
		}
		log.Println("--------------------")

	} else if len(filesToProcess) == 0 { // No manual files AND directory scan couldn't run
		errMsg := fmt.Sprintf("Error: Directory '%s' not found or is not a directory, and no valid manual files provided.", targetDir)
		log.Println(errMsg)
		return "", fmt.Errorf(errMsg)
	}

	// --- 3. Sort and Concatenate ---
	if len(filesToProcess) == 0 {
		log.Println("Warning: No valid files found to process.")
		// Return just the header if nothing was found
		return headerText + "\n", nil
	}

	allFilesSorted := make([]string, 0, len(filesToProcess))
	for absPath := range filesToProcess {
		allFilesSorted = append(allFilesSorted, absPath)
	}
	sort.Strings(allFilesSorted)

	totalFiles := len(allFilesSorted)
	processedCount := 0
	log.Printf("Processing %d unique files...", totalFiles)

	for i, absPath := range allFilesSorted {
		// Determine display path (relative to CWD if possible)
		displayPath := absPath
		if cwdAbs != "" {
			relPath, err := filepath.Rel(cwdAbs, absPath)
			if err == nil {
				displayPath = relPath
			}
		}
		displayPathPosix := filepath.ToSlash(displayPath)

		// Read file content
		contentBytes, err := os.ReadFile(absPath)
		if err != nil {
			// Still include header/footer for files that failed to read
			errorMsg := fmt.Sprintf("# Error reading file %s: %v", displayPathPosix, err)
			outputParts = append(outputParts, fmt.Sprintf("\n%s %s\n%s\n%s\n",
				commentStart, displayPathPosix, errorMsg, commentEnd))
			log.Print(strings.TrimSpace(errorMsg)) // Log error to stderr
			continue                               // Go to next file
		}

		// --- Handle Empty vs Non-Empty ---
		if len(contentBytes) == 0 {
			emptyFilePaths = append(emptyFilePaths, displayPathPosix)
			log.Printf("  Found empty file: %s", displayPathPosix)
		} else {
			// Append non-empty file content with markers
			outputParts = append(outputParts, fmt.Sprintf("\n%s %s\n", commentStart, displayPathPosix))
			outputParts = append(outputParts, string(contentBytes))
			outputParts = append(outputParts, fmt.Sprintf("\n%s\n", commentEnd))
			processedCount++ // Count non-empty files processed
		}

		// Log progress
		if (i+1)%20 == 0 || i == totalFiles-1 {
			log.Printf("  Checked %d/%d files...", i+1, totalFiles)
		}
	}

	// --- Append the list of empty files, if any ---
	if len(emptyFilePaths) > 0 {
		log.Printf("Appending list of %d empty files found.", len(emptyFilePaths))
		outputParts = append(outputParts, "\nEmpty files:\n")
		for _, emptyPath := range emptyFilePaths {
			// Indent empty file paths
			outputParts = append(outputParts, fmt.Sprintf("\t%s\n", emptyPath))
		}
	}

	log.Printf("Finished. Processed content of %d non-empty files. Found %d empty files.", processedCount, len(emptyFilePaths))
	return strings.Join(outputParts, ""), nil
}

func main() {
	flag.Parse()

	finalExtensionsSet := processExtensions(extensions)
	finalUseGitignore := !noGitignore

	if len(finalExtensionsSet) == 0 && len(manualFiles) == 0 {
		log.Println("Error: No valid file extensions specified/defaulted, and no manual files provided.")
		os.Exit(1)
	} else if len(finalExtensionsSet) == 0 && len(manualFiles) > 0 {
		log.Println("Warning: No extensions specified/defaulted. Only manually specified files (-f) will be included.")
	}

	finalOutput, err := generateConcatenatedCode(
		targetDir,
		finalExtensionsSet,
		manualFiles,
		excludePatterns,
		finalUseGitignore,
		// Pass derived values
		headerText,
		commentMarker,
	)
	if err != nil {
		os.Exit(1) // Error already logged
	}

	// Check if the output contains more than just the initial header
	expectedHeader := headerText + "\n"
	if finalOutput != "" && finalOutput != expectedHeader {
		fmt.Print(finalOutput)
	} else if finalOutput == expectedHeader {
		log.Println("Warning: No files were found or included after filtering. Output contains only the header.")
		// Optionally print the header anyway? Or suppress all output? Let's suppress.
		// fmt.Print(finalOutput)
	} else {
		// This case should ideally not happen if headerText is never empty,
		// but good to handle just in case.
		log.Println("Output is empty.")
	}
}
