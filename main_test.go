// main_test.go
package main

import (
	"bytes" // Import bytes for capturing log output
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing" // Import Go's testing package

	"github.com/stretchr/testify/assert"  // Import testify assertion library
	"github.com/stretchr/testify/require" // For essential setup checks
)

// Helper function to create a temporary test directory structure
// structure map: key = relative path, value = file content ("" for directory)
func setupTestDir(t *testing.T, structure map[string]string) string {
	t.Helper()
	tempDir := t.TempDir()

	paths := make([]string, 0, len(structure))
	for p := range structure {
		paths = append(paths, p)
	}
	sort.Strings(paths) // Ensure deterministic creation order

	for _, relPath := range paths {
		content := structure[relPath]
		absPath := filepath.Join(tempDir, relPath)

		if content == "" { // Directory marker
			err := os.MkdirAll(absPath, 0755)
			require.NoError(t, err, "Failed to create directory: %s", absPath)
		} else { // File
			parentDir := filepath.Dir(absPath)
			// Check if parent needs creation *before* trying to create it
			if _, err := os.Stat(parentDir); os.IsNotExist(err) {
				errMkdir := os.MkdirAll(parentDir, 0755)
				require.NoError(t, errMkdir, "Failed to create parent directory: %s", parentDir)
			} else {
				// Handle other potential errors from Stat if needed
				require.NoError(t, err, "Failed to stat parent directory: %s", parentDir)
			}
			err := os.WriteFile(absPath, []byte(content), 0644)
			require.NoError(t, err, "Failed to write file: %s", absPath)
		}
	}
	return tempDir
}

// Helper function to get the current working directory for relative path calculations
func getTestCWD(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return cwd
}

// Helper to create expected display path (relative to CWD, posix slash)
func expectDisplayPath(t *testing.T, cwd, absPath string) string {
	t.Helper()
	relPath, err := filepath.Rel(cwd, absPath)
	// If Rel fails (e.g., different drives on Windows), display path defaults to absolute
	if err != nil {
		relPath = absPath
	}
	return filepath.ToSlash(relPath)
}

// Helper to setup a test logger capturing output
func setupTestLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var logBuf bytes.Buffer
	// Use a low level like Debug for tests to capture everything if needed
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	return logger, &logBuf
}

// --- Tests for processExtensions ---

