// cmd/codecat/walk_test.go
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	// No sync import needed here anymore

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Helper Functions ---
// (No changes needed in setupTestDir, setupTestLogger, getPathsFromIncludedFiles, getSortedKeys)
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
		_ = os.MkdirAll(parentDir, 0755)

		if strings.HasSuffix(relPath, string(filepath.Separator)) ||
			(content == "" && !strings.Contains(filepath.Base(relPath), ".")) {
			_ = os.MkdirAll(absPath, 0755)
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
	sort.Strings(paths)
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

// --- Tests for generateConcatenatedCode ---

// Basic scan, relies mostly on default basename excludes
func TestGenerateConcatenatedCode_BasicScan(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"file1.txt":       "Content of file 1.",
		"script.py":       "print('hello')",
		"config.json":     `{"key": "value"}`,
		"other.log":       "some logs",
		"subdir/":         "",
		"subdir/file2.py": "print('world')",
		"build/":          "",
		"build/output":    "build stuff",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt", "json"}) // No "" needed
	manualFiles := []string{}
	excludeBasenames := defaultConfig.ExcludeBasenames // Use defaults
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "Test Header:"
	marker := "---"
	scanDirs := []string{tempDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	assertions.NoError(err)
	assertions.Contains(output, header+"\n\n")
	assertions.Contains(output, marker+" file1.txt\nContent of file 1.\n"+marker)
	assertions.Contains(output, marker+" config.json\n{\"key\": \"value\"}\n"+marker)
	assertions.Contains(output, marker+" script.py\nprint('hello')\n"+marker)
	assertions.Contains(output, marker+" subdir/data.txt\nSubdir data.\n"+marker) // <-- Added data.txt back if needed
	assertions.Contains(output, marker+" subdir/file2.py\nprint('world')\n"+marker)
	assertions.NotContains(output, "other.log")    // Excluded by basename rule
	assertions.NotContains(output, "build/output") // Excluded by basename rule on parent dir

	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Greater(totalSize, int64(0))
	expectedPaths := []string{"config.json", "file1.txt", "script.py", "subdir/data.txt", "subdir/file2.py"} // <-- Adjusted expected
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)

	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Excluding file.", "path=other.log", "reason=basename match", "pattern=*.log")
	assertions.Contains(logOutput, "Excluding directory and its contents.", "path=build", "reason=basename match", "pattern=build")
	// Check parent exclusion log
	assertions.Contains(logOutput, "Excluding file.", "path=build/output", "reason=parent build excluded", "pattern=build")
}

// Test manual files (-f) with CWD-relative and absolute paths
func TestGenerateConcatenatedCode_WithManualFiles(t *testing.T) {
	assertions := assert.New(t)
	cwdDir := t.TempDir()
	// Use setupTestDir for consistency, it creates relative to the provided tempDir (cwdDir)
	structure := map[string]string{
		"local_file.txt": "Local content.",
		"subdir/data.py": "print(123)",
	}
	setupTestDir(t, structure)

	manualExternalDir := t.TempDir()
	externalFilePath := filepath.Join(manualExternalDir, "external.log")
	errWrite := os.WriteFile(externalFilePath, []byte("External log content."), 0644)
	require.NoError(t, errWrite)

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt"})
	manualFiles := []string{"local_file.txt", externalFilePath}
	excludeBasenames := []string{}
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "Manual Test:"
	marker := "%%%"
	scanDirs := []string{cwdDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		cwdDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	assertions.NoError(err)
	assertions.Contains(output, header+"\n\n")
	assertions.Contains(output, marker+" local_file.txt\nLocal content.\n"+marker)
	assertions.Contains(output, marker+" subdir/data.py\nprint(123)\n"+marker)

	externalRelPath, relErr := filepath.Rel(cwdDir, externalFilePath)
	if relErr != nil {
		externalRelPath = externalFilePath
	}
	externalDisplayPath := filepath.ToSlash(externalRelPath)
	assertions.Contains(output, marker+" "+externalDisplayPath+"\nExternal log content.\n"+marker)

	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Greater(totalSize, int64(0))
	expectedPaths := []string{"local_file.txt", externalDisplayPath, "subdir/data.py"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)

	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Processing manually specified files", "count=2")
	assertions.Contains(logOutput, "relativeToCwd=local_file.txt")
	assertions.Contains(logOutput, "relativeToCwd="+externalDisplayPath)
	assertions.Contains(logOutput, "Walk: Skipping item already processed manually", "path=local_file.txt")
}

// Test various exclude patterns (-x and basename) and unified directory logic
func TestGenerateConcatenatedCode_WithExcludesUnified(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"include.txt":         "Include me.",
		"exclude_me.txt":      "Exclude this specific file.",
		"data":                "Exclude this file named data",
		"data/":               "",
		"data/nested.txt":     "Exclude via parent dir",
		"other_dir/":          "",
		"other_dir/foo.txt":   "Include this",
		"other_dir/bar.log":   "Exclude via basename *.log",
		"docs/":               "",
		"docs/README.md":      "Exclude via parent dir docs",
		"build/":              "",
		"build/output.o":      "Exclude via parent basename",
		"archive.zip":         "Exclude via -x *.zip",
		"final/report.txt":    "Include this",
		"final/temp_report":   "Exclude via basename temp_*",
		"another_file.config": "Include",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"txt", "md", "o", "config"}) // No "" needed unless extensionless files are expected
	manualFiles := []string{}
	excludeBasenames := []string{"*.log", "build", "temp_*"}
	projectExcludes := []string{}
	flagExcludes := []string{
		"exclude_me.txt",
		"data", // Should exclude file and dir contents
		"docs", // Should exclude dir contents
		"*.zip",
	}
	useGitignore := false
	header := "Exclude Unified Test:"
	marker := "!!!"
	scanDirs := []string{tempDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	assertions.NoError(err)
	assertions.Contains(output, header+"\n\n")
	assertions.Contains(output, marker+" include.txt\nInclude me.\n"+marker)
	assertions.Contains(output, marker+" other_dir/foo.txt\nInclude this\n"+marker)
	assertions.Contains(output, marker+" final/report.txt\nInclude this\n"+marker)
	assertions.Contains(output, marker+" another_file.config\nInclude\n"+marker)

	assertions.NotContains(output, "exclude_me.txt")
	assertions.NotContains(output, "Exclude this file named data")
	assertions.NotContains(output, "data/nested.txt")
	assertions.NotContains(output, "other_dir/bar.log")
	assertions.NotContains(output, "docs/README.md")
	assertions.NotContains(output, "build/output.o")
	assertions.NotContains(output, "archive.zip")
	assertions.NotContains(output, "final/temp_report")

	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Greater(totalSize, int64(0))
	expectedPaths := []string{"another_file.config", "final/report.txt", "include.txt", "other_dir/foo.txt"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths, "Mismatch in included files after unified excludes")

	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Excluding file.", "path=exclude_me.txt", "reason=CWD-relative match", "pattern=exclude_me.txt")
	assertions.Contains(logOutput, "Excluding file.", "path=data", "reason=CWD-relative match", "pattern=data")
	assertions.Contains(logOutput, "Excluding file.", "path=data/nested.txt", "reason=CWD-relative prefix match", "pattern=data")
	assertions.Contains(logOutput, "Excluding directory and its contents.", "path=docs", "reason=CWD-relative match", "pattern=docs")
	assertions.Contains(logOutput, "Excluding file.", "path=docs/README.md", "reason=parent docs excluded", "pattern=docs")
	assertions.Contains(logOutput, "Excluding file.", "path=other_dir/bar.log", "reason=basename match", "pattern=*.log")
	assertions.Contains(logOutput, "Excluding directory and its contents.", "path=build", "reason=basename match", "pattern=build")
	assertions.Contains(logOutput, "Excluding file.", "path=build/output.o", "reason=parent build excluded", "pattern=build")
	assertions.Contains(logOutput, "Excluding file.", "path=archive.zip", "reason=CWD-relative match", "pattern=*.zip")
	assertions.Contains(logOutput, "Excluding file.", "path=final/temp_report", "reason=basename match", "pattern=temp_*")
}

// Other tests (Gitignore, EmptyFiles, ReadError, etc.) need minimal changes
// Just ensure the function signature is correct when calling generateConcatenatedCode

// Test .gitignore integration
func TestGenerateConcatenatedCode_WithGitignore(t *testing.T) {
	// ... (setup as before) ...
	structure := map[string]string{
		".gitignore":              "*.log\nignored_dir/\n/root_ignored.txt\n!good_dir/include_me.txt",
		"include.py":              "print('include')",
		"ignored.log":             "Ignored by gitignore",
		"ignored_dir/file.txt":    "Ignored by gitignore",
		"root_ignored.txt":        "Ignored by gitignore",
		"subdir/root_ignored.txt": "Not ignored here.",
		"good_dir/include_me.txt": "Should be included",
		"temp/ignored.log":        "Ignored by gitignore", // Also matches basename *.log
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt", "log"})
	manualFiles := []string{}
	excludeBasenames := defaultConfig.ExcludeBasenames
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := true
	header := "Gitignore Test:"
	marker := "---"
	scanDirs := []string{tempDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	// ... (assertions as before, they should still hold) ...
	assertions.NoError(err)
	assertions.Contains(output, marker+" include.py")
	assertions.Contains(output, marker+" subdir/root_ignored.txt")
	assertions.Contains(output, marker+" good_dir/include_me.txt")
	assertions.NotContains(output, "ignored.log")
	assertions.NotContains(output, "ignored_dir/file.txt")
	assertions.NotContains(output, "root_ignored.txt")
	assertions.NotContains(output, "temp/ignored.log")
	expectedPaths := []string{"good_dir/include_me.txt", "include.py", "subdir/root_ignored.txt"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput) // Check logs manually if needed
}

// Test empty file handling
func TestGenerateConcatenatedCode_EmptyFiles(t *testing.T) {
	// ... (setup as before) ...
	structure := map[string]string{
		"file1.txt":         "Some content.",
		"empty1.txt":        "",
		"empty2.py":         "",
		"subdir/empty3.txt": "",
		"non_empty.py":      "pass",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)
	exts := processExtensions([]string{"py", "txt"})
	manualFiles := []string{}
	excludeBasenames := []string{}
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "Empty File Test:"
	marker := "---"
	scanDirs := []string{tempDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	// ... (assertions as before) ...
	assertions.NoError(err)
	assertions.Contains(output, marker+" file1.txt")
	assertions.Contains(output, marker+" non_empty.py")
	assertions.NotContains(output, marker+" empty1.txt")
	expectedEmptyPaths := []string{"empty1.txt", "empty2.py", "subdir/empty3.txt"}
	actualEmpty := emptyFiles
	sort.Strings(actualEmpty)
	assertions.Equal(expectedEmptyPaths, actualEmpty)
	expectedPaths := []string{"file1.txt", "non_empty.py"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Found empty file during scan.", "path=empty1.txt")
	assertions.Contains(logOutput, "Found empty file during scan.", "path=empty2.py")
	assertions.Contains(logOutput, "Found empty file during scan.", "path=subdir/empty3.txt")

}

// Test read error handling
func TestGenerateConcatenatedCode_ReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission-based read error test on Windows")
	}
	// ... (setup as before) ...
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
	excludeBasenames := []string{}
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "Read Error Test:"
	marker := "---"
	scanDirs := []string{tempDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	// ... (assertions as before) ...
	assertions.NoError(err, "generateConcatenatedCode itself should succeed")
	assertions.Contains(output, marker+" readable.txt")
	assertions.NotContains(output, "unreadable.txt")
	assertions.Len(errorFiles, 1)
	unreadableRelPath := "unreadable.txt"
	errRead, exists := errorFiles[unreadableRelPath]
	assertions.True(exists)
	if exists {
		assertions.Error(errRead)
		assertions.True(errors.Is(errRead, fs.ErrPermission) || strings.Contains(errRead.Error(), "permission denied"))
	}
	expectedPaths := []string{"readable.txt"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Error reading file content.", "path=unreadable.txt")
}

// Test scanning non-existent dir
func TestGenerateConcatenatedCode_NonExistentScanDir(t *testing.T) {
	// ... (setup as before) ...
	cwdDir := t.TempDir()
	nonExistentDir := filepath.Join(cwdDir, "nosuchdir")
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)
	exts := processExtensions([]string{"txt"})
	manualFiles := []string{}
	excludeBasenames := []string{}
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "No Dir Test:"
	marker := "---"
	scanDirs := []string{nonExistentDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		cwdDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	// ... (assertions as before) ...
	assertions.Error(err)
	assertions.True(errors.Is(err, fs.ErrNotExist))
	assertions.Equal(header+"\n\n", output)
	assertions.Empty(includedFiles)
	assertions.Empty(emptyFiles)
	relNonExistent, _ := filepath.Rel(cwdDir, nonExistentDir)
	relNonExistent = filepath.ToSlash(relNonExistent) + "/"
	_, exists := errorFiles[relNonExistent]
	assertions.True(exists)
	assertions.Equal(int64(0), totalSize)
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Target scan directory does not exist.")
}

// Test non-existent scan dir with manual files
func TestGenerateConcatenatedCode_NonExistentScanDir_WithManualFile(t *testing.T) {
	// ... (setup as before) ...
	cwdDir := t.TempDir()
	nonExistentDir := filepath.Join(cwdDir, "nosuchdir")
	manualFilePath := filepath.Join(cwdDir, "manual.txt")
	errWrite := os.WriteFile(manualFilePath, []byte("Manual content"), 0644)
	require.NoError(t, errWrite)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)
	exts := processExtensions([]string{"txt"})
	manualFiles := []string{"manual.txt"}
	excludeBasenames := []string{}
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "No Scan Dir But Manual File Test:"
	marker := "---"
	scanDirs := []string{nonExistentDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		cwdDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	// ... (assertions as before) ...
	assertions.Error(err)
	assertions.True(errors.Is(err, fs.ErrNotExist))
	assertions.Contains(output, marker+" manual.txt")
	assertions.Len(includedFiles, 1)
	if len(includedFiles) == 1 {
		assertions.Equal("manual.txt", includedFiles[0].Path)
		assertions.True(includedFiles[0].IsManual)
	}
	relNonExistent, _ := filepath.Rel(cwdDir, nonExistentDir)
	relNonExistent = filepath.ToSlash(relNonExistent) + "/"
	_, exists := errorFiles[relNonExistent]
	assertions.True(exists)
	assertions.Greater(totalSize, int64(0))
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Target scan directory does not exist.")
	assertions.Contains(logOutput, "File scan finished with errors.")
}

// Test no files found case
func TestGenerateConcatenatedCode_NoFilesFound(t *testing.T) {
	// ... (setup as before) ...
	structure := map[string]string{"other.log": "log data", "script.sh": "echo hello"}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)
	exts := processExtensions([]string{"txt", "py"})
	manualFiles := []string{}
	excludeBasenames := defaultConfig.ExcludeBasenames
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "No Files Found Test:"
	marker := "---"
	scanDirs := []string{tempDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	// ... (assertions as before) ...
	assertions.NoError(err)
	expectedOutput := header + "\n"
	assertions.Equal(expectedOutput, output)
	assertions.Empty(includedFiles)
	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	assertions.Equal(int64(0), totalSize)
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Excluding file.", "path=other.log", "reason=basename match")
	assertions.Contains(logOutput, "Skipping file with non-matching extension", "path=script.sh")
}

// Test non-existent manual file
func TestGenerateConcatenatedCode_NonExistentManualFile(t *testing.T) {
	// ... (setup as before) ...
	cwdDir := t.TempDir()
	errWrite := os.WriteFile(filepath.Join(cwdDir, "file1.txt"), []byte("Content"), 0644)
	require.NoError(t, errWrite)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)
	nonExistentManualPath := "nosuchfile.txt"
	existingManualPath := "file1.txt"
	exts := processExtensions([]string{"txt"})
	manualFiles := []string{existingManualPath, nonExistentManualPath}
	excludeBasenames := []string{}
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "Non-Existent Manual File Test:"
	marker := "---"
	scanDirs := []string{cwdDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		cwdDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	// ... (assertions as before) ...
	assertions.NoError(err)
	assertions.Contains(output, marker+" file1.txt")
	assertions.NotContains(output, "nosuchfile.txt")
	assertions.Len(errorFiles, 1)
	errManual, exists := errorFiles[nonExistentManualPath]
	assertions.True(exists)
	if exists {
		assertions.ErrorIs(errManual, fs.ErrNotExist)
	}
	expectedPaths := []string{"file1.txt"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Manual file not found.", "path=nosuchfile.txt")
}

// Test invalid exclude pattern
func TestGenerateConcatenatedCode_InvalidExcludePattern(t *testing.T) {
	// ... (setup as before) ...
	structure := map[string]string{"file1.txt": "Content", "[a-z.txt": "Include"}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)
	exts := processExtensions([]string{"txt"})
	manualFiles := []string{}
	invalidPattern := "[a-z"
	excludeBasenames := []string{invalidPattern}
	projectExcludes := []string{}
	flagExcludes := []string{invalidPattern}
	useGitignore := false
	header := "Invalid Exclude Pattern Test:"
	marker := "---"
	scanDirs := []string{tempDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	// ... (assertions as before) ...
	assertions.NoError(err)
	assertions.Contains(output, marker+" file1.txt")
	assertions.Contains(output, marker+" [a-z.txt") // Should still be included
	expectedPaths := []string{"[a-z.txt", "file1.txt"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Invalid global exclude basename pattern syntax, ignoring.", "pattern="+invalidPattern)
	assertions.Contains(logOutput, "Invalid CWD-relative exclude pattern syntax, ignoring.", "pattern="+invalidPattern)
}

// Test --no-scan flag
func TestGenerateConcatenatedCode_NoScan(t *testing.T) {
	// ... (setup as before) ...
	cwdDir := t.TempDir()
	errWrite := os.WriteFile(filepath.Join(cwdDir, "scanned.txt"), []byte("SKIP"), 0644)
	require.NoError(t, errWrite)
	manualFilePath := filepath.Join(cwdDir, "manual.txt")
	errWrite = os.WriteFile(manualFilePath, []byte("Manual only"), 0644)
	require.NoError(t, errWrite)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)
	exts := processExtensions([]string{"txt"})
	manualFiles := []string{"manual.txt"}
	excludeBasenames := []string{}
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "No Scan Test:"
	marker := "==="
	scanDirs := []string{cwdDir}
	noScan := true

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		cwdDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	// ... (assertions as before) ...
	assertions.NoError(err)
	assertions.Contains(output, marker+" manual.txt")
	assertions.NotContains(output, "scanned.txt")
	assertions.Len(includedFiles, 1)
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Skipping directory scan due to --no-scan flag.")
}

// Test project excludes
func TestGenerateConcatenatedCode_ProjectExcludes(t *testing.T) {
	// ... (setup as before) ...
	cwdDir := t.TempDir()
	projectExcludeContent := "project_exclude.txt\ndata/sub/*\nexclude_dir_no_slash\n"
	structure := map[string]string{
		"include.py":              "print('yes')",
		"project_exclude.txt":     "exclude",
		"data/config.json":        "config",
		"data/sub/model.bin":      "exclude",
		".codecat_exclude":        projectExcludeContent,
		"other_project_file.yaml": "include",
		"exclude_dir_no_slash/a":  "exclude",
	}
	setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)
	projectExcludes := loadProjectExcludes(cwdDir)                          // Load excludes for passing
	exts := processExtensions([]string{"py", "txt", "json", "yaml", "bin"}) // Include .bin etc.
	manualFiles := []string{}
	excludeBasenames := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "Project Exclude Test:"
	marker := "###"
	scanDirs := []string{cwdDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, totalSize, err := generateConcatenatedCode(
		cwdDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	// ... (assertions as before) ...
	assertions.NoError(err)
	assertions.Contains(output, marker+" include.py")
	assertions.Contains(output, marker+" data/config.json")
	assertions.Contains(output, marker+" other_project_file.yaml")
	assertions.NotContains(output, "project_exclude.txt")
	assertions.NotContains(output, "model.bin")
	assertions.NotContains(output, "exclude_dir_no_slash/a")
	expectedPaths := []string{"data/config.json", "include.py", "other_project_file.yaml"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Excluding file.", "path=project_exclude.txt", "reason=CWD-relative match")
	assertions.Contains(logOutput, "Excluding file.", "path=data/sub/model.bin", "reason=CWD-relative match")
	assertions.Contains(logOutput, "Excluding file.", "path=exclude_dir_no_slash/a", "reason=CWD-relative prefix match")
}

// Helper function - must be defined in the test file if not exported
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
