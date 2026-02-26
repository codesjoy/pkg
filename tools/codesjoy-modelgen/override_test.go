package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTables_WithOverrideTriState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	overridePath := filepath.Join(dir, "override.yaml")
	overrideYAML := []byte(`
gen_aipsql: true
timestamp_mode: unix_milli

include_tables:
  - users
exclude_tables:
  - archived

tables:
  users:
    model_name: User
    aipsql_builder: NewUserAIPTable
    gen_aipsql: false
    timestamp_mode: unix_sec
    columns:
      created_at:
        timestamp_mode: time
      is_active:
        filterable: false
        sortable: true
        implicit_filter: false
        bool_type: true
        match_modes: [exact]
      password_hash:
        skip: true
`)
	if err := os.WriteFile(overridePath, overrideYAML, 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}

	overrides, err := LoadOverrideConfig(overridePath)
	if err != nil {
		t.Fatalf("LoadOverrideConfig() error = %v", err)
	}

	metas := []TableMeta{
		{
			Schema: "public",
			Name:   "users",
			Columns: []ColumnMeta{
				{Name: "id", DataType: "bigint", RawType: "bigint", IsPrimaryKey: true},
				{Name: "created_at", DataType: "bigint", RawType: "bigint", Nullable: false},
				{Name: "is_active", DataType: "boolean", RawType: "bool", Nullable: false},
				{
					Name:     "password_hash",
					DataType: "varchar",
					RawType:  "varchar(255)",
					Nullable: false,
				},
			},
			Indexes: []IndexMeta{
				{Name: "idx_users_email_status", Columns: []string{"id", "is_active"}},
			},
		},
	}

	resolved, warnings, err := ResolveTables(metas, overrides, Options{})
	if err != nil {
		t.Fatalf("ResolveTables() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(resolved) != 1 {
		t.Fatalf("ResolveTables() table count = %d, want 1", len(resolved))
	}
	table := resolved[0]
	if table.ModelName != "User" {
		t.Fatalf("model name = %q, want %q", table.ModelName, "User")
	}
	if table.AIPSQLBuilder != "NewUserAIPTable" {
		t.Fatalf("builder = %q", table.AIPSQLBuilder)
	}
	if table.GenerateAIPTable {
		t.Fatalf("GenerateAIPTable = true, want false")
	}
	if table.TimestampMode != timestampModeUnixSec {
		t.Fatalf("TimestampMode = %q, want %q", table.TimestampMode, timestampModeUnixSec)
	}
	if len(table.Columns) != 3 {
		t.Fatalf("column count = %d, want 3", len(table.Columns))
	}

	var createdCol ResolvedColumn
	var activeCol ResolvedColumn
	for _, col := range table.Columns {
		if col.Name == "created_at" {
			createdCol = col
		}
		if col.Name == "is_active" {
			activeCol = col
		}
	}
	if createdCol.Name == "" {
		t.Fatalf("created_at column not found")
	}
	if createdCol.TimestampMode != timestampModeTime {
		t.Fatalf(
			"created_at timestamp mode = %q, want %q",
			createdCol.TimestampMode,
			timestampModeTime,
		)
	}
	if createdCol.GoType != "time.Time" {
		t.Fatalf("created_at go type = %q, want time.Time", createdCol.GoType)
	}
	if activeCol.Name == "" {
		t.Fatalf("is_active column not found")
	}
	if activeCol.Filterable {
		t.Fatalf("is_active filterable = true, want false")
	}
	if !activeCol.Sortable {
		t.Fatalf("is_active sortable = false, want true")
	}
	if !activeCol.BoolType {
		t.Fatalf("is_active boolType = false, want true")
	}
	if len(activeCol.MatchModes) != 1 || activeCol.MatchModes[0] != "exact" {
		t.Fatalf("is_active match modes = %#v, want [exact]", activeCol.MatchModes)
	}
}

