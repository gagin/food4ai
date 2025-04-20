#!/bin/bash

# Exit on error, treat unset variables as error
set -eu
# Exit on pipe failures
set -o pipefail

# --- Configuration ---
FOOD4AI_BIN_NAME="food4ai"
TEST_PASSED_COUNT=0
# TEST_FAILED_COUNT no longer needed
VERBOSE=false

# --- Helper Functions ---
info() { echo "[INFO] $*"; }
pass() { echo "[PASS] $*"; TEST_PASSED_COUNT=$((TEST_PASSED_COUNT + 1)); }
fail() {
  echo "[FAIL] $*" >&2 # Print errors to stderr
  # Optionally dump logs that might exist
  if [ -f stdout.log ]; then echo "--- stdout.log ---"; cat stdout.log; echo "--- end stdout.log ---"; fi
  if [ -f stderr.log ]; then echo "--- stderr.log ---"; cat stderr.log; echo "--- end stderr.log ---"; fi
  echo "[INFO] Preserving temporary directory for inspection: $TEST_DIR" >&2
  trap - EXIT # Disable cleanup trap
  exit 1      # Exit immediately
}
cleanup() {
  # Only cleanup if TEST_DIR is set and directory exists
  if [ -n "${TEST_DIR:-}" ] && [ -d "$TEST_DIR" ]; then
      info "Cleaning up temp directory: $TEST_DIR"
      rm -rf "$TEST_DIR"
  else
      info "Skipping cleanup (TEST_DIR not set or doesn't exist)."
  fi
}

# --- Argument Parsing ---
if [[ "${1:-}" == "-v" ]]; then
  VERBOSE=true
  info "Verbose mode enabled."
fi

# --- Setup ---
info "Checking for food4ai binary './${FOOD4AI_BIN_NAME}' in current directory (`pwd`)..."
if [ ! -x "./${FOOD4AI_BIN_NAME}" ]; then
  echo "[ERROR] ${FOOD4AI_BIN_NAME} binary not found or not executable in the current directory (`pwd`)." >&2
  echo "Please compile it first (e.g., go build -o ${FOOD4AI_BIN_NAME} .) and run this script from the same directory." >&2
  exit 1
fi
FOOD4AI_ABS_PATH="$(pwd)/${FOOD4AI_BIN_NAME}"
info "Absolute path to binary determined as: $FOOD4AI_ABS_PATH"

info "Creating timestamped temporary directory..."
BASE_TMP_DIR=$(mktemp -d -t food4ai_test_base_XXXXXX)
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
declare TEST_DIR="" # Declare variable first
trap cleanup EXIT   # Set trap *before* assigning directory path
TEST_DIR="$BASE_TMP_DIR/food4ai_test_$TIMESTAMP"
mkdir -p "$TEST_DIR"
info "Test directory: $TEST_DIR"

TEST_CONFIG_ABS_PATH="$TEST_DIR/test_config.toml"

info "Creating test file structure..."
mkdir -p "$TEST_DIR/proj"
mkdir -p "$TEST_DIR/proj/data"
echo 'package main // main.go content' > "$TEST_DIR/proj/main.go"
echo 'print("util") # utils.py content' > "$TEST_DIR/proj/utils.py"
echo 'text data' > "$TEST_DIR/proj/data/file.txt"
echo 'log stuff' > "$TEST_DIR/proj/data/ignored.log"
touch "$TEST_DIR/proj/empty.txt"
echo '*.log' > "$TEST_DIR/proj/.gitignore"
echo 'empty.txt' >> "$TEST_DIR/proj/.gitignore"
echo 'manual content' > "$TEST_DIR/manual.txt"
echo '{"key": "value"}' > "$TEST_DIR/proj/config.json"

info "Creating test config file ($TEST_CONFIG_ABS_PATH)..."
cat << EOF > "$TEST_CONFIG_ABS_PATH"
header_text = "Test Config Header"
include_extensions = ["py", "json"]
exclude_patterns = ["*.go"]
use_gitignore = false
comment_marker = "###"
EOF

