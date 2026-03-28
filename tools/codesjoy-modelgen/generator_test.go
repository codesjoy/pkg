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
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeIntrospector struct {
	tables []TableMeta
	err    error
}

func (f *fakeIntrospector) Inspect(
	_ context.Context,
	_ string,
	_ string,
	_ []string,
) ([]TableMeta, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tables, nil
}

func TestParseOptions_RequiredFlags(t *testing.T) {
	t.Parallel()

	_, err := parseOptions([]string{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("parseOptions() error = nil, want error")
	}
}

func TestGenerator_ProtectsNonGeneratedFile(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	nonGeneratedFile := filepath.Join(outDir, generatedModelFileName("users"))
	if err := os.WriteFile(nonGeneratedFile, []byte("package demo\n"), 0o600); err != nil {
		t.Fatalf("write non-generated file: %v", err)
	}

	gen := &Generator{
		Introspector: &fakeIntrospector{tables: sampleTableMetas()},
		Renderer:     &GoRenderer{},
		Writer:       &OSFileWriter{},
	}

	err := gen.Run(context.Background(), Options{
		DSN:         "demo",
		OutDir:      outDir,
		PackageName: "demo",
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run() error = nil, want overwrite protection error")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite non-generated file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerator_DryRunDoesNotWriteFiles(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	stdout := &bytes.Buffer{}
	gen := &Generator{
		Introspector: &fakeIntrospector{tables: sampleTableMetas()},
		Renderer:     &GoRenderer{},
		Writer:       &OSFileWriter{},
	}

	err := gen.Run(context.Background(), Options{
		DSN:         "demo",
		OutDir:      outDir,
		PackageName: "demo",
		DryRun:      true,
	}, stdout)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	modelFile := filepath.Join(outDir, generatedModelFileName("users"))
	if _, err := os.Stat(modelFile); !os.IsNotExist(err) {
		t.Fatalf("model file should not exist in dry-run mode")
	}
	if !strings.Contains(stdout.String(), "[dry-run]") {
		t.Fatalf("dry-run output missing: %q", stdout.String())
	}
}

func sampleTableMetas() []TableMeta {
	return []TableMeta{
		{
			Schema: "public",
			Name:   "users",
			Columns: []ColumnMeta{
				{
					Name:         "id",
					DataType:     "bigint",
					RawType:      "bigint",
					IsPrimaryKey: true,
					IsIndexed:    true,
				},
				{Name: "name", DataType: "varchar", RawType: "varchar(255)", Nullable: true},
				{
					Name:      "created_at",
					DataType:  "timestamp",
					RawType:   "timestamp",
					Nullable:  false,
					IsIndexed: true,
				},
			},
			Indexes: []IndexMeta{
				{Name: "idx_users_created_at_id", Columns: []string{"created_at", "id"}},
			},
		},
	}
}

func TestGenerator_WarnsLegacySplitFiles(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	legacyModel := filepath.Join(outDir, "users_model_gen.go")
	if err := os.WriteFile(legacyModel, []byte(generatedHeader), 0o600); err != nil {
		t.Fatalf("write legacy model: %v", err)
	}
	legacySingle := filepath.Join(outDir, "users_gen.go")
	if err := os.WriteFile(legacySingle, []byte(generatedHeader), 0o600); err != nil {
		t.Fatalf("write legacy single file: %v", err)
	}

	stdout := &bytes.Buffer{}
	gen := &Generator{
		Introspector: &fakeIntrospector{tables: sampleTableMetas()},
		Renderer:     &GoRenderer{},
		Writer:       &OSFileWriter{},
	}

	err := gen.Run(context.Background(), Options{
		DSN:         "demo",
		OutDir:      outDir,
		PackageName: "demo",
	}, stdout)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "[migrate]") {
		t.Fatalf("expected migrate warning, got %q", output)
	}
	if !strings.Contains(output, legacyModel) || !strings.Contains(output, legacySingle) {
		t.Fatalf("expected both legacy paths in output, got %q", output)
	}
	if !strings.Contains(output, "user_model_gen.go") {
		t.Fatalf("expected new file name in migrate output, got %q", output)
	}
}
