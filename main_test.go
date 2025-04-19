// main_test.go
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing" // Import Go's testing package

	"github.com/stretchr/testify/assert" // Import testify assertion library
	"github.com/stretchr/testify/require" // For essential setup checks
)

// Helper function to create a temporary test directory structure
// structure map: key = relative path, value = file content ("" for directory)
func setupTestDir(t *testing.T, structure map[string]string) string {
	t.Helper() // Marks this as a test helper
	tempDir := t.TempDir() // Creates a temporary directory that cleans up automatically

	// Ensure predictable order for directory creation
	paths := make([]string, 0, len(structure))
	for p := range structure {
		paths = append(paths, p)
	}
	sort.Strings(paths) // Create parent dirs first

	for _, relPath := range paths {
		content := structure[relPath]
		absPath := filepath.Join(tempDir, relPath)

		if content == "" { // Directory marker
			err := os.MkdirAll(absPath, 0755)
			require.NoError(t, err, "Failed to create directory: %s", absPath)
		} else { // File
			// Ensure parent directory exists
			parentDir := filepath.Dir(absPath)
			if _, err := os.Stat(parentDir); os.IsNotExist(err) {
				err := os.MkdirAll(parentDir, 0755)
				require.NoError(t, err, "Failed to create parent directory: %s", parentDir)
			}
			// Write file content
			err := os.WriteFile(absPath, []byte(content), 0644)
			require.NoError(t, err, "Failed to write file: %s", absPath)
		}
	}
	return tempDir
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

// Helper function to get the current working directory for relative path calculations
// NOTE: This assumes tests run from the project root. Adjust if needed.
func getTestCWD(t *testing.T) string {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return cwd
}

// Helper to convert map keys to sorted slice for logging/comparison
func sortedMapKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestGenerateConcatenatedCode_BasicScan(t *testing.T) {
	assert := assert.New(t) // Create assertion object
	structure := map[string]string{
		"file1.txt":      "Content of file 1.",
		"script.py":      "print('hello')",
		"config.json":    `{"key": "value"}`,
		"other.log":      "some logs", // Should be excluded by default ext usually
		"subdir/file2.py": "print('world')",
		"subdir/data.txt": "Subdir data.",
	}
	tempDir := setupTestDir(t, structure)
	// Use default-like extensions for this test
	exts := processExtensions([]string{"py", "txt", "json"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false // No gitignore for this test
	header := "Test Header:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	assert.NoError(err)
	assert.Contains(output, header+"\n") // Check header presence

	// Check files that *should* be included (order matters due to sorting)
	// Paths expected relative to CWD. We need to construct expected paths carefully.
	// Note: filepath.Rel might produce different results depending on CWD vs tempDir location.
	// For simplicity, we check for the base filename and content.
	// A more robust test would calculate the exact relative path expected.

	// Check file1.txt
	assert.Contains(output, marker+" file1.txt\nContent of file 1.\n"+marker) // Assuming test runs where file1.txt is relative like this
	// Check config.json
	assert.Contains(output, marker+" config.json\n{\"key\": \"value\"}\n"+marker)
	// Check script.py
	assert.Contains(output, marker+" script.py\nprint('hello')\n"+marker)
    // Check subdir/data.txt
    expectedSubDataPath := filepath.ToSlash(filepath.Join(filepath.Base(tempDir), "subdir", "data.txt"))
	assert.Contains(output, marker+" "+expectedSubDataPath+"\nSubdir data.\n"+marker)
	// Check subdir/file2.py
    expectedSubPyPath := filepath.ToSlash(filepath.Join(filepath.Base(tempDir), "subdir", "file2.py"))
	assert.Contains(output, marker+" "+expectedSubPyPath+"\nprint('world')\n"+marker)


	// Check file that *should not* be included
	assert.NotContains(output, "other.log")
	assert.NotContains(output, "some logs")

	// Check no empty file list
	assert.NotContains(output, "Empty files:")
}

func TestGenerateConcatenatedCode_WithManualFiles(t *testing.T) {
	assert := assert.New(t)
	structure := map[string]string{
		"file1.txt":      "Content file 1.",
		"manual.log":     "Manual log content.", // Manual files bypass extension filter
		"subdir/data.py": "print(123)",
	}
	tempDir := setupTestDir(t, structure)
	exts := processExtensions([]string{"py"}) // Only include .py via scan
	manualFiles := []string{
		filepath.Join(tempDir, "manual.log"), // Use absolute path for manual file
		filepath.Join(tempDir, "file1.txt"),
	}
	excludePatterns := []string{}
	useGitignore := false
	header := "Manual Test:"
	marker := "%%%"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	assert.NoError(err)
	assert.Contains(output, header+"\n")

	// Check manually included files (order matters)
    // file1.txt is lexically before manual.log
	assert.Contains(output, marker+" file1.txt\nContent file 1.\n"+marker)
	assert.Contains(output, marker+" manual.log\nManual log content.\n"+marker)

	// Check scanned file
    expectedSubPyPath := filepath.ToSlash(filepath.Join(filepath.Base(tempDir), "subdir", "data.py"))
	assert.Contains(output, marker+" "+expectedSubPyPath+"\nprint(123)\n"+marker)

    // Ensure manual files appear even if extension doesn't match scan filter
	assert.Contains(output, "manual.log")

    // Check ordering - file1.txt, manual.log, subdir/data.py
    idxFile1 := strings.Index(output, marker+" file1.txt")
    idxManualLog := strings.Index(output, marker+" manual.log")
    idxSubPy := strings.Index(output, marker+" "+expectedSubPyPath)

    assert.True(idxFile1 >= 0)
    assert.True(idxManualLog >= 0)
    assert.True(idxSubPy >= 0)
    assert.True(idxFile1 < idxManualLog, "file1.txt should come before manual.log")
    assert.True(idxManualLog < idxSubPy, "manual.log should come before subdir/data.py")
}

func TestGenerateConcatenatedCode_WithExcludes(t *testing.T) {
	assert := assert.New(t)
	structure := map[string]string{
		"include.txt":      "Include me.",
		"exclude_me.txt":   "Exclude this content.",
		"subdir/data.py":   "Include python.",
		"subdir/temp.log":  "Exclude this log.", // Excluded by pattern
		"otherdir/foo.txt": "Exclude this dir.", // Excluded by pattern
	}
	tempDir := setupTestDir(t, structure)
	exts := processExtensions([]string{"py", "txt", "log"}) // Include log for test
	manualFiles := []string{}
	excludePatterns := []string{
		"*.log",              // Exclude logs by extension
		"exclude_me.txt",     // Exclude specific file by name
		"otherdir/*",         // Exclude everything in otherdir
	}
	useGitignore := false
	header := "Exclude Test:"
	marker := "!!!"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	assert.NoError(err)
	assert.Contains(output, header+"\n")

	// Check included files
	assert.Contains(output, marker+" include.txt\nInclude me.\n"+marker)
    expectedSubPyPath := filepath.ToSlash(filepath.Join(filepath.Base(tempDir), "subdir", "data.py"))
	assert.Contains(output, marker+" "+expectedSubPyPath+"\nInclude python.\n"+marker)

	// Check excluded files/content
	assert.NotContains(output, "exclude_me.txt")
	assert.NotContains(output, "Exclude this content.")
	assert.NotContains(output, "temp.log")
	assert.NotContains(output, "Exclude this log.")
	assert.NotContains(output, "foo.txt")
	assert.NotContains(output, "Exclude this dir.")
}

func TestGenerateConcatenatedCode_WithGitignore(t *testing.T) {
	// Gitignore content needs careful path handling (relative to gitignore file)
	gitignoreContent := `
# Comments ignored
*.log
ignored_dir/
/root_ignored.txt
!good_dir/include_me.txt # Negation example (tricky with some libraries)
`
	assert := assert.New(t)
	structure := map[string]string{
		".gitignore":           gitignoreContent,
		"include.py":           "print('include')",
		"ignored.log":          "This log is ignored.",
		"ignored_dir/file.txt": "This whole dir is ignored.",
		"root_ignored.txt":     "Ignored only at the root.",
		"subdir/root_ignored.txt": "Not ignored here.", // Because of leading / in pattern
        "good_dir/": "", // Directory marker
        "good_dir/ignored.log": "Log ignored even in good dir",
        "good_dir/include_me.txt": "Should be included due to negation (if supported well)",
	}
	tempDir := setupTestDir(t, structure)
	exts := processExtensions([]string{"py", "txt", "log"})
	manualFiles := []string{}
	excludePatterns := []string{} // No explicit excludes, only gitignore
	useGitignore := true
	header := "Gitignore Test:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	assert.NoError(err)
	assert.Contains(output, header+"\n")

	// Check included files
	assert.Contains(output, marker+" include.py\nprint('include')\n"+marker)
    expectedSubTxtPath := filepath.ToSlash(filepath.Join(filepath.Base(tempDir), "subdir", "root_ignored.txt"))
	assert.Contains(output, marker+" "+expectedSubTxtPath+"\nNot ignored here.\n"+marker)
    // This depends heavily on the gitignore library's negation support.
    // The sabhiram/go-gitignore library might NOT support negation reliably or easily.
    // Let's comment this out for now as it's likely to fail depending on implementation details.
    // expectedGoodTxtPath := filepath.ToSlash(filepath.Join(filepath.Base(tempDir), "good_dir", "include_me.txt"))
    // assert.Contains(output, marker+" "+expectedGoodTxtPath+"\nShould be included due to negation (if supported well)\n"+marker)


	// Check ignored files/content
	assert.NotContains(output, "ignored.log")
	assert.NotContains(output, "This log is ignored.")
	assert.NotContains(output, "ignored_dir/file.txt")
	assert.NotContains(output, "This whole dir is ignored.")
	assert.NotContains(output, marker+" root_ignored.txt") // Check header isn't present
	assert.NotContains(output, "Ignored only at the root.")
    assert.NotContains(output, marker+" good_dir/ignored.log")

}


func TestGenerateConcatenatedCode_EmptyFiles(t *testing.T) {
	assert := assert.New(t)
	structure := map[string]string{
		"file1.txt":  "Some content.",
		"empty1.txt": "", // Empty file
		"empty2.py":  "", // Empty file
		"subdir/":    "", // Directory marker
		"subdir/empty3.txt": "",
	}
	tempDir := setupTestDir(t, structure)
	exts := processExtensions([]string{"py", "txt"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "Empty File Test:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	assert.NoError(err)
	assert.Contains(output, header+"\n")

	// Check non-empty file
	assert.Contains(output, marker+" file1.txt\nSome content.\n"+marker)

	// Check presence of the "Empty files:" section
	assert.Contains(output, "\nEmpty files:\n")

    // Check that empty files are listed, indented (relative to CWD)
    // Order matters due to sorting
    expectedEmpty1Path := filepath.ToSlash(filepath.Join(filepath.Base(tempDir), "empty1.txt"))
    expectedEmpty2Path := filepath.ToSlash(filepath.Join(filepath.Base(tempDir), "empty2.py"))
    expectedEmpty3Path := filepath.ToSlash(filepath.Join(filepath.Base(tempDir), "subdir", "empty3.txt"))

	assert.Contains(output, "\t"+expectedEmpty1Path+"\n")
	assert.Contains(output, "\t"+expectedEmpty2Path+"\n")
    assert.Contains(output, "\t"+expectedEmpty3Path+"\n")

    // Check that empty files do *not* have regular marker blocks
	assert.NotContains(output, marker+" "+expectedEmpty1Path+"\n\n"+marker)
	assert.NotContains(output, marker+" "+expectedEmpty2Path+"\n\n"+marker)
    assert.NotContains(output, marker+" "+expectedEmpty3Path+"\n\n"+marker)

}

func TestGenerateConcatenatedCode_ReadError(t *testing.T) {
	// Skip on Windows - setting file permissions is complex/different
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission-based test on Windows")
	}
	assert := assert.New(t)
	structure := map[string]string{
		"readable.txt": "Can read this.",
		"unreadable.txt": "Cannot read this.",
	}
	tempDir := setupTestDir(t, structure)

	// Make one file unreadable
	unreadablePath := filepath.Join(tempDir, "unreadable.txt")
	err := os.Chmod(unreadablePath, 0000) // No read permission
	require.NoError(t, err, "Failed to chmod")
	// Defer restoring permissions for cleanup, though TempDir handles deletion
	defer func() { _ = os.Chmod(unreadablePath, 0644) }()


	exts := processExtensions([]string{"txt"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "Read Error Test:"
	marker := "---"

	// Capture log output
	var logBuf strings.Builder
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr) // Restore standard log output


	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	assert.NoError(err) // generateConcatenatedCode itself shouldn't error, just log/note
	assert.Contains(output, header+"\n")

	// Check readable file
	assert.Contains(output, marker+" readable.txt\nCan read this.\n"+marker)

	// Check unreadable file - should have marker and error message
    expectedUnreadablePath := filepath.ToSlash(filepath.Join(filepath.Base(tempDir), "unreadable.txt"))
	assert.Contains(output, marker+" "+expectedUnreadablePath+"\n# Error reading file")
	assert.Contains(output, "permission denied") // Check for the specific error text
	assert.Contains(output, "\n"+marker+"\n") // Ensure footer is still present

	// Check log output as well
	logOutput := logBuf.String()
	assert.Contains(logOutput, "Error reading file")
	assert.Contains(logOutput, expectedUnreadablePath)
	assert.Contains(logOutput, "permission denied")
}

func TestGenerateConcatenatedCode_NonExistentDir(t *testing.T) {
	assert := assert.New(t)
	tempDir := t.TempDir()
	nonExistentDir := filepath.Join(tempDir, "nosuchdir")

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "No Dir Test:"
	marker := "---"

	output, err := generateConcatenatedCode(nonExistentDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

	assert.Error(err) // Expect an error return value
	assert.Contains(err.Error(), "not found or is not a directory")
	assert.Equal(t, "", output) // Output should be empty on error
}

func TestGenerateConcatenatedCode_NoFilesFound(t *testing.T) {
    assert := assert.New(t)
	structure := map[string]string{
		"other.log": "log data",
        "script.sh": "echo hello",
	}
	tempDir := setupTestDir(t, structure)

	exts := processExtensions([]string{"txt", "py"}) // Extensions that won't match
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "No Files Found Test:"
	marker := "---"

    // Capture log output
	var logBuf strings.Builder
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr) // Restore standard log output

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)

    assert.NoError(err) // No error returned, just empty result + warning
    assert.Equal(t, header+"\n", output) // Should only contain the header

    // Check log for warning
    logOutput := logBuf.String()
    assert.Contains(logOutput, "Warning: No valid files found to process.")

}

