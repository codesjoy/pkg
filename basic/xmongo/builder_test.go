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
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	otelmiddleware "github.com/codesjoy/pkg/basic/xmongo/middleware/otel"
)

func TestBuildClientOptionsAppliesNativeOptionOrder(t *testing.T) {
	t.Parallel()

	merged, err := buildClientOptions(
		Config{
			URI: "mongodb://127.0.0.1:27017",
			ClientOptions: []*options.ClientOptions{
				options.Client().SetAppName("cfg"),
			},
		},
		WithClientOptions(options.Client().SetAppName("first")),
		WithClientOptions(options.Client().SetAppName("last")),
	)
	require.NoError(t, err)
	require.NotNil(t, merged.AppName)
	require.Equal(t, "last", *merged.AppName)
}

func TestBuildClientOptionsAppendsMonitorsAfterNativeBaseline(t *testing.T) {
	t.Parallel()

	var events []string
	record := func(name string) *event.CommandMonitor {
		return &event.CommandMonitor{
			Started: func(context.Context, *event.CommandStartedEvent) {
				events = append(events, name+":started")
			},
			Succeeded: func(context.Context, *event.CommandSucceededEvent) {
				events = append(events, name+":succeeded")
			},
			Failed: func(context.Context, *event.CommandFailedEvent) {
				events = append(events, name+":failed")
			},
		}
	}

	merged, err := buildClientOptions(
		Config{
			URI: "mongodb://127.0.0.1:27017",
			ClientOptions: []*options.ClientOptions{
				options.Client().SetMonitor(record("baseline")),
			},
		},
		WithCommandMonitor(record("extra1"), record("extra2")),
	)
	require.NoError(t, err)
	require.NotNil(t, merged.Monitor)

	merged.Monitor.Started(context.Background(), &event.CommandStartedEvent{})
	merged.Monitor.Succeeded(context.Background(), &event.CommandSucceededEvent{})
	merged.Monitor.Failed(context.Background(), &event.CommandFailedEvent{})

	require.Equal(
		t,
		[]string{
			"baseline:started",
			"extra1:started",
			"extra2:started",
			"baseline:succeeded",
			"extra1:succeeded",
			"extra2:succeeded",
			"baseline:failed",
			"extra1:failed",
			"extra2:failed",
		},
		events,
	)
}

func TestBuildClientOptionsAppendsPoolAndServerMonitors(t *testing.T) {
	t.Parallel()

	var events []string

	merged, err := buildClientOptions(
		Config{
			URI: "mongodb://127.0.0.1:27017",
			ClientOptions: []*options.ClientOptions{
				options.Client().
					SetPoolMonitor(&event.PoolMonitor{
						Event: func(*event.PoolEvent) {
							events = append(events, "baseline-pool")
						},
					}).
					SetServerMonitor(&event.ServerMonitor{
						ServerHeartbeatStarted: func(*event.ServerHeartbeatStartedEvent) {
							events = append(events, "baseline-server")
						},
					}),
			},
		},
		WithPoolMonitor(&event.PoolMonitor{
			Event: func(*event.PoolEvent) {
				events = append(events, "extra-pool")
			},
		}),
		WithServerMonitor(&event.ServerMonitor{
			ServerHeartbeatStarted: func(*event.ServerHeartbeatStartedEvent) {
				events = append(events, "extra-server")
			},
		}),
	)
	require.NoError(t, err)

	require.NotNil(t, merged.PoolMonitor)
	merged.PoolMonitor.Event(&event.PoolEvent{})
	require.NotNil(t, merged.ServerMonitor)
	merged.ServerMonitor.ServerHeartbeatStarted(&event.ServerHeartbeatStartedEvent{})

	require.Equal(
		t,
		[]string{"baseline-pool", "extra-pool", "baseline-server", "extra-server"},
		events,
	)
}

func TestBuildClientOptionsRejectsNilMonitors(t *testing.T) {
	t.Parallel()

	_, err := buildClientOptions(
		Config{URI: "mongodb://127.0.0.1:27017"},
		WithCommandMonitor(nil),
	)
	require.ErrorIs(t, err, ErrNilCommandMonitor)

	_, err = buildClientOptions(
		Config{URI: "mongodb://127.0.0.1:27017"},
		WithPoolMonitor(nil),
	)
	require.ErrorIs(t, err, ErrNilPoolMonitor)

	_, err = buildClientOptions(
		Config{URI: "mongodb://127.0.0.1:27017"},
		WithServerMonitor(nil),
	)
	require.ErrorIs(t, err, ErrNilServerMonitor)
}

func TestWithOpenTelemetryAppendsCommandMonitor(t *testing.T) {
	t.Parallel()

	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	merged, err := buildClientOptions(
		Config{URI: "mongodb://127.0.0.1:27017"},
		WithOpenTelemetry(otelmiddleware.Config{
			TracerProvider:           provider,
			CommandAttributeDisabled: true,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, merged.Monitor)
}

func TestComposeCommandMonitorHandlesEmptyInput(t *testing.T) {
	t.Parallel()

	require.Nil(t, composeCommandMonitor(nil))
}

func TestBuildClientOptionsKeepsLaterNativeOptionOverrides(t *testing.T) {
	t.Parallel()

	merged, err := buildClientOptions(
		Config{
			URI: "mongodb://127.0.0.1:27017",
			ClientOptions: []*options.ClientOptions{
				options.Client().SetRetryWrites(false),
			},
		},
		WithClientOptions(options.Client().SetRetryWrites(true)),
	)
	require.NoError(t, err)
	require.NotNil(t, merged.RetryWrites)
	require.True(t, *merged.RetryWrites)
}

func TestBuildClientOptionsSkipsNilOptionEntries(t *testing.T) {
	t.Parallel()

	merged, err := buildClientOptions(
		Config{URI: "mongodb://127.0.0.1:27017"},
		nil,
		WithClientOptions(options.Client().SetAppName("named")),
	)
	require.NoError(t, err)
	require.NotNil(t, merged.AppName)
	require.Equal(t, "named", *merged.AppName)
}

func TestMergeNativeClientOptionsRejectsNilEntries(t *testing.T) {
	t.Parallel()

	_, err := mergeNativeClientOptions("mongodb://127.0.0.1:27017", []*options.ClientOptions{nil})
	require.ErrorIs(t, err, ErrNilClientOption)
}
