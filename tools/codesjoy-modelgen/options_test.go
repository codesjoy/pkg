// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestParseOptions_DefaultsAndExplicitFlags(t *testing.T) {
	t.Parallel()

	opts, err := parseOptions([]string{
		"--dsn", "postgres://demo:demo@127.0.0.1:5432/app?sslmode=disable",
		"--out-dir", "./internal/model",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if opts.OutDir != "./internal/model" {
		t.Fatalf("OutDir = %q, want ./internal/model", opts.OutDir)
	}
	if opts.PackageName != "model" {
		t.Fatalf("PackageName = %q, want model", opts.PackageName)
	}
	if !opts.GenAIPSQL {
		t.Fatalf("GenAIPSQL = false, want true")
	}
	if opts.GenAIPSQLSet {
		t.Fatalf("GenAIPSQLSet = true, want false")
	}
	if opts.TimestampMode != timestampModeUnixSec {
		t.Fatalf("TimestampMode = %q, want %q", opts.TimestampMode, timestampModeUnixSec)
	}
	if opts.TimestampModeSet {
		t.Fatalf("TimestampModeSet = true, want false")
	}

	explicit, err := parseOptions([]string{
		"--dsn", "postgres://demo:demo@127.0.0.1:5432/app?sslmode=disable",
		"--out-dir", "./out",
		"--package", "demo",
		"--gen-aipsql=false",
		"--timestamp-mode", "unix_nano",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseOptions(explicit) error = %v", err)
	}
	if explicit.PackageName != "demo" {
		t.Fatalf("PackageName = %q, want demo", explicit.PackageName)
	}
	if explicit.GenAIPSQL {
		t.Fatalf("GenAIPSQL = true, want false")
	}
	if !explicit.GenAIPSQLSet {
		t.Fatalf("GenAIPSQLSet = false, want true")
	}
	if explicit.TimestampMode != timestampModeUnixNano {
		t.Fatalf("TimestampMode = %q, want %q", explicit.TimestampMode, timestampModeUnixNano)
	}
	if !explicit.TimestampModeSet {
		t.Fatalf("TimestampModeSet = false, want true")
	}
}

func TestParseOptions_DefaultOutDirUsesCurrentDirectoryName(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "model")
	if err := os.MkdirAll(workDir, 0o750); err != nil {
		t.Fatalf("mkdir work dir: %v", err)
	}

	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir to work dir: %v", err)
	}
	defer func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	opts, err := parseOptions([]string{
		"--dsn", "postgres://demo:demo@127.0.0.1:5432/app?sslmode=disable",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if opts.OutDir != "./" {
		t.Fatalf("OutDir = %q, want ./", opts.OutDir)
	}
	if opts.PackageName != "model" {
		t.Fatalf("PackageName = %q, want model", opts.PackageName)
	}
}

func TestParseOptions_InvalidTimestampMode(t *testing.T) {
	t.Parallel()

	_, err := parseOptions([]string{
		"--dsn", "postgres://demo:demo@127.0.0.1:5432/app?sslmode=disable",
		"--out-dir", "./out",
		"--timestamp-mode", "invalid",
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("parseOptions() error = nil, want error")
	}
}

func TestParseOptions_TimeTimestampModeRejected(t *testing.T) {
	t.Parallel()

	_, err := parseOptions([]string{
		"--dsn", "postgres://demo:demo@127.0.0.1:5432/app?sslmode=disable",
		"--out-dir", "./out",
		"--timestamp-mode", "time",
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("parseOptions() error = nil, want error")
	}
	if got := err.Error(); got != "unsupported --timestamp-mode \"time\", expected \"unix_sec\", \"unix_milli\", or \"unix_nano\"" {
		t.Fatalf("unexpected error = %q", got)
	}
}

func TestParseOptions_InvalidInferredPackage(t *testing.T) {
	t.Parallel()

	_, err := parseOptions([]string{
		"--dsn", "postgres://demo:demo@127.0.0.1:5432/app?sslmode=disable",
		"--out-dir", "./invalid-package!",
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("parseOptions() error = nil, want error")
	}
	if got := err.Error(); got != "cannot infer valid --package from --out-dir \"./invalid-package!\"; please pass --package explicitly" {
		t.Fatalf("unexpected error = %q", got)
	}
}

func TestParseOptions_DoesNotValidateDSNFormat(t *testing.T) {
	t.Parallel()

	opts, err := parseOptions([]string{
		"--dsn", "not-a-valid-dsn",
		"--out-dir", "./model",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if opts.DSN != "not-a-valid-dsn" {
		t.Fatalf("DSN = %q, want not-a-valid-dsn", opts.DSN)
	}
}

func TestSplitCSV(t *testing.T) {
	t.Parallel()

	if got := splitCSV(""); got != nil {
		t.Fatalf("splitCSV(empty) = %#v, want nil", got)
	}
	got := splitCSV("users, users,events,,orders ")
	if len(got) != 3 || got[0] != "users" || got[1] != "events" || got[2] != "orders" {
		t.Fatalf("splitCSV() = %#v, want [users events orders]", got)
	}
}

func TestOptionsValidate_InvalidPackage(t *testing.T) {
	t.Parallel()

	err := (Options{
		DSN:           "demo",
		OutDir:        ".",
		PackageName:   "for",
		TimestampMode: timestampModeUnixSec,
	}).validate()
	if err == nil {
		t.Fatal("validate(invalid package) error = nil")
	}
}
