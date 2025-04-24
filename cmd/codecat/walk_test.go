// cmd/codecat/walk_test.go
package main

import (
	"bytes" // For setupTestLogger
	"errors"
	"fmt" // For getSortedKeys helper
	"io/fs"
	"log/slog" // For setupTestLogger
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Helper Functions (Keep here or move to shared file like test_helpers.go) ---

func setupTestDir(t *testing.T, structure map[string]string) string {
	t.Helper()
	tempDir := t.TempDir()
	paths := make([]string, 0, len(structure))
	for p := range structure {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, relPath := range paths {
		content := structure[relPath]
		absPath := filepath.Join(tempDir, relPath)
		parentDir := filepath.Dir(absPath)
		if _, err := os.Stat(parentDir); os.IsNotExist(err) {
			errMkdir := os.MkdirAll(parentDir, 0755)
			require.NoError(t, errMkdir, "Failed to create parent directory: %s", parentDir)
		} else {
			require.NoError(t, err, "Failed to stat parent directory: %s", parentDir)
		}

		if strings.HasSuffix(relPath, string(filepath.Separator)) || (content == "" && !strings.Contains(filepath.Base(relPath), ".")) {
			err := os.MkdirAll(absPath, 0755)
			require.NoError(t, err, "Failed to create directory: %s", absPath)
		} else {
			err := os.WriteFile(absPath, []byte(content), 0644)
			require.NoError(t, err, "Failed to write file: %s", absPath)
		}
	}
	return tempDir
}

func setupTestLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var logBuf bytes.Buffer
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	return logger, &logBuf
}

func getPathsFromIncludedFiles(files []FileInfo) []string {
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	sort.Strings(paths) // Ensure consistent order for comparison
	return paths
}

func getSortedKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	if len(keys) > 0 {
		sort.Slice(keys, func(i, j int) bool {
			return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j])
		})
	}
	return keys
}

// --- Tests for generateConcatenatedCode (from walk.go) ---

func TestGenerateConcatenatedCode_BasicScan(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"file1.txt":       "Content of file 1.",
		"script.py":       "print('hello')",
		"config.json":     `{"key": "value"}`,
		"other.log":       "some logs",
		"subdir/":         "",
		"subdir/file2.py": "print('world')",
		"subdir/data.txt": "Subdir data.",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt", "json"}) // In helpers.go
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false // Test without gitignore
	header := "Test Header:"
	marker := "---"

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker,
	)

	assertions.NoError(err)
	assertions.Contains(output, header+"\n\n")
	assertions.Contains(output, marker+" file1.txt\nContent of file 1.\n"+marker)
	assertions.Contains(output, marker+" config.json\n{\"key\": \"value\"}\n"+marker)
	assertions.Contains(output, marker+" script.py\nprint('hello')\n"+marker)
	assertions.Contains(output, marker+" subdir/data.txt\nSubdir data.\n"+marker)
	assertions.Contains(output, marker+" subdir/file2.py\nprint('world')\n"+marker)
	assertions.NotContains(output, "other.log")
	assertions.NotContains(output, "some logs")
	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Greater(totalSize, int64(0))
	expectedPaths := []string{"config.json", "file1.txt", "script.py", "subdir/data.txt", "subdir/file2.py"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)

	logOutput := logBuf.String()
	t.Logf("Log output: %q", logOutput)
	// Check basic logs, not implementation-specific walk logs
	assertions.Contains(logOutput, "Starting file scan.")
	assertions.Contains(logOutput, "File scan completed.")
}

