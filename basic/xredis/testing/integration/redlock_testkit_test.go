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
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type redlockHarness struct {
	addrs []string
	close func(context.Context) error
}

func startRedlockHarness(t *testing.T, count int) *redlockHarness {
	t.Helper()
	require.GreaterOrEqual(t, count, 3)

	ctx, cancel := integrationContext(t)
	defer cancel()

	containers := make([]*redisHarness, 0, count)
	addrs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		harness, err := startRedisHarness(ctx)
		require.NoError(t, err)
		containers = append(containers, harness)
		addrs = append(addrs, harness.addr)
	}

	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := integrationContext(t)
		defer shutdownCancel()
		for _, harness := range containers {
			require.NoError(t, harness.Close(shutdownCtx))
		}
	})

	return &redlockHarness{
		addrs: addrs,
		close: func(ctx context.Context) error {
			var closeErr error
			for _, harness := range containers {
				if err := harness.Close(ctx); err != nil && closeErr == nil {
					closeErr = err
				}
			}
			return closeErr
		},
	}
}

func (h *redlockHarness) clients(t *testing.T) []redis.UniversalClient {
	t.Helper()

	clients := make([]redis.UniversalClient, 0, len(h.addrs))
	for _, addr := range h.addrs {
		client := redis.NewUniversalClient(&redis.UniversalOptions{Addrs: []string{addr}})
		t.Cleanup(func() {
			require.NoError(t, client.Close())
		})
		clients = append(clients, client)
	}
	return clients
}