func TestProcessExtensions(t *testing.T) {
	testCases := []struct {
		name     string
		input    []string
		expected map[string]struct{}
	}{
		{
			name:     "Empty input",
			input:    []string{},
			expected: map[string]struct{}{},
		},
		{
			name:     "Basic extensions",
			input:    []string{"py", "txt", "json"},
			expected: map[string]struct{}{".py": {}, ".txt": {}, ".json": {}},
		},
		{
			name:     "With leading dots",
			input:    []string{".py", "txt", ".json"},
			expected: map[string]struct{}{".py": {}, ".txt": {}, ".json": {}},
		},
		{
			name:     "Mixed case",
			input:    []string{"Py", ".TXT", "jSoN"},
			expected: map[string]struct{}{".py": {}, ".txt": {}, ".json": {}},
		},
		{
			name:     "With whitespace",
			input:    []string{" py ", " .txt"},
			expected: map[string]struct{}{".py": {}, ".txt": {}},
		},
		{
			name:     "With empty strings",
			input:    []string{"py", "", " ", ".txt"},
			expected: map[string]struct{}{".py": {}, ".txt": {}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := processExtensions(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

// --- Tests for generateConcatenatedCode ---

// Setup default test logger for tests that don't need specific log checks
func TestMain(m *testing.M) {
	// Set a default discard logger for tests unless overridden
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))) // Discard by default
	os.Exit(m.Run())
}

func TestGenerateConcatenatedCode_BasicScan(t *testing.T) {
	assert := assert.New(t)
	structure := map[string]string{
		"file1.txt":       "Content of file 1.",
		"script.py":       "print('hello')",
		"config.json":     `{"key": "value"}`,
		"other.log":       "some logs",
		"subdir/file2.py": "print('world')",
		"subdir/data.txt": "Subdir data.",
	}
	tempDir := setupTestDir(t, structure)
	cwd := getTestCWD(t)

	// Setup logger for this test (optional, can use default discard)
	testLogger, _ := setupTestLogger(t)
	slog.SetDefault(testLogger) // Set as default for generateConcatenatedCode

	exts := processExtensions([]string{"py", "txt", "json"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "Test Header:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assert.NoError(err)
	assert.Contains(output, header+"\n")

	// Check file1.txt
	expectedPathFile1 := expectDisplayPath(t, cwd, filepath.Join(tempDir, "file1.txt"))
	assert.Contains(t, output, marker+" "+expectedPathFile1+"\nContent of file 1.\n"+marker)
	// Check config.json
	expectedPathConfig := expectDisplayPath(t, cwd, filepath.Join(tempDir, "config.json"))
	assert.Contains(t, output, marker+" "+expectedPathConfig+"\n{\"key\": \"value\"}\n"+marker)
	// Check script.py
	expectedPathScript := expectDisplayPath(t, cwd, filepath.Join(tempDir, "script.py"))
	assert.Contains(t, output, marker+" "+expectedPathScript+"\nprint('hello')\n"+marker)
	// Check subdir/data.txt
	expectedPathSubData := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/data.txt"))
	assert.Contains(t, output, marker+" "+expectedPathSubData+"\nSubdir data.\n"+marker)
	// Check subdir/file2.py
	expectedPathSubPy := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/file2.py"))
	assert.Contains(t, output, marker+" "+expectedPathSubPy+"\nprint('world')\n"+marker)

	// Check file that *should not* be included
	assert.NotContains(t, output, "other.log")
	assert.NotContains(t, output, "some logs")
	assert.NotContains(t, output, "Empty files:")
}

func TestGenerateConcatenatedCode_WithManualFiles(t *testing.T) {
	assert := assert.New(t)
	structure := map[string]string{
		"file1.txt":      "Content file 1.",
		"manual.log":     "Manual log content.",
		"subdir/data.py": "print(123)",
	}
	tempDir := setupTestDir(t, structure)
	cwd := getTestCWD(t)

	testLogger, _ := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py"}) // Only scan for .py
	manualLogPath := filepath.Join(tempDir, "manual.log")
	file1Path := filepath.Join(tempDir, "file1.txt")
	manualFiles := []string{manualLogPath, file1Path} // Manual files bypass filters

	excludePatterns := []string{}
	useGitignore := false
	header := "Manual Test:"
	marker := "%%%"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assert.NoError(err)
	assert.Contains(output, header+"\n")

	// Expected display paths calculated relative to CWD
	expectedPathFile1 := expectDisplayPath(t, cwd, file1Path)
	expectedPathManualLog := expectDisplayPath(t, cwd, manualLogPath)
	expectedPathSubPy := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/data.py"))

	// Check included files (actual output order depends on sorted absolute paths)
	assert.Contains(t, output, marker+" "+expectedPathFile1+"\nContent file 1.\n"+marker)
	assert.Contains(t, output, marker+" "+expectedPathManualLog+"\nManual log content.\n"+marker)
	assert.Contains(t, output, marker+" "+expectedPathSubPy+"\nprint(123)\n"+marker)

	// Ensure manual files appear even if extension doesn't match scan filter
	assert.Contains(t, output, "manual.log") // Base name check is still useful here

	// Check ordering
	// Get absolute paths to sort correctly, then map back to expected display paths
	absFile1Path, _ := filepath.Abs(file1Path)
	absManualLogPath, _ := filepath.Abs(manualLogPath)
	absSubPyPath, _ := filepath.Abs(filepath.Join(tempDir, "subdir/data.py"))
	absPathsSorted := []string{absFile1Path, absManualLogPath, absSubPyPath}
	sort.Strings(absPathsSorted)

	expectedPathsSorted := []string{
		expectDisplayPath(t, cwd, absPathsSorted[0]),
		expectDisplayPath(t, cwd, absPathsSorted[1]),
		expectDisplayPath(t, cwd, absPathsSorted[2]),
	}

	idx0 := strings.Index(output, marker+" "+expectedPathsSorted[0])
	idx1 := strings.Index(output, marker+" "+expectedPathsSorted[1])
	idx2 := strings.Index(output, marker+" "+expectedPathsSorted[2])

	assert.True(t, idx0 >= 0, "Index path0 missing: %s", expectedPathsSorted[0])
	assert.True(t, idx1 >= 0, "Index path1 missing: %s", expectedPathsSorted[1])
	assert.True(t, idx2 >= 0, "Index path2 missing: %s", expectedPathsSorted[2])
	assert.True(t, idx0 < idx1, "%q should come before %q", expectedPathsSorted[0], expectedPathsSorted[1])
	assert.True(t, idx1 < idx2, "%q should come before %q", expectedPathsSorted[1], expectedPathsSorted[2])
}

func TestGenerateConcatenatedCode_WithExcludes(t *testing.T) {
	assert := assert.New(t)
	structure := map[string]string{
		"include.txt":      "Include me.",
		"exclude_me.txt":   "Exclude this content.",
		"subdir/data.py":   "Include python.",
		"subdir/temp.log":  "Exclude this log.",
		"otherdir/foo.txt": "Exclude this dir.",
	}
	tempDir := setupTestDir(t, structure)
	cwd := getTestCWD(t)

	testLogger, _ := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt", "log"}) // Include log initially
	manualFiles := []string{}
	excludePatterns := []string{
		"*.log",          // Exclude all logs
		"exclude_me.txt", // Exclude specific file by name
		"otherdir/*",     // Exclude everything in otherdir
	}
	useGitignore := false
	header := "Exclude Test:"
	marker := "!!!"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	// ***** THIS IS THE CORRECTED LINE *****
	// Ensure 't' is the first argument and 'err' is the second.
	assert.NoError(t, err, "generateConcatenatedCode failed unexpectedly")
	// ***** END CORRECTION *****

	assert.Contains(t, output, header+"\n")

	// Check included files
	expectedPathInclude := expectDisplayPath(t, cwd, filepath.Join(tempDir, "include.txt"))
	assert.Contains(t, output, marker+" "+expectedPathInclude+"\nInclude me.\n"+marker)
	expectedPathSubPy := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/data.py"))
	assert.Contains(t, output, marker+" "+expectedPathSubPy+"\nInclude python.\n"+marker)

	// Check excluded files/content
	expectedPathExcludeMe := expectDisplayPath(t, cwd, filepath.Join(tempDir, "exclude_me.txt"))
	assert.NotContains(t, output, marker+" "+expectedPathExcludeMe, "exclude_me.txt block should not be present")
	assert.NotContains(t, output, "Exclude this content.", "exclude_me.txt content should not be present")

	expectedPathTempLog := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/temp.log"))
	assert.NotContains(t, output, marker+" "+expectedPathTempLog, "temp.log block should not be present")
	assert.NotContains(t, output, "Exclude this log.", "temp.log content should not be present")

	expectedPathOtherFoo := expectDisplayPath(t, cwd, filepath.Join(tempDir, "otherdir/foo.txt"))
	assert.NotContains(t, output, marker+" "+expectedPathOtherFoo, "otherdir/foo.txt block should not be present")
	assert.NotContains(t, output, "Exclude this dir.", "otherdir/foo.txt content should not be present")
}

func TestGenerateConcatenatedCode_WithGitignore(t *testing.T) {
	gitignoreContent := `
# Comments ignored
*.log
ignored_dir/
/root_ignored.txt
!good_dir/include_me.txt
`
	assert := assert.New(t)
	structure := map[string]string{
		".gitignore":              gitignoreContent,
		"include.py":              "print('include')",
		"ignored.log":             "This log is ignored.",
		"ignored_dir/":            "", // Directory marker
		"ignored_dir/file.txt":    "This whole dir is ignored.",
		"root_ignored.txt":        "Ignored only at the root.",
		"subdir/":                 "", // Directory marker
		"subdir/root_ignored.txt": "Not ignored here.",
		"good_dir/":               "",
		"good_dir/ignored.log":    "Log ignored even in good dir",
		"good_dir/include_me.txt": "Should be included due to negation",
	}
	tempDir := setupTestDir(t, structure)
	cwd := getTestCWD(t)

	testLogger, logBuf := setupTestLogger(t) // Capture logs
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt", "log"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := true
	header := "Gitignore Test:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assert.NoError(err)
	assert.Contains(output, header+"\n")
	logOutput := logBuf.String() // Get logs after execution

	// Check included files
	expectedPathIncludePy := expectDisplayPath(t, cwd, filepath.Join(tempDir, "include.py"))
	assert.Contains(t, output, marker+" "+expectedPathIncludePy+"\nprint('include')\n"+marker)

	expectedPathSubTxt := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/root_ignored.txt"))
	assert.Contains(t, output, marker+" "+expectedPathSubTxt+"\nNot ignored here.\n"+marker)

	// Check negation: sabhiram/go-gitignore DOES support basic negation like this.
	// We expect include_me.txt to be present.
	expectedGoodTxtPath := expectDisplayPath(t, cwd, filepath.Join(tempDir, "good_dir/include_me.txt"))
	assert.Contains(t, output, marker+" "+expectedGoodTxtPath+"\nShould be included due to negation\n"+marker, "Negation pattern !good_dir/include_me.txt failed")

	// Check ignored files/content are NOT present
	expectedPathIgnoredLog := expectDisplayPath(t, cwd, filepath.Join(tempDir, "ignored.log"))
	assert.NotContains(t, output, marker+" "+expectedPathIgnoredLog, "ignored.log block should not be present")
	assert.NotContains(t, output, "This log is ignored.")

	expectedPathIgnoredDirFile := expectDisplayPath(t, cwd, filepath.Join(tempDir, "ignored_dir/file.txt"))
	assert.NotContains(t, output, marker+" "+expectedPathIgnoredDirFile, "ignored_dir/file.txt block should not be present")
	assert.NotContains(t, output, "This whole dir is ignored.")

	expectedPathRootIgnored := expectDisplayPath(t, cwd, filepath.Join(tempDir, "root_ignored.txt"))
	assert.NotContains(t, output, marker+" "+expectedPathRootIgnored, "root_ignored.txt block should not be present")
	assert.NotContains(t, output, "Ignored only at the root.")

	expectedPathGoodIgnoredLog := expectDisplayPath(t, cwd, filepath.Join(tempDir, "good_dir/ignored.log"))
	assert.NotContains(t, output, marker+" "+expectedPathGoodIgnoredLog, "good_dir/ignored.log block should not be present")

	// Optional: Check logs for skipped files if needed
	assert.Contains(t, logOutput, "Skipping ignored file (gitignore).", "Expected log message for ignored file")
	assert.Contains(t, logOutput, "path=ignored.log", "Expected ignored file path in logs") // Example check
}

func TestGenerateConcatenatedCode_EmptyFiles(t *testing.T) {
	assert := assert.New(t)
	structure := map[string]string{
		"file1.txt":         "Some content.",
		"empty1.txt":        "", // Empty file
		"empty2.py":         "", // Empty file
		"subdir/":           "", // Directory marker
		"subdir/empty3.txt": "",
		"non_empty.py":      "pass",
	}
	tempDir := setupTestDir(t, structure)
	cwd := getTestCWD(t)

	testLogger, _ := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "Empty File Test:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assert.NoError(err)
	assert.Contains(output, header+"\n")

	// Check non-empty files
	expectedPathFile1 := expectDisplayPath(t, cwd, filepath.Join(tempDir, "file1.txt"))
	assert.Contains(t, output, marker+" "+expectedPathFile1+"\nSome content.\n"+marker)
	expectedPathNonEmptyPy := expectDisplayPath(t, cwd, filepath.Join(tempDir, "non_empty.py"))
	assert.Contains(t, output, marker+" "+expectedPathNonEmptyPy+"\npass\n"+marker)

	// Check presence of the "Empty files:" section
	assert.Contains(t, output, "\nEmpty files:\n")

	// Check that empty files are listed, indented
	expectedEmpty1Path := expectDisplayPath(t, cwd, filepath.Join(tempDir, "empty1.txt"))
	expectedEmpty2Path := expectDisplayPath(t, cwd, filepath.Join(tempDir, "empty2.py"))
	expectedEmpty3Path := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/empty3.txt"))

	// Build expected empty file list section (order matters due to sorting in main code)
	emptyPaths := []string{expectedEmpty1Path, expectedEmpty2Path, expectedEmpty3Path}
	sort.Strings(emptyPaths) // Sort them to match output order expected from main func
	expectedEmptyList := "\nEmpty files:\n"
	for _, p := range emptyPaths {
		expectedEmptyList += "\t" + p + "\n"
	}
	// Use TrimSpace because the final output might have extra trailing newline
	assert.Contains(t, output, expectedEmptyList)

	// Check that empty files do *not* have regular marker blocks
	assert.NotContains(t, output, marker+" "+expectedEmpty1Path+"\n\n"+marker)
	assert.NotContains(t, output, marker+" "+expectedEmpty2Path+"\n\n"+marker)
	assert.NotContains(t, output, marker+" "+expectedEmpty3Path+"\n\n"+marker)
}

func TestGenerateConcatenatedCode_ReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission-based read error test on Windows")
	}
	assert := assert.New(t)
	structure := map[string]string{
		"readable.txt":   "Can read this.",
		"unreadable.txt": "Cannot read this.",
	}
	tempDir := setupTestDir(t, structure)
	cwd := getTestCWD(t)

	testLogger, logBuf := setupTestLogger(t) // Capture logs
	slog.SetDefault(testLogger)

	unreadablePath := filepath.Join(tempDir, "unreadable.txt")
	err := os.Chmod(unreadablePath, 0000) // Make unreadable
	require.NoError(t, err, "Failed to chmod unreadable.txt")
	// Defer restoring permissions AFTER the function call returns
	defer func() {
		errChmodBack := os.Chmod(unreadablePath, 0644)
		assert.NoError(t, errChmodBack, "Failed to chmod unreadable.txt back")
	}()

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "Read Error Test:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	// The function itself shouldn't error out, just include the error message
	assert.NoError(t, err, "generateConcatenatedCode should not return error for file read errors")
	assert.Contains(output, header+"\n")

	// Check readable file
	expectedReadablePath := expectDisplayPath(t, cwd, filepath.Join(tempDir, "readable.txt"))
	assert.Contains(t, output, marker+" "+expectedReadablePath+"\nCan read this.\n"+marker)

	// Check unreadable file block structure
	expectedUnreadablePath := expectDisplayPath(t, cwd, unreadablePath)
	// Check for the marker block surrounding the error message
	assert.Contains(t, output, marker+" "+expectedUnreadablePath+"\n# Error reading file")
	// Check specific error text within the block
	assert.Contains(t, output, "permission denied")
	// Check for the closing marker
	assert.Contains(t, output, "\n"+marker+"\n")

	// Check log output
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "level=WARN", "Expected WARN level log")
	assert.Contains(t, logOutput, "Error reading file content", "Expected log message")
	assert.Contains(t, logOutput, "file="+expectedUnreadablePath, "Expected file path in log")
	assert.Contains(t, logOutput, "permission denied", "Expected error details in log")
}

func TestGenerateConcatenatedCode_NonExistentDir(t *testing.T) {
	assert := assert.New(t)
	tempDir := t.TempDir()
	nonExistentDir := filepath.Join(tempDir, "nosuchdir")

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{} // No manual files to fall back on
	excludePatterns := []string{}
	useGitignore := false
	header := "No Dir Test:"
	marker := "---"

	output, err := generateConcatenatedCode(nonExistentDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	assert.Error(t, err, "Expected an error when target directory doesn't exist")
	assert.Contains(t, err.Error(), "not found or is not accessible", "Error message mismatch")
	assert.Equal(t, "", output, "Output should be empty on error return")

	// Check logs
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "level=ERROR", "Expected ERROR level log")
	assert.Contains(t, logOutput, "Target directory", "Expected log message")
	assert.Contains(t, logOutput, "not found or is not accessible", "Expected error details in log")
}

func TestGenerateConcatenatedCode_NonExistentDir_WithManualFile(t *testing.T) {
	assert := assert.New(t)
	tempDir := t.TempDir()
	nonExistentDir := filepath.Join(tempDir, "nosuchdir")
	// Create a single manual file outside the non-existent dir
	manualFilePath := filepath.Join(tempDir, "manual.txt")
	errWrite := os.WriteFile(manualFilePath, []byte("Manual content"), 0644)
	require.NoError(t, errWrite)

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{manualFilePath} // Provide a valid manual file
	excludePatterns := []string{}
	useGitignore := false
	header := "No Dir But Manual File Test:"
	marker := "---"

	output, err := generateConcatenatedCode(nonExistentDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	// Should NOT return an error because the manual file allows processing to continue
	assert.NoError(t, err, "Should not error out if manual files are provided, even if target dir fails")
	assert.Contains(output, header+"\n")

	// Check that the manual file was processed
	cwd := getTestCWD(t)
	expectedManualPath := expectDisplayPath(t, cwd, manualFilePath)
	assert.Contains(t, output, marker+" "+expectedManualPath+"\nManual content\n"+marker)

	// Check logs for the warning about the directory
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "level=ERROR", "Expected ERROR level log for dir") // Changed to Error based on latest code
	assert.Contains(t, logOutput, "Target directory", "Expected log message about dir")
	assert.Contains(t, logOutput, "not found or is not accessible", "Expected dir error details")
	assert.Contains(t, logOutput, "level=WARN", "Expected WARN level log for proceeding") // Changed to Warn based on latest code
	assert.Contains(t, logOutput, "Proceeding with only manually specified files", "Expected proceeding message")
}

func TestGenerateConcatenatedCode_NoFilesFound(t *testing.T) {
	assert := assert.New(t)
	structure := map[string]string{
		"other.log": "log data",
		"script.sh": "echo hello",
	}
	tempDir := setupTestDir(t, structure)

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"txt", "py"}) // Extensions that won't match
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "No Files Found Test:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	assert.NoError(t, err)
	// Output should only contain the header and a newline
	assert.Equal(t, header+"\n", output, "Output should only be the header when no files are found")

	logOutput := logBuf.String()
	// Check for the specific warning log message
	assert.Contains(t, logOutput, "level=WARN", "Expected WARN level log")
	assert.Contains(t, logOutput, "No files found to process", "Expected 'No files found' warning log not found")
}
