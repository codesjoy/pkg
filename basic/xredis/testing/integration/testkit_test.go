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
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const (
	redisImage           = "redis:7"
	redisStartupTimeout  = 2 * time.Minute
	redisShutdownTimeout = 30 * time.Second
	integrationTimeout   = 60 * time.Second
)

var integrationHarness *redisHarness

type redisHarness struct {
	container *tcredis.RedisContainer
	addr      string
}

func TestMain(m *testing.M) {
	startupCtx, startupCancel := context.WithTimeout(context.Background(), redisStartupTimeout)
	defer startupCancel()

	harness, err := startRedisHarness(startupCtx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start redis harness: %v\n", err)
		os.Exit(1)
	}
	integrationHarness = harness

	exitCode := m.Run()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), redisShutdownTimeout)
	defer shutdownCancel()
	if closeErr := integrationHarness.Close(shutdownCtx); closeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to stop redis harness: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

func startRedisHarness(ctx context.Context) (*redisHarness, error) {
	container, err := tcredis.Run(ctx, redisImage)
	if err != nil {
		return nil, err
	}

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, err
	}

	opts, err := redis.ParseURL(connStr)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, err
	}

	return &redisHarness{container: container, addr: opts.Addr}, nil
}

func (h *redisHarness) Close(_ context.Context) error {
	if h == nil || h.container == nil {
		return nil
	}
	return testcontainers.TerminateContainer(h.container)
}

func integrationContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), integrationTimeout)
}

func mustAddr(t *testing.T) string {
	t.Helper()
	require.NotNil(t, integrationHarness)
	require.NotEmpty(t, integrationHarness.addr)
	return integrationHarness.addr
}
