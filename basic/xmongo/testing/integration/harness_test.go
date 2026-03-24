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
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcmongodb "github.com/testcontainers/testcontainers-go/modules/mongodb"
)

const (
	mongoImage           = "mongo:8.0.13"
	mongoStartupTimeout  = 2 * time.Minute
	mongoShutdownTimeout = 30 * time.Second
	integrationTimeout   = 60 * time.Second
)

var integrationHarness *mongoHarness

type mongoHarness struct {
	container *tcmongodb.MongoDBContainer
	uri       string
}

func TestMain(m *testing.M) {
	if err := ensureDockerConfig(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to prepare docker config: %v\n", err)
		os.Exit(1)
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), mongoStartupTimeout)
	defer startupCancel()

	harness, err := startMongoHarness(startupCtx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start mongo harness: %v\n", err)
		os.Exit(1)
	}
	integrationHarness = harness

	exitCode := m.Run()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), mongoShutdownTimeout)
	defer shutdownCancel()
	if closeErr := integrationHarness.Close(shutdownCtx); closeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to stop mongo harness: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

func startMongoHarness(ctx context.Context) (*mongoHarness, error) {
	container, err := tcmongodb.Run(ctx, mongoImage, tcmongodb.WithReplicaSet("rs0"))
	if err != nil {
		return nil, err
	}

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, err
	}
	uri, err = withDirectConnection(uri)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, err
	}

	return &mongoHarness{container: container, uri: uri}, nil
}

func (h *mongoHarness) Close(ctx context.Context) error {
	if h == nil || h.container == nil {
		return nil
	}
	return testcontainers.TerminateContainer(h.container)
}

func integrationContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), integrationTimeout)
}

func mustURI(t *testing.T) string {
	t.Helper()
	require.NotNil(t, integrationHarness)
	require.NotEmpty(t, integrationHarness.uri)
	return integrationHarness.uri
}

func ensureDockerConfig() error {
	if os.Getenv("DOCKER_CONFIG") != "" {
		return nil
	}

	dir := filepath.Join(os.TempDir(), "codex-empty-docker-config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0o644); err != nil {
		return err
	}

	return os.Setenv("DOCKER_CONFIG", dir)
}

func withDirectConnection(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	query := parsed.Query()
	query.Set("directConnection", "true")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