func TestResolveTables_CLIOverridesYAML(t *testing.T) {
	t.Parallel()

	overrides := OverrideConfig{
		GenAIPSQL:     boolPtr(false),
		TimestampMode: timestampModeTime,
		Tables: map[string]TableOverride{
			"users": {
				TimestampMode: timestampModeUnixSec,
				Columns: map[string]ColumnOverride{
					"updated_at": {TimestampMode: timestampModeTime},
				},
			},
		},
	}

	metas := []TableMeta{
		{
			Name: "users",
			Columns: []ColumnMeta{
				{Name: "updated_at", DataType: "bigint", RawType: "bigint", Nullable: false},
			},
		},
	}

	resolved, _, err := ResolveTables(metas, overrides, Options{
		GenAIPSQL:        true,
		GenAIPSQLSet:     true,
		TimestampMode:    timestampModeUnixMilli,
		TimestampModeSet: true,
	})
	if err != nil {
		t.Fatalf("ResolveTables() error = %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("resolved table count = %d, want 1", len(resolved))
	}
	if !resolved[0].GenerateAIPTable {
		t.Fatalf("GenerateAIPTable = false, want true")
	}
	col := resolved[0].Columns[0]
	if col.TimestampMode != timestampModeUnixMilli {
		t.Fatalf("TimestampMode = %q, want %q", col.TimestampMode, timestampModeUnixMilli)
	}
	if col.GoType != "int64" {
		t.Fatalf("GoType = %q, want int64", col.GoType)
	}
	if !col.UseIntTimestamp {
		t.Fatalf("UseIntTimestamp = false, want true")
	}
}

func TestResolveTables_TimestampFieldNamingAndSoftDelete(t *testing.T) {
	t.Parallel()

	metas := []TableMeta{
		{
			Name: "users",
			Columns: []ColumnMeta{
				{Name: "create", DataType: "bigint", RawType: "bigint", Nullable: false},
				{Name: "update", DataType: "bigint", RawType: "bigint", Nullable: false},
				{
					Name:      "deleted_at",
					DataType:  "bigint",
					RawType:   "bigint",
					Nullable:  false,
					IsIndexed: true,
				},
			},
		},
	}

	resolved, warnings, err := ResolveTables(metas, OverrideConfig{}, Options{
		TimestampMode:    timestampModeUnixNano,
		TimestampModeSet: true,
	})
	if err != nil {
		t.Fatalf("ResolveTables() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(resolved) != 1 {
		t.Fatalf("resolved table count = %d, want 1", len(resolved))
	}

	fieldByName := map[string]ResolvedColumn{}
	for _, col := range resolved[0].Columns {
		fieldByName[col.Name] = col
	}

	createdCol := fieldByName["create"]
	if createdCol.GoField != "CreatedAt" {
		t.Fatalf("create field name = %q, want %q", createdCol.GoField, "CreatedAt")
	}
	updatedCol := fieldByName["update"]
	if updatedCol.GoField != "UpdatedAt" {
		t.Fatalf("update field name = %q, want %q", updatedCol.GoField, "UpdatedAt")
	}

	deletedCol := fieldByName["deleted_at"]
	if deletedCol.GoField != "DeletedAt" {
		t.Fatalf("deleted_at field name = %q, want %q", deletedCol.GoField, "DeletedAt")
	}
	if deletedCol.GoType != "soft_delete.DeletedAt" {
		t.Fatalf("deleted_at go type = %q, want soft_delete.DeletedAt", deletedCol.GoType)
	}
	if deletedCol.SoftDeleteKind != softDeleteKindPlugin {
		t.Fatalf(
			"deleted_at soft delete kind = %q, want %q",
			deletedCol.SoftDeleteKind,
			softDeleteKindPlugin,
		)
	}
	if !deletedCol.HideFromAIPTable {
		t.Fatalf("deleted_at HideFromAIPTable = false, want true")
	}
}

func TestResolveTables_DeletedColumnCanBeExposedByOverride(t *testing.T) {
	t.Parallel()

	metas := []TableMeta{
		{
			Name: "users",
			Columns: []ColumnMeta{
				{Name: "id", DataType: "bigint", RawType: "bigint", IsPrimaryKey: true},
				{Name: "deleted_at", DataType: "bigint", RawType: "bigint", Nullable: false},
			},
		},
	}

	overrides := OverrideConfig{
		Tables: map[string]TableOverride{
			"users": {
				Columns: map[string]ColumnOverride{
					"deleted_at": {
						Filterable: boolPtr(true),
					},
				},
			},
		},
	}

	resolved, _, err := ResolveTables(metas, overrides, Options{})
	if err != nil {
		t.Fatalf("ResolveTables() error = %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("resolved table count = %d, want 1", len(resolved))
	}

	var deletedCol ResolvedColumn
	for _, col := range resolved[0].Columns {
		if col.Name == "deleted_at" {
			deletedCol = col
		}
	}
	if deletedCol.Name == "" {
		t.Fatal("deleted_at column not found")
	}
	if deletedCol.HideFromAIPTable {
		t.Fatalf("deleted_at HideFromAIPTable = true, want false")
	}
}

func TestLoadOverrideConfig_AcceptsUnixNano(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	overridePath := filepath.Join(dir, "override.yaml")
	overrideYAML := []byte(`
timestamp_mode: unix_nano
tables:
  users:
    timestamp_mode: unix_nano
    columns:
      updated_at:
        timestamp_mode: unix_nano
`)
	if err := os.WriteFile(overridePath, overrideYAML, 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}

	_, err := LoadOverrideConfig(overridePath)
	if err != nil {
		t.Fatalf("LoadOverrideConfig() error = %v", err)
	}
}

func TestResolveTables_PostgresCharacterVarying_DefaultAndOverrideMatchModes(t *testing.T) {
	t.Parallel()

	metas := []TableMeta{
		{
			Name: "users",
			Columns: []ColumnMeta{
				{
					Name:     "name",
					DataType: "character varying",
					RawType:  "varchar",
					Nullable: false,
				},
			},
		},
	}

	resolvedDefault, _, err := ResolveTables(metas, OverrideConfig{}, Options{})
	if err != nil {
		t.Fatalf("ResolveTables(default) error = %v", err)
	}
	if len(resolvedDefault) != 1 || len(resolvedDefault[0].Columns) != 1 {
		t.Fatalf("unexpected default resolved shape: %#v", resolvedDefault)
	}
	defaultCol := resolvedDefault[0].Columns[0]
	if len(defaultCol.MatchModes) != 1 || defaultCol.MatchModes[0] != "exact" {
		t.Fatalf("default match modes = %#v, want [exact]", defaultCol.MatchModes)
	}

	overrides := OverrideConfig{
		Tables: map[string]TableOverride{
			"users": {
				Columns: map[string]ColumnOverride{
					"name": {
						MatchModes: []string{"prefix"},
					},
				},
			},
		},
	}
	resolvedOverride, _, err := ResolveTables(metas, overrides, Options{})
	if err != nil {
		t.Fatalf("ResolveTables(override) error = %v", err)
	}
	if len(resolvedOverride) != 1 || len(resolvedOverride[0].Columns) != 1 {
		t.Fatalf("unexpected override resolved shape: %#v", resolvedOverride)
	}
	overrideCol := resolvedOverride[0].Columns[0]
	if len(overrideCol.MatchModes) != 1 || overrideCol.MatchModes[0] != "prefix" {
		t.Fatalf("override match modes = %#v, want [prefix]", overrideCol.MatchModes)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