func TestGenerateConcatenatedCode_WithManualFiles(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"file1.txt":      "Content file 1.",
		"subdir/":        "",
		"subdir/data.py": "print(123)",
	}
	tempDir := setupTestDir(t, structure)
	manualDir := t.TempDir()
	manualLogPath := filepath.Join(manualDir, "manual.log")
	errWrite := os.WriteFile(manualLogPath, []byte("Manual log content."), 0644)
	require.NoError(t, errWrite)
	manualIgnoredExtPath := filepath.Join(tempDir, "manual_ignored.dat")
	errWrite = os.WriteFile(manualIgnoredExtPath, []byte("Manual data content."), 0644)
	require.NoError(t, errWrite)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt"})
	manualFiles := []string{manualLogPath, manualIgnoredExtPath}
	excludePatterns := []string{}
	useGitignore := false
	header := "Manual Test:"
	marker := "%%%"

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker,
	)

	assertions.NoError(err)
	assertions.Contains(output, header+"\n\n")
	assertions.Contains(output, marker+" file1.txt\nContent file 1.\n"+marker)
	assertions.Contains(output, marker+" subdir/data.py\nprint(123)\n"+marker)
	absManualLogPath, _ := filepath.Abs(manualLogPath)
	expectedManualLogDisplayPath := filepath.ToSlash(absManualLogPath)
	assertions.Contains(output, marker+" "+expectedManualLogDisplayPath+"\nManual log content.\n"+marker)
	expectedManualIgnoredDisplayPath := "manual_ignored.dat"
	assertions.Contains(output, marker+" "+expectedManualIgnoredDisplayPath+"\nManual data content.\n"+marker)
	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Greater(totalSize, int64(0))
	expectedPaths := []string{"file1.txt", expectedManualIgnoredDisplayPath, expectedManualLogDisplayPath, "subdir/data.py"}
	sort.Strings(expectedPaths) // Sort expected list for stable comparison
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths) // Compare sorted lists

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "Processing manually specified files", "count=2")
	absManualIgnoredPath, _ := filepath.Abs(manualIgnoredExtPath)
	assertions.Contains(logOutput, "Attempting to process manual file.", "path="+absManualLogPath)
	assertions.Contains(logOutput, "Attempting to process manual file.", "path="+absManualIgnoredPath)
}

func TestGenerateConcatenatedCode_WithExcludes(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"include.txt":       "Include me.",
		"exclude_me.txt":    "Exclude this content.",
		"subdir/":           "",
		"subdir/data.py":    "Include python.",
		"subdir/temp.log":   "Exclude this log.",
		"otherdir/":         "",
		"otherdir/foo.txt":  "Exclude this dir.",
		"otherdir/bar.py":   "Exclude this dir.",
		"exclude_dir/":      "",
		"exclude_dir/a.txt": "Exclude whole dir by name",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt", "log"})
	manualFiles := []string{}
	excludePatterns := []string{ // These should be passed to gocodewalker's LocationExcludePattern
		"*.log",
		"exclude_me.txt",
		"otherdir/*",  // Test if walker handles this
		"exclude_dir", // Test if walker handles this
		"*_dir/a.txt",
	}
	useGitignore := false // Test excludes without gitignore interference
	header := "Exclude Test:"
	marker := "!!!"

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker,
	)

	assertions.NoError(err)
	assertions.Contains(output, header+"\n\n")
	assertions.Contains(output, marker+" include.txt\nInclude me.\n"+marker)
	assertions.Contains(output, marker+" subdir/data.py\nInclude python.\n"+marker)
	assertions.NotContains(output, "exclude_me.txt")
	assertions.NotContains(output, "subdir/temp.log")
	assertions.NotContains(output, "otherdir/foo.txt")
	assertions.NotContains(output, "otherdir/bar.py")
	assertions.NotContains(output, "exclude_dir/a.txt")
	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Greater(totalSize, int64(0))
	expectedPaths := []string{"include.txt", "subdir/data.py"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths, "Mismatch in included files after exclude patterns")

	logOutput := logBuf.String()
	t.Logf("Log output: %q", logOutput)
	// Cannot reliably assert specific "skipping" logs from walker internals
}

