// main_test.go
package main_test

import (
	"bytes"
	"fmt"
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

		if content == "" {
			err := os.MkdirAll(absPath, 0755)
			require.NoError(t, err, "Failed to create directory: %s", absPath)
		} else {
			parentDir := filepath.Dir(absPath)
			if _, err := os.Stat(parentDir); os.IsNotExist(err) {
				errMkdir := os.MkdirAll(parentDir, 0755)
				require.NoError(t, errMkdir, "Failed to create parent directory: %s", parentDir)
			} else {
				require.NoError(t, err, "Failed to stat parent directory: %s", parentDir)
			}
			err := os.WriteFile(absPath, []byte(content), 0644)
			require.NoError(t, err, "Failed to write file: %s", absPath)
		}
	}
	return tempDir
}

func getTestCWD(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return cwd
}

func expectDisplayPath(t *testing.T, cwd, absPath string) string {
	t.Helper()
	relPath, err := filepath.Rel(cwd, absPath)
	if err != nil {
		absPath = filepath.Clean(absPath)
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(relPath)
}

func setupTestLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var logBuf bytes.Buffer
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	return logger, &logBuf
}

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

func TestMain(m *testing.M) {
	var logBuf bytes.Buffer
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))

	code := m.Run()

	if code != 0 {
		fmt.Fprintln(os.Stderr, "Test logs:")
		fmt.Fprintln(os.Stderr, logBuf.String())
	}

	os.Exit(code)
}

func TestGenerateConcatenatedCode_BasicScan(t *testing.T) {
	assertions := assert.New(t)
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

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt", "json"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "Test Header:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assertions.NoError(err) // Line ~161
	assertions.Contains(output, header+"\n")

	expectedPathFile1 := expectDisplayPath(t, cwd, filepath.Join(tempDir, "file1.txt"))
	assertions.Contains(output, marker+" "+expectedPathFile1+"\nContent of file 1.\n"+marker)
	expectedPathConfig := expectDisplayPath(t, cwd, filepath.Join(tempDir, "config.json"))
	assertions.Contains(output, marker+" "+expectedPathConfig+"\n{\"key\": \"value\"}\n"+marker)
	expectedPathScript := expectDisplayPath(t, cwd, filepath.Join(tempDir, "script.py"))
	assertions.Contains(output, marker+" "+expectedPathScript+"\nprint('hello')\n"+marker)
	expectedPathSubData := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/data.txt"))
	assertions.Contains(output, marker+" "+expectedPathSubData+"\nSubdir data.\n"+marker)
	expectedPathSubPy := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/file2.py"))
	assertions.Contains(output, marker+" "+expectedPathSubPy+"\nprint('world')\n"+marker)

	assertions.NotContains(output, "other.log")
	assertions.NotContains(output, "some logs")
	assertions.NotContains(output, "Empty files:")

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "Scanning directory")
	assertions.Contains(logOutput, "path="+tempDir)
}

func TestGenerateConcatenatedCode_WithManualFiles(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"file1.txt":      "Content file 1.",
		"manual.log":     "Manual log content.",
		"subdir/data.py": "print(123)",
	}
	tempDir := setupTestDir(t, structure)
	cwd := getTestCWD(t)

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py"})
	manualLogPath := filepath.Join(tempDir, "manual.log")
	file1Path := filepath.Join(tempDir, "file1.txt")
	manualFiles := []string{manualLogPath, file1Path}
	excludePatterns := []string{}
	useGitignore := false
	header := "Manual Test:"
	marker := "%%%"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assertions.NoError(err) // Line ~207
	assertions.Contains(output, header+"\n")

	expectedPathFile1 := expectDisplayPath(t, cwd, file1Path)
	expectedPathManualLog := expectDisplayPath(t, cwd, manualLogPath)
	expectedPathSubPy := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/data.py"))

	assertions.Contains(output, marker+" "+expectedPathFile1+"\nContent file 1.\n"+marker)
	assertions.Contains(output, marker+" "+expectedPathManualLog+"\nManual log content.\n"+marker)
	assertions.Contains(output, marker+" "+expectedPathSubPy+"\nprint(123)\n"+marker)

	absPaths := []string{
		filepath.Join(tempDir, "file1.txt"),
		filepath.Join(tempDir, "manual.log"),
		filepath.Join(tempDir, "subdir/data.py"),
	}
	absPathsSorted := make([]string, len(absPaths))
	copy(absPathsSorted, absPaths)
	sort.Strings(absPathsSorted)
	expectedPathsSorted := make([]string, len(absPathsSorted))
	for i, absPath := range absPathsSorted {
		expectedPathsSorted[i] = expectDisplayPath(t, cwd, absPath)
	}

	actualPaths := []string{}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, marker+" ") {
			path := strings.TrimPrefix(line, marker+" ")
			path = strings.TrimSpace(path)
			actualPaths = append(actualPaths, path)
		}
	}

	assertions.Equal(expectedPathsSorted, actualPaths, "File order mismatch")

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "Processing manually specified files")
}

