#!/bin/bash

# Test script for the Go version incrementer utility.
# Exits on first failure and only cleans up temporary files on full success.

# --- Configuration ---
# Path to the Go script relative to the repo root (where this test script is run)
GO_SCRIPT_REL="./dev_process_utils/increment_version.go"
# Temporary directory for test files
TEST_DIR=$(mktemp -d -t version_test_XXXXXX)
# Get absolute path for robust execution
GO_SCRIPT_ABS=$(realpath "$GO_SCRIPT_REL")
# Path for the compiled binary
GO_BIN="$TEST_DIR/increment_version_bin"

# Colors for output (optional)
COLOR_GREEN='\033[0;32m'
COLOR_RED='\033[0;31m'
COLOR_YELLOW='\033[0;33m'
COLOR_RESET='\033[0m'

# --- State ---
tests_run=0
# tests_passed=0 # No longer needed within run_test due to exit-on-fail

# --- Helper Functions ---

# Function to clean up the temporary directory (only called on success)
cleanup() {
  echo -e "${COLOR_YELLOW}Cleaning up temporary directory: $TEST_DIR${COLOR_RESET}"
  rm -rf "$TEST_DIR"
}
# NO trap - cleanup is conditional at the end

# Function to run a test and exit on failure
# Usage: run_test TEST_NUMBER "Test Description" command_to_run expected_exit_code [check_file_content_grep_pattern] [check_stderr_grep_pattern]
run_test() {
  # Arguments shifted due to adding test_num
  local test_num="$1"
  local description="$2"
  local command="$3" # Command now directly uses the compiled binary path $GO_BIN
  local expected_exit_code="$4"
  local file_grep_pattern="${5:-}" # Optional: pattern to grep for in the modified file
  local stderr_grep_pattern="${6:-}" # Optional: pattern to grep for in stderr output

  # Test number is now passed in, counter incremented before call
  echo -e "\n--- Running Test $test_num: $description ---"
  echo "Command: $command"

  # Execute the command, capturing stdout, stderr, and exit code
  local combined_output
  # Use eval carefully; ensure paths in command are quoted if they might contain spaces
  combined_output=$(eval "$command" 2>&1)
  local actual_exit_code=$?
  echo "Script Output:"
  echo "$combined_output" | sed 's/^/  /' # Indent output for clarity

  local test_passed_flag=true

  # 1. Check Exit Code
  if [ "$actual_exit_code" -ne "$expected_exit_code" ]; then
    echo -e "${COLOR_RED}FAIL:${COLOR_RESET} Expected exit code $expected_exit_code, but got $actual_exit_code."
    test_passed_flag=false
  else
    echo -e "${COLOR_GREEN}PASS:${COLOR_RESET} Correct exit code ($actual_exit_code)."
  fi

  # 2. Check File Content (if pattern provided)
  # *** Start of Revised File Check Logic ***
  if [ -n "$file_grep_pattern" ]; then
    local target_file_to_check=""
    # Determine the target file based on the command structure
    local last_arg
    local last_arg_clean
    # Get the last word/argument from the command string
    last_arg=$(echo "$command" | awk '{print $NF}')
    # Remove leading/trailing quotes that might be included due to eval/command structure
    last_arg_clean=$(echo "$last_arg" | sed 's/^"//;s/"$//')

    # Check if the cleaned last argument looks like a .go file path within our TEST_DIR
    # This indicates a specific file was passed to the command
    if [[ "$last_arg_clean" == *.go ]] && [[ "$last_arg_clean" == "$TEST_DIR"* ]]; then
        target_file_to_check="$last_arg_clean"
        # echo "DEBUG: Checking specified file: $target_file_to_check"
    # Check if the cleaned last argument doesn't look like a .go file path.
    # This implies the default ./main.go (within TEST_DIR context) should be checked.
    # This handles the case where the command is just the binary path.
    elif [[ "$last_arg_clean" != *.go ]]; then
         target_file_to_check="$TEST_DIR/main.go"
         # echo "DEBUG: Checking default file: $target_file_to_check"
    fi

    # Now perform the check if we determined a file to check and it exists
    if [ -n "$target_file_to_check" ] && [ -f "$target_file_to_check" ]; then
        # Use grep -F for fixed string matching if pattern doesn't need regex, otherwise grep -q
        if grep -q -- "$file_grep_pattern" "$target_file_to_check"; then
            echo -e "${COLOR_GREEN}PASS:${COLOR_RESET} File '$target_file_to_check' contains expected pattern: '$file_grep_pattern'."
        else
            echo -e "${COLOR_RED}FAIL:${COLOR_RESET} File '$target_file_to_check' does NOT contain expected pattern: '$file_grep_pattern'."
            echo "File content:"
            cat "$target_file_to_check" | sed 's/^/  /'
            test_passed_flag=false
        fi
    else
        # Only report failure if we expected to check a file but couldn't find/determine it
        echo -e "${COLOR_RED}FAIL:${COLOR_RESET} Could not determine or find target file to check for pattern: '$file_grep_pattern'."
        # Show the cleaned argument in debug message
        echo "  (Determined target: '$target_file_to_check', Cleaned Last arg: '$last_arg_clean')"
        test_passed_flag=false
    fi
  fi
  # *** End of Revised File Check Logic ***

  # 3. Check Stderr/Stdout Output (if pattern provided)
  # Renamed check from Stderr to Output as we capture both now
  if [ -n "$stderr_grep_pattern" ]; then
    # Use grep -F for fixed string matching if pattern doesn't need regex, otherwise grep -q
    if echo "$combined_output" | grep -q -- "$stderr_grep_pattern"; then
      echo -e "${COLOR_GREEN}PASS:${COLOR_RESET} Output contains expected pattern: '$stderr_grep_pattern'."
    else
      echo -e "${COLOR_RED}FAIL:${COLOR_RESET} Output does NOT contain expected pattern: '$stderr_grep_pattern'."
      test_passed_flag=false
    fi
  fi

  # --- Exit on first failure ---
  if ! $test_passed_flag; then
    echo -e "\n${COLOR_RED}Test failed. Exiting immediately. Temporary files kept at: $TEST_DIR${COLOR_RESET}"
    exit 1 # Exit script
  fi

  # If we reach here, the test passed
  # ((tests_passed++)) # Removed - not needed inside function anymore
  echo "-------------------------------------"
}

