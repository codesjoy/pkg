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
	"fmt"

	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type optionState struct {
	nativeOptions   []*options.ClientOptions
	commandMonitors []*event.CommandMonitor
	poolMonitors    []*event.PoolMonitor
	serverMonitors  []*event.ServerMonitor
	healthTracker   *healthTracker
	healthEnabled   bool
}

type optionFunc func(*optionState) error

func (f optionFunc) apply(state *optionState) error {
	return f(state)
}

func buildClientOptions(cfg Config, opts ...Option) (*options.ClientOptions, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	state, err := buildOptionState(opts...)
	if err != nil {
		return nil, err
	}

	return buildClientOptionsWithState(cfg, state)
}

func buildClientOptionsWithState(
	cfg Config,
	state *optionState,
) (*options.ClientOptions, error) {
	if state == nil {
		state = &optionState{}
	}

	merged, err := mergeNativeClientOptions(
		cfg.URI,
		append(append([]*options.ClientOptions(nil), cfg.ClientOptions...), state.nativeOptions...),
	)
	if err != nil {
		return nil, err
	}

	merged = applyAppendedMonitors(merged, state)
	if err := merged.Validate(); err != nil {
		return nil, err
	}
	return merged, nil
}

func buildOptionState(opts ...Option) (*optionState, error) {
	state := &optionState{}
	for idx, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt.apply(state); err != nil {
			return nil, fmt.Errorf("apply option #%d: %w", idx, err)
		}
	}
	return state, nil
}

func mergeNativeClientOptions(
	uri string,
	clientOptions []*options.ClientOptions,
) (*options.ClientOptions, error) {
	ordered := make([]*options.ClientOptions, 0, len(clientOptions)+1)
	ordered = append(ordered, options.Client().ApplyURI(uri))
	for idx, opt := range clientOptions {
		if opt == nil {
			return nil, fmt.Errorf("%w at index %d", ErrNilClientOption, idx)
		}
		ordered = append(ordered, opt)
	}
	return options.MergeClientOptions(ordered...), nil
}

func applyAppendedMonitors(
	merged *options.ClientOptions,
	state *optionState,
) *options.ClientOptions {
	if merged == nil || state == nil {
		return merged
	}

	commandMonitor := composeCommandMonitor(
		append([]*event.CommandMonitor{merged.Monitor}, state.commandMonitors...)...,
	)
	if commandMonitor != nil {
		merged.SetMonitor(commandMonitor)
	}

	poolMonitor := composePoolMonitor(
		append([]*event.PoolMonitor{merged.PoolMonitor}, state.poolMonitors...)...,
	)
	if poolMonitor != nil {
		merged.SetPoolMonitor(poolMonitor)
	}

	serverMonitor := composeServerMonitor(
		append([]*event.ServerMonitor{merged.ServerMonitor}, state.serverMonitors...)...,
	)
	if serverMonitor != nil {
		merged.SetServerMonitor(serverMonitor)
	}

	return merged
}

func composeCommandMonitor(monitors ...*event.CommandMonitor) *event.CommandMonitor {
	nonNil := compactNonNil(monitors)
	switch len(nonNil) {
	case 0:
		return nil
	case 1:
		return nonNil[0]
	}

	return &event.CommandMonitor{
		Started: collectCommandHandlers(
			nonNil,
			func(monitor *event.CommandMonitor) func(context.Context, *event.CommandStartedEvent) {
				return monitor.Started
			},
		),
		Succeeded: collectCommandHandlers(
			nonNil,
			func(monitor *event.CommandMonitor) func(context.Context, *event.CommandSucceededEvent) {
				return monitor.Succeeded
			},
		),
		Failed: collectCommandHandlers(
			nonNil,
			func(monitor *event.CommandMonitor) func(context.Context, *event.CommandFailedEvent) {
				return monitor.Failed
			},
		),
	}
}

