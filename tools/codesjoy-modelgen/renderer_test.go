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
	"strings"
	"testing"
)

func TestGoRenderer_Golden(t *testing.T) {
	t.Parallel()

	renderer := &GoRenderer{}
	tables := []ResolvedTable{
		{
			Schema:           "public",
			Name:             "users",
			ModelName:        "User",
			AIPSQLBuilder:    "NewUserAIPTable",
			GenerateAIPTable: true,
			TimestampMode:    timestampModeUnixSec,
			Columns: []ResolvedColumn{
				{
					Name:         "id",
					GoField:      "ID",
					GoType:       "int64",
					JSONName:     "id",
					FieldPath:    "id",
					Filterable:   true,
					Sortable:     true,
					MatchModes:   nil,
					IsPrimaryKey: true,
				},
				{
					Name:       "name",
					GoField:    "Name",
					GoType:     "*string",
					JSONName:   "name",
					FieldPath:  "name",
					Filterable: true,
					MatchModes: []string{"exact"},
				},
				{
					Name:            "updated_at",
					GoField:         "UpdatedAt",
					GoType:          "int64",
					JSONName:        "updated_at",
					FieldPath:       "updated_at",
					Filterable:      true,
					Sortable:        true,
					MatchModes:      nil,
					TimestampRole:   timestampRoleUpdated,
					TimestampMode:   timestampModeUnixMilli,
					UseIntTimestamp: true,
				},
			},
			CompositeIndexes: []IndexMeta{
				{Name: "idx_users_updated_at_id", Columns: []string{"updated_at", "id"}},
			},
		},
	}

	files, err := renderer.Render("demo", tables)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Render() file count = %d, want 1", len(files))
	}
	if files[0].Name != "user_model_gen.go" {
		t.Fatalf("Render() file name = %q, want user_model_gen.go", files[0].Name)
	}

	filesAgain, err := renderer.Render("demo", tables)
	if err != nil {
		t.Fatalf("Render() second call error = %v", err)
	}
	for idx := range files {
		if !bytes.Equal(files[idx].Content, filesAgain[idx].Content) {
			t.Fatalf("non-idempotent output for %s", files[idx].Name)
		}
	}

	goldenPath := filepath.Join("testdata", "golden", "user_model_gen.go")
	// #nosec G304 -- testdata path is derived from fixed fixture names.
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v", goldenPath, err)
	}
	if !bytes.Equal(files[0].Content, golden) {
		t.Fatalf(
			"golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
			files[0].Name,
			string(files[0].Content),
			string(golden),
		)
	}
}

func TestGoRenderer_NoAIPTable(t *testing.T) {
	t.Parallel()

	renderer := &GoRenderer{}
	files, err := renderer.Render("demo", []ResolvedTable{
		{
			Name:             "users",
			ModelName:        "User",
			AIPSQLBuilder:    "NewUserAIPTable",
			GenerateAIPTable: false,
			Columns: []ResolvedColumn{
				{
					Name:         "id",
					GoField:      "ID",
					GoType:       "int64",
					JSONName:     "id",
					FieldPath:    "id",
					Filterable:   true,
					Sortable:     true,
					IsPrimaryKey: true,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("file count = %d, want 1", len(files))
	}
	content := string(files[0].Content)
	if strings.Contains(content, "AIPTable()") {
		t.Fatalf("unexpected AIPTable method in output: %s", content)
	}
	if strings.Contains(content, "NewUserAIPTable") {
		t.Fatalf("unexpected NewUserAIPTable wrapper in output: %s", content)
	}
}

func TestGoRenderer_DeletedColumnHiddenFromAIPTable(t *testing.T) {
	t.Parallel()

	renderer := &GoRenderer{}
	files, err := renderer.Render("demo", []ResolvedTable{
		{
			Name:             "users",
			ModelName:        "User",
			AIPSQLBuilder:    "NewUserAIPTable",
			GenerateAIPTable: true,
			Columns: []ResolvedColumn{
				{
					Name:         "id",
					GoField:      "ID",
					GoType:       "int64",
					JSONName:     "id",
					FieldPath:    "id",
					Filterable:   true,
					Sortable:     true,
					IsPrimaryKey: true,
				},
				{
					Name:             "deleted_at",
					GoField:          "DeletedAt",
					GoType:           "gorm.DeletedAt",
					JSONName:         "deleted_at",
					FieldPath:        "deleted_at",
					TimestampRole:    timestampRoleDeleted,
					TimestampMode:    timestampModeUnixSec,
					SoftDeleteKind:   softDeleteKindGORM,
					HideFromAIPTable: true,
				},
			},
			CompositeIndexes: []IndexMeta{
				{Name: "idx_users_deleted_id", Columns: []string{"deleted_at", "id"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("file count = %d, want 1", len(files))
	}

	content := string(files[0].Content)
	if !strings.Contains(content, "\"gorm.io/gorm\"") {
		t.Fatalf("missing gorm import: %s", content)
	}
	if !strings.Contains(content, "DeletedAt gorm.DeletedAt") {
		t.Fatalf("missing gorm.DeletedAt field: %s", content)
	}
	if !strings.Contains(content, "column:deleted_at;index") {
		t.Fatalf("missing deleted_at index gorm tag: %s", content)
	}
	if strings.Contains(content, "WithDatabaseName(\"deleted_at\")") {
		t.Fatalf("deleted_at should be hidden from AIP table: %s", content)
	}
	if strings.Contains(content, "idx_users_deleted_id") {
		t.Fatalf("index using hidden deleted_at should be skipped: %s", content)
	}
}

func TestGoRenderer_DeletedPluginAndNano(t *testing.T) {
	t.Parallel()

	renderer := &GoRenderer{}
	files, err := renderer.Render("demo", []ResolvedTable{
		{
			Name:             "users",
			ModelName:        "User",
			AIPSQLBuilder:    "NewUserAIPTable",
			GenerateAIPTable: true,
			Columns: []ResolvedColumn{
				{
					Name:             "deleted_at",
					GoField:          "DeletedAt",
					GoType:           "soft_delete.DeletedAt",
					JSONName:         "deleted_at",
					FieldPath:        "deleted_at",
					TimestampRole:    timestampRoleDeleted,
					TimestampMode:    timestampModeUnixNano,
					SoftDeleteKind:   softDeleteKindPlugin,
					Filterable:       true,
					HideFromAIPTable: false,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("file count = %d, want 1", len(files))
	}

	content := string(files[0].Content)
	if !strings.Contains(content, "soft_delete \"gorm.io/plugin/soft_delete\"") {
		t.Fatalf("missing soft_delete import: %s", content)
	}
	if !strings.Contains(content, "gorm:\"column:deleted_at;softDelete:nano\"") {
		t.Fatalf("missing softDelete:nano tag: %s", content)
	}
	if !strings.Contains(content, "WithDatabaseName(\"deleted_at\")") {
		t.Fatalf("deleted_at should be present in AIP table when explicitly exposed: %s", content)
	}
}
