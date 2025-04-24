// walk_library_test.go
package main // Assuming it's in the same package as main.go

import (
	// Needed by TestGoCodeWalkerGitignoreBehavior check
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/boyter/gocodewalker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helper function to create test directories ---
func setupWalkTestDir(t *testing.T, structure map[string]string) string {
	t.Helper()
	rootDir := t.TempDir()

	paths := make([]string, 0, len(structure))
	for p := range structure {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, relPath := range paths {
		content := structure[relPath]
		absPath := filepath.Join(rootDir, relPath)
		parentDir := filepath.Dir(absPath)

		if _, err := os.Stat(parentDir); os.IsNotExist(err) {
			errMkdir := os.MkdirAll(parentDir, 0755)
			require.NoError(t, errMkdir, "Failed to create parent directory: %s", parentDir)
		} else if err != nil {
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
	t.Logf("Test directory structure created at: %s", rootDir)
	return rootDir
}

// --- Test 1: Verify Gitignore Behavior ---
func TestGoCodeWalkerGitignoreBehavior(t *testing.T) {
	t.Logf("--- Starting TestGoCodeWalkerGitignoreBehavior ---")
	// Structure to test nested gitignores, parent ignores, negations
	structure := map[string]string{
		"repo/.gitignore": `
*.out
global_ignored_dir/
`,
		"repo/file_in_repo.txt":                         "Repo level file",
		"repo/ignored_global.out":                       "Should be ignored by root gitignore",
		"repo/global_ignored_dir/":                      "",
		"repo/global_ignored_dir/in_global_ignored.txt": "Should not be visited",
		"repo/target_dir/":                              "",
		"repo/target_dir/.gitignore": `
*.log
/root_ignored_in_target.log
another_ignored_dir/
!important.log
`,
		"repo/target_dir/file1.txt":                                  "Include this",
		"repo/target_dir/ignored.log":                                "Ignore this log",
		"repo/target_dir/important.log":                              "Re-included log file",
		"repo/target_dir/root_ignored_in_target.log":                 "Should be ignored by / rule in target",
		"repo/target_dir/another_ignored_dir/":                       "",
		"repo/target_dir/another_ignored_dir/in_another_ignored.txt": "Should not be visited",
		"repo/target_dir/subdir/":                                    "",
		"repo/target_dir/subdir/.gitignore": `
nested_ignored.log
*.tmp
`,
		"repo/target_dir/subdir/file2.txt":          "Include this nested file",
		"repo/target_dir/subdir/another.log":        "Ignore this (parent rule *.log)",
		"repo/target_dir/subdir/nested_ignored.log": "Ignore this (subdir rule)",
		"repo/target_dir/subdir/tempfile.tmp":       "Ignore this (subdir rule *.tmp)",
		"repo/another_repo_file.txt":                "Should not be visited if walk starts at target_dir",
	}

	rootDir := setupWalkTestDir(t, structure)
	targetDir := filepath.Join(rootDir, "repo", "target_dir")

	expectedRelativePaths := []string{
		"file1.txt",
		"important.log",
		"subdir/file2.txt",
	}
	sort.Strings(expectedRelativePaths)

	visitedRelativePaths := []string{}
	fileListQueue := make(chan *gocodewalker.File, 100)
	fileWalker := gocodewalker.NewFileWalker(targetDir, fileListQueue)

	// Enable gitignore processing (should be default, but explicit is fine)
	fileWalker.IgnoreGitIgnore = false
	// Also test .ignore processing if library supports it (enable for realism)
	fileWalker.IgnoreIgnoreFile = false

	// Allow all extensions for this test to focus on ignore logic
	fileWalker.AllowListExtensions = []string{"txt", "log", "tmp", "out"} // Needs to include all potential extensions

	errorHandler := func(e error) bool {
		t.Errorf("gocodewalker (gitignore test) encountered an error: %v", e)
		return false
	}
	fileWalker.SetErrorHandler(errorHandler)

	// Start and Process
	go func() {
		walkErr := fileWalker.Start()
		if walkErr != nil {
			t.Errorf("fileWalker.Start() (gitignore test) returned error: %v", walkErr)
		}
	}()

	t.Logf("Gitignore Test: Waiting for results...")
	for f := range fileListQueue {
		relPath, err := filepath.Rel(targetDir, f.Location)
		require.NoError(t, err)
		relPath = filepath.ToSlash(relPath)

		fileInfo, err := os.Stat(f.Location)
		if err == nil && !fileInfo.IsDir() {
			visitedRelativePaths = append(visitedRelativePaths, relPath)
			t.Logf("Gitignore Test: Recorded visited file: %s", relPath)
		}
	}
	t.Logf("Gitignore Test: Finished processing channel.")

	// Assert
	sort.Strings(visitedRelativePaths)
	assert.Equal(t, expectedRelativePaths, visitedRelativePaths, "Gitignore Test: Visited file paths mismatch")
	t.Logf("--- Finished TestGoCodeWalkerGitignoreBehavior ---")
}

// --- Test 2: Verify Allow/Deny Precedence ---
func TestGoCodeWalkerAllowDenyPrecedence(t *testing.T) {
	t.Logf("--- Starting TestGoCodeWalkerAllowDenyPrecedence ---")
	structure := map[string]string{
		"include_me.go":    "package main",
		"also_include.txt": "Some text",
		"exclude_me.go":    "package secret", // Allowed extension, but denied path
		"exclude_too.txt":  "More text",      // Allowed extension, but denied path
		"not_allowed.py":   "print('no')",    // Disallowed extension
		"build/":           "",
		"build/output.go":  "package build",  // Allowed extension, but denied directory
		"build/data.txt":   "build data",     // Allowed extension, but denied directory
		"vendor/lib.go":    "package vendor", // Allowed extension, but denied directory via pattern
	}
	targetDir := setupWalkTestDir(t, structure)

	expectedRelativePaths := []string{
		"include_me.go",
		"also_include.txt",
	}
	sort.Strings(expectedRelativePaths)

	visitedRelativePaths := []string{}
	fileListQueue := make(chan *gocodewalker.File, 100)
	fileWalker := gocodewalker.NewFileWalker(targetDir, fileListQueue)

	// Configure Walker
	fileWalker.IgnoreGitIgnore = true // Disable gitignore for this specific test
	fileWalker.IgnoreIgnoreFile = true
	fileWalker.AllowListExtensions = []string{"go", "txt"}
	fileWalker.DenyListPatterns = []string{"exclude_me.go", "exclude_too.txt", "build/", "vendor/*"}

	t.Logf("Precedence Test: Walker Config: AllowExt=%v, DenyPatterns=%v, IgnoreGitIgnore=%v",
		fileWalker.AllowListExtensions, fileWalker.DenyListPatterns, fileWalker.IgnoreGitIgnore)

	errorHandler := func(e error) bool {
		t.Errorf("gocodewalker (precedence test) encountered an error: %v", e)
		return false
	}
	fileWalker.SetErrorHandler(errorHandler)

	// Start and Process
	go func() {
		walkErr := fileWalker.Start()
		if walkErr != nil {
			t.Errorf("fileWalker.Start() (precedence test) returned error: %v", walkErr)
		}
	}()

	t.Logf("Precedence Test: Waiting for results...")
	for f := range fileListQueue {
		relPath, err := filepath.Rel(targetDir, f.Location)
		require.NoError(t, err)
		relPath = filepath.ToSlash(relPath)

		fileInfo, err := os.Stat(f.Location)
		if err == nil && !fileInfo.IsDir() {
			visitedRelativePaths = append(visitedRelativePaths, relPath)
			t.Logf("Precedence Test: Recorded visited file: %s", relPath)
		}
	}
	t.Logf("Precedence Test: Finished processing channel.")

	// Assert
	sort.Strings(visitedRelativePaths)
	assert.Equal(t, expectedRelativePaths, visitedRelativePaths, "Precedence Test: DenyListPatterns should take precedence over AllowListExtensions")
	t.Logf("--- Finished TestGoCodeWalkerAllowDenyPrecedence ---")
}