func TestGenerateConcatenatedCode_WithExcludes(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"include.txt":      "Include me.",
		"exclude_me.txt":   "Exclude this content.",
		"subdir/data.py":   "Include python.",
		"subdir/temp.log":  "Exclude this log.",
		"otherdir/foo.txt": "Exclude this dir.",
	}
	tempDir := setupTestDir(t, structure)
	cwd := getTestCWD(t)

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt", "log"})
	manualFiles := []string{}
	excludePatterns := []string{
		"*.log",
		"exclude_me.txt",
		"otherdir/*",
	}
	useGitignore := false
	header := "Exclude Test:"
	marker := "!!!"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assertions.NoError(err) // Line ~274
	assertions.Contains(output, header+"\n")

	expectedPathInclude := expectDisplayPath(t, cwd, filepath.Join(tempDir, "include.txt"))
	assertions.Contains(output, marker+" "+expectedPathInclude+"\nInclude me.\n"+marker)
	expectedPathSubPy := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/data.py"))
	assertions.Contains(output, marker+" "+expectedPathSubPy+"\nInclude python.\n"+marker)

	expectedPathExcludeMe := expectDisplayPath(t, cwd, filepath.Join(tempDir, "exclude_me.txt"))
	assertions.NotContains(output, marker+" "+expectedPathExcludeMe)
	assertions.NotContains(output, "Exclude this content.")

	expectedPathTempLog := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/temp.log"))
	assertions.NotContains(output, marker+" "+expectedPathTempLog)
	assertions.NotContains(output, "Exclude this log.")

	expectedPathOtherFoo := expectDisplayPath(t, cwd, filepath.Join(tempDir, "otherdir/foo.txt"))
	assertions.NotContains(output, marker+" "+expectedPathOtherFoo)
	assertions.NotContains(output, "Exclude this dir.")

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "Skipping excluded file (pattern)")
}

