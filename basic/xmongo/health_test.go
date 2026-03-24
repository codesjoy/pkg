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

package xmongo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func TestHealthSnapshotZeroValue(t *testing.T) {
	t.Parallel()

	client := &Client{}
	require.Equal(t, HealthSnapshot{}, client.HealthSnapshot())
}

func TestWithHealthTrackingIsIdempotent(t *testing.T) {
	t.Parallel()

	state, err := buildOptionState(WithHealthTracking(), WithHealthTracking())
	require.NoError(t, err)
	require.NotNil(t, state.healthTracker)
	require.Len(t, state.poolMonitors, 1)
	require.Len(t, state.serverMonitors, 1)
}

func TestPingPrimaryUpdatesHealthSnapshot(t *testing.T) {
	original := pingClient
	defer func() {
		pingClient = original
	}()

	boom := errors.New("boom")
	pingClient = func(*mongo.Client, context.Context, *readpref.ReadPref) error {
		return boom
	}

	client := &Client{Client: &mongo.Client{}, healthTracker: newHealthTracker()}
	err := client.PingPrimary(context.Background())
	require.ErrorIs(t, err, boom)

	snapshot := client.HealthSnapshot()
	require.NotZero(t, snapshot.LastPingAt)
	require.ErrorIs(t, snapshot.LastPingErr, boom)
	require.NotZero(t, snapshot.UpdatedAt)
}

func TestHealthSnapshotUpdatesFromMonitors(t *testing.T) {
	t.Parallel()

	tracker := newHealthTracker()
	tracker.poolMonitor().Event(&event.PoolEvent{Type: event.ConnectionPoolCleared})
	tracker.serverMonitor().ServerHeartbeatFailed(&event.ServerHeartbeatFailedEvent{
		Failure: errors.New("heartbeat failed"),
	})

	snapshot := tracker.snapshot()
	require.Equal(t, event.ConnectionPoolCleared, snapshot.LastPoolEventType)
	require.NotZero(t, snapshot.LastPoolEventAt)
	require.Equal(t, "heartbeat failed", snapshot.LastHeartbeatErr)
	require.NotZero(t, snapshot.LastHeartbeatAt)
	require.NotZero(t, snapshot.UpdatedAt)
}

func TestPingPrimaryUsesPrimaryReadPreference(t *testing.T) {
	original := pingClient
	defer func() {
		pingClient = original
	}()

	var gotClient *mongo.Client
	var gotReadPref *readpref.ReadPref
	pingClient = func(client *mongo.Client, _ context.Context, rp *readpref.ReadPref) error {
		gotClient = client
		gotReadPref = rp
		return nil
	}

	client := &Client{Client: &mongo.Client{}}
	require.NoError(t, client.PingPrimary(context.Background()))
	require.Same(t, client.Client, gotClient)
	require.NotNil(t, gotReadPref)
	require.Equal(t, readpref.PrimaryMode, gotReadPref.Mode())
}

func TestPingPrimaryNilClient(t *testing.T) {
	t.Parallel()

	var client *Client
	require.ErrorIs(t, client.PingPrimary(context.Background()), mongo.ErrClientDisconnected)
}
