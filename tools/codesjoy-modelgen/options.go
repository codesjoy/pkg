package main

import (
	"flag"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"strings"
)

// Options contains CLI arguments for model generation.
type Options struct {
	Dialect          string
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

	dialect := fs.String("dialect", "", "database dialect: mysql or postgres")
	dsn := fs.String("dsn", "", "database DSN")
	schema := fs.String("schema", "", "database schema (optional)")
	tables := fs.String("tables", "", "comma-separated tables (optional)")
	overrideFile := fs.String("override", "", "path to YAML override file (optional)")
	outDir := fs.String("out-dir", "", "output directory")
	packageName := fs.String("package", "", "go package name for generated files")
	genAIPSQL := fs.Bool("gen-aipsql", true, "generate AIPTable method and wrapper")
	timestampMode := fs.String(
		"timestamp-mode",
		timestampModeUnixSec,
		"timestamp mode: unix_sec, unix_milli, unix_nano, or time",
	)
	dryRun := fs.Bool("dry-run", false, "validate and render without writing files")
	force := fs.Bool("force", false, "overwrite files without generated header")

	fs.Usage = func() {
		_, _ = fmt.Fprintln(
			errOut,
			"Usage: codesjoy-modelgen --dialect mysql|postgres --dsn <dsn> --out-dir <dir> --package <name> [--schema <schema>] [--tables t1,t2] [--override file.yaml] [--gen-aipsql=true] [--timestamp-mode unix_sec|unix_milli|unix_nano|time] [--dry-run] [--force]",
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
		Dialect:       strings.TrimSpace(*dialect),
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

	if err := opts.validate(); err != nil {
		return Options{}, err
	}

	return opts, nil
}

func (o Options) validate() error {
	if err := o.validateRequired(); err != nil {
		return err
	}
	if _, err := normalizeDialect(o.Dialect); err != nil {
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

func validateTimestampMode(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case timestampModeUnixSec, timestampModeUnixMilli, timestampModeUnixNano, timestampModeTime:
		return nil
	default:
		return fmt.Errorf(
			"unsupported --timestamp-mode %q, expected %q, %q, %q, or %q",
			mode,
			timestampModeUnixSec,
			timestampModeUnixMilli,
			timestampModeUnixNano,
			timestampModeTime,
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
	if o.Dialect == "" {
		return fmt.Errorf("--dialect is required")
	}
	if o.DSN == "" {
		return fmt.Errorf("--dsn is required")
	}
	if o.OutDir == "" {
		return fmt.Errorf("--out-dir is required")
	}
	if o.PackageName == "" {
		return fmt.Errorf("--package is required")
	}
	return nil
}
