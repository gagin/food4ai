// config.go
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Config struct and defaultConfig var remain the same
type Config struct {
	IncludeExtensions []string `toml:"include_extensions"`
	ExcludePatterns   []string `toml:"exclude_patterns"`
	CommentMarker     *string  `toml:"comment_marker"`
	UseGitignore      *bool    `toml:"use_gitignore"`
	HeaderText        *string  `toml:"header_text"`
}

var defaultConfig = Config{
	IncludeExtensions: []string{"py", "json", "sh", "txt", "rst", "md", "go", "mod", "sum", "yaml", "yml"},
	ExcludePatterns:   []string{},
	CommentMarker:     func(s string) *string { return &s }("---"),
	UseGitignore:      func(b bool) *bool { return &b }(true),
	HeaderText:        func(s string) *string { return &s }("Codebase for analysis:"),
}

// loadConfig finds and loads the configuration from a specific path or the default location.
func loadConfig(customConfigPath string) (Config, error) {
	// Add a timestamp comment for context
	// Current time: Sunday, April 20, 2025 at 12:55:18 AM PDT
	_ = time.Now()

	cfg := defaultConfig // Start with hardcoded defaults
	configFile := ""
	isCustomPath := customConfigPath != ""
	var determinationErr error

	// *** DEBUG: Check Go's CWD ***
	cwd, errCwd := os.Getwd()
	if errCwd != nil {
		slog.Error("DEBUG: Failed to get CWD in loadConfig", "error", errCwd)
		// If we can't get CWD, resolving paths might fail, treat as error
		determinationErr = fmt.Errorf("failed to get current working directory: %w", errCwd)
	} else {
		slog.Debug("DEBUG: Current working directory in loadConfig", "cwd", cwd)
	}
	// *** END DEBUG ***

	if determinationErr == nil { // Only proceed if CWD was determined
		if isCustomPath {
			// Use the path provided by the flag
			var err error
			slog.Debug("DEBUG: Resolving custom config path relative to CWD", "custom_path_arg", customConfigPath, "cwd", cwd)
			configFile, err = filepath.Abs(customConfigPath) // Resolve based on CWD
			if err != nil {
				slog.Error("Could not determine absolute path for custom config file.", "path", customConfigPath, "error", err)
				determinationErr = fmt.Errorf("invalid custom config path '%s': %w", customConfigPath, err)
			} else {
				slog.Debug("Attempting to load configuration from custom path.", "resolved_absolute_path", configFile)
			}
		} else {
			// Determine default path if no custom path provided
			homeDir, err := os.UserHomeDir()
			if err != nil {
				slog.Warn("Could not determine user home directory. Using default settings only.", "error", err)
				// Cannot determine default path, return defaults (not fatal)
				return cfg, nil
			}
			configDir := filepath.Join(homeDir, ".config", "codecat")
			configFile = filepath.Join(configDir, "config.toml")
			slog.Debug("Attempting to load configuration from default path.", "path", configFile)
		}
	}

	// If path determination failed earlier, return now
	if determinationErr != nil {
		return defaultConfig, determinationErr
	}

	// Check if configFile path is actually determined
	if configFile == "" && !isCustomPath {
		// This case happens if home dir wasn't found, already logged. Use defaults.
		return cfg, nil
	}
	if configFile == "" && isCustomPath {
		// This case happens if filepath.Abs failed for custom path, error already set.
		return defaultConfig, fmt.Errorf("failed to resolve custom config path: %s", customConfigPath)
	}

	// Read the determined configuration file
	slog.Debug("Reading configuration file", "path", configFile)
	content, err := os.ReadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if isCustomPath {
				slog.Error("Specified configuration file not found.", "path_read_attempted", configFile) // Log the path it actually tried
				return defaultConfig, fmt.Errorf("specified configuration file '%s' not found", configFile)
			} else {
				slog.Info("No default config file found, using default settings.", "path", configFile)
				return cfg, nil
			}
		} else {
			slog.Error("Error reading config file.", "path", configFile, "error", err)
			return defaultConfig, fmt.Errorf("error reading config file '%s': %w", configFile, err)
		}
	}

	// Handle empty configuration file
	if len(content) == 0 {
		if isCustomPath {
			slog.Warn("Specified configuration file is empty, using default settings.", "path", configFile)
		} else {
			slog.Info("Default configuration file is empty, using default settings.", "path", configFile)
		}
		return cfg, nil
	}

	// Decode TOML content
	slog.Info("Loading configuration.", "path", configFile)
	loadedCfg := defaultConfig
	if meta, err := toml.Decode(string(content), &loadedCfg); err != nil {
		slog.Error("Error decoding TOML config file, using default settings.", "path", configFile, "error", err)
		return defaultConfig, fmt.Errorf("error decoding TOML from '%s': %w", configFile, err)
	} else if len(meta.Undecoded()) > 0 {
		slog.Warn("Unrecognized keys found in config file.", "path", configFile, "keys", meta.Undecoded())
	}

	cfg = loadedCfg

	// Ensure pointer fields have defaults if they were nil after decoding
	if cfg.CommentMarker == nil {
		cfg.CommentMarker = defaultConfig.CommentMarker
		slog.Debug("Config key 'comment_marker' not set, using default.", "value", *cfg.CommentMarker)
	}
	if cfg.UseGitignore == nil {
		cfg.UseGitignore = defaultConfig.UseGitignore
		slog.Debug("Config key 'use_gitignore' not set, using default.", "value", *cfg.UseGitignore)
	}
	if cfg.HeaderText == nil {
		cfg.HeaderText = defaultConfig.HeaderText
		slog.Debug("Config key 'header_text' not set, using default.", "value", *cfg.HeaderText)
	}

	slog.Debug("Configuration loaded successfully.",
		"source", configFile,
		"header", *cfg.HeaderText,
		"include_extensions", cfg.IncludeExtensions,
		"exclude_patterns", cfg.ExcludePatterns,
		"use_gitignore", *cfg.UseGitignore,
		"comment_marker", *cfg.CommentMarker,
	)

	return cfg, nil
}
