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
	"github.com/codesjoy/pkg/basic/xredis/lock"
)

func TestClusterLockLifecycle(t *testing.T) {
	harness := startClusterHarness(t)

	client, err := xredis.New(xredis.Config{UniversalOptions: *harness.options})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	locker, err := lock.New(client, lock.Config{
		RetryInterval: 20 * time.Millisecond,
		RetryJitter:   0,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	lease, err := locker.Acquire(
		ctx,
		"it:ha:cluster:lock",
		1200*time.Millisecond,
		lock.WithAutoRenewInterval(300*time.Millisecond),
	)
	require.NoError(t, err)

	time.Sleep(1800 * time.Millisecond)

	_, err = locker.TryAcquire(ctx, "it:ha:cluster:lock", 1200*time.Millisecond)
	require.ErrorIs(t, err, lock.ErrNotObtained)

	require.NoError(t, lease.Release(ctx))

	reacquired, err := locker.TryAcquire(ctx, "it:ha:cluster:lock", 1200*time.Millisecond)
	require.NoError(t, err)
	require.NoError(t, reacquired.Release(ctx))
}
