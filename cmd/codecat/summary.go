// cmd/codecat/summary.go
package main

import (
	"context" // Needed for slog.Enabled check
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
)

// FileInfo - IsManual field is used
type FileInfo struct {
	Path     string
	Size     int64
	IsManual bool // Field is relevant again
}

// TreeNode remains the same
type TreeNode struct {
	Name     string
	Children map[string]*TreeNode
	FileInfo *FileInfo
}

// buildTree remains the same
func buildTree(files []FileInfo) *TreeNode {
	root := &TreeNode{Name: ".", Children: make(map[string]*TreeNode)}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	for i := range files {
		file := &files[i]
		parts := strings.Split(file.Path, "/")
		currentNode := root

		for j, part := range parts {
			if part == "" {
				continue
			}
			isLastPart := (j == len(parts)-1)
			childNode, exists := currentNode.Children[part]

			if !exists {
				childNode = &TreeNode{Name: part, Children: make(map[string]*TreeNode)}
				currentNode.Children[part] = childNode
			}

			if isLastPart {
				if childNode.FileInfo != nil {
					slog.Warn("Tree building conflict: Node already has FileInfo, overwriting.",
						"nodeName", childNode.Name, "existingPath", childNode.FileInfo.Path, "newPath", file.Path)
				}
				childNode.FileInfo = file
			}
			currentNode = childNode
		}
	}
	return root
}

// printTreeRecursive - Conditionally add [M] marker based on log level
func printTreeRecursive(writer io.Writer, node *TreeNode, indent string, isLast bool) {
	if node.Name == "." {
		childNames := make([]string, 0, len(node.Children))
		for name := range node.Children {
			childNames = append(childNames, name)
		}
		sort.Strings(childNames)
		for i, name := range childNames {
			printTreeRecursive(writer, node.Children[name], indent, i == len(childNames)-1)
		}
		return
	}

	connector := tern(isLast, "└── ", "├── ")
	fileInfoStr := ""
	manualMarker := "" // Initialize as empty

	if node.FileInfo != nil {
		fileInfoStr = fmt.Sprintf(" (%s)", formatBytes(node.FileInfo.Size))
		// Check IsManual AND if the default logger is enabled for DEBUG level
		if node.FileInfo.IsManual && slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			manualMarker = " [M]" // Add marker only if DEBUG is active
		}
	}

	// Use the potentially updated manualMarker
	fmt.Fprintf(writer, "%s%s%s%s%s\n", indent, connector, node.Name, manualMarker, fileInfoStr)

	childIndent := indent + tern(isLast, "    ", "│   ")
	childNames := make([]string, 0, len(node.Children))
	for name := range node.Children {
		childNames = append(childNames, name)
	}
	sort.Strings(childNames)
	for i, name := range childNames {
		printTreeRecursive(writer, node.Children[name], childIndent, i == len(childNames)-1)
	}
}

// printSummaryListSection remains the same
func printSummaryListSection[K comparable, V any](
	writer io.Writer,
	titleFormat string,
	items map[K]V,
	getPath func(K) string,
	getDetails func(K, V) string,
) {
	fmt.Fprintf(writer, titleFormat, len(items))
	if len(items) > 0 {
		keys := make([]K, 0, len(items))
		for k := range items {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return getPath(keys[i]) < getPath(keys[j]) })
		for _, k := range keys {
			pathStr := getPath(k)
			detailsStr := ""
			if getDetails != nil {
				detailsStr = getDetails(k, items[k])
			}
			if detailsStr != "" {
				fmt.Fprintf(writer, "- %s: %s\n", pathStr, detailsStr)
			} else {
				fmt.Fprintf(writer, "- %s\n", pathStr)
			}
		}
	}
}

// printSummaryTree remains the same
func printSummaryTree(
	includedFiles []FileInfo,
	emptyFiles []string,
	errorFiles map[string]error,
	totalSize int64,
	cwd string,
	outputWriter io.Writer,
) {
	fmt.Fprintln(outputWriter, "\n--- Summary ---")

	if len(includedFiles) > 0 {
		base := filepath.Base(cwd)
		cwdDisplay := tern(base != "." && base != string(filepath.Separator),
			fmt.Sprintf("'%s'", base), fmt.Sprintf("'%s'", cwd))
		fmt.Fprintf(outputWriter, "Included %d files (%s total) relative to CWD %s:\n",
			len(includedFiles), formatBytes(totalSize), cwdDisplay)
		fileTree := buildTree(includedFiles)
		printTreeRecursive(outputWriter, fileTree, "", true) // Calls modified func
	} else {
		fmt.Fprintln(outputWriter, "No files included in the output.")
	}

	emptyFilesMap := make(map[string]struct{}, len(emptyFiles))
	for _, path := range emptyFiles {
		emptyFilesMap[path] = struct{}{}
	}
	printSummaryListSection(outputWriter, "\nEmpty files found (%d):\n",
		emptyFilesMap, func(path string) string { return path }, nil)

	printSummaryListSection(outputWriter, "\nErrors encountered (%d):\n",
		errorFiles, func(path string) string { return path },
		func(path string, err error) string { return err.Error() })

	fmt.Fprintln(outputWriter, "---------------")
}
