package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRun_ParseOptionsError(t *testing.T) {
	t.Parallel()

	err := run(
		context.Background(),
		nil,
		&bytes.Buffer{},
		&bytes.Buffer{},
	)
	if err == nil {
		t.Fatal("run() error = nil, want parseOptions error")
	}
}

func TestRun_GeneratorError(t *testing.T) {
	t.Parallel()

	err := run(
		context.Background(),
		[]string{
			"--dsn", "invalid-dsn",
			"--out-dir", t.TempDir(),
			"--package", "demo",
		},
		&bytes.Buffer{},
		&bytes.Buffer{},
	)
	if err == nil {
		t.Fatal("run() error = nil, want generator error")
	}
	if !strings.Contains(err.Error(), "inspect metadata") &&
		!strings.Contains(err.Error(), "infer database dialect from DSN") {
		t.Fatalf("run() error = %v, want inspect/infer dialect failure", err)
	}
}
