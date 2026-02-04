package fileio

import (
	"fmt"
	"io"
	"os"
)

// Read reads a file and returns its content
func Read(filePath string) (string, error) {
	if filePath == "-" {
		// Read from stdin
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read from stdin: %w", err)
		}
		return string(bytes), nil
	}

	// Read from file
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	return string(bytes), nil
}
