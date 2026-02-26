package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OSFileWriter writes generated files to local disk.
type OSFileWriter struct{}

// Write writes one file and protects non-generated files unless force is enabled.
func (w *OSFileWriter) Write(file GeneratedFile, force bool, dryRun bool) error {
	if file.Path == "" {
		return fmt.Errorf("file path is required")
	}
	if len(file.Content) == 0 {
		return fmt.Errorf("file %s has empty content", file.Path)
	}

	if err := verifyDestination(file.Path, force); err != nil {
		return err
	}
	if dryRun {
		return nil
	}

	// #nosec G301 -- generated source directories are expected to be world-readable.
	if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	// #nosec G306 -- generated source files follow conventional 0644 permissions.
	if err := os.WriteFile(file.Path, file.Content, 0o644); err != nil {
		return fmt.Errorf("write generated file %s: %w", file.Path, err)
	}

	return nil
}

func verifyDestination(path string, force bool) error {
	// #nosec G304 -- path comes from explicit CLI argument.
	current, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read existing file %s: %w", path, err)
	}
	if strings.Contains(string(current), generatedHeader) {
		return nil
	}
	if force {
		return nil
	}
	return fmt.Errorf(
		"refusing to overwrite non-generated file %s; use --force to override",
		path,
	)
}
