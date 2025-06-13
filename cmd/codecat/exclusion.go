// cmd/codecat/exclusion.go
package main

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
)

// PathInfo holds information about a path being considered for exclusion.
type PathInfo struct {
	AbsPath    string // Absolute path on the filesystem
	RelPathCwd string // Path relative to CWD, using slashes
	BaseName   string // Final component of the path
	IsDir      bool   // Is the path a directory?
}

// Excluder defines the interface for checking if a path should be excluded.
type Excluder interface {
	IsExcluded(info PathInfo) (excluded bool, reason string, pattern string)
}

// DefaultExcluder implements the Excluder interface using basename and CWD-relative rules.
type DefaultExcluder struct {
	basenamePatterns       []string
	cwdRelativePatterns    []string
	excludedDirRelPathsCwd map[string]string // CWD-relative path -> causing pattern
	mu                     sync.RWMutex
}

// NewDefaultExcluder creates and initializes a DefaultExcluder.
func NewDefaultExcluder(basenamePatterns, cwdRelativePatterns []string) *DefaultExcluder {
	return &DefaultExcluder{
		basenamePatterns:       basenamePatterns,
		cwdRelativePatterns:    cwdRelativePatterns,
		excludedDirRelPathsCwd: make(map[string]string),
	}
}

// IsExcluded implements the Excluder interface with ancestor checking.
func (e *DefaultExcluder) IsExcluded(info PathInfo) (excluded bool, reason string, pattern string) {
	// --- ANCESTOR CHECKS ---

	// Check 1: Robustly check if any parent directory's BASENAME is in the global exclude list.
	// This fixes the bug where a subdirectory like 'exclude-me' wasn't being excluded by a basename rule.
	pathParts := strings.Split(filepath.ToSlash(info.RelPathCwd), "/")
	if len(pathParts) > 1 {
		// Check all parts except the last one (the item's own basename)
		for _, part := range pathParts[:len(pathParts)-1] {
			if match, p := matchesGlob(part, e.basenamePatterns); match {
				slog.Debug("Exclusion check: path excluded due to ancestor basename match",
					"path", info.RelPathCwd, "ancestor", part, "pattern", p)
				return true, fmt.Sprintf("ancestor %s basename match", part), p
			}
		}
	}

	// Check 2: Check for CWD-relative patterns matching any ancestor path.
	// This preserves the working logic for '.codecat_exclude' files (e.g., excluding 'sample-docs').
	currentParent := info.RelPathCwd
	for {
		currentParent = filepath.Dir(currentParent)
		if currentParent == "." || currentParent == "" || currentParent == "/" {
			break
		}
		// CWD-relative glob match
		if match, p := matchesGlob(currentParent, e.cwdRelativePatterns); match {
			slog.Debug("Exclusion check: path excluded due to ancestor CWD exact/glob match", "path", info.RelPathCwd, "ancestorDir", currentParent, "pattern", p)
			return true, fmt.Sprintf("ancestor %s CWD match", currentParent), p
		}
		// CWD-relative prefix match (e.g., 'docs/' matches 'docs/file.txt')
		for _, patt := range e.cwdRelativePatterns {
			cleanPattern := strings.TrimRight(patt, `\/`)
			if cleanPattern != "" && strings.HasPrefix(currentParent, cleanPattern+"/") {
				slog.Debug("Exclusion check: path excluded due to ancestor CWD prefix match", "path", info.RelPathCwd, "ancestorDir", currentParent, "pattern", patt)
				return true, fmt.Sprintf("ancestor %s CWD prefix match", currentParent), patt
			}
		}
	}

	// --- CURRENT ITEM CHECKS (if not excluded by an ancestor) ---

	// Check Basename Excludes for the item itself
	if match, p := matchesGlob(info.BaseName, e.basenamePatterns); match {
		slog.Debug("Exclusion check: item excluded by basename",
			"path", info.RelPathCwd, "basename", info.BaseName, "pattern", p)
		if info.IsDir {
			e.mu.Lock()
			if _, exists := e.excludedDirRelPathsCwd[info.RelPathCwd]; !exists {
				slog.Debug("Adding dir to excluded map (item basename match).", "relPathCwd", info.RelPathCwd, "pattern", p)
				e.excludedDirRelPathsCwd[info.RelPathCwd] = p
			}
			e.mu.Unlock()
		}
		return true, "basename match", p
	}

	// Check CWD Relative Patterns for the item itself
	for _, p := range e.cwdRelativePatterns {
		match, _ := filepath.Match(p, info.RelPathCwd)
		// Also check if a pattern like "foo/" matches directory "foo"
		if !match && info.IsDir && strings.HasSuffix(p, "/") {
			match, _ = filepath.Match(strings.TrimRight(p, "/"), info.RelPathCwd)
		}
		if match {
			slog.Debug("Exclusion check: item excluded by CWD-relative pattern",
				"path", info.RelPathCwd, "pattern", p)
			if info.IsDir {
				e.mu.Lock()
				if _, exists := e.excludedDirRelPathsCwd[info.RelPathCwd]; !exists {
					slog.Debug("Adding dir to excluded map (item CWD match).", "relPathCwd", info.RelPathCwd, "pattern", p)
					e.excludedDirRelPathsCwd[info.RelPathCwd] = p
				}
				e.mu.Unlock()
			}
			return true, "CWD-relative match", p
		}
	}

	// Not excluded by any rule
	slog.Debug("Exclusion check: path not excluded", "path", info.RelPathCwd)
	return false, "", ""
}