# --- Pre-Test Setup ---

echo "Starting version incrementer tests..."
echo "Using Go script: $GO_SCRIPT_REL (Absolute: $GO_SCRIPT_ABS)"
echo "Using temporary directory: $TEST_DIR"
echo "Compiled binary will be at: $GO_BIN"

# Check if Go script exists
if [ ! -f "$GO_SCRIPT_ABS" ]; then
    echo -e "${COLOR_RED}ERROR: Go script '$GO_SCRIPT_ABS' not found. Cannot run tests.${COLOR_RESET}"
    exit 1
fi

# Compile the Go script
echo "Compiling Go script..."
go build -o "$GO_BIN" "$GO_SCRIPT_ABS"
if [ $? -ne 0 ]; then
    echo -e "${COLOR_RED}ERROR: Failed to compile Go script '$GO_SCRIPT_ABS'. Cannot run tests.${COLOR_RESET}"
    # Attempt cleanup even on build failure, as TEST_DIR exists
    rm -rf "$TEST_DIR"
    exit 1
fi
echo "Compilation successful."

# --- Test Execution ---
# Increment tests_run before each call and pass it to run_test

# === Test Case 1: Basic Increment ===
TEST_FILE_1="$TEST_DIR/test_main_1.go"
cat << EOF > "$TEST_FILE_1"
package main
const Version = "0.1.1" // Initial
func main() {}
EOF
((tests_run++))
run_test "$tests_run" "Basic Increment (0.1.1 -> 0.1.2)" \
  "$GO_BIN \"$TEST_FILE_1\"" \
  0 \
  'const Version = "0.1.2"' # Expect double quotes in file

# === Test Case 2: Multiple Increments ===
# File should now be 0.1.2 from previous test
((tests_run++))
run_test "$tests_run" "Multiple Increments (0.1.2 -> 0.1.3)" \
  "$GO_BIN \"$TEST_FILE_1\"" \
  0 \
  'const Version = "0.1.3"' # Expect double quotes in file

# === Test Case 3: Using Default Filename ===
cp "$TEST_FILE_1" "$TEST_DIR/main.go" # Contains 0.1.3 now
# Change working directory for this test so default ./main.go works
((tests_run++))
current_test_num_3=$tests_run # Store current test number before subshell
(cd "$TEST_DIR" && \
  run_test "$current_test_num_3" "Default Filename './main.go' (0.1.3 -> 0.1.4)" \
    "$GO_BIN" \
    0 \
    'const Version = "0.1.4"' # Expect double quotes in file
) # Subshell ensures we return to original directory

# === Test Case 4: Increment with Single Quotes ===
TEST_FILE_4="$TEST_DIR/test_main_4.go"
cat << EOF > "$TEST_FILE_4"
package main
const Version = '10.2.99' // Single quotes
func main() {}
EOF
((tests_run++))
run_test "$tests_run" "Increment with Single Quotes (10.2.99 -> 10.2.100)" \
  "$GO_BIN \"$TEST_FILE_4\"" \
  0 \
  "const Version = '10.2.100'" # Expect single quotes in file

# === Test Case 5: Failure - Missing Version Line ===
TEST_FILE_5="$TEST_DIR/test_main_5.go"
cat << EOF > "$TEST_FILE_5"
package main
// No Version constant here
func main() {}
EOF
((tests_run++))
run_test "$tests_run" "Failure - Missing Version Line" \
  "$GO_BIN \"$TEST_FILE_5\"" \
  1 \
  "" \
  "Error: 'const Version = .* line not found or format mismatch"

# === Test Case 6: Failure - File Not Found ===
((tests_run++))
run_test "$tests_run" "Failure - File Not Found" \
  "$GO_BIN \"$TEST_DIR/non_existent_file.go\"" \
  1 \
  "" \
  "Error: Cannot read target version file"

# === Test Case 7: Increment with Trailing Comment ===
TEST_FILE_7="$TEST_DIR/test_main_7.go"
cat << EOF > "$TEST_FILE_7"
package main
  const Version = "1.1.1" // Some comment
func main() {}
EOF
((tests_run++))
run_test "$tests_run" "Increment with Trailing Comment (1.1.1 -> 1.1.2)" \
  "$GO_BIN \"$TEST_FILE_7\"" \
  0 \
  'const Version = "1.1.2" // Some comment' # Expect double quotes


# --- Summary ---
# This part is only reached if all tests passed because run_test exits on failure
echo -e "\n--- Test Summary ---"
# Corrected to use the dynamic tests_run counter
echo -e "${COLOR_GREEN}All $tests_run tests passed!${COLOR_RESET}"

# Perform cleanup only if all tests passed
cleanup
exit 0
