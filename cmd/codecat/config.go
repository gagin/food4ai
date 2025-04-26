// cmd/codecat/config.go
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	// include_extensions is handled by code
	IncludeExtensions []string `toml:"include_extensions"`
	// exclude_basenames are glob patterns matched against the final file/directory name anywhere.
	ExcludeBasenames []string `toml:"exclude_basenames"`
	// comment_marker is handled by code
	CommentMarker *string `toml:"comment_marker"`
	// header_text is handled by code
	HeaderText *string `toml:"header_text"`
	// use_gitignore is handled by code
	UseGitignore *bool `toml:"use_gitignore"`
	// Add future fields here
	// IncludeFileListInOutput bool   `toml:"include_file_list_in_output"`
	// IncludeEmptyFilesInOutput bool   `toml:"include_empty_files_in_output"`
}

var defaultConfig = Config{
	IncludeExtensions: []string{"py", "json", "sh", "txt", "rst", "md", "go", "mod", "sum", "yaml", "yml"},
	ExcludeBasenames: []string{ // Default universal excludes based on name
		"*.log",
		"*.pyc",
		"*.pyo",
		"*.swp",
		"*.swo",
		"*~",
		".DS_Store",
		".git", // Exclude the directory itself by basename
		".hg",
		".svn",
		"__pycache__",
		"node_modules",
		"venv",
		".venv",
		"build",
		"dist",
		"target", // Common in Java/Rust
	},
	CommentMarker: func(s string) *string { return &s }("---"),
	HeaderText:    func(s string) *string { return &s }("----- Codebase for analysis -----\n"),
	UseGitignore:  func(b bool) *bool { return &b }(true),
}

// loadConfig loads configuration from default or custom paths.
func loadConfig(customConfigPath string) (Config, error) {
	cfg := defaultConfig
	configFile := ""
	isCustomPath := customConfigPath != ""
	var determinationErr error

	cwd, errCwd := os.Getwd()
	if errCwd != nil {
		slog.Error("DEBUG: Failed to get CWD in loadConfig", "error", errCwd)
		determinationErr = fmt.Errorf("failed to get current working directory: %w", errCwd)
	} else {
		slog.Debug("DEBUG: Current working directory in loadConfig", "cwd", cwd)
	}

	if determinationErr == nil {
		if isCustomPath {
			var err error
			slog.Debug("DEBUG: Resolving custom config path", "custom_path_arg", customConfigPath, "cwd", cwd)
			configFile, err = filepath.Abs(customConfigPath)
			if err != nil {
				slog.Error("Could not determine absolute path for custom config file.", "path", customConfigPath, "error", err)
				determinationErr = fmt.Errorf("invalid custom config path '%s': %w", customConfigPath, err)
			} else {
				slog.Debug("Attempting to load configuration from custom path.", "resolved_absolute_path", configFile)
			}
		} else {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				slog.Warn("Could not determine user home directory. Using default settings only.", "error", err)
				return cfg, nil // Non-fatal, just use defaults
			}
			configDir := filepath.Join(homeDir, ".config", "codecat")
			configFile = filepath.Join(configDir, "config.toml")
			slog.Debug("Attempting to load configuration from default path.", "path", configFile)
		}
	}

	if determinationErr != nil {
		// If it was a custom path error, return it. Otherwise, CWD error is non-fatal for default path.
		if isCustomPath {
			return defaultConfig, determinationErr
		}
		slog.Warn("Proceeding with default config due to error determining config path.", "error", determinationErr)
		return cfg, nil
	}

	// If no path was determined (e.g., home dir error for default path), return defaults
	if configFile == "" {
		return cfg, nil
	}

	slog.Debug("Reading configuration file", "path", configFile)
	content, err := os.ReadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if isCustomPath {
				slog.Error("Specified configuration file not found.", "path_read_attempted", configFile)
				return defaultConfig, fmt.Errorf("specified configuration file '%s' not found", configFile)
			} else {
				slog.Info("No default config file found, using default settings.", "path", configFile)
				return cfg, nil // Default config is fine if default file doesn't exist
			}
		} else {
			slog.Error("Error reading config file.", "path", configFile, "error", err)
			// Return error only if it was a custom path, otherwise use defaults
			if isCustomPath {
				return defaultConfig, fmt.Errorf("error reading config file '%s': %w", configFile, err)
			}
			slog.Warn("Using default settings due to error reading default config file.")
			return cfg, nil
		}
	}

	if len(content) == 0 {
		if isCustomPath {
			slog.Warn("Specified configuration file is empty, using default settings.", "path", configFile)
		} else {
			slog.Info("Default configuration file is empty, using default settings.", "path", configFile)
		}
		return cfg, nil // Empty config means use defaults
	}

	slog.Info("Loading configuration.", "path", configFile)
	loadedCfg := defaultConfig // Start with defaults, TOML overlays
	if meta, err := toml.Decode(string(content), &loadedCfg); err != nil {
		slog.Error("Error decoding TOML config file, using default settings.", "path", configFile, "error", err)
		// Return error only if it was a custom path, otherwise use defaults
		if isCustomPath {
			return defaultConfig, fmt.Errorf("error decoding TOML from '%s': %w", configFile, err)
		}
		slog.Warn("Using default settings due to error decoding default config file.")
		return cfg, nil
	} else if len(meta.Undecoded()) > 0 {
		slog.Warn("Unrecognized keys found in config file.", "path", configFile, "keys", meta.Undecoded())
	}

	// Merge loaded fields with defaults carefully, ensuring pointers are handled
	cfg = loadedCfg // Start with potentially partially loaded config

	// Ensure pointer fields have defaults if not set in TOML
	if cfg.CommentMarker == nil {
		cfg.CommentMarker = defaultConfig.CommentMarker
		slog.Debug("Config key 'comment_marker' not set in file, using default.", "value", *cfg.CommentMarker)
	}
	if cfg.HeaderText == nil {
		cfg.HeaderText = defaultConfig.HeaderText
		slog.Debug("Config key 'header_text' not set in file, using default.", "value", *cfg.HeaderText)
	}
	if cfg.UseGitignore == nil {
		cfg.UseGitignore = defaultConfig.UseGitignore
		slog.Debug("Config key 'use_gitignore' not set in file, using default.", "value", *cfg.UseGitignore)
	}
	// Slice fields like IncludeExtensions and ExcludeBasenames are handled directly by TOML decoding over the default struct.

	slog.Debug("Configuration loaded successfully.",
		"source", configFile,
		"header", *cfg.HeaderText,
		"include_extensions", cfg.IncludeExtensions,
		"exclude_basenames", cfg.ExcludeBasenames,
		"comment_marker", *cfg.CommentMarker,
		"use_gitignore", *cfg.UseGitignore,
	)

	return cfg, nil
}
