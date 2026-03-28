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

package jetstream

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
)

func TestRuntimeSequentialWithinShard(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	rt := New(ctx, 2, 4, func(_ context.Context, msgCtx *consume.MessageContext) error {
		switch msgCtx.Subject {
		case "first":
			close(firstStarted)
			<-releaseFirst
		case "second":
			close(secondStarted)
		}
		return nil
	})
	defer rt.Shutdown()

	require.NoError(t, rt.Enqueue(&consume.MessageContext{Subject: "first", Shard: 0}))
	require.NoError(t, rt.Enqueue(&consume.MessageContext{Subject: "second", Shard: 0}))

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first task to start")
	}

	select {
	case <-secondStarted:
		t.Fatal("second task started before first task finished")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second task to start")
	}
}

func TestRuntimeParallelAcrossShards(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	rt := New(ctx, 2, 4, func(_ context.Context, msgCtx *consume.MessageContext) error {
		switch msgCtx.Subject {
		case "first":
			close(firstStarted)
			<-releaseFirst
		case "second":
			close(secondStarted)
		}
		return nil
	})
	defer rt.Shutdown()

	require.NoError(t, rt.Enqueue(&consume.MessageContext{Subject: "first", Shard: 0}))
	require.NoError(t, rt.Enqueue(&consume.MessageContext{Subject: "second", Shard: 1}))

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first task to start")
	}

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second shard task to start")
	}

	close(releaseFirst)
}

func TestRuntimeWorkerErrorStopsRuntime(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt := New(ctx, 1, 1, func(context.Context, *consume.MessageContext) error {
		return errors.New("worker failed")
	})
	defer rt.Shutdown()

	require.NoError(t, rt.Enqueue(&consume.MessageContext{Shard: 0}))
	require.Eventually(t, func() bool {
		return rt.FatalErr() != nil
	}, time.Second, 10*time.Millisecond)
	require.EqualError(t, rt.FatalErr(), "worker failed")
}

func TestRuntimeEnqueueInvalidShard(t *testing.T) {
	t.Parallel()

	rt := New(context.Background(), 1, 1, func(context.Context, *consume.MessageContext) error {
		return nil
	})
	defer rt.Shutdown()

	err := rt.Enqueue(&consume.MessageContext{Shard: 2})
	require.EqualError(t, err, "invalid shard 2")
}
