#!/bin/bash

# Simple Integration Test Script for CodeCat

set -e

# --- Configuration ---
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
BUILD_OUTPUT_NAME="codecat_test_binary"
BUILD_OUTPUT_PATH="${SCRIPT_DIR}/${BUILD_OUTPUT_NAME}"
CODECAT_CMD="${BUILD_OUTPUT_PATH}"
TEST_DIR_BASE="codecat_test_env_"
VERBOSE=true

# --- Helper Functions ---
# (print_msg, info, pass, fail, warn - unchanged)
print_msg() {
    local color_code="$1"; local message="$2"; local reset_code="\033[0m"
    echo -e "${color_code}${message}${reset_code}"
}
info() { print_msg "\033[0;36m" "INFO: $1"; }
pass() { print_msg "\033[0;32m" "PASS: $1"; }
fail() { print_msg "\033[0;31m" "FAIL: $1"; }
warn() { print_msg "\033[0;33m" "WARN: $1"; }


setup_test_case() {
    local test_name="$1"
    TEST_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${TEST_DIR_BASE}${test_name}_XXXXXX")
    if [ -z "$TEST_DIR" ]; then fail "Could not create temp dir"; exit 1; fi
    info "Created test directory: $TEST_DIR"
    ORIGINAL_PWD=$(pwd)
    cd "$TEST_DIR" || exit 1
}

cleanup() {
    local exit_code=$?
    if [ -n "$ORIGINAL_PWD" ] && [ -d "$ORIGINAL_PWD" ]; then
        cd "$ORIGINAL_PWD" || echo "[Cleanup Warn] Failed cd back" >&2
    else
         echo "[Cleanup Warn] Original PWD not set/invalid." >&2
    fi
    if [ -d "$TEST_DIR" ]; then
        info "Cleaning up test directory: $TEST_DIR"; rm -rf "$TEST_DIR"
    fi
    if [ -f "$BUILD_OUTPUT_PATH" ]; then
         info "Cleaning up test binary: $BUILD_OUTPUT_PATH"; rm -f "$BUILD_OUTPUT_PATH"
    fi
    if [ "$exit_code" -ne 0 ]; then exit "$exit_code"; fi
}

extract_output_filename() {
    local args_string="$1"; local after_o="${args_string#*-o }"; local filename="${after_o%% *}"
    if [[ "$filename" == "$after_o" ]]; then :; fi
    if [[ "$filename" == "$args_string" ]] || [[ -z "$filename" ]]; then echo ""; else echo "$filename"; fi
}

build_binary() {
    info "Building test binary..."
    (cd "$SCRIPT_DIR" && \
        go build -ldflags="-X main.Version=test" -o "$BUILD_OUTPUT_NAME" ./cmd/codecat)
    if [ $? -ne 0 ]; then fail "Failed to build codecat binary."; exit 1; fi
    info "Test binary built successfully: $BUILD_OUTPUT_PATH"
}