func TestGenerateConcatenatedCode_WithGitignore(t *testing.T) {
	gitignoreContent := `
# Comments ignored
*.log
ignored_dir/
/root_ignored.txt
!good_dir/include_me.txt
`
	assertions := assert.New(t)
	structure := map[string]string{
		".gitignore":              gitignoreContent,
		"include.py":              "print('include')",
		"ignored.log":             "This log is ignored.",
		"ignored_dir/":            "",
		"ignored_dir/file.txt":    "This whole dir is ignored.",
		"root_ignored.txt":        "Ignored only at the root.",
		"subdir/":                 "",
		"subdir/root_ignored.txt": "Not ignored here.",
		"good_dir/":               "",
		"good_dir/ignored.log":    "Log ignored even in good dir",
		"good_dir/include_me.txt": "Should be included due to negation",
	}
	tempDir := setupTestDir(t, structure)
	cwd := getTestCWD(t)

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt", "log"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := true
	header := "Gitignore Test:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assertions.NoError(err) // Line ~334
	assertions.Contains(output, header+"\n")

	expectedPathIncludePy := expectDisplayPath(t, cwd, filepath.Join(tempDir, "include.py"))
	assertions.Contains(output, marker+" "+expectedPathIncludePy+"\nprint('include')\n"+marker)

	expectedPathSubTxt := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/root_ignored.txt"))
	assertions.Contains(output, marker+" "+expectedPathSubTxt+"\nNot ignored here.\n"+marker)

	expectedGoodTxtPath := expectDisplayPath(t, cwd, filepath.Join(tempDir, "good_dir/include_me.txt"))
	assertions.Contains(output, marker+" "+expectedGoodTxtPath+"\nShould be included due to negation\n"+marker)

	expectedPathIgnoredLog := expectDisplayPath(t, cwd, filepath.Join(tempDir, "ignored.log"))
	assertions.NotContains(output, marker+" "+expectedPathIgnoredLog)
	assertions.NotContains(output, "This log is ignored.")

	expectedPathIgnoredDirFile := expectDisplayPath(t, cwd, filepath.Join(tempDir, "ignored_dir/file.txt"))
	assertions.NotContains(output, marker+" "+expectedPathIgnoredDirFile)
	assertions.NotContains(output, "This whole dir is ignored.")

	expectedPathRootIgnored := expectDisplayPath(t, cwd, filepath.Join(tempDir, "root_ignored.txt"))
	assertions.NotContains(output, marker+" "+expectedPathRootIgnored)
	assertions.NotContains(output, "Ignored only at the root.")

	expectedPathGoodIgnoredLog := expectDisplayPath(t, cwd, filepath.Join(tempDir, "good_dir/ignored.log"))
	assertions.NotContains(output, marker+" "+expectedPathGoodIgnoredLog)

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "Skipping ignored file (gitignore)")
	assertions.Contains(logOutput, "path=ignored.log")
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
	cwd := getTestCWD(t)

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"py", "txt"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "Empty File Test:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assertions.NoError(err) // Line ~390
	assertions.Contains(output, header+"\n")

	expectedPathFile1 := expectDisplayPath(t, cwd, filepath.Join(tempDir, "file1.txt"))
	assertions.Contains(output, marker+" "+expectedPathFile1+"\nSome content.\n"+marker)
	expectedPathNonEmptyPy := expectDisplayPath(t, cwd, filepath.Join(tempDir, "non_empty.py"))
	assertions.Contains(output, marker+" "+expectedPathNonEmptyPy+"\npass\n"+marker)

	assertions.Contains(output, "\nEmpty files:\n")

	expectedEmpty1Path := expectDisplayPath(t, cwd, filepath.Join(tempDir, "empty1.txt"))
	expectedEmpty2Path := expectDisplayPath(t, cwd, filepath.Join(tempDir, "empty2.py"))
	expectedEmpty3Path := expectDisplayPath(t, cwd, filepath.Join(tempDir, "subdir/empty3.txt"))

	emptyPaths := []string{expectedEmpty1Path, expectedEmpty2Path, expectedEmpty3Path}
	sort.Strings(emptyPaths)
	expectedEmptyList := "\nEmpty files:\n"
	for _, p := range emptyPaths {
		expectedEmptyList += "\t" + p + "\n"
	}
	assertions.Contains(output, expectedEmptyList)

	assertions.NotContains(output, marker+" "+expectedEmpty1Path+"\n\n"+marker)
	assertions.NotContains(output, marker+" "+expectedEmpty2Path+"\n\n"+marker)
	assertions.NotContains(output, marker+" "+expectedEmpty3Path+"\n\n"+marker)

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "Found empty file")
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
	cwd := getTestCWD(t)

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	unreadablePath := filepath.Join(tempDir, "unreadable.txt")
	err := os.Chmod(unreadablePath, 0000)
	require.NoError(t, err, "Failed to chmod unreadable.txt")

	t.Cleanup(func() {
		errChmodBack := os.Chmod(unreadablePath, 0644)
		assertions.NoError(errChmodBack) // Line ~441
	})

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{}
	excludePatterns := []string{}
	useGitignore := false
	header := "Read Error Test:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assertions.NoError(err) // Line ~452
	assertions.Contains(output, header+"\n")

	expectedReadablePath := expectDisplayPath(t, cwd, filepath.Join(tempDir, "readable.txt"))
	assertions.Contains(output, marker+" "+expectedReadablePath+"\nCan read this.\n"+marker)

	expectedUnreadablePath := expectDisplayPath(t, cwd, unreadablePath)
	assertions.Contains(output, marker+" "+expectedUnreadablePath+"\n# Error reading file")
	assertions.Contains(output, "permission denied")
	assertions.Contains(output, "\n"+marker+"\n")

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "level=WARN")
	assertions.Contains(logOutput, "Error reading file content")
	assertions.Contains(logOutput, "file="+expectedUnreadablePath)
	assertions.Contains(logOutput, "permission denied")
}

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

	output, err := generateConcatenatedCode(nonExistentDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assertions.Error(err) // Line ~486
	assertions.Contains(err.Error(), "not found or is not accessible")
	assertions.Equal("", output)

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "level=ERROR")
	assertions.Contains(logOutput, "Target directory")
	assertions.Contains(logOutput, "not found or is not accessible")
}

