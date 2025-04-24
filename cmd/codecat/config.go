// cmd/codecat/config.go
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

// Config struct using original UseGitignore bool for now
type Config struct {
	IncludeExtensions []string `toml:"include_extensions"`
	ExcludePatterns   []string `toml:"exclude_patterns"`
	CommentMarker     *string  `toml:"comment_marker"`
	HeaderText        *string  `toml:"header_text"`
	UseGitignore      *bool    `toml:"use_gitignore"` // Keep until flags are updated
}

var defaultConfig = Config{
	IncludeExtensions: []string{"py", "json", "sh", "txt", "rst", "md", "go", "mod", "sum", "yaml", "yml"},
	ExcludePatterns:   []string{},
	CommentMarker:     func(s string) *string { return &s }("---"),
	HeaderText:        func(s string) *string { return &s }("Codebase for analysis:"),
	UseGitignore:      func(b bool) *bool { return &b }(true), // Default to using gitignore
}

// loadConfig finds and loads the configuration
func loadConfig(customConfigPath string) (Config, error) {
	_ = time.Now() // Keep timestamp context

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
			slog.Debug("DEBUG: Resolving custom config path relative to CWD", "custom_path_arg", customConfigPath, "cwd", cwd)
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
				return cfg, nil
			}
			configDir := filepath.Join(homeDir, ".config", "codecat")
			configFile = filepath.Join(configDir, "config.toml")
			slog.Debug("Attempting to load configuration from default path.", "path", configFile)
		}
	}

	if determinationErr != nil {
		return defaultConfig, determinationErr
	}

	if configFile == "" && !isCustomPath {
		return cfg, nil
	}
	if configFile == "" && isCustomPath {
		return defaultConfig, fmt.Errorf("failed to resolve custom config path: %s", customConfigPath)
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
				return cfg, nil
			}
		} else {
			slog.Error("Error reading config file.", "path", configFile, "error", err)
			return defaultConfig, fmt.Errorf("error reading config file '%s': %w", configFile, err)
		}
	}

	if len(content) == 0 {
		if isCustomPath {
			slog.Warn("Specified configuration file is empty, using default settings.", "path", configFile)
		} else {
			slog.Info("Default configuration file is empty, using default settings.", "path", configFile)
		}
		return cfg, nil
	}

	slog.Info("Loading configuration.", "path", configFile)
	loadedCfg := defaultConfig
	if meta, err := toml.Decode(string(content), &loadedCfg); err != nil {
		slog.Error("Error decoding TOML config file, using default settings.", "path", configFile, "error", err)
		return defaultConfig, fmt.Errorf("error decoding TOML from '%s': %w", configFile, err)
	} else if len(meta.Undecoded()) > 0 {
		slog.Warn("Unrecognized keys found in config file.", "path", configFile, "keys", meta.Undecoded())
	}

	cfg = loadedCfg

	// Ensure pointer fields have defaults if nil after decoding
	if cfg.CommentMarker == nil {
		cfg.CommentMarker = defaultConfig.CommentMarker
		slog.Debug("Config key 'comment_marker' not set, using default.", "value", *cfg.CommentMarker)
	}
	if cfg.HeaderText == nil {
		cfg.HeaderText = defaultConfig.HeaderText
		slog.Debug("Config key 'header_text' not set, using default.", "value", *cfg.HeaderText)
	}
	if cfg.UseGitignore == nil { // Keep this check for now
		cfg.UseGitignore = defaultConfig.UseGitignore
		slog.Debug("Config key 'use_gitignore' not set, using default.", "value", *cfg.UseGitignore)
	}

	slog.Debug("Configuration loaded successfully.",
		"source", configFile,
		"header", *cfg.HeaderText,
		"include_extensions", cfg.IncludeExtensions,
		"exclude_patterns", cfg.ExcludePatterns,
		"use_gitignore", *cfg.UseGitignore, // Log the loaded value
		"comment_marker", *cfg.CommentMarker,
	)

	return cfg, nil
}
