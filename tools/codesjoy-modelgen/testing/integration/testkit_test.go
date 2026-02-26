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
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	mysqlc "github.com/testcontainers/testcontainers-go/modules/mysql"
	postgresc "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	dialectMySQL    = "mysql"
	dialectPostgres = "postgres"

	mysqlDriverName    = "mysql"
	postgresDriverName = "pgx"

	integrationMySQLImage    = "mysql:8.4"
	integrationPostgresImage = "postgres:15-alpine"

	integrationDBName     = "modelgen_it"
	integrationDBUser     = "modelgen"
	integrationDBPassword = "modelgen"
	integrationSchemaPG   = "public"

	integrationStartupTimeout  = 3 * time.Minute
	integrationShutdownTimeout = 45 * time.Second
	integrationCaseTimeout     = 90 * time.Second
)

type integrationHarness struct {
	mysqlDSN     string
	postgresDSN  string
	generatorBin string
	terminate    func(context.Context) error
}

var globalIntegrationHarness *integrationHarness

func TestMain(m *testing.M) {
	startCtx, startCancel := context.WithTimeout(context.Background(), integrationStartupTimeout)
	defer startCancel()

	harness, err := startIntegrationHarness(startCtx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start integration harness: %v\n", err)
		os.Exit(1)
	}
	globalIntegrationHarness = harness

	exitCode := m.Run()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), integrationShutdownTimeout)
	defer stopCancel()
	if closeErr := globalIntegrationHarness.close(stopCtx); closeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to stop integration harness: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

func startIntegrationHarness(ctx context.Context) (*integrationHarness, error) {
	generatorBin, generatorBinDir, err := buildGeneratorBinary(ctx)
	if err != nil {
		return nil, err
	}

	mysqlContainer, mysqlDSN, mysqlDB, err := startMySQLContainer(ctx)
	if err != nil {
		return nil, errors.Join(err, os.RemoveAll(generatorBinDir))
	}

	postgresContainer, postgresDSN, postgresDB, err := startPostgresContainer(ctx)
	if err != nil {
		return nil, errors.Join(
			err,
			mysqlDB.Close(),
			os.RemoveAll(generatorBinDir),
			testcontainers.TerminateContainer(mysqlContainer),
		)
	}

	if err := prepareIntegrationSchema(ctx, mysqlDB, dialectMySQL); err != nil {
		return nil, errors.Join(
			err,
			mysqlDB.Close(),
			postgresDB.Close(),
			os.RemoveAll(generatorBinDir),
			testcontainers.TerminateContainer(mysqlContainer),
			testcontainers.TerminateContainer(postgresContainer),
		)
	}
	if err := prepareIntegrationSchema(ctx, postgresDB, dialectPostgres); err != nil {
		return nil, errors.Join(
			err,
			mysqlDB.Close(),
			postgresDB.Close(),
			os.RemoveAll(generatorBinDir),
			testcontainers.TerminateContainer(mysqlContainer),
			testcontainers.TerminateContainer(postgresContainer),
		)
	}

	return &integrationHarness{
		mysqlDSN:     mysqlDSN,
		postgresDSN:  postgresDSN,
		generatorBin: generatorBin,
		terminate: func(_ context.Context) error {
			return errors.Join(
				mysqlDB.Close(),
				postgresDB.Close(),
				os.RemoveAll(generatorBinDir),
				testcontainers.TerminateContainer(mysqlContainer),
				testcontainers.TerminateContainer(postgresContainer),
			)
		},
	}, nil
}

func buildGeneratorBinary(ctx context.Context) (string, string, error) {
	binDir, err := os.MkdirTemp("", "codesjoy-modelgen-integration-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp bin dir: %w", err)
	}

	binName := "codesjoy-modelgen"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(binDir, binName)

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	cmd.Dir = integrationModuleRoot()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", errors.Join(
			fmt.Errorf("build integration generator binary: %w", err),
			fmt.Errorf("go build output: %s", strings.TrimSpace(string(output))),
			os.RemoveAll(binDir),
		)
	}

	return binPath, binDir, nil
}

