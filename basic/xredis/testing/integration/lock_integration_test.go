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
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xredis"
	"github.com/codesjoy/pkg/basic/xredis/lock"
)

func TestStandaloneLockLifecycle(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client1, err := xredis.New(
		xredis.Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mustAddr(t)}}},
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client1.Close())
	})

	client2, err := xredis.New(
		xredis.Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mustAddr(t)}}},
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client2.Close())
	})

	locker1, err := lock.New(client1, lock.Config{
		RetryInterval: 10 * time.Millisecond,
		RetryJitter:   0,
	})
	require.NoError(t, err)

	locker2, err := lock.New(client2, lock.Config{
		RetryInterval: 10 * time.Millisecond,
		RetryJitter:   0,
	})
	require.NoError(t, err)

	lease1, err := locker1.Acquire(
		ctx,
		"it:standalone:lock",
		400*time.Millisecond,
		lock.WithAutoRenew(),
	)
	require.NoError(t, err)

	time.Sleep(800 * time.Millisecond)

	_, err = locker2.TryAcquire(ctx, "it:standalone:lock", 400*time.Millisecond)
	require.ErrorIs(t, err, lock.ErrNotObtained)

	lease2Ch := make(chan *lock.Lease, 1)
	errCh := make(chan error, 1)
	go func() {
		lease, acquireErr := locker2.Acquire(ctx, "it:standalone:lock", 400*time.Millisecond)
		if acquireErr != nil {
			errCh <- acquireErr
			return
		}
		lease2Ch <- lease
	}()

	require.NoError(t, lease1.Release(ctx))

	select {
	case acquireErr := <-errCh:
		require.NoError(t, acquireErr)
	case lease2 := <-lease2Ch:
		require.NotNil(t, lease2)
		require.NoError(t, lease2.Release(ctx))
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second locker")
	}
}
