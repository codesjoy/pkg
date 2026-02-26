package main

import (
	"bytes"
	"testing"
)

func TestParseOptions_DefaultsAndExplicitFlags(t *testing.T) {
	t.Parallel()

	opts, err := parseOptions([]string{
		"--dialect", "mysql",
		"--dsn", "demo",
		"--out-dir", "./out",
		"--package", "demo",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
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
		"--dialect", "postgres",
		"--dsn", "demo",
		"--out-dir", "./out",
		"--package", "demo",
		"--gen-aipsql=false",
		"--timestamp-mode", "unix_nano",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseOptions(explicit) error = %v", err)
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

func TestParseOptions_InvalidTimestampMode(t *testing.T) {
	t.Parallel()

	_, err := parseOptions([]string{
		"--dialect", "postgres",
		"--dsn", "demo",
		"--out-dir", "./out",
		"--package", "demo",
		"--timestamp-mode", "invalid",
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("parseOptions() error = nil, want error")
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

func TestOptionsValidate_InvalidPackageAndDialect(t *testing.T) {
	t.Parallel()

	err := (Options{
		Dialect:       "mysql",
		DSN:           "demo",
		OutDir:        ".",
		PackageName:   "for",
		TimestampMode: timestampModeUnixSec,
	}).validate()
	if err == nil {
		t.Fatal("validate(invalid package) error = nil")
	}

	err = (Options{
		Dialect:       "sqlite",
		DSN:           "demo",
		OutDir:        ".",
		PackageName:   "demo",
		TimestampMode: timestampModeUnixSec,
	}).validate()
	if err == nil {
		t.Fatal("validate(invalid dialect) error = nil")
	}
}