run_test() {
    local test_name="$1"; local codecat_args="$2"; local expected_output_file="$3"; local expected_summary_file="$4"
    info "Running test: $test_name"
    info "Command: $CODECAT_CMD $codecat_args"

    local output_file=""; local summary_output_target="stderr"; local code_output_target="stdout"
    output_file=$(extract_output_filename "$codecat_args")
    if [ -n "$output_file" ]; then
        summary_output_target="stdout"; code_output_target="file"
        info "Expecting code in '$output_file', summary on stdout."
    else info "Expecting code on stdout, summary on stderr."; fi

    local actual_stdout_file="actual_stdout.log"; local actual_stderr_file="actual_stderr.log"
    set +e; "$CODECAT_CMD" $codecat_args > "$actual_stdout_file" 2> "$actual_stderr_file"; local exit_code=$?; set -e

    if [ "$exit_code" -ne 0 ]; then
        fail "Command failed with exit code $exit_code."; echo "--- Stdout ---"; cat "$actual_stdout_file"; echo "--- Stderr ---"; cat "$actual_stderr_file"; return 1
    else info "Command executed successfully."; fi

    local test_passed=true; local diff_output

    # Check Code Output
    info "Checking code output (target: $code_output_target)..."
    if [ "$code_output_target" == "file" ]; then
        if [ ! -f "$output_file" ]; then fail "Expected output file '$output_file' not created."; test_passed=false
        elif [ -n "$expected_output_file" ] && [ -f "$expected_output_file" ]; then
            if ! diff -u "$expected_output_file" "$output_file" > diff_code.log; then
                fail "Code output file '$output_file' mismatch."; [ "$VERBOSE" = true ] && cat diff_code.log; test_passed=false
            else info "Code output file '$output_file' matches expected."; fi; rm -f diff_code.log
        elif [ -z "$expected_output_file" ]; then
             if [ -s "$output_file" ]; then fail "Code output file '$output_file' not empty."; [ "$VERBOSE" = true ] && cat "$output_file"; test_passed=false
             else info "Code output file '$output_file' empty as expected."; fi
        else fail "Expected output ref '$expected_output_file' not found."; test_passed=false; fi
    elif [ "$code_output_target" == "stdout" ]; then
         if [ -n "$expected_output_file" ] && [ -f "$expected_output_file" ]; then
            if ! diff -u "$expected_output_file" "$actual_stdout_file" > diff_stdout.log; then
                fail "Code output (stdout) mismatch."; [ "$VERBOSE" = true ] && cat diff_stdout.log; test_passed=false
            else info "Code output (stdout) matches expected."; fi; rm -f diff_stdout.log
        elif [ -z "$expected_output_file" ]; then
             if [ -s "$actual_stdout_file" ]; then fail "Code output (stdout) not empty."; [ "$VERBOSE" = true ] && cat "$actual_stdout_file"; test_passed=false
             else info "Code output (stdout) empty as expected."; fi
        else fail "Expected output ref '$expected_output_file' not found."; test_passed=false; fi
    fi

    # Check Summary Output
    info "Checking summary output (target: $summary_output_target)..."
    local summary_source_file="$actual_stderr_file"; if [ "$summary_output_target" == "stdout" ]; then summary_source_file="$actual_stdout_file"; fi

    if [ -n "$expected_summary_file" ] && [ -f "$expected_summary_file" ]; then
        if [ -f "$summary_source_file" ]; then
             awk '/--- Summary ---/{summary_started=1; next} /---------------/{exit} summary_started && !/^Included .* relative to CWD/{print}' "$summary_source_file" > actual_summary_body.txt
        else warn "Summary source file '$summary_source_file' not found."; touch actual_summary_body.txt; fi
        tail -n +2 "$expected_summary_file" > expected_summary_body.txt # Skip placeholder header in expected

        if ! diff -u expected_summary_body.txt actual_summary_body.txt > diff_summary.log; then
            fail "Summary output body mismatch."; warn "Comparing summary content below CWD header line."
            [ "$VERBOSE" = true ] && cat diff_summary.log; test_passed=false
        else info "Summary output body matches expected."; fi
        rm -f diff_summary.log actual_summary_body.txt expected_summary_body.txt
    elif [ -z "$expected_summary_file" ]; then
         summary_content=$(awk '/--- Summary ---/{flag=1; next} /---------------/{flag=0} flag' "$summary_source_file" 2>/dev/null || true)
         if grep -q -E "Empty files found \(0\):|Errors encountered \(0\):" "$summary_source_file" && [ -z "$summary_content" ]; then
            info "Summary output ($summary_output_target) empty (only headers) as expected."
         else fail "Summary output ($summary_output_target) not empty."; [ "$VERBOSE" = true ] && cat "$summary_source_file"; test_passed=false; fi
    else fail "Expected summary ref '$expected_summary_file' not found."; test_passed=false; fi

    # --- Final Result ---
    if [ "$test_passed" = true ]; then pass "Test '$test_name' success."; return 0
    else fail "Test '$test_name' FAILED."; return 1; fi
}

# --- Test Definitions ---

test_case_1() {
    local test_name="basename_exclusion"
    setup_test_case "$test_name"

    # Create structure
    mkdir -p scantest/sample-docs
    echo "include me" > scantest/a.txt
    echo "exclude me" > scantest/sample-docs/b.txt
    echo "another exclude" > scantest/c.log

    # --- Create test-specific config ---
    local test_config_file="test_config.toml"
    cat << EOF > "$test_config_file"
# Test-specific config to ensure exclusions
include_extensions = ["txt", "log"]
exclude_basenames = ["sample-docs", "*.log"]
use_gitignore = false
EOF

    # --- Create expected output ---
    # FIX: Update the header and remove the extra blank line to match the default config
    cat << EOF > expected_output.txt
----- Codebase for analysis -----
--- scantest/a.txt
include me
---
EOF
    # --- Create expected summary ---
    # (No change needed for summary structure)
    cat << EOF > expected_summary.txt
Included 1 files (11 B total) relative to CWD 'PLACEHOLDER_CWD':
└── scantest
    └── a.txt (11 B)

Empty files found (0):

Errors encountered (0):
EOF

    # --- Run the test with the test-specific config ---
    run_test "$test_name" \
        "-d scantest -o output.txt --loglevel=warn -c $test_config_file" \
        "expected_output.txt" \
        "expected_summary.txt"

    return $?
}

# --- Main Execution ---
trap cleanup EXIT ERR

build_binary

# Run tests
test_case_1

info "All tests completed."
exit 0