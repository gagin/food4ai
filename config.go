// config.go
// Handles loading configuration from TOML files for food4ai.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time" // Added for timestamp comment

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
	CommentMarker:     func(s string) *string { return &s }("---"),
	UseGitignore:      func(b bool) *bool { return &b }(true),
	HeaderText:        func(s string) *string { return &s }("Codebase for analysis:"),
}

// loadConfig finds and loads the configuration from a specific path (if provided)
// or the default location (~/.config/food4ai/config.toml).
// It returns the loaded configuration or the hardcoded defaults if loading fails appropriately.
func loadConfig(customConfigPath string) (Config, error) {
	// Add a timestamp comment for context
	// Current time: Saturday, April 19, 2025 at 11:31:48 PM PDT
	_ = time.Now()

	cfg := defaultConfig // Start with hardcoded defaults
	configFile := ""
	isCustomPath := customConfigPath != ""
	var determinationErr error // To store path determination errors

	if isCustomPath {
		// Use the path provided by the flag, resolve to absolute path
		var err error
		configFile, err = filepath.Abs(customConfigPath)
		if err != nil {
			slog.Error("Could not determine absolute path for custom config file.", "path", customConfigPath, "error", err)
			// Return default config but signal the path error was critical
			return defaultConfig, fmt.Errorf("invalid custom config path '%s': %w", customConfigPath, err)
		}
		slog.Debug("Attempting to load configuration from custom path.", "path", configFile)
	} else {
		// Determine default path if no custom path provided
		homeDir, err := os.UserHomeDir()
		if err != nil {
			// If home dir fails, we cannot find default path, use only hardcoded defaults.
			slog.Warn("Could not determine user home directory. Using default settings only.", "error", err)
			return cfg, nil // Not a fatal error for the app, just can't load defaults.
		}
		// Standard config location
		configDir := filepath.Join(homeDir, ".config", "food4ai")
		configFile = filepath.Join(configDir, "config.toml")
		slog.Debug("Attempting to load configuration from default path.", "path", configFile)
	}

	// If path determination failed earlier, return now
	if determinationErr != nil {
		return defaultConfig, determinationErr
	}

	// Read the determined configuration file
	content, err := os.ReadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// File not found is handled differently for custom vs default paths
			if isCustomPath {
				// If a specific custom path was given but doesn't exist, that's an error.
				slog.Error("Specified configuration file not found.", "path", configFile)
				return defaultConfig, fmt.Errorf("specified configuration file '%s' not found", configFile)
			} else {
				// If the default path doesn't exist, it's okay, just use defaults silently.
				slog.Info("No default config file found, using default settings.", "path", configFile)
				return cfg, nil // Return current defaults (which are hardcoded initially)
			}
		} else {
			// Any other read error (e.g., permissions) is potentially serious.
			slog.Error("Error reading config file.", "path", configFile, "error", err)
			// Return defaults but signal the error, allowing main to decide if it's fatal.
			return defaultConfig, fmt.Errorf("error reading config file '%s': %w", configFile, err)
		}
	}

	// Handle empty configuration file - treat same as non-existent (use defaults)
	if len(content) == 0 {
		if isCustomPath {
			// Warn if a specified file was empty
			slog.Warn("Specified configuration file is empty, using default settings.", "path", configFile)
		} else {
			// Info if the default file was empty
			slog.Info("Default configuration file is empty, using default settings.", "path", configFile)
		}
		return cfg, nil // Return current defaults
	}

	// Decode TOML content
	slog.Info("Loading configuration.", "path", configFile)
	// Start parsing into a copy of defaults, allowing TOML to override
	loadedCfg := defaultConfig
	if meta, err := toml.Decode(string(content), &loadedCfg); err != nil {
		// Treat decoding errors (bad TOML syntax) as fatal.
		slog.Error("Error decoding TOML config file, using default settings.", "path", configFile, "error", err)
		return defaultConfig, fmt.Errorf("error decoding TOML from '%s': %w", configFile, err)
	} else if len(meta.Undecoded()) > 0 {
		// Warn about unrecognized keys in the config file
		slog.Warn("Unrecognized keys found in config file.", "path", configFile, "keys", meta.Undecoded())
	}

	// Assign successfully loaded and decoded config back to cfg
	cfg = loadedCfg

	// Ensure pointer fields have defaults if they were nil after decoding (not set in TOML)
	if cfg.CommentMarker == nil {
		cfg.CommentMarker = defaultConfig.CommentMarker // Assign default pointer
		slog.Debug("Config key 'comment_marker' not set, using default.", "value", *cfg.CommentMarker)
	}
	if cfg.UseGitignore == nil {
		cfg.UseGitignore = defaultConfig.UseGitignore // Assign default pointer
		slog.Debug("Config key 'use_gitignore' not set, using default.", "value", *cfg.UseGitignore)
	}
	if cfg.HeaderText == nil {
		cfg.HeaderText = defaultConfig.HeaderText // Assign default pointer
		slog.Debug("Config key 'header_text' not set, using default.", "value", *cfg.HeaderText)
	}

	slog.Debug("Configuration loaded successfully.",
		"source", configFile, // Log the actual file path used
		"header", *cfg.HeaderText,
		"include_extensions", cfg.IncludeExtensions,
		"exclude_patterns", cfg.ExcludePatterns,
		"use_gitignore", *cfg.UseGitignore,
		"comment_marker", *cfg.CommentMarker,
	)

	return cfg, nil // Return the final config and nil error if successful
}