// TestGenerateConcatenatedCode_WithGitignore (Updated assertions)
func TestGenerateConcatenatedCode_WithGitignore(t *testing.T) {
	assertions := assert.New(t)
	gitignoreContent := `
*.log
ignored_dir/
/root_ignored.txt
!good_dir/include_me.txt
`
	structure := map[string]string{
		".gitignore":              gitignoreContent,
		"include.py":              "print('include')",
		"ignored.log":             "This log is ignored.",
		"ignored_dir/":            "",
		"ignored_dir/file.txt":    "This whole dir is ignored.",
		"root_ignored.txt":        "Ignored only at the root.",
		"subdir/":                 "",
		"subdir/root_ignored.txt": "Not ignored here.", // Should be included
		"subdir/another.log":      "Ignored in subdir too.",
		"good_dir/":               "",
		"good_dir/ignored.log":    "Log ignored even in good dir",
		"good_dir/include_me.txt": "Should be included due to negation",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt", "log"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := true // Enable gitignore processing (recursive via gocodewalker)
	header := "Gitignore Test:"
	marker := "---"

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker,
	)

	assertions.NoError(err)
	assertions.Contains(output, header+"\n\n")
	assertions.Contains(output, marker+" include.py\nprint('include')\n"+marker)
	assertions.Contains(output, marker+" subdir/root_ignored.txt\nNot ignored here.\n"+marker) // Verify included
	assertions.Contains(output, marker+" good_dir/include_me.txt\nShould be included due to negation\n"+marker)
	assertions.NotContains(output, "ignored.log")
	assertions.NotContains(output, "ignored_dir/file.txt")
	assertions.NotContains(output, marker+" root_ignored.txt\nIgnored only at the root.\n"+marker) // Verify root one ignored
	assertions.NotContains(output, "good_dir/ignored.log")
	assertions.NotContains(output, "subdir/another.log")
	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Greater(totalSize, int64(0))

	expectedPaths := []string{"good_dir/include_me.txt", "include.py", "subdir/root_ignored.txt"} // Updated expected list
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)

	logOutput := logBuf.String()
	t.Logf("Log output: %q", logOutput)
	// Cannot reliably check for specific internal skip logs
}

func TestGenerateConcatenatedCode_EmptyFiles(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"file1.txt":         "Some content.",
		"empty1.txt":        "",
		"empty2.py":         "",
		"subdir/":           "",
		"subdir/empty3.txt": "",
		"non_empty.py":      "pass",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "Empty File Test:"
	marker := "---"

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker,
	)

	assertions.NoError(err)
	assertions.Contains(output, header+"\n\n")
	assertions.Contains(output, marker+" file1.txt\nSome content.\n"+marker)
	assertions.Contains(output, marker+" non_empty.py\npass\n"+marker)
	assertions.NotContains(output, marker+" empty1.txt")
	assertions.NotContains(output, marker+" empty2.py")
	assertions.NotContains(output, marker+" subdir/empty3.txt")
	expectedEmptyPaths := []string{"empty1.txt", "empty2.py", "subdir/empty3.txt"}
	sort.Strings(emptyFiles)
	assertions.Equal(expectedEmptyPaths, emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Greater(totalSize, int64(0))
	expectedPaths := []string{"file1.txt", "non_empty.py"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)
	logOutput := logBuf.String()
	t.Logf("Log output: %q", logOutput)
	assertions.Contains(logOutput, "Found empty file during scan.", "path=empty1.txt")
	assertions.Contains(logOutput, "Found empty file during scan.", "path=empty2.py")
	assertions.Contains(logOutput, "Found empty file during scan.", "path=subdir/empty3.txt")
}

