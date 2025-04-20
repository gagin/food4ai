// config.go
package main

import (
	"errors"
	"fmt"
	"log/slog" // Import slog
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds the application's configurable settings.
type Config struct {
	IncludeExtensions []string `toml:"include_extensions"`
	ExcludePatterns   []string `toml:"exclude_patterns"`
	CommentMarker     *string  `toml:"comment_marker"`
	UseGitignore      *bool    `toml:"use_gitignore"`
	HeaderText        *string  `toml:"header_text"`
}

// Default configuration values (used if config file is missing or keys are absent)
var defaultConfig = Config{
	IncludeExtensions: []string{"py", "json", "sh", "txt", "rst", "md", "go", "mod", "sum", "yaml", "yml"},
	ExcludePatterns:   []string{},
	CommentMarker:     func(s string) *string { return &s }("'''"),
	UseGitignore:      func(b bool) *bool { return &b }(true),
	HeaderText:        func(s string) *string { return &s }("Our current code base"),
}

// loadConfig finds and loads the configuration
// It now uses the slog default logger configured in main().
func loadConfig() (Config, error) {
	cfg := defaultConfig // Start with hardcoded defaults

	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Use slog for logging
		slog.Warn("Could not determine user home directory, using default settings.", "error", err)
		return cfg, nil // Not returning error, just using defaults
	}
	configDir := filepath.Join(homeDir, ".config")
	configFile := filepath.Join(configDir, "food4ai", "config.toml")

	slog.Debug("Attempting to load configuration.", "path", configFile)

	content, err := os.ReadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Info("No config file found, using default settings.", "path", configFile)
			return cfg, nil // Not an error, just use defaults
		}
		// Log actual error reading file as an error, but return defaults
		slog.Error("Error reading config file, using default settings.", "path", configFile, "error", err)
		// We return the default config but also the error to signal failure
		return defaultConfig, fmt.Errorf("error reading config file %s: %w", configFile, err)
	}

	slog.Info("Loading configuration.", "path", configFile)

	// Decode TOML, overriding the defaults stored in cfg
	var loadedCfg = defaultConfig // Start fresh with defaults for this load attempt
	if _, err := toml.Decode(string(content), &loadedCfg); err != nil {
		slog.Error("Error decoding TOML config file, using default settings.", "path", configFile, "error", err)
		return defaultConfig, fmt.Errorf("error decoding TOML from %s: %w", configFile, err) // Return error to main
	}

	// Assign the successfully loaded values back to cfg
	cfg = loadedCfg

	// Handle defaults for pointer fields if they were *not* set in the TOML file
	// These logs are debug level as they are usually not critical info
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
		"header", *cfg.HeaderText,
		"include_extensions", cfg.IncludeExtensions,
		"exclude_patterns", cfg.ExcludePatterns,
		"use_gitignore", *cfg.UseGitignore,
		"comment_marker", *cfg.CommentMarker,
	)

	return cfg, nil // Return the final config and nil error
}
