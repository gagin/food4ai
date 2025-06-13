#!/bin/bash

# Exit on error, treat unset variables as error
set -eu
# Exit on pipe failures
set -o pipefail

# --- Configuration ---
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
BUILD_OUTPUT_NAME="codecat_test_binary"
BUILD_OUTPUT_PATH="${SCRIPT_DIR}/${BUILD_OUTPUT_NAME}"
CODECAT_CMD="${BUILD_OUTPUT_PATH}"
TEST_DIR_BASE="codecat_test_env_"
VERBOSE=true # Set to false to reduce diff output on failure
TEST_PASSED_COUNT=0
TEST_FAILED_COUNT=0

# --- Test Numbering ---
TEST_CASE_NUMBER=0
TEST_CASE_NAME="" # Store current test name globally for helpers

# --- State Variables ---
declare TEST_DIR=""          # Holds path to current test's temp dir
declare ORIGINAL_PWD=""      # Holds the PWD before cd'ing into TEST_DIR
declare CODE_DIFF_LOG=""     # Path to code diff log if created
declare SUMMARY_DIFF_LOG=""  # Path to summary diff log if created
declare STDOUT_LOG=""        # Path to stdout log
declare STDERR_LOG=""        # Path to stderr log

# --- Helper Functions (With Test Numbering) ---
info() { echo -e "\033[0;36m[INFO] $*\033[0m"; } # General info
test_info() { echo -e "\033[0;36m[INFO] Test #${TEST_CASE_NUMBER} ($TEST_CASE_NAME): $*\033[0m"; } # Test-specific info
pass() {
    local msg="$1"
    echo -e "\033[0;32m[PASS] Test #${TEST_CASE_NUMBER} ($TEST_CASE_NAME): $msg\033[0m"
    TEST_PASSED_COUNT=$((TEST_PASSED_COUNT + 1))
}
fail() {
    local reason="$1"
    TEST_FAILED_COUNT=$((TEST_FAILED_COUNT + 1))
    echo -e "\033[0;31m[FAIL] Test #${TEST_CASE_NUMBER} ($TEST_CASE_NAME): $reason\033[0m" >&2 # Print errors to stderr

    # Dump logs/diffs if they exist
    if [ -n "${STDOUT_LOG:-}" ] && [ -f "$STDOUT_LOG" ]; then
        echo -e "\033[0;33m--- Stdout Log (${STDOUT_LOG}) ---\033[0m" >&2
        cat "$STDOUT_LOG" >&2
        echo -e "\033[0;33m--- End Stdout Log ---\033[0m" >&2
    fi
    if [ -n "${STDERR_LOG:-}" ] && [ -f "$STDERR_LOG" ]; then
        echo -e "\033[0;33m--- Stderr Log (${STDERR_LOG}) ---\033[0m" >&2
        cat "$STDERR_LOG" >&2
        echo -e "\033[0;33m--- End Stderr Log ---\033[0m" >&2
    fi
    if [ -n "${CODE_DIFF_LOG:-}" ] && [ -f "$CODE_DIFF_LOG" ]; then
        echo -e "\033[0;33m--- Code Diff (${CODE_DIFF_LOG}) ---\033[0m" >&2
        cat "$CODE_DIFF_LOG" >&2
        echo -e "\033[0;33m--- End Code Diff ---\033[0m" >&2
    fi
     if [ -n "${SUMMARY_DIFF_LOG:-}" ] && [ -f "$SUMMARY_DIFF_LOG" ]; then
        echo -e "\033[0;33m--- Summary Diff (${SUMMARY_DIFF_LOG}) ---\033[0m" >&2
        cat "$SUMMARY_DIFF_LOG" >&2
        echo -e "\033[0;33m--- End Summary Diff ---\033[0m" >&2
    fi

    # Preserve directory
    if [ -n "${TEST_DIR:-}" ] && [ -d "$TEST_DIR" ]; then
      echo -e "\033[0;33m[INFO] Preserving temporary directory for inspection: $TEST_DIR\033[0m" >&2
    else
       echo -e "\033[0;33m[WARN] Cannot preserve TEST_DIR (not set or doesn't exist).\033[0m" >&2
    fi

    # IMPORTANT: Disable the cleanup trap so the directory isn't removed
    trap - EXIT ERR

    exit 1 # Exit immediately
}

