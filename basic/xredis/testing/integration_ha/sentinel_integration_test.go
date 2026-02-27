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

//go:build integration_ha

package integration_ha

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xredis"
)

func TestSentinelTopology(t *testing.T) {
	harness := startSentinelHarness(t)

	client, err := xredis.New(xredis.Config{UniversalOptions: *harness.options})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	retryUntil(t, 40, 500*time.Millisecond, func() error {
		return client.Set(ctx, "it:ha:sentinel", "value", 0).Err()
	})

	value, err := client.Get(ctx, "it:ha:sentinel").Result()
	require.NoError(t, err)
	require.Equal(t, "value", value)
}
