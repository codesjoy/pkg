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

//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIntegration_Generator_EndToEnd_MySQL_UnixNano(t *testing.T) {
	t.Parallel()
	ctx, cancel := integrationContext(t)
	defer cancel()

	outDir := t.TempDir()
	stdout, stderr, err := runGenerator(
		ctx,
		t,
		"--dsn", mustMySQLDSN(t),
		"--schema", integrationDBName,
		"--tables", "users,events",
		"--out-dir", outDir,
		"--package", "model",
		"--gen-aipsql=true",
		"--timestamp-mode", "unix_nano",
	)
	if err != nil {
		t.Fatalf("runGenerator(mysql) error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	usersFile := mustReadFile(t, filepath.Join(outDir, "user_model_gen.go"))
	eventsFile := mustReadFile(t, filepath.Join(outDir, "event_model_gen.go"))

	assertContains(t, usersFile, "CreatedAt int64")
	assertContains(t, usersFile, "column:created_at;autoCreateTime:nano")
	assertContains(t, usersFile, "UpdatedAt int64")
	assertContains(t, usersFile, "column:update;autoUpdateTime:nano")
	assertContains(t, usersFile, "DeletedAt soft_delete.DeletedAt")
	assertContains(t, usersFile, "column:deleted_at;softDelete:nano")
	assertContains(t, usersFile, "func NewUserAIPTable() *aipsql.Table")
	assertNotContains(t, usersFile, "WithDatabaseName(\"deleted_at\")")

	assertContains(t, eventsFile, "CreatedAt time.Time")
	assertContains(t, eventsFile, "DeletedAt gorm.DeletedAt")
	assertContains(t, eventsFile, "column:deleted_at;index")
	assertContains(t, eventsFile, "func NewEventAIPTable() *aipsql.Table")
	assertContains(t, stdout, "[warn] table events")
}

func TestIntegration_Generator_EndToEnd_Postgres_UnixNano(t *testing.T) {
	t.Parallel()
	ctx, cancel := integrationContext(t)
	defer cancel()

	outDir := t.TempDir()
	stdout, stderr, err := runGenerator(
		ctx,
		t,
		"--dsn", mustPostgresDSN(t),
		"--schema", integrationSchemaPG,
		"--tables", "users,events",
		"--out-dir", outDir,
		"--package", "model",
		"--gen-aipsql=true",
		"--timestamp-mode", "unix_nano",
	)
	if err != nil {
		t.Fatalf("runGenerator(postgres) error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	usersFile := mustReadFile(t, filepath.Join(outDir, "user_model_gen.go"))
	eventsFile := mustReadFile(t, filepath.Join(outDir, "event_model_gen.go"))

	assertContains(t, usersFile, "CreatedAt int64")
	assertContains(t, usersFile, "column:created_at;autoCreateTime:nano")
	assertContains(t, usersFile, "UpdatedAt int64")
	assertContains(t, usersFile, "column:update;autoUpdateTime:nano")
	assertContains(t, usersFile, "DeletedAt soft_delete.DeletedAt")
	assertContains(t, usersFile, "column:deleted_at;softDelete:nano")
	assertNotContains(t, usersFile, "WithDatabaseName(\"deleted_at\")")

	assertContains(t, eventsFile, "CreatedAt time.Time")
	assertContains(t, eventsFile, "DeletedAt gorm.DeletedAt")
	assertContains(t, eventsFile, "column:deleted_at;index")
	assertContains(t, eventsFile, "func NewEventAIPTable() *aipsql.Table")
	assertContains(t, stdout, "[warn] table events")
}

func TestIntegration_Generator_OverrideExposeDeletedAt(t *testing.T) {
	t.Parallel()
	ctx, cancel := integrationContext(t)
	defer cancel()

	outDir := t.TempDir()
	overridePath := filepath.Join(t.TempDir(), "override.yaml")
	overrideContent := []byte(`
tables:
  users:
    columns:
      deleted_at:
        filterable: true
`)
	if err := os.WriteFile(overridePath, overrideContent, 0o600); err != nil {
		t.Fatalf("write override file: %v", err)
	}

	stdout, stderr, err := runGenerator(
		ctx,
		t,
		"--dsn", mustPostgresDSN(t),
		"--schema", integrationSchemaPG,
		"--tables", "users",
		"--out-dir", outDir,
		"--package", "model",
		"--gen-aipsql=true",
		"--timestamp-mode", "unix_nano",
		"--override", overridePath,
	)
	if err != nil {
		t.Fatalf("runGenerator(override) error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	usersFile := mustReadFile(t, filepath.Join(outDir, "user_model_gen.go"))
	assertContains(t, usersFile, "WithDatabaseName(\"deleted_at\")")
	assertContains(t, usersFile, "func NewUserAIPTable() *aipsql.Table")
}

func TestIntegration_AIPIndexes_VisibleCompositeIncluded(t *testing.T) {
	t.Parallel()
	ctx, cancel := integrationContext(t)
	defer cancel()

	outDir := t.TempDir()
	stdout, stderr, err := runGenerator(
		ctx,
		t,
		"--dsn", mustMySQLDSN(t),
		"--schema", integrationDBName,
		"--tables", "users",
		"--out-dir", outDir,
		"--package", "model",
		"--gen-aipsql=true",
		"--timestamp-mode", "unix_nano",
	)
	if err != nil {
		t.Fatalf(
			"runGenerator(visible composite) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout,
			stderr,
		)
	}

	usersFile := mustReadFile(t, filepath.Join(outDir, "user_model_gen.go"))
	assertContains(t, usersFile, "Name:    \"idx_users_created_id\"")
	assertContains(t, usersFile, "Columns: []string{\"created_at\", \"id\"}")
	assertNotContains(t, usersFile, "Name:    \"idx_users_created_at\"")
	assertNotContains(t, usersFile, "Name:    \"uk_users_email\"")
}

func TestIntegration_AIPIndexes_HiddenColumnCompositePruned(t *testing.T) {
	t.Parallel()
	ctx, cancel := integrationContext(t)
	defer cancel()

	outDir := t.TempDir()
	stdout, stderr, err := runGenerator(
		ctx,
		t,
		"--dsn", mustPostgresDSN(t),
		"--schema", integrationSchemaPG,
		"--tables", "events",
		"--out-dir", outDir,
		"--package", "model",
		"--gen-aipsql=true",
		"--timestamp-mode", "unix_nano",
	)
	if err != nil {
		t.Fatalf(
			"runGenerator(hidden composite) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout,
			stderr,
		)
	}

	eventsFile := mustReadFile(t, filepath.Join(outDir, "event_model_gen.go"))
	assertNotContains(t, eventsFile, "Name:    \"idx_events_user_deleted\"")
}

func TestIntegration_AIPIndexes_ExposeDeletedRestoresComposite(t *testing.T) {
	t.Parallel()
	ctx, cancel := integrationContext(t)
	defer cancel()

	outDir := t.TempDir()
	overridePath := integrationOverridePath(t, "aip_indexes_expose_deleted.yaml")

	stdout, stderr, err := runGenerator(
		ctx,
		t,
		"--dsn", mustPostgresDSN(t),
		"--schema", integrationSchemaPG,
		"--tables", "events",
		"--out-dir", outDir,
		"--package", "model",
		"--gen-aipsql=true",
		"--timestamp-mode", "unix_nano",
		"--override", overridePath,
	)
	if err != nil {
		t.Fatalf(
			"runGenerator(expose deleted composite) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout,
			stderr,
		)
	}

	eventsFile := mustReadFile(t, filepath.Join(outDir, "event_model_gen.go"))
	assertContains(t, eventsFile, "WithDatabaseName(\"deleted_at\")")
	assertContains(t, eventsFile, "Name:    \"idx_events_user_deleted\"")
	assertContains(t, eventsFile, "Columns: []string{\"user_id\", \"deleted_at\"}")
}

func TestIntegration_PostgresCharacterVarying_DefaultExactMatchMode(t *testing.T) {
	t.Parallel()
	ctx, cancel := integrationContext(t)
	defer cancel()

	outDir := t.TempDir()
	stdout, stderr, err := runGenerator(
		ctx,
		t,
		"--dsn", mustPostgresDSN(t),
		"--schema", integrationSchemaPG,
		"--tables", "users",
		"--out-dir", outDir,
		"--package", "model",
		"--gen-aipsql=true",
		"--timestamp-mode", "unix_nano",
	)
	if err != nil {
		t.Fatalf(
			"runGenerator(pg character varying exact) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout,
			stderr,
		)
	}

	usersFile := mustReadFile(t, filepath.Join(outDir, "user_model_gen.go"))
	assertContains(t, usersFile, "WithDatabaseName(\"name\")")
	assertContains(t, usersFile, "WithMatchModes(aipsql.MatchModeExact)")
}

func TestIntegration_AIPIndexes_IndexHintFromOverride(t *testing.T) {
	t.Parallel()
	ctx, cancel := integrationContext(t)
	defer cancel()

	outDir := t.TempDir()
	overridePath := integrationOverridePath(t, "aip_index_hint.yaml")

	stdout, stderr, err := runGenerator(
		ctx,
		t,
		"--dsn", mustPostgresDSN(t),
		"--schema", integrationSchemaPG,
		"--tables", "users",
		"--out-dir", outDir,
		"--package", "model",
		"--gen-aipsql=true",
		"--timestamp-mode", "unix_nano",
		"--override", overridePath,
	)
	if err != nil {
		t.Fatalf(
			"runGenerator(index hint override) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout,
			stderr,
		)
	}

	usersFile := mustReadFile(t, filepath.Join(outDir, "user_model_gen.go"))
	assertContains(t, usersFile, "WithDatabaseName(\"name\")")
	assertContains(t, usersFile, "WithMatchModes(aipsql.MatchModeExact)")
	assertContains(t, usersFile, "WithIndexHint(\"idx_users_name_hint\")")
}
