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
	if err := g.validateDependencies(); err != nil {
		return err
	}

	normalizedDialect, err := normalizeDialect(opts.Dialect)
	if err != nil {
		return err
	}
	opts.Dialect = normalizedDialect

	tables, err := g.inspectTables(ctx, opts)
	if err != nil {
		return err
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

	generated := toGeneratedFiles(opts.OutDir, renderedFiles)
	warnLegacySplitFilesForTables(opts.OutDir, resolvedTables, stdout)
	if err := g.writeGeneratedFiles(generated, opts, stdout); err != nil {
		return err
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

func (g *Generator) validateDependencies() error {
	if g.Introspector == nil {
		return fmt.Errorf("introspector is required")
	}
	if g.Renderer == nil {
		return fmt.Errorf("renderer is required")
	}
	if g.Writer == nil {
		return fmt.Errorf("writer is required")
	}
	return nil
}

func (g *Generator) inspectTables(ctx context.Context, opts Options) ([]TableMeta, error) {
	tables, err := g.Introspector.Inspect(
		ctx,
		opts.Dialect,
		opts.DSN,
		opts.Schema,
		opts.Tables,
	)
	if err != nil {
		return nil, fmt.Errorf("inspect metadata: %w", err)
	}
	return tables, nil
}

func toGeneratedFiles(outDir string, files []RenderedFile) []GeneratedFile {
	generated := make([]GeneratedFile, 0, len(files))
	for _, file := range files {
		generated = append(generated, GeneratedFile{
			Path:    filepath.Join(outDir, file.Name),
			Content: file.Content,
		})
	}
	return generated
}

func warnLegacySplitFilesForTables(outDir string, tables []ResolvedTable, stdout io.Writer) {
	for _, table := range tables {
		warnLegacySplitFiles(outDir, table.Name, stdout)
	}
}

func (g *Generator) writeGeneratedFiles(files []GeneratedFile, opts Options, stdout io.Writer) error {
	for _, file := range files {
		if err := g.Writer.Write(file, opts.Force, opts.DryRun); err != nil {
			return err
		}
		if opts.DryRun {
			_, _ = fmt.Fprintf(stdout, "[dry-run] %s\n", file.Path)
		}
	}
	return nil
}