func TestGenerateConcatenatedCode_ReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission-based read error test on Windows")
	}
	assertions := assert.New(t)
	structure := map[string]string{
		"readable.txt":   "Can read this.",
		"unreadable.txt": "Cannot read this.",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)
	unreadablePath := filepath.Join(tempDir, "unreadable.txt")
	errChmod := os.Chmod(unreadablePath, 0000)
	require.NoError(t, errChmod)
	t.Cleanup(func() { _ = os.Chmod(unreadablePath, 0644) })

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "Read Error Test:"
	marker := "---"

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker,
	)

	assertions.NoError(err)
	assertions.Contains(output, header+"\n\n")
	assertions.Contains(output, marker+" readable.txt\nCan read this.\n"+marker)
	assertions.NotContains(output, "unreadable.txt")
	assertions.Len(errorFiles, 1)
	unreadableRelPath := "unreadable.txt"
	errRead, exists := errorFiles[unreadableRelPath]
	assertions.True(exists, "Error for unreadable.txt should be in errorFiles map")
	if exists {
		assertions.Error(errRead)
		assertions.True(errors.Is(errRead, fs.ErrPermission) || strings.Contains(errRead.Error(), "permission denied"), "Expected permission error")
	}
	assertions.Empty(emptyFiles)
	assertions.Greater(totalSize, int64(0))
	expectedPaths := []string{"readable.txt"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)
	logOutput := logBuf.String()
	t.Logf("Log output: %q", logOutput)
	assertions.Contains(logOutput, "Error reading file content.", "path=unreadable.txt")
}

// TestGenerateConcatenatedCode_NonExistentDir (Should Pass)
func TestGenerateConcatenatedCode_NonExistentDir(t *testing.T) {
	assertions := assert.New(t)
	tempDir := t.TempDir()
	nonExistentDir := filepath.Join(tempDir, "nosuchdir")
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "No Dir Test:"
	marker := "---"

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		nonExistentDir, exts, manualFiles, excludePatterns, useGitignore, header, marker,
	)

	assertions.Error(err)
	assertions.True(errors.Is(err, fs.ErrNotExist), "Expected fs.ErrNotExist, got: %v", err)
	assertions.Equal("", output)
	assertions.Empty(includedFiles)
	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Equal(int64(0), totalSize)
	logOutput := logBuf.String()
	t.Logf("Log output: %q", logOutput)
	assertions.Contains(logOutput, "Target directory does not exist.")
}

// TestGenerateConcatenatedCode_NonExistentDir_WithManualFile (Still expected to FAIL output assertion until main() is fixed)
func TestGenerateConcatenatedCode_NonExistentDir_WithManualFile(t *testing.T) {
	assertions := assert.New(t)
	baseDir := t.TempDir()
	nonExistentDir := filepath.Join(baseDir, "nosuchdir")
	manualFilePath := filepath.Join(baseDir, "manual.txt")
	errWrite := os.WriteFile(manualFilePath, []byte("Manual content"), 0644)
	require.NoError(t, errWrite)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{manualFilePath}
	excludePatterns := []string{}
	useGitignore := false
	header := "No Dir But Manual File Test:"
	marker := "---"

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		nonExistentDir, exts, manualFiles, excludePatterns, useGitignore, header, marker,
	)

	assertions.Error(err)
	assertions.True(errors.Is(err, fs.ErrNotExist), "Expected fs.ErrNotExist for target dir, got: %v", err)

	// Mark this part as potentially failing until main() is updated
	t.Log("NOTE: Output assertions might fail here until main() logic is updated for --no-scan")
	assertions.Contains(output, header+"\n\n", "Test might fail here")
	absManualPath, _ := filepath.Abs(manualFilePath)
	expectedManualDisplayPath := filepath.ToSlash(absManualPath)
	assertions.Contains(output, marker+" "+expectedManualDisplayPath+"\nManual content\n"+marker, "Test might fail here")

	assertions.Len(includedFiles, 1)
	if len(includedFiles) == 1 {
		assertions.Equal(expectedManualDisplayPath, includedFiles[0].Path)
		assertions.True(includedFiles[0].IsManual)
	}
	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Greater(totalSize, int64(0))
	logOutput := logBuf.String()
	t.Logf("Log output: %q", logOutput)
	assertions.Contains(logOutput, "Processing manually specified files.")
	assertions.Contains(logOutput, "Target directory does not exist.")
}