func composePoolMonitor(monitors ...*event.PoolMonitor) *event.PoolMonitor {
	nonNil := compactNonNil(monitors)
	switch len(nonNil) {
	case 0:
		return nil
	case 1:
		return nonNil[0]
	}

	return &event.PoolMonitor{
		Event: collectEventHandlers(
			nonNil,
			func(monitor *event.PoolMonitor) func(*event.PoolEvent) {
				return monitor.Event
			},
		),
	}
}

func composeServerMonitor(monitors ...*event.ServerMonitor) *event.ServerMonitor {
	nonNil := compactNonNil(monitors)
	switch len(nonNil) {
	case 0:
		return nil
	case 1:
		return nonNil[0]
	}

	return &event.ServerMonitor{
		ServerDescriptionChanged: collectEventHandlers(
			nonNil,
			func(monitor *event.ServerMonitor) func(*event.ServerDescriptionChangedEvent) {
				return monitor.ServerDescriptionChanged
			},
		),
		ServerOpening: collectEventHandlers(
			nonNil,
			func(monitor *event.ServerMonitor) func(*event.ServerOpeningEvent) {
				return monitor.ServerOpening
			},
		),
		ServerClosed: collectEventHandlers(
			nonNil,
			func(monitor *event.ServerMonitor) func(*event.ServerClosedEvent) {
				return monitor.ServerClosed
			},
		),
		TopologyDescriptionChanged: collectEventHandlers(
			nonNil,
			func(monitor *event.ServerMonitor) func(*event.TopologyDescriptionChangedEvent) {
				return monitor.TopologyDescriptionChanged
			},
		),
		TopologyOpening: collectEventHandlers(
			nonNil,
			func(monitor *event.ServerMonitor) func(*event.TopologyOpeningEvent) {
				return monitor.TopologyOpening
			},
		),
		TopologyClosed: collectEventHandlers(
			nonNil,
			func(monitor *event.ServerMonitor) func(*event.TopologyClosedEvent) {
				return monitor.TopologyClosed
			},
		),
		ServerHeartbeatStarted: collectEventHandlers(
			nonNil,
			func(monitor *event.ServerMonitor) func(*event.ServerHeartbeatStartedEvent) {
				return monitor.ServerHeartbeatStarted
			},
		),
		ServerHeartbeatSucceeded: collectEventHandlers(
			nonNil,
			func(monitor *event.ServerMonitor) func(*event.ServerHeartbeatSucceededEvent) {
				return monitor.ServerHeartbeatSucceeded
			},
		),
		ServerHeartbeatFailed: collectEventHandlers(
			nonNil,
			func(monitor *event.ServerMonitor) func(*event.ServerHeartbeatFailedEvent) {
				return monitor.ServerHeartbeatFailed
			},
		),
	}
}

func compactNonNil[T any](values []*T) []*T {
	compacted := make([]*T, 0, len(values))
	for _, value := range values {
		if value != nil {
			compacted = append(compacted, value)
		}
	}
	return compacted
}

func collectCommandHandlers[T any](
	monitors []*event.CommandMonitor,
	pick func(*event.CommandMonitor) func(context.Context, *T),
) func(context.Context, *T) {
	handlers := make([]func(context.Context, *T), 0, len(monitors))
	for _, monitor := range monitors {
		if handler := pick(monitor); handler != nil {
			handlers = append(handlers, handler)
		}
	}
	return func(ctx context.Context, evt *T) {
		for _, handler := range handlers {
			handler(ctx, evt)
		}
	}
}

func collectEventHandlers[M any, E any](
	monitors []*M,
	pick func(*M) func(*E),
) func(*E) {
	handlers := make([]func(*E), 0, len(monitors))
	for _, monitor := range monitors {
		if handler := pick(monitor); handler != nil {
			handlers = append(handlers, handler)
		}
	}
	return func(evt *E) {
		for _, handler := range handlers {
			handler(evt)
		}
	}
}