cleanup() {
    local exit_code=$?
    if [ -n "$ORIGINAL_PWD" ] && [ -d "$ORIGINAL_PWD" ]; then
        (cd "$ORIGINAL_PWD") || echo "[Cleanup Warn] Failed cd back to $ORIGINAL_PWD" >&2
    fi
    if [ "$exit_code" -eq 0 ]; then
        if [ -n "${TEST_DIR:-}" ] && [ -d "$TEST_DIR" ]; then
            info "Cleaning up test directory: $TEST_DIR"
            rm -rf "$TEST_DIR"
        fi
        if [ -f "$BUILD_OUTPUT_PATH" ]; then
             info "Cleaning up test binary: $BUILD_OUTPUT_PATH"
             rm -f "$BUILD_OUTPUT_PATH"
        fi
        info "All $TEST_PASSED_COUNT tests passed successfully."
    else
         if [ -n "${TEST_DIR:-}" ] && [ -d "$TEST_DIR" ]; then
            info "Exiting due to error. Cleanup skipped for: $TEST_DIR"
         fi
         info "Test run failed. See errors above."
         # Keep binary on failure for retry
    fi
    # Preserve original exit code if non-zero
    if [ "$exit_code" -ne 0 ]; then exit "$exit_code"; fi
}

# --- Setup ---
trap cleanup EXIT ERR
ORIGINAL_PWD=$(pwd)

build_binary() {
    info "Building test binary..."
    (cd "$SCRIPT_DIR" && \
        go build -ldflags="-X main.Version=test" -o "$BUILD_OUTPUT_NAME" ./cmd/codecat)
    if [ $? -ne 0 ]; then fail "Build failed."; fi # Use fail (which knows test #/name if setup)
    info "Test binary built successfully: $BUILD_OUTPUT_PATH"
    info "Checking existence and permissions of built binary..."
    if [ ! -x "$BUILD_OUTPUT_PATH" ]; then
        fail "Built binary not found or not executable at: $BUILD_OUTPUT_PATH"
    fi
    info "Binary verified."
}

# Setup common file structure - now called explicitly by tests that need it
setup_common_files() {
    test_info "Creating common test file structure..." # Use test_info
    mkdir -p proj/data
    echo 'package main // main.go content' > proj/main.go
    echo 'print("util") # utils.py content' > proj/utils.py
    echo 'text data' > proj/data/file.txt
    echo 'log stuff' > proj/data/ignored.log
    touch proj/empty.txt
    echo '*.log' > proj/.gitignore
    echo 'empty.txt' >> proj/.gitignore
    echo 'manual content' > manual.txt
    echo '{"key": "value"}' > proj/config.json
}

# Base setup: Creates dir, sets paths, cds into it. DOES NOT create files.
setup_test_case_base() {
    local test_name="$1"
    TEST_CASE_NUMBER=$((TEST_CASE_NUMBER + 1))
    TEST_CASE_NAME="$test_name" # Set global name for helpers

    TEST_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${TEST_DIR_BASE}${test_name}_XXXXXX") || \
        fail "Could not create temp dir" # fail() uses TEST_CASE_NAME
    test_info "Created test directory: $TEST_DIR" # Use test_info

    STDOUT_LOG="$TEST_DIR/actual_stdout.log"
    STDERR_LOG="$TEST_DIR/actual_stderr.log"
    CODE_DIFF_LOG="$TEST_DIR/diff_code.log"
    SUMMARY_DIFF_LOG="$TEST_DIR/diff_summary.log"

    cd "$TEST_DIR" || fail "Could not cd into test directory: $TEST_DIR"
}

extract_output_filename() {
    local args_string="$1"
    local filename=""
    if [[ "$args_string" == *"--output "* ]]; then
        local after_output="${args_string#*--output }"
        filename="${after_output%% *}"
    elif [[ "$args_string" == *"-o "* ]]; then
        local after_o="${args_string#*-o }"
        filename="${after_o%% *}"
    fi
    if [[ -z "$filename" ]] || [[ "$filename" == -* ]]; then
        echo ""
    else
        echo "$filename"
    fi
}

