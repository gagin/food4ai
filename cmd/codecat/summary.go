// cmd/codecat/summary.go
package main

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
)

// FileInfo represents information about an included file for the summary.
type FileInfo struct {
	Path     string
	Size     int64
	IsManual bool
}

// TreeNode represents a node in the directory tree for summary printing.
type TreeNode struct {
	Name     string
	Children map[string]*TreeNode
	FileInfo *FileInfo // Link to FileInfo if it's a file node
}

// buildTree builds the file tree structure for the summary.
func buildTree(files []FileInfo) *TreeNode {
	root := &TreeNode{Name: ".", Children: make(map[string]*TreeNode)}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for i := range files {
		file := &files[i]
		normalizedPath := filepath.ToSlash(file.Path)
		parts := strings.Split(normalizedPath, "/")
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
				if childNode.FileInfo == nil {
					childNode.FileInfo = file
				} else {
					slog.Warn("Tree building conflict: node already has FileInfo", "nodeName", childNode.Name)
				}
			}
			currentNode = childNode
		}
	}
	return root
}

// printTreeRecursive recursively prints the file tree.
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
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	fileInfoStr := ""
	manualMarker := ""
	if node.FileInfo != nil {
		fileInfoStr = fmt.Sprintf(" (%s)", formatBytes(node.FileInfo.Size)) // formatBytes is in helpers.go
		if node.FileInfo.IsManual {
			manualMarker = " [M]"
		}
	}
	fmt.Fprintf(writer, "%s%s%s%s%s\n", indent, connector, node.Name, manualMarker, fileInfoStr)
	childIndent := indent
	if isLast {
		childIndent += "    "
	} else {
		childIndent += "│   "
	}
	childNames := make([]string, 0, len(node.Children))
	for name := range node.Children {
		childNames = append(childNames, name)
	}
	sort.Strings(childNames)
	for i, name := range childNames {
		printTreeRecursive(writer, node.Children[name], childIndent, i == len(childNames)-1)
	}
}

// printSummaryTree prints the final summary output.
func printSummaryTree(
	includedFiles []FileInfo, emptyFiles []string, errorFiles map[string]error,
	totalSize int64, targetDir string, outputWriter io.Writer,
) {
	fmt.Fprintln(outputWriter, "\n--- Summary ---")
	if len(includedFiles) > 0 {
		treeRootName := filepath.Base(targetDir)
		if treeRootName == "." || treeRootName == string(filepath.Separator) {
			if abs, err := filepath.Abs(targetDir); err == nil {
				treeRootName = abs
			} else {
				treeRootName = targetDir
			}
		}
		fmt.Fprintf(outputWriter, "Included %d files (%s total) from '%s':\n", len(includedFiles), formatBytes(totalSize), treeRootName)
		fileTree := buildTree(includedFiles)
		printTreeRecursive(outputWriter, fileTree, "", true)
	} else {
		fmt.Fprintln(outputWriter, "No files included in the output.")
	}
	if len(emptyFiles) > 0 {
		fmt.Fprintf(outputWriter, "\nEmpty files found (%d):\n", len(emptyFiles))
		sort.Strings(emptyFiles)
		for _, p := range emptyFiles {
			fmt.Fprintf(outputWriter, "- %s\n", p)
		}
	}
	if len(errorFiles) > 0 {
		fmt.Fprintf(outputWriter, "\nErrors encountered (%d):\n", len(errorFiles))
		errorPaths := make([]string, 0, len(errorFiles))
		for p := range errorFiles {
			errorPaths = append(errorPaths, p)
		}
		sort.Strings(errorPaths)
		for _, p := range errorPaths {
			fmt.Fprintf(outputWriter, "- %s: %v\n", p, errorFiles[p])
		}
	}
	fmt.Fprintln(outputWriter, "---------------")
}
