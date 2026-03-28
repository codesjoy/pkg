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
	"flag"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Options contains CLI arguments for model generation.
type Options struct {
	DSN              string
	Schema           string
	Tables           []string
	OverrideFile     string
	OutDir           string
	PackageName      string
	GenAIPSQL        bool
	GenAIPSQLSet     bool
	TimestampMode    string
	TimestampModeSet bool
	DryRun           bool
	Force            bool
}

func parseOptions(args []string, errOut io.Writer) (Options, error) {
	fs := flag.NewFlagSet("codesjoy-modelgen", flag.ContinueOnError)
	fs.SetOutput(errOut)

	dsn := fs.String(
		"dsn",
		"",
		"database DSN ("+
			"MySQL: user:pass@tcp(127.0.0.1:3306)/db?parseTime=true; "+
			"PostgreSQL: postgres://user:pass@127.0.0.1:5432/db?sslmode=disable)",
	)
	schema := fs.String("schema", "", "database schema (optional)")
	tables := fs.String("tables", "", "comma-separated tables (optional)")
	overrideFile := fs.String("override", "", "path to YAML override file (optional)")
	outDir := fs.String("out-dir", "./", "output directory")
	packageName := fs.String(
		"package",
		"",
		"go package name for generated files (optional, inferred from out-dir when omitted)",
	)
	genAIPSQL := fs.Bool("gen-aipsql", true, "generate AIPTable method and wrapper")
	timestampMode := fs.String(
		"timestamp-mode",
		timestampModeUnixSec,
		"integer timestamp precision: unix_sec, unix_milli, or unix_nano",
	)
	dryRun := fs.Bool("dry-run", false, "validate and render without writing files")
	force := fs.Bool("force", false, "overwrite files without generated header")

	fs.Usage = func() {
		_, _ = fmt.Fprintln(
			errOut,
			"Usage: codesjoy-modelgen --dsn <dsn> [--out-dir <dir>] [--package <name>] "+
				"[--schema <schema>] [--tables t1,t2] [--override file.yaml] "+
				"[--gen-aipsql=true] [--timestamp-mode unix_sec|unix_milli|unix_nano] "+
				"[--dry-run] [--force]",
		)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if fs.NArg() != 0 {
		return Options{}, fmt.Errorf("unexpected args: %s", strings.Join(fs.Args(), " "))
	}

	visitedFlags := collectVisitedFlags(fs)

	opts := Options{
		DSN:           strings.TrimSpace(*dsn),
		Schema:        strings.TrimSpace(*schema),
		Tables:        splitCSV(*tables),
		OverrideFile:  strings.TrimSpace(*overrideFile),
		OutDir:        strings.TrimSpace(*outDir),
		PackageName:   strings.TrimSpace(*packageName),
		GenAIPSQL:     *genAIPSQL,
		TimestampMode: strings.TrimSpace(*timestampMode),
		DryRun:        *dryRun,
		Force:         *force,
	}
	_, opts.GenAIPSQLSet = visitedFlags["gen-aipsql"]
	_, opts.TimestampModeSet = visitedFlags["timestamp-mode"]

	if err := opts.validateRequired(); err != nil {
		return Options{}, err
	}
	if err := opts.resolveDefaults(); err != nil {
		return Options{}, err
	}
	if err := opts.validate(); err != nil {
		return Options{}, err
	}

	return opts, nil
}

func (o Options) validate() error {
	if err := o.validateRequired(); err != nil {
		return err
	}
	if !token.IsIdentifier(o.PackageName) || types.Universe.Lookup(o.PackageName) != nil {
		return fmt.Errorf("invalid --package value %q", o.PackageName)
	}
	if err := validateTimestampMode(o.TimestampMode); err != nil {
		return err
	}

	return nil
}

func (o *Options) resolveDefaults() error {
	if strings.TrimSpace(o.PackageName) == "" {
		packageName, err := resolvePackageName(o.OutDir)
		if err != nil {
			return err
		}
		o.PackageName = packageName
	}

	return nil
}

func resolvePackageName(outDir string) (string, error) {
	cleanedOutDir := filepath.Clean(strings.TrimSpace(outDir))
	if cleanedOutDir == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve current working directory: %w", err)
		}
		cleanedOutDir = filepath.Base(cwd)
	}

	packageName := filepath.Base(cleanedOutDir)
	if packageName == "." || packageName == string(filepath.Separator) || packageName == "" {
		return "", fmt.Errorf(
			"cannot infer --package from --out-dir %q; please pass --package explicitly",
			outDir,
		)
	}

	if !token.IsIdentifier(packageName) || types.Universe.Lookup(packageName) != nil {
		return "", fmt.Errorf(
			"cannot infer valid --package from --out-dir %q; please pass --package explicitly",
			outDir,
		)
	}

	return packageName, nil
}

func validateTimestampMode(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case timestampModeUnixSec, timestampModeUnixMilli, timestampModeUnixNano:
		return nil
	default:
		return fmt.Errorf(
			"unsupported --timestamp-mode %q, expected %q, %q, or %q",
			mode,
			timestampModeUnixSec,
			timestampModeUnixMilli,
			timestampModeUnixNano,
		)
	}
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func collectVisitedFlags(fs *flag.FlagSet) map[string]struct{} {
	visited := make(map[string]struct{})
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = struct{}{}
	})
	return visited
}

func (o Options) validateRequired() error {
	if o.DSN == "" {
		return fmt.Errorf("--dsn is required")
	}
	return nil
}