# Modified run_test to use order-insensitive block checking
run_test() {
    # Parameters remain the same: test_name is now implicit via global TEST_CASE_NAME
    local codecat_args="$1"; local expected_output_file="$2"; local expected_summary_file="$3"
    test_info "Running test case..." # Use test_info
    test_info "Command: $CODECAT_CMD $codecat_args"

    local output_file=""; local summary_output_target="stderr"; local code_output_target="stdout"
    output_file=$(extract_output_filename "$codecat_args")
    if [ -n "$output_file" ]; then
        summary_output_target="stdout"; code_output_target="file"
        test_info "Expecting code in '$output_file', summary on stdout."
    else test_info "Expecting code on stdout, summary on stderr."; fi

    # Execute command
    set +e
    "$CODECAT_CMD" $codecat_args > "$STDOUT_LOG" 2> "$STDERR_LOG"
    local exit_code=$?
    set -e

    if [ "$exit_code" -ne 0 ]; then
        fail "Command failed with exit code $exit_code."
    else test_info "Command executed successfully."; fi

    # Check Code Output (Order-insensitive block check)
    test_info "Checking code output (target: $code_output_target)..."
    local actual_code_source=""
    if [ "$code_output_target" == "file" ]; then
        actual_code_source="$output_file"
        if [ ! -f "$output_file" ]; then fail "Expected output file '$output_file' not created."; fi
    else # stdout
        actual_code_source="$STDOUT_LOG"
    fi

    local expected_blocks_tmp_file="$TEST_DIR/expected_blocks_null.tmp" # Use full path

    if [ -n "$expected_output_file" ] && [ -f "$expected_output_file" ]; then
        test_info "Performing order-insensitive check of code blocks..."
        local all_blocks_found=true
        local header_line=""
        local marker=""
        local expected_block_count=0
        local actual_block_count=0

        # Extract header
        if read -r header_line < "$expected_output_file" && [[ "$header_line" != "--- "* ]]; then
             actual_header_line=""
             read -r actual_header_line < "$actual_code_source" || true # Allow read fail if file empty
             if [[ "$header_line" != "$actual_header_line" ]]; then
                fail "Header mismatch. Expected '$header_line', Got '$actual_header_line'."
             fi
             test_info "Header matched: '$header_line'"
        else
            header_line=""
        fi

        # Deduce marker
        while IFS= read -r line || [[ -n "$line" ]]; do
            if [[ "$line" == "--- "* ]]; then
                marker_candidate=$(echo "$line" | awk '{print $1}')
                if [[ "$marker_candidate" == "---" ]]; then
                    marker="$marker_candidate"
                    test_info "Deduced marker as: '$marker'"
                    break
                fi
            elif [[ -n "$header_line" && "$line" == "$header_line" ]]; then
                continue
            fi
        done < "$expected_output_file"
        if [[ -z "$marker" ]]; then
            info "[Warn] Could not deduce marker from expected file '$expected_output_file', defaulting to '---'. Comparison might be inaccurate."
            marker="---"
        fi

        # Extract expected blocks using simpler AWK script, separated by null bytes
        awk -v marker_pattern="^${marker} " -v end_marker="${marker}" '
            # If line matches the start marker pattern
            $0 ~ marker_pattern {
                # If a block was already started, print it with end marker and null byte
                if (block_content != "") {
                    printf "%s\n%s%s", block_content, end_marker, "\0";
                }
                # Start the new block content with the current marker line
                block_content = $0;
                next; # Move to next line
            }
            # If inside a block, append the current line
            block_content != "" {
                block_content = block_content ORS $0;
            }
            # After processing all lines, print the last block if any
            END {
                if (block_content != "") {
                    printf "%s\n%s%s", block_content, end_marker, "\0";
                }
            }
        ' marker="${marker}" "$expected_output_file" > "$expected_blocks_tmp_file"


        # Count blocks by counting null bytes using od and wc -l (more robust)
        expected_block_count=$(od -An -t x1 "$expected_blocks_tmp_file" | grep -o '00' | wc -l | tr -d '[:space:]' || echo 0)


        # Explicitly check if the result is numeric, default to 0 otherwise
        if ! [[ "$expected_block_count" =~ ^[0-9]+$ ]] ; then
            expected_block_count=0
        fi

        test_info "Found $expected_block_count expected code blocks."

        # Check each expected block against the actual output
        if [ "$expected_block_count" -gt 0 ]; then
             while IFS= read -r -d $'\0' expected_block; do
                if [[ -z "$expected_block" ]]; then continue; fi # Skip empty strings if any
                if ! grep -Fzq -- "$expected_block" "$actual_code_source"; then
                    echo "--- Expected Block Not Found ---" >&2
                    echo "$expected_block" | cat -v >&2 # Use cat -v for visibility
                    echo "--- End Expected Block ---" >&2
                    all_blocks_found=false
                    # break # Optimization: Exit loop on first mismatch (optional)
                fi
            done < "$expected_blocks_tmp_file"
        fi
        rm -f "$expected_blocks_tmp_file" # Clean up temp file

        if [ "$all_blocks_found" = false ]; then
            diff -u "$expected_output_file" "$actual_code_source" > "$CODE_DIFF_LOG" || true
            fail "One or more expected code blocks not found in output ($actual_code_source)."
        fi

        # Additionally, check if the number of marker lines matches
        actual_block_count=$(grep -Ec "^${marker} " "$actual_code_source" || true) # Count start marker lines
         if [[ "$expected_block_count" -ne "$actual_block_count" ]]; then
            diff -u "$expected_output_file" "$actual_code_source" > "$CODE_DIFF_LOG" || true
            fail "Number of code blocks mismatch. Expected $expected_block_count, found $actual_block_count in output ($actual_code_source)."
        fi

        test_info "All $expected_block_count expected code blocks found in output."
        rm -f "$CODE_DIFF_LOG" # Clean diff log if checks passed

    elif [ -z "$expected_output_file" ]; then
        # Expect empty output (or just header)
        actual_content=$(cat "$actual_code_source")
        actual_header_line=""
         if read -r actual_header_line < "$actual_code_source" && [[ "$actual_header_line" != "--- "* ]]; then
             if [[ "$(echo "${actual_content}" | sed -e "s/^[[:space:]]*//;s/[[:space:]]*$//")" != "$actual_header_line" ]]; then
                 fail "Code output ($actual_code_source) not empty when expected empty (has content beyond header)."
             fi
         elif [ -s "$actual_code_source" ]; then
             fail "Code output ($actual_code_source) not empty when expected empty."
         fi
         test_info "Code output ($actual_code_source) empty (or header-only) as expected."
    else
        fail "Expected output reference file '$expected_output_file' not found."
    fi

    # --- Check Summary Output ---
    test_info "Checking summary output (target: $summary_output_target)..."
    local summary_source_file="$STDERR_LOG"; if [ "$summary_output_target" == "stdout" ]; then summary_source_file="$STDOUT_LOG"; fi
    local actual_summary_body="$TEST_DIR/actual_summary_body.txt"
    local expected_summary_body="$TEST_DIR/expected_summary_body.txt"

    if [ -n "$expected_summary_file" ] && [ -f "$expected_summary_file" ]; then
        if [ -f "$summary_source_file" ]; then
             awk '/--- Summary ---/{summary_started=1; next} /---------------/{exit} summary_started && !/^Included .* relative to CWD/{print}' "$summary_source_file" > "$actual_summary_body"
        else
            test_info "[Warn] Summary source file '$summary_source_file' not found."; touch "$actual_summary_body"
        fi
        tail -n +2 "$expected_summary_file" > "$expected_summary_body"
        if ! diff -u "$expected_summary_body" "$actual_summary_body" > "$SUMMARY_DIFF_LOG"; then
            fail "Summary output body mismatch. Comparing content below CWD header line."
        else
            test_info "Summary output body matches expected."
            rm -f "$SUMMARY_DIFF_LOG" "$actual_summary_body" "$expected_summary_body"
        fi
    elif [ -z "$expected_summary_file" ]; then
        summary_content=$(awk '/--- Summary ---/{flag=1; next} /---------------/{flag=0} flag' "$summary_source_file" 2>/dev/null || true)
        if grep -q '--- Summary ---' "$summary_source_file" && [ -z "$summary_content" ] && \
           ! grep -q -E 'Included [1-9]|Empty files found \([1-9]|Errors encountered \([1-9]' "$summary_source_file"; then
            test_info "Summary output ($summary_output_target) empty (only headers) as expected."
        else
            echo "--- Summary Content Found (Expected Empty) ---" >&2
            awk '/--- Summary ---/,/---------------/' "$summary_source_file" >&2
            echo "--- End Summary Content ---" >&2
            fail "Summary output ($summary_output_target) not empty when expected empty."
        fi
    else
        fail "Expected summary reference file '$expected_summary_file' not found."
    fi

    # If we reached here, the test passed
    pass "Success." # pass() adds test # and name
    rm -f "$STDOUT_LOG" "$STDERR_LOG" # Clean up logs on pass
}


# --- Test Definitions ---

# Test Case 1: Basename Exclusion
test_case_1_basename_exclusion() {
    local test_name="basename_exclusion"
    setup_test_case_base "$test_name" # Use base setup only

    # --- Create test-specific files ---
    test_info "Creating specific files for $test_name"
    mkdir -p scantest/sample-docs
    echo "include me" > scantest/a.txt
    echo "exclude me" > scantest/sample-docs/b.txt
    echo "another exclude" > scantest/c.log

    # --- Create test-specific config ---
    local test_config_file="test_config.toml"
    cat << EOF > "$test_config_file"
include_extensions = ["txt", "log"]
exclude_basenames = ["sample-docs", "*.log"]
use_gitignore = false
header_text = "----- Codebase for analysis -----\n"
EOF

    # --- Create expected output ---
    cat << EOF > expected_output.txt
----- Codebase for analysis -----
--- scantest/a.txt
include me
---
EOF
    # --- Create expected summary ---
    cat << EOF > expected_summary.txt
Included 1 files (11 B total) relative to CWD 'PLACEHOLDER_CWD':
└── scantest
    └── a.txt (11 B)
Empty files found (0):
Errors encountered (0):
EOF
    run_test \
        "-d scantest -o output.txt --loglevel=warn -c $test_config_file" \
        "expected_output.txt" \
        "expected_summary.txt"
}

# Test Case 2: Flags mode (-d proj -e go,py) using Hardcoded Defaults
test_case_2_flags_exts_defaults() {
    local test_name="flags_exts_defaults"
    setup_test_case_base "$test_name" # Use base setup
    setup_common_files # Create the common files needed for this test

    cat << EOF > expected_output.txt
----- Codebase for analysis -----
--- proj/main.go
package main // main.go content
---
--- proj/utils.py
print("util") # utils.py content
---
EOF
    cat << EOF > expected_summary.txt
Included 2 files (65 B total) relative to CWD 'PLACEHOLDER_CWD':
└── proj
    ├── main.go (32 B)
    └── utils.py (33 B)
Empty files found (0):
Errors encountered (0):
EOF
    run_test \
        "--config /dev/null -d proj -e go,py" \
        "expected_output.txt" \
        "expected_summary.txt"
}

# Test Case 3: Flags mode with output file (-o output.txt) using Hardcoded Defaults
test_case_3_flags_output_defaults() {
    local test_name="flags_output_defaults"
    setup_test_case_base "$test_name" # Use base setup
    setup_common_files # Create the common files needed for this test

    cat << EOF > expected_output.txt
----- Codebase for analysis -----
--- proj/config.json
{"key": "value"}
---
--- proj/data/file.txt
text data
---
--- proj/main.go
package main // main.go content
---
--- proj/utils.py
print("util") # utils.py content
---
EOF
    cat << EOF > expected_summary.txt
Included 4 files (101 B total) relative to CWD 'PLACEHOLDER_CWD':
└── proj
    ├── config.json (17 B)
    ├── data
    │   └── file.txt (10 B)
    ├── main.go (31 B)
    └── utils.py (33 B)
Empty files found (0):
Errors encountered (0):
EOF
    run_test \
        "--config /dev/null -d proj -o output.txt" \
        "expected_output.txt" \
        "expected_summary.txt"
}

# Test Case 4: Flags mode with --no-gitignore using Hardcoded Defaults
test_case_4_flags_no_gitignore_defaults() {
    local test_name="flags_no_gitignore_defaults"
    setup_test_case_base "$test_name" # Use base setup
    setup_common_files # Create the common files needed for this test

    cat << EOF > expected_output.txt
----- Codebase for analysis -----
--- proj/config.json
{"key": "value"}
---
--- proj/data/file.txt
text data
---
--- proj/data/ignored.log
log stuff
---
--- proj/main.go
package main // main.go content
---
--- proj/utils.py
print("util") # utils.py content
---
EOF
    cat << EOF > expected_summary.txt
Included 5 files (111 B total) relative to CWD 'PLACEHOLDER_CWD':
└── proj
    ├── config.json (17 B)
    ├── data
    │   ├── file.txt (10 B)
    │   └── ignored.log (10 B)
    ├── main.go (31 B)
    └── utils.py (33 B)
Empty files found (1):
- proj/empty.txt
Errors encountered (0):
EOF
    run_test \
        "--config /dev/null -d proj --no-gitignore -x \"\" -e go,py,txt,log,json" \
        "expected_output.txt" \
        "expected_summary.txt"
}

# Test Case 5: Flags mode with manual file (-f manual.txt -d proj -e go) using Hardcoded Defaults
test_case_5_flags_manual_defaults() {
    local test_name="flags_manual_defaults"
    setup_test_case_base "$test_name" # Use base setup
    setup_common_files # Create the common files needed for this test

    cat << EOF > expected_output.txt
----- Codebase for analysis -----
--- manual.txt
manual content
---
--- proj/main.go
package main // main.go content
---
EOF
    cat << EOF > expected_summary.txt
Included 2 files (46 B total) relative to CWD 'PLACEHOLDER_CWD':
├── manual.txt (15 B)
└── proj
    └── main.go (31 B)
Empty files found (0):
Errors encountered (0):
EOF
    run_test \
        "--config /dev/null -f manual.txt -d proj -e go" \
        "expected_output.txt" \
        "expected_summary.txt"
}

# Test Case 6: Ambiguity - Multiple Positional Args
test_case_6_ambiguous_positional() {
    local test_name="ambiguous_positional"
    setup_test_case_base "$test_name" # Use base setup, no common files needed
    test_info "Running test case (expecting failure)..."
    test_info "Command: $CODECAT_CMD proj manual.txt"
    set +e
    "$CODECAT_CMD" proj manual.txt > "$STDOUT_LOG" 2> "$STDERR_LOG"
    local exit_code=$?
    set -e
    if [ "$exit_code" -eq 0 ]; then fail "Expected non-zero exit code, got 0"; fi
    grep -q 'Too many positional arguments' "$STDERR_LOG" || fail "Expected 'Too many positional arguments' message missing from stderr."
    grep -q 'Error: Expected at most one positional argument' "$STDERR_LOG" || fail "Expected 'Error: Expected at most one positional argument' message missing from stderr."
    [ ! -s "$STDOUT_LOG" ] || fail "Stdout not empty."
    pass "Detected expected failure."
    rm -f "$STDOUT_LOG" "$STDERR_LOG"
}

# Test Case 7: Ambiguity - Positional Arg + Flag
test_case_7_ambiguous_positional_flag() {
    local test_name="ambiguous_positional_flag"
    setup_test_case_base "$test_name" # Use base setup, no common files needed
    test_info "Running test case (expecting failure)..."
    test_info "Command: $CODECAT_CMD proj -e go"
    set +e
    "$CODECAT_CMD" proj -e go > "$STDOUT_LOG" 2> "$STDERR_LOG"
    local exit_code=$?
    set -e
    if [ "$exit_code" -eq 0 ]; then fail "Expected non-zero exit code, got 0"; fi
    grep -q 'Cannot specify a target directory via positional argument' "$STDERR_LOG" || fail "Expected positional/flag conflict message missing from stderr."
    grep -q -- '-d flag' "$STDERR_LOG" || fail "Reference to conflicting '-d' flag missing/incorrect."
    [ ! -s "$STDOUT_LOG" ] || fail "Stdout not empty."
    pass "Detected expected failure."
    rm -f "$STDOUT_LOG" "$STDERR_LOG"
}

# Test Case 8: Ambiguity - Positional Arg + Output Flag
test_case_8_ambiguous_positional_output() {
    local test_name="ambiguous_positional_output"
    setup_test_case_base "$test_name" # Use base setup, no common files needed
    test_info "Running test case (expecting failure)..."
    test_info "Command: $CODECAT_CMD proj -o out.txt"
    set +e
    "$CODECAT_CMD" proj -o out.txt > "$STDOUT_LOG" 2> "$STDERR_LOG"
    local exit_code=$?
    set -e
    if [ "$exit_code" -eq 0 ]; then fail "Expected non-zero exit code, got 0"; fi
    grep -q 'Cannot specify a target directory via positional argument' "$STDERR_LOG" || fail "Expected positional/flag conflict message missing from stderr."
    grep -q -- '-d flag' "$STDERR_LOG" || fail "Reference to conflicting '-d' flag missing/incorrect."
    [ ! -s "$STDOUT_LOG" ] || fail "Stdout not empty."
    [ ! -f out.txt ] || fail "Output file 'out.txt' was incorrectly created."
    pass "Detected expected failure."
    rm -f "$STDOUT_LOG" "$STDERR_LOG"
}

# Test Case 9: Non-existent directory
test_case_9_non_existent_dir() {
    local test_name="non_existent_dir"
    setup_test_case_base "$test_name" # Use base setup, no common files needed
    test_info "Running test case (expecting failure)..."
    test_info "Command: $CODECAT_CMD -d no_such_dir"
    set +e
    "$CODECAT_CMD" -d no_such_dir > "$STDOUT_LOG" 2> "$STDERR_LOG"
    local exit_code=$?
    set -e
    if [ "$exit_code" -eq 0 ]; then fail "Expected non-zero exit code, got 0"; fi
    grep -q 'Target scan directory does not exist' "$STDERR_LOG" || \
    grep -q 'no_such_dir\/' "$STDERR_LOG" && grep -q 'Errors encountered (1):' "$STDERR_LOG" || \
    fail "Expected 'Target scan directory does not exist' message or summary error missing from stderr."
    grep -q '--- Summary ---' "$STDERR_LOG" || fail "Summary block missing from stderr despite directory error."
    [ ! -s "$STDOUT_LOG" ] || fail "Stdout not empty."
    pass "Detected expected failure."
    rm -f "$STDOUT_LOG" "$STDERR_LOG"
}

# Test Case 10: Custom Config File
test_case_10_custom_config() {
    local test_name="custom_config"
    setup_test_case_base "$test_name" # Use base setup
    setup_common_files # Create the common files needed for this test

    local test_config_file="test_config.toml"
    cat << EOF > "$test_config_file"
header_text = "Test Config Header\n"
include_extensions = ["py", "json"]
exclude_basenames = ["main.go"]
use_gitignore = false
comment_marker = "###"
EOF
    test_info "Created custom config: $test_config_file"

    cat << EOF > expected_output.txt
Test Config Header

### proj/config.json
{"key": "value"}
###
### proj/utils.py
print("util") # utils.py content
###
EOF
    cat << EOF > expected_summary.txt
Included 2 files (50 B total) relative to CWD 'PLACEHOLDER_CWD':
└── proj
    ├── config.json (17 B)
    └── utils.py (33 B)
Empty files found (0):
Errors encountered (0):
EOF
    run_test \
        "-d proj --config $test_config_file" \
        "expected_output.txt" \
        "expected_summary.txt"
}


# --- Main Execution ---

build_binary

# Run tests - script will exit via fail() if any test fails
test_case_1_basename_exclusion
test_case_2_flags_exts_defaults
test_case_3_flags_output_defaults
test_case_4_flags_no_gitignore_defaults
test_case_5_flags_manual_defaults
test_case_6_ambiguous_positional
test_case_7_ambiguous_positional_flag
test_case_8_ambiguous_positional_output
test_case_9_non_existent_dir
test_case_10_custom_config


# If execution reaches here, all tests passed
# Final summary message is now handled in the cleanup function upon success
exit 0