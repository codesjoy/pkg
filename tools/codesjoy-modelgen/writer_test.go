package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOSFileWriter_WriteValidationAndForce(t *testing.T) {
	t.Parallel()

	writer := &OSFileWriter{}

	if err := writer.Write(GeneratedFile{Path: "", Content: []byte("x")}, false, false); err == nil {
		t.Fatal("Write(empty path) error = nil")
	}
	if err := writer.Write(GeneratedFile{Path: "a.go", Content: nil}, false, false); err == nil {
		t.Fatal("Write(empty content) error = nil")
	}

	outDir := t.TempDir()
	path := filepath.Join(outDir, "users_gen.go")
	if err := writer.Write(
		GeneratedFile{Path: path, Content: []byte(generatedHeader + "\npackage demo\n")},
		true,
		false,
	); err != nil {
		t.Fatalf("Write(seed generated file) error = %v", err)
	}
	if err := writer.Write(
		GeneratedFile{Path: path, Content: []byte(generatedHeader + "\npackage demo\n")},
		false,
		false,
	); err != nil {
		t.Fatalf("Write(existing generated file) error = %v", err)
	}
}

func TestVerifyDestinationBranches(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	notExist := filepath.Join(outDir, "none.go")
	if err := verifyDestination(notExist, false); err != nil {
		t.Fatalf("verifyDestination(not exist) error = %v", err)
	}

	dirPath := filepath.Join(outDir, "dir-as-file")
	if err := os.MkdirAll(dirPath, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := verifyDestination(dirPath, false); err == nil {
		t.Fatal("verifyDestination(directory) error = nil")
	}

	filePath := filepath.Join(outDir, "not-generated.go")
	if err := os.WriteFile(filePath, []byte("package demo\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := verifyDestination(filePath, false); err == nil {
		t.Fatal("verifyDestination(non generated) error = nil")
	}
	if err := verifyDestination(filePath, true); err != nil {
		t.Fatalf("verifyDestination(force=true) error = %v", err)
	}
}
