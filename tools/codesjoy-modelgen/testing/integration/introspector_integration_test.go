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
	"strings"
	"testing"
)

func TestIntegration_SQLIntrospector_MySQL_Metadata(t *testing.T) {
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
		"--timestamp-mode", "unix_nano",
	)
	if err != nil {
		t.Fatalf("runGenerator(mysql) error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	usersFile := mustReadFile(t, filepath.Join(outDir, "user_model_gen.go"))
	assertContains(t, usersFile, "ID")
	assertContains(t, usersFile, "primaryKey;autoIncrement")
	assertContains(t, usersFile, "gorm:\"column:name\"")
	assertContains(t, usersFile, "column:created_at;autoCreateTime:nano")
	assertContains(t, usersFile, "Name:    \"idx_users_created_id\"")
	assertContains(t, usersFile, "Columns: []string{\"created_at\", \"id\"}")
}

func TestIntegration_SQLIntrospector_Postgres_Metadata(t *testing.T) {
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
		"--timestamp-mode", "unix_nano",
	)
	if err != nil {
		t.Fatalf(
			"runGenerator(postgres) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout,
			stderr,
		)
	}

	usersFile := mustReadFile(t, filepath.Join(outDir, "user_model_gen.go"))
	eventsFile := mustReadFile(t, filepath.Join(outDir, "event_model_gen.go"))

	assertContains(t, usersFile, "column:update;autoUpdateTime:nano")
	assertContains(t, usersFile, "Name:    \"idx_users_created_id\"")
	assertContains(t, usersFile, "Columns: []string{\"created_at\", \"id\"}")
	assertNotContains(t, eventsFile, "idx_events_lower_title")
	assertContains(t, eventsFile, "DeletedAt gorm.DeletedAt")
}

func TestIntegration_SQLIntrospector_TableFilterAndMissing(t *testing.T) {
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
	)
	if err != nil {
		t.Fatalf("runGenerator(filtered) error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if !fileExists(filepath.Join(outDir, "user_model_gen.go")) {
		t.Fatal("user_model_gen.go should exist in filtered generation")
	}
	if fileExists(filepath.Join(outDir, "event_model_gen.go")) {
		t.Fatal("event_model_gen.go should not exist when --tables=users")
	}

	_, stderrMissing, err := runGenerator(
		ctx,
		t,
		"--dsn", mustPostgresDSN(t),
		"--schema", integrationSchemaPG,
		"--tables", "users,missing_table",
		"--out-dir", filepath.Join(outDir, "missing"),
		"--package", "model",
	)
	if err == nil {
		t.Fatal("runGenerator(missing table) error = nil, want error")
	}
	if !strings.Contains(stderrMissing, "tables not found") &&
		!strings.Contains(stderrMissing, "missing_table") {
		t.Fatalf("missing table stderr = %q, want missing table hint", stderrMissing)
	}
}

func TestIntegration_SQLIntrospector_IndexOrderingStable(t *testing.T) {
	t.Parallel()
	ctx, cancel := integrationContext(t)
	defer cancel()

	cases := []struct {
		name   string
		dsn    string
		schema string
	}{
		{
			name:   "mysql",
			dsn:    mustMySQLDSN(t),
			schema: integrationDBName,
		},
		{
			name:   "postgres",
			dsn:    mustPostgresDSN(t),
			schema: integrationSchemaPG,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			outDir := t.TempDir()
			stdout, stderr, err := runGenerator(
				ctx,
				t,
				"--dsn", tc.dsn,
				"--schema", tc.schema,
				"--tables", "users",
				"--out-dir", outDir,
				"--package", "model",
				"--timestamp-mode", "unix_nano",
			)
			if err != nil {
				t.Fatalf(
					"runGenerator(%s order stable) error = %v\nstdout:\n%s\nstderr:\n%s",
					tc.name,
					err,
					stdout,
					stderr,
				)
			}

			usersFile := mustReadFile(t, filepath.Join(outDir, "user_model_gen.go"))
			assertContains(t, usersFile, "Name:    \"idx_users_created_id\"")
			assertContains(t, usersFile, "Columns: []string{\"created_at\", \"id\"}")
			assertNotContains(t, usersFile, "Columns: []string{\"id\", \"created_at\"}")
		})
	}
}

func TestIntegration_SQLIntrospector_IndexSetAffectsSortable(t *testing.T) {
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
			"runGenerator(indexed sortable) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout,
			stderr,
		)
	}

	usersFile := mustReadFile(t, filepath.Join(outDir, "user_model_gen.go"))
	assertContains(
		t,
		usersFile,
		"WithDatabaseName(\"created_at\").\n\t\t\tFilterable().\n\t\t\tSortable().",
	)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