func TestGenerateConcatenatedCode_NoFilesFound(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"other.log": "log data",
		"script.sh": "echo hello",
		"emptydir/": "",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"txt", "py"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "No Files Found Test:"
	marker := "---"

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker,
	)

	assertions.NoError(err)
	expectedOutput := ""
	if header != "" {
		expectedOutput = header + "\n"
	}
	// Trim output for comparison as trailing newlines might differ slightly
	assertions.Equal(expectedOutput, strings.TrimSpace(output)+"\n", "Output should be empty or just header")
	assertions.Empty(includedFiles)
	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Equal(int64(0), totalSize)
	logOutput := logBuf.String()
	t.Logf("Log output: %q", logOutput)
	assertions.Contains(logOutput, "Starting file scan.")
	assertions.Contains(logOutput, "File scan completed.")
	// Cannot reliably assert skipping logs from walker internals
}

// TestGenerateConcatenatedCode_NonExistentManualFile (with fixed log assertion)
func TestGenerateConcatenatedCode_NonExistentManualFile(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"file1.txt": "Content",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)
	nonExistentManualPath := filepath.Join(tempDir, "nosuchfile.txt")
	absNonExistentPath, _ := filepath.Abs(nonExistentManualPath)
	existingManualPath := filepath.Join(tempDir, "file1.txt")

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{existingManualPath, nonExistentManualPath}
	excludePatterns := []string{}
	useGitignore := false
	header := "Non-Existent Manual File Test:"
	marker := "---"

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker,
	)

	assertions.NoError(err)
	assertions.Contains(output, header+"\n\n")
	assertions.Contains(output, marker+" file1.txt\nContent\n"+marker)
	assertions.NotContains(output, "nosuchfile.txt")
	assertions.Len(errorFiles, 1)
	errManual, exists := errorFiles[absNonExistentPath]
	assertions.True(exists, "Error for non-existent manual file missing")
	if exists {
		assertions.Error(errManual)
		assertions.True(errors.Is(errManual, fs.ErrNotExist))
	}
	assertions.Empty(emptyFiles)
	assertions.Greater(totalSize, int64(0))
	expectedPaths := []string{"file1.txt"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)
	if len(includedFiles) > 0 {
		assertions.True(includedFiles[0].IsManual)
	}
	logOutput := logBuf.String()
	t.Logf("Log output: %q", logOutput)
	assertions.Contains(logOutput, "Processing manually specified files.")
	assertions.Contains(logOutput, "Manual file not found.", "Log should indicate missing manual file") // Fixed assertion text
	assertions.Contains(logOutput, "path="+absNonExistentPath)                                          // Check path is logged
	assertions.Contains(logOutput, "Walk: Skipping file already processed manually.", "path=file1.txt") // Verify skip log
}

func TestGenerateConcatenatedCode_InvalidExcludePattern(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"file1.txt": "Content",
		"[a-z.txt":  "Should be included if pattern invalid",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{}
	invalidPattern := "[a-z"
	excludePatterns := []string{invalidPattern, "*.log"}
	useGitignore := false
	header := "Invalid Exclude Pattern Test:"
	marker := "---"

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker,
	)

	assertions.NoError(err)
	assertions.Contains(output, header+"\n\n")
	assertions.Contains(output, marker+" file1.txt\nContent\n"+marker)
	assertions.Contains(output, marker+" [a-z.txt\nShould be included if pattern invalid\n"+marker)
	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Greater(totalSize, int64(0))
	expectedPaths := []string{"[a-z.txt", "file1.txt"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)
	logOutput := logBuf.String()
	t.Logf("Log output: %q", logOutput)
	assertions.Contains(logOutput, "Invalid exclude pattern, ignoring.", "Log should indicate invalid pattern was ignored") // Fixed assertion text
	assertions.Contains(logOutput, "pattern="+invalidPattern, "Log should show the invalid pattern")
}
