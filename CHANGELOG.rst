# CHANGELOG.rst

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

.. _Unreleased: https://github.com/gagin/codecat/compare/v0.4.0...HEAD
.. _0.4.0: https://github.com/gagin/codecat/compare/v0.3.0...v0.4.0
.. _0.3.0: https://github.com/gagin/codecat/compare/v0.2.2...v0.3.0
.. _0.2.2: https://github.com/gagin/codecat/compare/v0.2.1...v0.2.2
.. _0.2.1: https://github.com/gagin/codecat/compare/v0.1.0...v0.2.1
.. _0.1.0: https://github.com/gagin/codecat/releases/tag/v0.1.0


`Unreleased`_
=============

Added
+++++

*   Unit tests for config loading, helpers, summary generation.
*   Integration tests covering CWD excludes, -f priority, etc.

Changed
+++++++

*   Refine unit tests after integration test fixes.


`0.4.0`_ - 2025-04-25
=====================

Added
+++++

*   **Basename Exclusions:** Introduced ``exclude_basenames`` list in ``config.toml`` for globally excluding files/directories based only on their name (e.g., ``*.log``, ``node_modules``, ``build``), regardless of path. Includes sensible defaults.
*   **Project Exclusions:** Added support for a ``.codecat_exclude`` file in the Current Working Directory (CWD) for project-specific CWD-relative exclusion patterns (syntax like ``-x``). See ``.codecat_exclude.example``.
*   **Exclusion Interface:** Refactored exclusion logic into an ``Excluder`` interface and ``DefaultExcluder`` implementation (``exclusion.go``) for better separation.
*   **Makefile:** Added a ``Makefile`` with a ``local-bin`` target for easier building and installation to ``~/.local/bin``.
*   **Integration Tests:** Added ``test_integration.sh`` script for integration testing (not working yet, unfinished).
*   Moved manual file processing logic into ``manual_files.go``.
*   Made ``[M]`` marker for manually added files (``-f``) in summary tree visible only when log level is ``debug``.

Changed
+++++++

*   **Exclusion Logic:**
    *   CWD-relative excludes (``-x``, ``.codecat_exclude``) no longer require a trailing slash (``/``) to exclude a directory and its contents; specifying the directory name is sufficient and excludes contents by prefix matching.
    *   Exclusion precedence: Basename (``exclude_basenames``) -> CWD-relative (``-x``, ``.codecat_exclude``) -> Gitignore.
    *   Excluding a directory now correctly prevents its contents from being processed, regardless of walker order.
    *   Manually specified files (``-f``) bypass any exclusions.
*   **Header Formatting:** Removed automatic newline addition after ``header_text``. The header string defined in config (or the default) should now include its own desired trailing newline(s). Default ``header_text`` updated to include one trailing ``\n``.
*   **Logging:** Default log level changed from ``info`` to ``warn``. Refined log levels for various messages (more detailed processing steps moved to ``debug``).
*   **Output Formatting:** Simplified final output formatting, relying on header and file appending logic for newlines.
*   **Extensionless Files Handling:** Solidified approach: Decided against using a special ``.`` marker in ``include_extensions``; use ``-f`` flag to include specific extensionless files like Makefiles or LICENSE files.
*   Refactored helper functions (``matchesGlob``, ``contains``, etc.) into ``helpers.go``.
*   Refactored ``generateConcatenatedCode`` to use the new ``Excluder`` interface and call ``processManualFiles``.
*   Refactored ``printSummaryTree`` to use a generic helper for list sections (DRY).

Removed
+++++++

*   Removed global ``exclude_patterns`` config option (use ``exclude_basenames`` or ``.codecat_exclude`` instead).


`0.3.0`_ - 2025-04-24
=====================

Added
+++++

*   **Recursive Gitignore Processing:** Replaced simple gitignore handling with ``boyter/gocodewalker`` library for robust, recursive processing of ``.gitignore`` and ``.ignore`` files, mimicking standard Git behavior by default.
*   **No Scan Flag:** Added ``-n, --no-scan`` flag to skip directory scanning entirely and only process files specified manually with ``-f``.
*   Split source code into multiple files (``main.go``, ``walk.go``, ``config.go``, ``helpers.go``, ``summary.go``) within ``cmd/codecat/`` subdirectory for better organization.
*   Split tests into corresponding ``*_test.go`` files (``walk_test.go``, ``helpers_test.go``, etc.).

Changed
+++++++

*   **Default Gitignore Behavior:** Now defaults to recursive, Git-compatible ignore file processing.
*   Reverted to using the ``--no-gitignore`` flag and ``use_gitignore`` config option (boolean).
*   Simplified core walking logic by always using ``gocodewalker``.

Fixed
+++++

*   Fixed panic when targeting a non-existent directory.
*   Fixed test failures related to incorrect gitignore handling.
*   Fixed test failure where output was empty when target dir didn't exist but manual files were processed.
*   Fixed incorrect test assertion logic for invalid exclude patterns and non-existent manual files.
*   Fixed exclude pattern implementation for ``gocodewalker``. Manual filtering using ``filepath.Match`` was implemented.

Removed
+++++++

*   Removed ``target_only`` gitignore mode concept.
*   Removed direct dependency on ``sabhiram/go-gitignore``.
*   Removed ``dev_process_utils`` directory.


`0.2.2`_ - 2025-04-23 (Internal)
================================

Fixed
+++++

*   Corrected initial exclude pattern logic (``-x``) to match relative paths within the target directory.


`0.2.1`_ - 2025-04-20 (Internal Refactor/Rename)
================================================

Changed
+++++++

*   Project renamed from ``food4ai`` to ``codecat``.
*   Internal code improvements and flag parsing adjustments.


`0.1.0`_ - 2025-04-19 (Initial Version)
=======================================

*   Initial release as ``food4ai``.
*   Core functionality: concatenate files based on extensions, simple excludes, basic ``.gitignore`` support (target root only), output to stdout or file.