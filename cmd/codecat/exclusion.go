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
	// --- Check Ancestors ---
	// Iterate through parent directories of the current item's CWD-relative path
	currentParent := info.RelPathCwd
	for {
		// Get the directory containing the current path component
		currentParent = filepath.Dir(currentParent)
		// Stop if we reach the root (".") or an empty string
		if currentParent == "." || currentParent == "" || currentParent == "/" {
			break
		}

		// Check 1: Is this ancestor explicitly in the excluded map?
		e.mu.RLock()
		causingPattern, exists := e.excludedDirRelPathsCwd[currentParent]
		e.mu.RUnlock()
		if exists {
			slog.Debug("Exclusion check: path excluded due to ancestor in map",
				"path", info.RelPathCwd, "ancestorDir", currentParent, "causingPattern", causingPattern)
			return true, fmt.Sprintf("ancestor %s excluded", currentParent), causingPattern
		}

		// Check 2: Would this ancestor be excluded by basename?
		ancestorBasename := filepath.Base(currentParent)
		if match, p := matchesGlob(ancestorBasename, e.basenamePatterns); match {
			slog.Debug("Exclusion check: path excluded due to ancestor basename match",
				"path", info.RelPathCwd, "ancestorDir", currentParent, "basename", ancestorBasename, "pattern", p)
			// We don't add the ancestor to the map here, just determine exclusion
			return true, fmt.Sprintf("ancestor %s basename match", currentParent), p
		}

		// Check 3: Would this ancestor be excluded by CWD rules (exact/glob or prefix)?
		// Check exact/glob match for the ancestor path
		if match, p := matchesGlob(currentParent, e.cwdRelativePatterns); match {
			slog.Debug("Exclusion check: path excluded due to ancestor CWD exact/glob match",
				"path", info.RelPathCwd, "ancestorDir", currentParent, "pattern", p)
			return true, fmt.Sprintf("ancestor %s CWD match", currentParent), p
		}
		// Check prefix match using the original patterns against the ancestor
		for _, patt := range e.cwdRelativePatterns {
			cleanPattern := strings.TrimRight(patt, `\/`)
			if cleanPattern != "" && (currentParent == cleanPattern || strings.HasPrefix(currentParent, cleanPattern+"/")) {
				slog.Debug("Exclusion check: path excluded due to ancestor CWD prefix match",
					"path", info.RelPathCwd, "ancestorDir", currentParent, "pattern", patt)
				return true, fmt.Sprintf("ancestor %s CWD prefix match", currentParent), patt
			}
		}
	}
	// --- End Ancestor Check ---

	// --- Check Current Item (if not excluded by ancestors) ---
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

	// Check CWD Relative Patterns (Match only, prefix handled by ancestor check now)
	if match, p := matchesGlob(info.RelPathCwd, e.cwdRelativePatterns); match {
		slog.Debug("Exclusion check: item excluded by CWD-relative exact/glob",
			"path", info.RelPathCwd, "pattern", p)
		if info.IsDir {
			e.mu.Lock()
			if _, exists := e.excludedDirRelPathsCwd[info.RelPathCwd]; !exists {
				slog.Debug("Adding dir to excluded map (item CWD exact/glob match).", "relPathCwd", info.RelPathCwd, "pattern", p)
				e.excludedDirRelPathsCwd[info.RelPathCwd] = p
			}
			e.mu.Unlock()
		}
		return true, "CWD-relative match", p
	}
	// --- End Current Item Check ---

	// Not excluded by any rule
	slog.Debug("Exclusion check: path not excluded", "path", info.RelPathCwd)
	return false, "", ""
}
