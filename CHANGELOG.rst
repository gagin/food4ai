# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

*   **Metadata in Output (Optional):** Config options (`include_file_list_in_output`, `include_empty_files_in_output`) and flags (`--include-file-list`, `--include-empty-files`) to append lists of included/empty files to the main code output. [See TODO.rst]
*   Unit tests for config loading, helpers, summary generation.

### Changed

*   Refine `-x dir/` exclusion logic and add specific tests.

## [0.3.0] - 2025-04-24

### Added

*   **Recursive Gitignore Processing:** Replaced simple gitignore handling with `boyter/gocodewalker` library for robust, recursive processing of `.gitignore` and `.ignore` files, mimicking standard Git behavior by default.
*   **No Scan Flag:** Added `-n, --no-scan` flag to skip directory scanning entirely and only process files specified manually with `-f`.
*   Added `globToRegex` helper function (internal).
*   Split source code into multiple files (`main.go`, `walk.go`, `config.go`, `helpers.go`, `summary.go`) within `cmd/codecat/` subdirectory for better organization.
*   Split tests into corresponding `*_test.go` files (`walk_test.go`, `helpers_test.go`, etc.).

### Changed

*   **Default Gitignore Behavior:** Now defaults to recursive, Git-compatible ignore file processing.
*   **Exclusion Logic:** Command-line `-x` patterns now *add* to the patterns defined in the config file's `exclude_patterns` list, instead of replacing them.
*   Reverted to using the `--no-gitignore` flag and `use_gitignore` config option (boolean) instead of the more complex `gitignore_mode` string option.
*   Simplified core walking logic by always using `gocodewalker` and configuring its ignore settings, removing dependency on `sabhiram/go-gitignore`.

### Fixed

*   Fixed panic when `codecat` was run targeting a non-existent directory (`TestGenerateConcatenatedCode_NonExistentDir`).
*   Fixed test failures related to incorrect gitignore handling (`TestGenerateConcatenatedCode_WithGitignore`).
*   Fixed test failure where output was empty when target dir didn't exist but manual files were processed (`TestGenerateConcatenatedCode_NonExistentDir_WithManualFile`).
*   Fixed incorrect test assertion logic for invalid exclude patterns and non-existent manual files.
*   Fixed exclude pattern implementation which was not working correctly with `gocodewalker`. Manual filtering using `filepath.Match` was implemented.

### Removed

*   Removed `target_only` gitignore mode concept due to implementation complexity and lack of clear use cases vs. `--no-gitignore` + `-x`.
*   Removed direct dependency on `sabhiram/go-gitignore`.
*   Removed `dev_process_utils` directory and related scripts (manual versioning now).

## [0.2.2] - 2025-04-23 (Internal)
### Fixed
*   Corrected initial exclude pattern logic (`-x`) to match relative paths within the target directory.

## [0.2.1] - 2025-04-20 (Internal Refactor/Rename)
*   Project renamed from `food4ai` to `codecat`.
*   Internal code improvements and flag parsing adjustments.

## [0.1.0] - 2025-04-19 (Initial Version)
*   Initial release as `food4ai`.
*   Core functionality: concatenate files based on extensions, simple excludes, basic `.gitignore` support (target root only), output to stdout or file.