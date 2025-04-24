=======
TODO List
=======

High Priority / Next Steps
--------------------------

*   **Implement `-x dir/` Robustly:**
    *   Verify the current manual check (`strings.HasPrefix`) is sufficient.
    *   Add specific unit tests in `walk_test.go` (e.g., `TestGenerateConcatenatedCode_ExcludeDirSyntax`) covering various scenarios (root level dir, nested dir, pattern with/without trailing slash). Ref: UC-004.
*   **Implement Metadata Output Options:**
    *   Add `include_file_list_in_output` and `include_empty_files_in_output` options to `Config` struct (`config.go`) and `defaultConfig`.
    *   Update `loadConfig` (`config.go`) to handle loading these options.
    *   Add corresponding command-line flags (e.g., `--include-file-list`, `--include-empty-files`) in `main.go` (`init` and settings determination).
    *   Update `generateConcatenatedCode` signature (`walk.go`) to accept these booleans.
    *   Add logic at the end of `generateConcatenatedCode` (`walk.go`) to append formatted lists to `outputBuilder` if flags are true.
    *   Add unit tests (`TestGenerateConcatenatedCode_MetadataOutput` in `walk_test.go`).
*   **Add Missing Unit Tests:**
    *   Add tests for new flags/modes (`--no-gitignore`, `-n`) in `walk_test.go`.
    *   Add tests verifying additive exclude logic (`-x` + config) in `walk_test.go`.
    *   Add tests for `loadConfig` in `config_test.go`.
    *   Add tests for `formatBytes`, `mapsKeys` in `helpers_test.go`.
    *   Add tests for `buildTree` in `summary_test.go`.


Medium Priority / Refinements
-----------------------------

*   **Refactor Tests:** Ensure all tests are in the most appropriate `*_test.go` file (`walk_test`, `config_test`, `helpers_test`, `summary_test`).
*   **`gocodewalker` Excludes:** Re-evaluate if `gocodewalker`'s exclude fields (`ExcludeDirectoryRegex`, `LocationExcludePattern` with better patterns) can reliably replace the manual `filepath.Match` checks for better performance, especially for complex globs (`**`). Requires careful testing.
*   **Error Handling:** Refine error messages and potentially wrap errors for more context. Consider specific error types.
*   **Performance:** Benchmark performance on very large repositories, especially the `gocodewalker` integration.


Low Priority / Future Ideas
---------------------------

*   **Configurable Ignore Files:** Allow specifying custom ignore filenames beyond `.gitignore` and `.ignore` (e.g., `--ignore-file .p4ignore`). Ref: Non-Git VCS scenario.
*   **Piping Improvements:** Ensure piping (`codecat | other_cmd`) works flawlessly, potentially adding flags to suppress summary output to stderr if needed. Ref: UC-006.
*   **Alternative Walkers:** Keep an eye on other potential walking libraries.
*   **Metadata Output File:** Option to write summary/tree/lists to a separate file (`--metadata-out`) instead of embedding in code output or printing to stderr/stdout.