func startMySQLContainer(
	ctx context.Context,
) (
	*mysqlc.MySQLContainer,
	string,
	*sql.DB,
	error,
) {
	container, err := mysqlc.Run(
		ctx,
		integrationMySQLImage,
		mysqlc.WithDatabase(integrationDBName),
		mysqlc.WithUsername(integrationDBUser),
		mysqlc.WithPassword(integrationDBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("ready for connections").
				WithOccurrence(1).
				WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		return nil, "", nil, fmt.Errorf("start mysql container: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "parseTime=true", "multiStatements=true")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, "", nil, fmt.Errorf("resolve mysql dsn: %w", err)
	}

	db, err := sql.Open(mysqlDriverName, dsn)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, "", nil, fmt.Errorf("open mysql connection: %w", err)
	}

	if err := waitForIntegrationDB(ctx, db); err != nil {
		_ = db.Close()
		_ = testcontainers.TerminateContainer(container)
		return nil, "", nil, fmt.Errorf("wait mysql ready: %w", err)
	}

	return container, dsn, db, nil
}

func startPostgresContainer(
	ctx context.Context,
) (
	*postgresc.PostgresContainer,
	string,
	*sql.DB,
	error,
) {
	container, err := postgresc.Run(
		ctx,
		integrationPostgresImage,
		postgresc.WithDatabase(integrationDBName),
		postgresc.WithUsername(integrationDBUser),
		postgresc.WithPassword(integrationDBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		return nil, "", nil, fmt.Errorf("start postgres container: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, "", nil, fmt.Errorf("resolve postgres dsn: %w", err)
	}

	db, err := sql.Open(postgresDriverName, dsn)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, "", nil, fmt.Errorf("open postgres connection: %w", err)
	}

	if err := waitForIntegrationDB(ctx, db); err != nil {
		_ = db.Close()
		_ = testcontainers.TerminateContainer(container)
		return nil, "", nil, fmt.Errorf("wait postgres ready: %w", err)
	}

	return container, dsn, db, nil
}

func prepareIntegrationSchema(ctx context.Context, db *sql.DB, dialect string) error {
	statements := integrationPostgresSchemaStatements
	if dialect == dialectMySQL {
		statements = integrationMySQLSchemaStatements
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("execute schema statement: %w", err)
		}
	}
	return nil
}

var integrationMySQLSchemaStatements = []string{
	`DROP TABLE IF EXISTS events`,
	`DROP TABLE IF EXISTS users`,
	`CREATE TABLE users (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(255) NULL,
		email VARCHAR(255) NOT NULL,
		is_active TINYINT(1) NOT NULL DEFAULT 1,
		created_at BIGINT NOT NULL,
		` + "`update`" + ` BIGINT NOT NULL,
		deleted_at BIGINT NULL,
		UNIQUE KEY uk_users_email (email),
		KEY idx_users_created_at (created_at),
		KEY idx_users_created_id (created_at, id),
		KEY idx_users_deleted_at (deleted_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE events (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		user_id BIGINT NOT NULL,
		title VARCHAR(255) NOT NULL,
		deleted_at DATETIME NULL,
		created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
		KEY idx_events_user_deleted (user_id, deleted_at),
		KEY idx_events_deleted_at (deleted_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
}

var integrationPostgresSchemaStatements = []string{
	`DROP TABLE IF EXISTS events`,
	`DROP TABLE IF EXISTS users`,
	`CREATE TABLE users (
		id BIGSERIAL PRIMARY KEY,
		name VARCHAR(255) NULL,
		email VARCHAR(255) NOT NULL,
		is_active BOOLEAN NOT NULL DEFAULT true,
		created_at BIGINT NOT NULL,
		"update" BIGINT NOT NULL,
		deleted_at BIGINT NULL
	)`,
	`CREATE UNIQUE INDEX uk_users_email ON users(email)`,
	`CREATE INDEX idx_users_created_at ON users(created_at)`,
	`CREATE INDEX idx_users_created_id ON users(created_at, id)`,
	`CREATE INDEX idx_users_deleted_at ON users(deleted_at)`,
	`CREATE TABLE events (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL,
		title VARCHAR(255) NOT NULL,
		deleted_at TIMESTAMP NULL,
		created_at TIMESTAMPTZ NULL DEFAULT NOW()
	)`,
	`CREATE INDEX idx_events_user_deleted ON events(user_id, deleted_at)`,
	`CREATE INDEX idx_events_deleted_at ON events(deleted_at)`,
	`CREATE INDEX idx_events_lower_title ON events ((lower(title)))`,
}

func waitForIntegrationDB(ctx context.Context, db *sql.DB) error {
	deadline := time.Now().Add(45 * time.Second)
	for {
		if err := db.PingContext(ctx); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("database did not become ready before timeout")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (h *integrationHarness) close(ctx context.Context) error {
	if h == nil || h.terminate == nil {
		return nil
	}
	return h.terminate(ctx)
}

func integrationContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), integrationCaseTimeout)
}

func mustMySQLDSN(t *testing.T) string {
	t.Helper()
	if globalIntegrationHarness == nil || strings.TrimSpace(globalIntegrationHarness.mysqlDSN) == "" {
		t.Fatal("mysql integration harness is not ready")
	}
	return globalIntegrationHarness.mysqlDSN
}

func mustPostgresDSN(t *testing.T) string {
	t.Helper()
	if globalIntegrationHarness == nil ||
		strings.TrimSpace(globalIntegrationHarness.postgresDSN) == "" {
		t.Fatal("postgres integration harness is not ready")
	}
	return globalIntegrationHarness.postgresDSN
}

func mustGeneratorBin(t *testing.T) string {
	t.Helper()
	if globalIntegrationHarness == nil ||
		strings.TrimSpace(globalIntegrationHarness.generatorBin) == "" {
		t.Fatal("generator binary is not ready")
	}
	return globalIntegrationHarness.generatorBin
}

func integrationModuleRoot() string {
	root := filepath.Clean(filepath.Join("..", ".."))
	absRoot, err := filepath.Abs(root)
	if err == nil {
		return absRoot
	}
	return root
}

func integrationOverridePath(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(
		integrationModuleRoot(),
		"testing",
		"integration",
		"testdata",
		"overrides",
		name,
	)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("override fixture %s not found: %v", path, err)
	}
	return path
}

func runGenerator(
	ctx context.Context,
	t *testing.T,
	args ...string,
) (stdout string, stderr string, err error) {
	t.Helper()
	cmd := exec.CommandContext(ctx, mustGeneratorBin(t), args...)
	cmd.Dir = integrationModuleRoot()
	var outBuf strings.Builder
	var errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

func assertContains(t *testing.T, text string, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("content missing %q\nfull content:\n%s", want, text)
	}
}

func assertNotContains(t *testing.T, text string, unexpected string) {
	t.Helper()
	if strings.Contains(text, unexpected) {
		t.Fatalf("content unexpectedly contains %q\nfull content:\n%s", unexpected, text)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return string(content)
}
