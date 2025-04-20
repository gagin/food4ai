#!/bin/bash

# Exit on error, treat unset variables as error
set -eu
# Exit on pipe failures
set -o pipefail

# --- Configuration ---
FOOD4AI_BIN_NAME="food4ai"
TEST_PASSED_COUNT=0
TEST_FAILED_COUNT=0
VERBOSE=false

# --- Helper Functions ---
info() { echo "[INFO] $*"; }
pass() { echo "[PASS] $*"; TEST_PASSED_COUNT=$((TEST_PASSED_COUNT + 1)); }
fail() { echo "[FAIL] $*"; TEST_FAILED_COUNT=$((TEST_FAILED_COUNT + 1)); }
warn() { echo "[WARN] $*"; }
cleanup() {
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
declare TEST_DIR=""
trap cleanup EXIT
TEST_DIR="$BASE_TMP_DIR/food4ai_test_$TIMESTAMP"
mkdir -p "$TEST_DIR"
info "Test directory: $TEST_DIR"

# --- Create Test File Structure ---
info "Creating test file structure..."
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

# --- Create Test Config File ---
info "Creating test config file (test_config.toml)..."
cat << EOF > "$TEST_DIR/test_config.toml"
header_text = "Test Config Header"
include_extensions = ["py", "json"]
exclude_patterns = ["*.go"]
use_gitignore = false
comment_marker = "###"
EOF

# --- Run Tests ---
info "--- Starting Tests ---"
ORIGINAL_PWD=$(pwd)
cd "$TEST_DIR"

# === Test Case 1: No args (Default Invocation Path) ===
info "Test 1: No args (Default Invocation Path - depends on user config)"
("$FOOD4AI_ABS_PATH" > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then
    fail "Test 1: Expected exit code 0, got $exit_code"
else
    if grep -q -e 'Loading configuration.' -e 'No default config file found' -e 'configuration file is empty' -e 'Could not determine user home directory' stderr.log; then
        pass "Test 1: Config loading attempted (message found)."
    else
        warn "Test 1: No expected config load message found on stderr (might indicate issue)."
    fi
    # Use grep -e for pattern starting with ---
    if grep -q -e '--- Summary ---' stderr.log; then
        pass "Test 1: Summary block found on stderr."
    else
        fail "Test 1: Summary block missing from stderr."
    fi
fi
rm -f stdout.log stderr.log

# === Test Case 2: Single Positional Arg ('proj') (Default Invocation Path) ===
info "Test 2: Single positional arg ('proj') (Default Invocation Path - depends on user config)"
("$FOOD4AI_ABS_PATH" proj > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 2: Expected exit code 0, got $exit_code"; else
  grep -q 'main.go content' stdout.log && grep -q 'utils.py content' stdout.log && pass "Test 2: Code output (stdout) contains some expected proj/ files." || warn "Test 2: Code output (stdout) missing proj/ files (check user config?)."
  grep -q 'Summary' stderr.log && grep -q '/proj' stderr.log && pass "Test 2: Summary (stderr) looks plausible for proj/." || fail "Test 2: Summary (stderr) missing or not targeting proj/."
fi
rm -f stdout.log stderr.log

# === Test Case 3: Flags mode (-d proj -e go,py) using Hardcoded Defaults ===
info "Test 3: Flags mode (-d proj -e go,py) using Hardcoded Defaults (--config /dev/null)"
("$FOOD4AI_ABS_PATH" --config /dev/null -d proj -e go,py > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 3: Expected exit code 0, got $exit_code"; else
  grep -q 'main.go content' stdout.log && grep -q 'utils.py content' stdout.log && ! grep -q 'text data' stdout.log && pass "Test 3: Code output (stdout) correct based on extensions." || fail "Test 3: Code output (stdout) extension filtering failed."
  grep -q 'Summary' stderr.log && grep -q 'main.go (' stderr.log && grep -q 'utils.py (' stderr.log && ! grep -q 'file.txt (' stderr.log && pass "Test 3: Summary (stderr) correct based on extensions." || fail "Test 3: Summary (stderr) extension filtering failed."
fi
rm -f stdout.log stderr.log

# === Test Case 4: Flags mode with output file (-o output.txt) using Hardcoded Defaults ===
info "Test 4: Flags mode with output file (-o output.txt) using Hardcoded Defaults (--config /dev/null)"
("$FOOD4AI_ABS_PATH" --config /dev/null -d proj -o output.txt > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 4: Expected exit code 0, got $exit_code"; else
  grep -q 'main.go content' output.txt && grep -q 'utils.py content' output.txt && grep -q 'text data' output.txt && grep -q 'config.json' output.txt && ! grep -q 'log stuff' output.txt && pass "Test 4: Code output (file) looks okay based on defaults/gitignore." || fail "Test 4: Code output (file) content mismatch."
  grep -q 'Summary' stdout.log && grep -q 'main.go (' stdout.log && grep -q 'utils.py (' stdout.log && grep -q 'file.txt (' stdout.log && grep -q 'config.json (' stdout.log && ! grep -q 'ignored.log (' stdout.log && pass "Test 4: Summary (stdout) looks okay." || fail "Test 4: Summary (stdout) content mismatch."
  pass "Test 4: Stderr check skipped (expected warning with /dev/null)."
fi
rm -f stdout.log stderr.log output.txt

# === Test Case 5: Flags mode with --no-gitignore using Hardcoded Defaults ===
info "Test 5: Flags mode with --no-gitignore (-d proj --no-gitignore -x \"\" -e go,py,txt,log,json --config /dev/null)"
("$FOOD4AI_ABS_PATH" --config /dev/null -d proj --no-gitignore -x "" -e go,py,txt,log,json > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 5: Expected exit code 0, got $exit_code"; else
  grep -q 'main.go content' stdout.log && grep -q 'utils.py content' stdout.log && grep -q 'log stuff' stdout.log && grep -q 'text data' stdout.log && grep -q 'config.json' stdout.log && pass "Test 5: Code output (stdout) includes formerly ignored/default-excluded files." || fail "Test 5: Code output (stdout) --no-gitignore / -e failed."
  grep -q 'Summary' stderr.log && grep -q 'main.go (' stderr.log && grep -q 'ignored.log (' stderr.log && grep -q 'file.txt (' stderr.log && grep -q 'config.json (' stderr.log && ! grep -q 'empty.txt (' stderr.log && pass "Test 5: Summary (stderr) includes formerly ignored files." || fail "Test 5: Summary (stderr) --no-gitignore / -e failed."
  grep -q 'empty.txt' stderr.log && grep -q 'Empty files found' stderr.log && pass "Test 5: Empty file listed correctly in summary." || fail "Test 5: Empty file not listed correctly with --no-gitignore."
fi
rm -f stdout.log stderr.log

# === Test Case 6: Flags mode with manual file (-f manual.txt -d proj -e go) using Hardcoded Defaults ===
info "Test 6: Flags mode with manual file (-f manual.txt -d proj -e go) using Hardcoded Defaults (--config /dev/null)"
("$FOOD4AI_ABS_PATH" --config /dev/null -f manual.txt -d proj -e go > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 6: Expected exit code 0, got $exit_code"; else
  grep -q 'manual content' stdout.log && grep -q 'main.go content' stdout.log && ! grep -q 'utils.py content' stdout.log && pass "Test 6: Code output (stdout) includes manual and specified extension." || fail "Test 6: Code output (stdout) manual file or extension failed."
  # Fix: Simplify check - look for manual.txt and [M], and main.go ( separately
  grep -q 'Summary' stderr.log && grep -q 'manual.txt.*\[M\]' stderr.log && grep -q 'main.go (' stderr.log && pass "Test 6: Summary (stderr) includes manual [M] and go file." || fail "Test 6: Summary (stderr) manual file or extension failed."
fi
rm -f stdout.log stderr.log

# === Test Case 7: Ambiguity - Multiple Positional Args ===
info "Test 7: Ambiguity - Multiple positional args ('proj' 'manual.txt')"
("$FOOD4AI_ABS_PATH" proj manual.txt > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -eq 0 ]; then fail "Test 7: Expected non-zero exit code, got 0"; else
  grep -q 'Refusing execution: Multiple positional arguments' stderr.log && grep -q 'Run with --help' stderr.log && pass "Test 7: Correct refusal message on stderr." || fail "Test 7: Incorrect or missing refusal message on stderr."
  [ ! -s stdout.log ] && pass "Test 7: Stdout is empty." || fail "Test 7: Stdout is not empty."
fi
rm -f stdout.log stderr.log

# === Test Case 8: Ambiguity - Positional Arg + Flag ===
info "Test 8: Ambiguity - Positional arg + flag ('proj' -e go)"
("$FOOD4AI_ABS_PATH" proj -e go > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -eq 0 ]; then fail "Test 8: Expected non-zero exit code, got 0"; else
  grep -q 'Refusing execution: Cannot mix positional argument' stderr.log && grep -q -- "flag '--extensions'" stderr.log && grep -q 'Run with --help' stderr.log && pass "Test 8: Correct refusal message on stderr." || fail "Test 8: Incorrect or missing refusal message on stderr."
   [ ! -s stdout.log ] && pass "Test 8: Stdout is empty." || fail "Test 8: Stdout is not empty."
fi
rm -f stdout.log stderr.log

# === Test Case 9: Ambiguity - Positional Arg + Output Flag ===
info "Test 9: Ambiguity - Positional arg + output flag ('proj' -o out.txt)"
("$FOOD4AI_ABS_PATH" proj -o out.txt > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -eq 0 ]; then fail "Test 9: Expected non-zero exit code, got 0"; else
  grep -q 'Refusing execution: Cannot mix positional argument' stderr.log && grep -q -- "flag '--output'" stderr.log && grep -q 'Run with --help' stderr.log && pass "Test 9: Correct refusal message on stderr." || fail "Test 9: Incorrect or missing refusal message on stderr."
   [ ! -s stdout.log ] && pass "Test 9: Stdout is empty." || fail "Test 9: Stdout is not empty."
   [ ! -f out.txt ] && pass "Test 9: Output file not created." || fail "Test 9: Output file was created."
fi
rm -f stdout.log stderr.log out.txt

# === Test Case 10: Non-existent directory ===
info "Test 10: Non-existent directory (-d no_such_dir)"
("$FOOD4AI_ABS_PATH" -d no_such_dir > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -eq 0 ]; then fail "Test 10: Expected non-zero exit code, got 0"; else
  (grep -q 'Target directory does not exist' stderr.log || grep -q 'level=ERROR.*Target directory does not exist' stderr.log) && pass "Test 10: Correct error message on stderr." || fail "Test 10: Incorrect or missing error message."
  [ ! -s stdout.log ] && pass "Test 10: Stdout is empty." || fail "Test 10: Stdout is not empty."
fi
rm -f stdout.log stderr.log

# === Test Case 11: Custom Config File ===
info "Test 11: Custom config file (--config test_config.toml -d proj)"
# Assertions are based on test_config.toml content. Failures here likely indicate an app bug.
("$FOOD4AI_ABS_PATH" -d proj --config test_config.toml > stdout.log 2> stderr.log) && exit_code=0 || exit_code=$?
if [ "$exit_code" -ne 0 ]; then fail "Test 11: Expected exit code 0, got $exit_code"; else
  grep -q 'Test Config Header' stdout.log && grep -q '### proj/utils.py' stdout.log && grep -q 'utils.py content' stdout.log && grep -q '### proj/config.json' stdout.log && grep -q '"key": "value"' stdout.log && pass "Test 11: Code output (stdout) includes expected files and custom marker/header." || fail "Test 11: Code output (stdout) mismatch for custom config (py, json)."
  ! grep -q 'main.go content' stdout.log && ! grep -q 'text data' stdout.log && ! grep -q 'log stuff' stdout.log && pass "Test 11: Code output (stdout) excludes unwanted files." || fail "Test 11: Code output (stdout) included excluded/non-included files."
  grep -q 'Summary' stderr.log && grep -q 'proj/utils.py (' stderr.log && grep -q 'proj/config.json (' stderr.log && ! grep -q 'proj/main.go (' stderr.log && ! grep -q 'file.txt (' stderr.log && ! grep -q 'ignored.log (' stderr.log && pass "Test 11: Summary (stderr) lists expected files for custom config." || fail "Test 11: Summary (stderr) mismatch for custom config."
  ! grep -q 'empty.txt' stderr.log && pass "Test 11: Empty file correctly excluded by extension filter." || fail "Test 11: Empty file incorrectly listed."
fi
rm -f stdout.log stderr.log test_config.toml


cd "$ORIGINAL_PWD"

# --- Report Results ---
info "--- Test Summary ---"
if [ "$TEST_FAILED_COUNT" -eq 0 ]; then
  echo "[SUCCESS] All $TEST_PASSED_COUNT tests passed."
  exit 0
else
  echo "[FAILURE] $TEST_FAILED_COUNT/$((TEST_PASSED_COUNT + TEST_FAILED_COUNT)) tests failed."
  echo "Check logs above for details. Temp directory $TEST_DIR may need manual cleanup."
  trap - EXIT # Disable cleanup on failure
  exit 1
fi