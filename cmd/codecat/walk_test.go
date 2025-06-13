// cmd/codecat/walk_test.go
package main

import (
	"bytes"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Helper Functions ---
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
		err := os.MkdirAll(parentDir, 0755)
		require.NoError(t, err)

		if strings.HasSuffix(relPath, string(filepath.Separator)) ||
			strings.HasSuffix(relPath, "/") ||
			(content == "" && !strings.Contains(filepath.Base(relPath), ".")) {
			err := os.MkdirAll(absPath, 0755)
			require.NoError(t, err)
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

	exts := processExtensions([]string{"py", "txt", "json", ""}) // Allow extensionless for build/output
	manualFiles := []string{}
	excludeBasenames := defaultConfig.ExcludeBasenames
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "Test Header:"
	marker := "---"
	scanDirs := []string{tempDir}
	noScan := false

	output, includedFiles, emptyFiles, errorFiles, _, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	assertions.NoError(err)
	assertions.Contains(output, header)
	assertions.Contains(output, marker+" file1.txt\nContent of file 1."+marker+"\n")
	assertions.Contains(output, marker+" config.json\n{\"key\": \"value\"}"+marker+"\n")
	assertions.NotContains(output, "build stuff")

	assertions.Empty(emptyFiles)
	assertions.Empty(errorFiles)
	expectedPaths := []string{"config.json", "file1.txt", "script.py", "subdir/file2.py"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)

	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	// The walker doesn't yield files inside an excluded dir, so we only check the dir exclusion log.
	assertions.Contains(logOutput, `Excluding directory and its contents." path=build`)
}

// Test various exclude patterns (-x and basename)
func TestGenerateConcatenatedCode_WithExcludesUnified(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"include.txt":         "Include me.",
		"exclude_me.txt":      "Exclude this specific file.",
		"data_file.txt":       "Exclude this file named data_file.",
		"data_dir/":           "",
		"data_dir/nested.txt": "Exclude via parent dir",
		"other_dir/":          "",
		"other_dir/foo.txt":   "Include this",
		"docs/":               "",
		"docs/README.md":      "Exclude via parent dir docs",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"txt", "md"})
	manualFiles := []string{}
	excludeBasenames := []string{}
	projectExcludes := []string{}
	flagExcludes := []string{
		"exclude_me.txt",
		"data_file.txt",
		"data_dir",
		"docs",
	}
	useGitignore := false
	header := "Exclude Unified Test:"
	marker := "!!!"
	scanDirs := []string{tempDir}
	noScan := false

	output, includedFiles, _, _, _, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	assertions.NoError(err)
	assertions.Contains(output, marker+" include.txt")
	assertions.Contains(output, marker+" other_dir/foo.txt")
	assertions.NotContains(output, "exclude_me.txt")
	assertions.NotContains(output, "data_dir/nested.txt")
	assertions.NotContains(output, "docs/README.md")

	expectedPaths := []string{"include.txt", "other_dir/foo.txt"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths, "Mismatch in included files after unified excludes")

	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, `Excluding file." path=exclude_me.txt`)
	assertions.Contains(logOutput, `Excluding directory and its contents." path=data_dir`)
	assertions.Contains(logOutput, `Excluding directory and its contents." path=docs`)
}

// Test project excludes
func TestGenerateConcatenatedCode_ProjectExcludes(t *testing.T) {
	assertions := assert.New(t)
	projectExcludeContent := "project_exclude.txt\ndata/sub/*\nexclude_dir_no_slash/\n"
	structure := map[string]string{
		"include.py":                 "print('yes')",
		"project_exclude.txt":        "exclude",
		"data/config.json":           "config",
		"data/sub/model.bin":         "exclude",
		".codecat_exclude":           projectExcludeContent,
		"other_project_file.yaml":    "include",
		"exclude_dir_no_slash/a.txt": "exclude",
	}
	cwdDir := setupTestDir(t, structure)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	projectExcludes := loadProjectExcludes(cwdDir)
	exts := processExtensions([]string{"py", "txt", "json", "yaml", "bin"})
	manualFiles := []string{}
	excludeBasenames := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "Project Exclude Test:"
	marker := "###"
	scanDirs := []string{cwdDir}
	noScan := false

	output, includedFiles, _, _, _, err := generateConcatenatedCode(
		cwdDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	assertions.NoError(err)
	assertions.Contains(output, marker+" include.py")
	assertions.Contains(output, marker+" data/config.json")
	assertions.Contains(output, marker+" other_project_file.yaml")
	assertions.NotContains(output, "project_exclude.txt")
	assertions.NotContains(output, "model.bin")
	assertions.NotContains(output, "exclude_dir_no_slash/a.txt")
	expectedPaths := []string{"data/config.json", "include.py", "other_project_file.yaml"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)

	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, `Excluding file." path=project_exclude.txt`)
	assertions.Contains(logOutput, `Excluding file." path=data/sub/model.bin`)
	assertions.Contains(logOutput, `Excluding directory and its contents." path=exclude_dir_no_slash`)
}