info "Verifying test config file creation..."
ls -l "$TEST_CONFIG_ABS_PATH"
if [ ! -s "$TEST_CONFIG_ABS_PATH" ]; then
    fail "Test config file was created empty or verification failed: $TEST_CONFIG_ABS_PATH" # fail will exit
fi
info "Test config file verified (exists and is not empty)."

# --- Run Tests ---
info "--- Starting Tests ---"
ORIGINAL_PWD=$(pwd)
cd "$TEST_DIR" # Run tests from within the temp dir

# === Test Case 1: No args (Default Invocation Path) ===
info "Test 1: No args (Default Invocation Path - depends on user config)"
("$FOOD4AI_ABS_PATH" > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 1: Expected exit code 0, got $exit_code"; fi
# Relaxed checks: just check for config attempt and summary presence
grep -q -e 'Loading configuration.' -e 'No default config file found' -e 'configuration file is empty' -e 'Could not determine user home directory' stderr.log || fail "Test 1: No config load message found on stderr."
grep -q -e '--- Summary ---' stderr.log || fail "Test 1: Summary block missing from stderr."
pass "Test 1: Ran successfully (basic checks passed)."
# No rm needed

# === Test Case 2: Single Positional Arg ('proj') (Default Invocation Path) ===
info "Test 2: Single positional arg ('proj') (Default Invocation Path - depends on user config)"
("$FOOD4AI_ABS_PATH" proj > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 2: Expected exit code 0, got $exit_code"; fi
# Relaxed checks: check summary seems plausible
grep -q 'Summary' stderr.log || fail "Test 2: Summary block missing from stderr."
grep -q '/proj' stderr.log || fail "Test 2: Summary does not seem to target proj/."
# Warn if key content missing, but don't fail the test
grep -q 'main.go content' stdout.log || warn "Test 2: main.go content missing (check user config?)."
grep -q 'utils.py content' stdout.log || warn "Test 2: utils.py content missing (check user config?)."
pass "Test 2: Ran successfully (basic checks passed)."
# No rm needed

# === Test Case 3: Flags mode (-d proj -e go,py) using Hardcoded Defaults ===
info "Test 3: Flags mode (-d proj -e go,py) using Hardcoded Defaults (--config /dev/null)"
("$FOOD4AI_ABS_PATH" --config /dev/null -d proj -e go,py > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 3: Expected exit code 0, got $exit_code"; fi
grep -q 'main.go content' stdout.log || fail "Test 3: main.go content missing."
grep -q 'utils.py content' stdout.log || fail "Test 3: utils.py content missing."
! grep -q 'text data' stdout.log || fail "Test 3: file.txt content unexpectedly present."
grep -q 'Summary' stderr.log || fail "Test 3: Summary missing."
grep -q 'main.go (' stderr.log || fail "Test 3: main.go missing from summary."
grep -q 'utils.py (' stderr.log || fail "Test 3: utils.py missing from summary."
! grep -q 'file.txt (' stderr.log || fail "Test 3: file.txt unexpectedly in summary."
pass "Test 3: Passed."
# No rm needed

# === Test Case 4: Flags mode with output file (-o output.txt) using Hardcoded Defaults ===
info "Test 4: Flags mode with output file (-o output.txt) using Hardcoded Defaults (--config /dev/null)"
("$FOOD4AI_ABS_PATH" --config /dev/null -d proj -o output.txt > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 4: Expected exit code 0, got $exit_code"; fi
grep -q 'main.go content' output.txt || fail "Test 4: main.go content missing from output.txt."
grep -q 'utils.py content' output.txt || fail "Test 4: utils.py content missing from output.txt."
grep -q 'text data' output.txt || fail "Test 4: file.txt content missing from output.txt."
grep -q 'config.json' output.txt || fail "Test 4: config.json content missing from output.txt."
! grep -q 'log stuff' output.txt || fail "Test 4: ignored.log content unexpectedly in output.txt."
grep -q 'Summary' stdout.log || fail "Test 4: Summary missing from stdout."
grep -q 'main.go (' stdout.log || fail "Test 4: main.go missing from summary."
grep -q 'utils.py (' stdout.log || fail "Test 4: utils.py missing from summary."
grep -q 'file.txt (' stdout.log || fail "Test 4: file.txt missing from summary."
grep -q 'config.json (' stdout.log || fail "Test 4: config.json missing from summary."
! grep -q 'ignored.log (' stdout.log || fail "Test 4: ignored.log unexpectedly in summary."
# Skip strict stderr check due to expected /dev/null warning
pass "Test 4: Passed."
# No rm needed (output.txt will remain on failure)

# === Test Case 5: Flags mode with --no-gitignore using Hardcoded Defaults ===
info "Test 5: Flags mode with --no-gitignore (-d proj --no-gitignore -x \"\" -e go,py,txt,log,json --config /dev/null)"
("$FOOD4AI_ABS_PATH" --config /dev/null -d proj --no-gitignore -x "" -e go,py,txt,log,json > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 5: Expected exit code 0, got $exit_code"; fi
grep -q 'main.go content' stdout.log || fail "Test 5: main.go content missing."
grep -q 'utils.py content' stdout.log || fail "Test 5: utils.py content missing."
grep -q 'log stuff' stdout.log || fail "Test 5: ignored.log content missing (should be included)."
grep -q 'text data' stdout.log || fail "Test 5: file.txt content missing."
grep -q 'config.json' stdout.log || fail "Test 5: config.json content missing."
grep -q 'Summary' stderr.log || fail "Test 5: Summary missing."
grep -q 'main.go (' stderr.log || fail "Test 5: main.go missing from summary."
grep -q 'ignored.log (' stderr.log || fail "Test 5: ignored.log missing from summary (should be included)."
grep -q 'file.txt (' stderr.log || fail "Test 5: file.txt missing from summary."
grep -q 'config.json (' stderr.log || fail "Test 5: config.json missing from summary."
! grep -q 'empty.txt (' stderr.log || fail "Test 5: empty.txt incorrectly in main summary list."
grep -q 'empty.txt' stderr.log && grep -q 'Empty files found' stderr.log || fail "Test 5: empty.txt not listed in 'Empty files found'."
pass "Test 5: Passed."
# No rm needed

# === Test Case 6: Flags mode with manual file (-f manual.txt -d proj -e go) using Hardcoded Defaults ===
info "Test 6: Flags mode with manual file (-f manual.txt -d proj -e go) using Hardcoded Defaults (--config /dev/null)"
("$FOOD4AI_ABS_PATH" --config /dev/null -f manual.txt -d proj -e go > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 6: Expected exit code 0, got $exit_code"; fi
grep -q 'manual content' stdout.log || fail "Test 6: manual.txt content missing."
grep -q 'main.go content' stdout.log || fail "Test 6: main.go content missing."
! grep -q 'utils.py content' stdout.log || fail "Test 6: utils.py content unexpectedly present."
grep -q 'Summary' stderr.log || fail "Test 6: Summary missing."
grep -q 'manual.txt.*\[M\]' stderr.log || fail "Test 6: manual.txt with [M] marker missing from summary."
grep -q 'main.go (' stderr.log || fail "Test 6: main.go missing from summary."
pass "Test 6: Passed."
# No rm needed

# === Test Case 7: Ambiguity - Multiple Positional Args ===
info "Test 7: Ambiguity - Multiple positional args ('proj' 'manual.txt')"
# No --config needed
("$FOOD4AI_ABS_PATH" proj manual.txt > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -eq 0 ]; then fail "Test 7: Expected non-zero exit code, got 0"; fi
grep -q 'Refusing execution: Multiple positional arguments' stderr.log || fail "Test 7: Refusal message missing or incorrect."
grep -q 'Run with --help' stderr.log || fail "Test 7: Help hint missing."
[ ! -s stdout.log ] || fail "Test 7: Stdout not empty."
pass "Test 7: Passed."
# No rm needed

# === Test Case 8: Ambiguity - Positional Arg + Flag ===
info "Test 8: Ambiguity - Positional arg + flag ('proj' -e go)"
# No --config needed
("$FOOD4AI_ABS_PATH" proj -e go > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -eq 0 ]; then fail "Test 8: Expected non-zero exit code, got 0"; fi
grep -q 'Refusing execution: Cannot mix positional argument' stderr.log || fail "Test 8: Refusal message missing or incorrect."
grep -q -- "flag '--extensions'" stderr.log || fail "Test 8: Conflicting flag name missing/incorrect."
grep -q 'Run with --help' stderr.log || fail "Test 8: Help hint missing."
[ ! -s stdout.log ] || fail "Test 8: Stdout not empty."
pass "Test 8: Passed."
# No rm needed

# === Test Case 9: Ambiguity - Positional Arg + Output Flag ===
info "Test 9: Ambiguity - Positional arg + output flag ('proj' -o out.txt)"
# No --config needed
("$FOOD4AI_ABS_PATH" proj -o out.txt > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -eq 0 ]; then fail "Test 9: Expected non-zero exit code, got 0"; fi
grep -q 'Refusing execution: Cannot mix positional argument' stderr.log || fail "Test 9: Refusal message missing or incorrect."
grep -q -- "flag '--output'" stderr.log || fail "Test 9: Conflicting flag name missing/incorrect."
grep -q 'Run with --help' stderr.log || fail "Test 9: Help hint missing."
[ ! -s stdout.log ] || fail "Test 9: Stdout not empty."
[ ! -f out.txt ] || fail "Test 9: Output file was created."
pass "Test 9: Passed."
# No rm needed (out.txt shouldn't exist)

# === Test Case 10: Non-existent directory ===
info "Test 10: Non-existent directory (-d no_such_dir)"
# No --config needed
("$FOOD4AI_ABS_PATH" -d no_such_dir > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -eq 0 ]; then fail "Test 10: Expected non-zero exit code, got 0"; fi
grep -q 'Target directory does not exist' stderr.log || grep -q 'level=ERROR.*Target directory does not exist' stderr.log || fail "Test 10: Error message missing or incorrect."
[ ! -s stdout.log ] || fail "Test 10: Stdout not empty."
pass "Test 10: Passed."
# No rm needed

# === Test Case 11: Custom Config File ===
info "Test 11: Custom config file (--config $TEST_CONFIG_ABS_PATH -d proj)"
# Assertions are based on test_config.toml content. Failures here likely indicate an app bug.
("$FOOD4AI_ABS_PATH" -d proj --config "$TEST_CONFIG_ABS_PATH" > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 11: Expected exit code 0, got $exit_code"; fi
# Check includes
grep -q 'Test Config Header' stdout.log || fail "Test 11: Missing custom header."
grep -q '### utils.py' stdout.log || fail "Test 11: Python marker/path missing."
grep -q 'utils.py content' stdout.log || fail "Test 11: Python content missing."
grep -q '### config.json' stdout.log || fail "Test 11: JSON marker/path missing."
grep -q '"key": "value"' stdout.log || fail "Test 11: JSON content missing."
# Check excludes
! grep -q 'main.go content' stdout.log || fail "Test 11: main.go unexpectedly present."
! grep -q 'text data' stdout.log || fail "Test 11: file.txt unexpectedly present."
! grep -q 'log stuff' stdout.log || fail "Test 11: ignored.log unexpectedly present."
# Check summary includes
grep -q 'Summary' stderr.log || fail "Test 11: Summary missing."
grep -q 'utils.py (' stderr.log || fail "Test 11: utils.py missing from summary."
grep -q 'config.json (' stderr.log || fail "Test 11: config.json missing from summary."
# Check summary excludes
! grep -q 'main.go (' stderr.log || fail "Test 11: main.go unexpectedly in summary."
! grep -q 'file.txt (' stderr.log || fail "Test 11: file.txt unexpectedly in summary."
! grep -q 'ignored.log (' stderr.log || fail "Test 11: ignored.log unexpectedly in summary."
# Check empty list exclude
! grep -q 'empty.txt' stderr.log || fail "Test 11: empty.txt incorrectly listed."
# If all checks passed:
pass "Test 11: Passed."
# No individual rm needed here - main cleanup trap handles success


# Change back to the original directory AFTER all tests (or after failure exit)
cd "$ORIGINAL_PWD"

# --- Report Results ---
# This section is only reached if NO test called fail()
info "--- Test Summary ---"
echo "[SUCCESS] All $TEST_PASSED_COUNT tests passed."
# The main 'cleanup' function runs automatically via trap on successful exit 0
exit 0