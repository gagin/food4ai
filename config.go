// config.go
package main

import (
	"errors"
	"fmt"
	"log" // Make sure log is imported if not already
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
	HeaderText        *string  `toml:"header_text"` // Added: Configurable header
}

// Default configuration values
var defaultConfig = Config{
	IncludeExtensions: []string{"py", "json", "sh", "txt", "rst", "md", "go", "mod", "sum", "yaml", "yml"},
	ExcludePatterns:   []string{},
	CommentMarker:     func(s string) *string { return &s }("'''"),
	UseGitignore:      func(b bool) *bool { return &b }(true),
	HeaderText:        func(s string) *string { return &s }("Our current code base"), // Added: Default header
}

// loadConfig finds and loads the configuration
func loadConfig() (Config, error) {
	cfg := defaultConfig // Start with hardcoded defaults

	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("Warning: Could not determine user config directory: %v. Using default settings.", err)
		return cfg, nil
	}

	configFile := filepath.Join(configDir, "food4ai", "config.toml")

	if _, err := os.Stat(configFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("Info: No config file found at %s. Using default settings.", configFile)
			return cfg, nil
		}
		log.Printf("Warning: Error checking config file %s: %v. Using default settings.", configFile, err)
		return cfg, nil
	}

	log.Printf("Info: Loading configuration from %s", configFile)
	content, err := os.ReadFile(configFile)
	if err != nil {
		// Return defaults here too, maybe log error more prominently?
		log.Printf("Error reading config file %s: %v. Using default settings.", configFile, err)
		return defaultConfig, fmt.Errorf("error reading config file %s: %w", configFile, err) // Return error to main
	}

	// Decode TOML, overriding the defaults stored in cfg
	// We store the result in a temporary variable first to check for decode errors
	var loadedCfg Config = defaultConfig // Start fresh with defaults for this load attempt
	if _, err := toml.Decode(string(content), &loadedCfg); err != nil {
		log.Printf("Error decoding TOML from %s: %v. Using default settings.", configFile, err)
		return defaultConfig, fmt.Errorf("error decoding TOML from %s: %w", configFile, err) // Return error to main
	}

	// Now assign the successfully loaded values (or defaults if not present in TOML) back to cfg
	cfg = loadedCfg

	// Handle defaults for pointer fields if they were *not* set in the TOML file
	if cfg.CommentMarker == nil {
		cfg.CommentMarker = defaultConfig.CommentMarker
		log.Printf("Info: 'comment_marker' not set in config, using default: %q", *cfg.CommentMarker)
	}
	if cfg.UseGitignore == nil {
		cfg.UseGitignore = defaultConfig.UseGitignore
		log.Printf("Info: 'use_gitignore' not set in config, using default: %t", *cfg.UseGitignore)
	}
	// Add handling for HeaderText
	if cfg.HeaderText == nil {
		cfg.HeaderText = defaultConfig.HeaderText
		log.Printf("Info: 'header_text' not set in config, using default: %q", *cfg.HeaderText)
	}

	log.Printf("Info: Loaded config: Header=%q, IncludeExts=%v, ExcludePatterns=%v, UseGitignore=%t, CommentMarker=%q",
		*cfg.HeaderText, cfg.IncludeExtensions, cfg.ExcludePatterns, *cfg.UseGitignore, *cfg.CommentMarker)

	return cfg, nil // Return the final config
}
