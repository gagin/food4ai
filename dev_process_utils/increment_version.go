// Located in: dev_process_utils/increment_version.go (example name)
package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// updateVersionInFile attempts to find 'const Version = "..."' in the specified file,
// increment the patch number, and write the file back.
// It returns true on success, false on failure.
func updateVersionInFile(versionFile string) bool {
	// Read the target file (e.g., "./main.go" in the repo root)
	content, err := os.ReadFile(versionFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot read target version file '%s': %v\n", versionFile, err)
		fmt.Fprintf(os.Stderr, "Ensure the file exists or the correct path was provided.\n")
		return false
	}
	originalContent := string(content)
	lines := strings.Split(originalContent, "\n")

	// Regex Explanation:
	// ^                  - Start of the line
	// (\s*)              - Capture group 1: Leading whitespace (if any)
	// (const Version\s*=\s*) - Capture group 2: "const Version = " part
	// (['"])            - Capture group 3: The opening quote (' or ")
	// (\d+\.\d+\.)       - Capture group 4: The "major.minor." part (e.g., "0.1.")
	// (\d+)              - Capture group 5: The patch number (e.g., "20")
	// (['"])            - Capture group 6: The closing quote (' or ") - assumes it matches the opening one
	// (.*)               - Capture group 7: The rest of the line (e.g., comments)
	// $                  - End of the line
	re := regexp.MustCompile(`^(\s*)(const Version\s*=\s*)(['"])(\d+\.\d+\.)(\d+)(['"])(.*)$`)

	versionUpdated := false
	// versionLineIndex := -1 // Removed - Was unused
	updatedLines := make([]string, len(lines))

	for i, line := range lines {
		updatedLines[i] = line // Keep original line by default

		if !versionUpdated { // Only attempt match if version hasn't been updated yet
			matches := re.FindStringSubmatch(line)
			// Expect 8 matches: full string + 7 capture groups
			if len(matches) == 8 {
				leadingSpace := matches[1]
				prefix := matches[2]
				openingQuote := matches[3]
				majorMinorPart := matches[4]
				patchNumberStr := matches[5]
				closingQuote := matches[6]
				suffix := matches[7]

				patchNumber, err := strconv.Atoi(patchNumberStr)
				if err != nil {
					// Regex matched digits, so this is unlikely but indicates corrupt data
					fmt.Fprintf(os.Stderr, "Error: Invalid patch number format '%s' found in line: %s\n", patchNumberStr, line)
					return false
				}

				newPatchNumber := patchNumber + 1
				// Reconstruct the line using all captured parts
				updatedLine := fmt.Sprintf("%s%s%s%s%d%s%s",
					leadingSpace,
					prefix,
					openingQuote,
					majorMinorPart,
					newPatchNumber,
					closingQuote,
					suffix,
				)

				// Store the updated line
				updatedLines[i] = updatedLine
				versionUpdated = true
				// versionLineIndex = i // Removed - Was unused
				fmt.Printf("Version updated in %s (Line %d): Patch %d -> %d\n", versionFile, i+1, patchNumber, newPatchNumber)
				// break // Optional: break if only one version line expected
			}
		}
	} // End of for loop

	if !versionUpdated {
		// Version line missing or format wrong - required for successful operation.
		fmt.Fprintf(os.Stderr, "Error: 'const Version = \"major.minor.patch\"' line not found or format mismatch in '%s'.\n", versionFile)
		fmt.Fprintf(os.Stderr, "Automatic version update failed.\n")
		return false
	}

	// Write the updated content back to the file ONLY if changes were actually made.
	updatedContent := strings.Join(updatedLines, "\n")

	if updatedContent == originalContent {
		fmt.Printf("No version change needed in %s.\n", versionFile)
		return true // Success, no changes needed
	}

	err = os.WriteFile(versionFile, []byte(updatedContent), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not write updated content to file '%s': %v\n", versionFile, err)
		return false
	}

	fmt.Printf("Successfully updated version in %s\n", versionFile)
	return true
} // End of updateVersionInFile

func main() {
	var versionFile string

	// Check for a command-line argument specifying the target file.
	if len(os.Args) > 1 {
		// Use the first argument as the filename.
		versionFile = os.Args[1]
		fmt.Printf("Using provided filename: %s\n", versionFile)
	} else {
		// Default to "./main.go" if no argument is provided.
		// Assumes the script is run from the repository root directory.
		versionFile = "./main.go"
		fmt.Printf("No filename provided, defaulting to: %s\n", versionFile)
	}

	if updateVersionInFile(versionFile) {
		// IMPORTANT for pre-commit hooks: Exit 0 on success allows the commit.
		os.Exit(0)
	} else {
		// IMPORTANT for pre-commit hooks: Exit > 0 on failure blocks the commit.
		os.Exit(1)
	}
} // End of main