func TestGenerateConcatenatedCode_NonExistentDir_WithManualFile(t *testing.T) {
	assertions := assert.New(t)
	tempDir := t.TempDir()
	nonExistentDir := filepath.Join(tempDir, "nosuchdir")
	manualFilePath := filepath.Join(tempDir, "manual.txt")
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

	output, err := generateConcatenatedCode(nonExistentDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assertions.NoError(err) // Line ~515
	assertions.Contains(output, header+"\n")

	cwd := getTestCWD(t)
	expectedManualPath := expectDisplayPath(t, cwd, manualFilePath)
	assertions.Contains(output, marker+" "+expectedManualPath+"\nManual content\n"+marker)

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "level=ERROR")
	assertions.Contains(logOutput, "Target directory")
	assertions.Contains(logOutput, "not found or is not accessible")
	assertions.Contains(logOutput, "level=WARN")
	assertions.Contains(logOutput, "Proceeding with only manually specified files")
}

func TestGenerateConcatenatedCode_NoFilesFound(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"other.log": "log data",
		"script.sh": "echo hello",
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

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assertions.NoError(err) // Line ~545
	assertions.Equal(header+"\n", output)

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "level=WARN")
	assertions.Contains(logOutput, "No files found to process")
}

func TestGenerateConcatenatedCode_NonExistentManualFile(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"file1.txt": "Content",
	}
	tempDir := setupTestDir(t, structure)
	cwd := getTestCWD(t)

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{
		filepath.Join(tempDir, "file1.txt"),
		filepath.Join(tempDir, "nosuchfile.txt"),
	}
	excludePatterns := []string{}
	useGitignore := false
	header := "Non-Existent Manual File Test:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assertions.NoError(err) // Line ~549
	assertions.Contains(output, header+"\n")

	expectedPath := expectDisplayPath(t, cwd, filepath.Join(tempDir, "file1.txt"))
	assertions.Contains(output, marker+" "+expectedPath+"\nContent\n"+marker)
	assertions.NotContains(output, "nosuchfile.txt")

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "level=WARN")
	assertions.Contains(logOutput, "Manual file not found, skipping.")
	assertions.Contains(logOutput, "file=nosuchfile.txt")
}

func TestGenerateConcatenatedCode_InvalidExcludePattern(t *testing.T) {
	assertions := assert.New(t)
	structure := map[string]string{
		"file1.txt": "Content",
	}
	tempDir := setupTestDir(t, structure)
	cwd := getTestCWD(t)

	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)

	exts := processExtensions([]string{"txt"})
	manualFiles := []string{}
	excludePatterns := []string{"[a-z"}
	useGitignore := false
	header := "Invalid Exclude Pattern Test:"
	marker := "---"

	output, err := generateConcatenatedCode(tempDir, exts, manualFiles, excludePatterns, useGitignore, header, marker)
	assertions.NoError(err) // Line ~583
	assertions.Contains(output, header+"\n")

	expectedPath := expectDisplayPath(t, cwd, filepath.Join(tempDir, "file1.txt"))
	assertions.Contains(output, marker+" "+expectedPath+"\nContent\n"+marker)

	logOutput := logBuf.String()
	assertions.Contains(logOutput, "level=WARN")
	assertions.Contains(logOutput, "Invalid exclude pattern")
	assertions.Contains(logOutput, "pattern=[a-z")
}

func TestLogger(t *testing.T) {
	assertions := assert.New(t)
	testLogger, logBuf := setupTestLogger(t)
	slog.SetDefault(testLogger)
	slog.Info("Test log", "key", "value")
	logOutput := logBuf.String()
	t.Logf("Log output: %q", logOutput)
	assertions.Contains(logOutput, "Test log")
	assertions.Contains(logOutput, "key=value")
}
