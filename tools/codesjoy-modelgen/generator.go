package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Generator wires introspection, override merge, rendering, and writing.
type Generator struct {
	Introspector Introspector
	Renderer     Renderer
	Writer       FileWriter
}

// NewGenerator creates a generator with production dependencies.
func NewGenerator() *Generator {
	return &Generator{
		Introspector: &SQLIntrospector{},
		Renderer:     &GoRenderer{},
		Writer:       &OSFileWriter{},
	}
}

// Run executes generation with the provided options.
func (g *Generator) Run(ctx context.Context, opts Options, stdout io.Writer) error {
	if g.Introspector == nil {
		return fmt.Errorf("introspector is required")
	}
	if g.Renderer == nil {
		return fmt.Errorf("renderer is required")
	}
	if g.Writer == nil {
		return fmt.Errorf("writer is required")
	}

	normalizedDialect, err := normalizeDialect(opts.Dialect)
	if err != nil {
		return err
	}
	opts.Dialect = normalizedDialect

	tables, err := g.Introspector.Inspect(
		ctx,
		opts.Dialect,
		opts.DSN,
		opts.Schema,
		opts.Tables,
	)
	if err != nil {
		return fmt.Errorf("inspect metadata: %w", err)
	}

	overrides, err := LoadOverrideConfig(opts.OverrideFile)
	if err != nil {
		return fmt.Errorf("load override config: %w", err)
	}

	resolvedTables, warnings, err := ResolveTables(tables, overrides, opts)
	if err != nil {
		return fmt.Errorf("resolve tables: %w", err)
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(stdout, "[warn] %s\n", warning)
	}

	renderedFiles, err := g.Renderer.Render(opts.PackageName, resolvedTables)
	if err != nil {
		return fmt.Errorf("render files: %w", err)
	}

	generated := make([]GeneratedFile, 0, len(renderedFiles))
	for _, rf := range renderedFiles {
		generated = append(generated, GeneratedFile{
			Path:    filepath.Join(opts.OutDir, rf.Name),
			Content: rf.Content,
		})
	}
	for _, table := range resolvedTables {
		warnLegacySplitFiles(opts.OutDir, table.Name, stdout)
	}

	for _, file := range generated {
		if err := g.Writer.Write(file, opts.Force, opts.DryRun); err != nil {
			return err
		}
		if opts.DryRun {
			_, _ = fmt.Fprintf(stdout, "[dry-run] %s\n", file.Path)
		}
	}

	if !opts.DryRun {
		_, _ = fmt.Fprintf(
			stdout,
			"generated %d files in %s\n",
			len(generated),
			opts.OutDir,
		)
	}

	return nil
}

func warnLegacySplitFiles(outDir string, tableName string, stdout io.Writer) {
	legacyFiles := []string{
		filepath.Join(outDir, fmt.Sprintf("%s_model_gen.go", tableName)),
		filepath.Join(outDir, fmt.Sprintf("%s_aipsql_gen.go", tableName)),
	}
	for _, legacy := range legacyFiles {
		if _, err := os.Stat(legacy); err == nil {
			_, _ = fmt.Fprintf(
				stdout,
				"[migrate] found legacy file %s, new generator emits %s_gen.go; remove legacy file manually if no longer needed\n",
				legacy,
				tableName,
			)
		}
	}
}
