package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func updateVersionInFile(versionFile string) bool {
	// Read the file
	content, err := os.ReadFile(versionFile)
	if err != nil {
		fmt.Printf("Error: File '%s' not found.\n", versionFile)
		return false
	}
	lines := strings.Split(string(content), "\n")

	// Regex to match: const Version = "major.minor.patch" or 'major.minor.patch'
	// Captures: pre-patch (const Version = "major.minor.), patch (patch), post-patch (")
	re := regexp.MustCompile(`^(const Version\s*=\s*['"]?(\d+\.\d+\.)(\d+)(['"]?\s*.*))$`)

	versionUpdated := false
	updatedLines := make([]string, 0, len(lines))

	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 5 {
			prePatchString := matches[2]  // e.g., "0.1."
			patchNumberStr := matches[3]  // e.g., "20"
			postPatchString := matches[4] // e.g., "\""

			patchNumber, err := strconv.Atoi(patchNumberStr)
			if err != nil {
				fmt.Println("Error: Invalid version format.")
				return false
			}

			newPatchNumber := patchNumber + 1
			updatedLine := fmt.Sprintf("const Version = %s%d%s", prePatchString, newPatchNumber, postPatchString)
			updatedLines = append(updatedLines, updatedLine)
			versionUpdated = true
		} else {
			updatedLines = append(updatedLines, line)
		}
	} // End of for loop

	if !versionUpdated {
		fmt.Println("Error: VERSION constant not found.")
		return false
	}

	// Write the updated content back to the file
	updatedContent := strings.Join(updatedLines, "\n")
	err = os.WriteFile(versionFile, []byte(updatedContent), 0644)
	if err != nil {
		fmt.Printf("Error: Could not write to file '%s'.\n", versionFile)
		return false
	}

	fmt.Printf("Version updated in %s\n", versionFile)
	return true
} // End of updateVersionInFile

func main() {
	versionFile := "main.go"
	if updateVersionInFile(versionFile) {
		os.Exit(0) // Success
	} else {
		os.Exit(1) // Failure
	}
} // End of main