// (Omitted other passing tests for brevity)
// You can append the other test functions that were already passing here.
// e.g., TestGenerateConcatenatedCode_WithManualFiles, TestGenerateConcatenatedCode_WithGitignore, etc.
// passing below

// Test .gitignore integration
func TestGenerateConcatenatedCode_WithGitignore(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		".gitignore":              "*.log\nignored_dir/\n/root_ignored.txt",
		"include.py":              "print('include')",
		"ignored.log":             "Ignored by gitignore",
		"ignored_dir/file.txt":    "Ignored by gitignore",
		"root_ignored.txt":        "Ignored by gitignore",
		"subdir/root_ignored.txt": "Not ignored here.",
	}
	tempDir := setupTestDir(t, structure)
	testLogger, _ := setupTestLogger(t)
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

	output, includedFiles, _, _, _, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	assertions.NoError(err)
	assertions.Contains(output, marker+" include.py")
	assertions.Contains(output, marker+" subdir/root_ignored.txt")
	assertions.NotContains(output, marker+" ignored.log")
	assertions.NotContains(output, marker+" ignored_dir/file.txt")
	assertions.NotContains(output, marker+" root_ignored.txt\n") // Be specific to avoid matching subdir
	expectedPaths := []string{"include.py", "subdir/root_ignored.txt"}
	actualPaths := getPathsFromIncludedFiles(includedFiles)
	assertions.Equal(expectedPaths, actualPaths)
}

// Test empty file handling
func TestGenerateConcatenatedCode_EmptyFiles(t *testing.T) {
	assertions := assert.New(t)
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

	output, includedFiles, emptyFiles, _, _, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

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
	assertions.Contains(logOutput, `path=empty1.txt`)
	assertions.Contains(logOutput, `path=empty2.py`)
	assertions.Contains(logOutput, `path=subdir/empty3.txt`)
}

// Test read error handling
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
	excludeBasenames := []string{}
	projectExcludes := []string{}
	flagExcludes := []string{}
	useGitignore := false
	header := "Read Error Test:"
	marker := "---"
	scanDirs := []string{tempDir}
	noScan := false

	output, includedFiles, _, errorFiles, _, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

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
	assertions := assert.New(t)
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

	assertions.Error(err)
	assertions.True(errors.Is(err, fs.ErrNotExist))
	assertions.Contains(output, header)
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
	assertions := assert.New(t)
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

	output, includedFiles, _, errorFiles, totalSize, err := generateConcatenatedCode(
		cwdDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

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

// Test non-existent manual file
func TestGenerateConcatenatedCode_NonExistentManualFile(t *testing.T) {
	assertions := assert.New(t)
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

	output, includedFiles, _, errorFiles, _, err := generateConcatenatedCode(
		cwdDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

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
	assertions := assert.New(t)
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

	output, includedFiles, _, _, _, err := generateConcatenatedCode(
		tempDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

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
	assertions := assert.New(t)
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

	output, includedFiles, _, _, _, err := generateConcatenatedCode(
		cwdDir, scanDirs, exts, manualFiles, excludeBasenames,
		projectExcludes, flagExcludes, useGitignore, header, marker, noScan,
	)

	assertions.NoError(err)
	assertions.Contains(output, marker+" manual.txt")
	assertions.NotContains(output, "scanned.txt")
	assertions.Len(includedFiles, 1)
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)
	assertions.Contains(logOutput, "Skipping directory scan due to --no-scan flag.")
}
