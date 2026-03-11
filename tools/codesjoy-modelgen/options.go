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

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
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

	dialect := fs.String("dialect", "", "database dialect: mysql or postgres (optional, inferred from DSN when omitted)")
	dsn := fs.String("dsn", "", "database DSN")
	schema := fs.String("schema", "", "database schema (optional)")
	tables := fs.String("tables", "", "comma-separated tables (optional)")
	overrideFile := fs.String("override", "", "path to YAML override file (optional)")
	outDir := fs.String("out-dir", "./", "output directory")
	packageName := fs.String("package", "", "go package name for generated files (optional, inferred from out-dir when omitted)")
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
			"Usage: codesjoy-modelgen --dsn <dsn> [--out-dir <dir>] [--dialect mysql|postgres] [--package <name>] [--schema <schema>] [--tables t1,t2] [--override file.yaml] [--gen-aipsql=true] [--timestamp-mode unix_sec|unix_milli|unix_nano] [--dry-run] [--force]",
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

func (o *Options) resolveDefaults() error {
	if strings.TrimSpace(o.Dialect) == "" {
		dialect, err := resolveDialect(o.DSN)
		if err != nil {
			return err
		}
		o.Dialect = dialect
	}

	if strings.TrimSpace(o.PackageName) == "" {
		packageName, err := resolvePackageName(o.OutDir)
		if err != nil {
			return err
		}
		o.PackageName = packageName
	}

	return nil
}

func resolveDialect(dsn string) (string, error) {
	trimmedDSN := strings.TrimSpace(dsn)
	if trimmedDSN == "" {
		return "", fmt.Errorf("--dsn is required")
	}

	if _, err := pgx.ParseConfig(trimmedDSN); err == nil {
		return dialectPostgres, nil
	}
	if _, err := mysql.ParseDSN(trimmedDSN); err == nil {
		return dialectMySQL, nil
	}

	return "", fmt.Errorf("cannot infer --dialect from DSN; please pass --dialect explicitly")
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
