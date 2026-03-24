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
	"fmt"

	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	logmiddleware "github.com/codesjoy/pkg/basic/xmongo/middleware/logger"
	otelmiddleware "github.com/codesjoy/pkg/basic/xmongo/middleware/otel"
)

// Option customizes client construction behavior.
type Option interface {
	apply(*optionState) error
}

// WithClientOptions appends native mongo driver options in the same order as
// arguments. Later options override earlier ones following driver semantics.
func WithClientOptions(opts ...*options.ClientOptions) Option {
	copied := append([]*options.ClientOptions(nil), opts...)
	return optionFunc(func(state *optionState) error {
		return appendValidatedOptions(&state.nativeOptions, copied, ErrNilClientOption)
	})
}

// WithCommandMonitor appends command monitors in the same order as arguments.
func WithCommandMonitor(monitors ...*event.CommandMonitor) Option {
	copied := append([]*event.CommandMonitor(nil), monitors...)
	return optionFunc(func(state *optionState) error {
		return appendValidatedOptions(&state.commandMonitors, copied, ErrNilCommandMonitor)
	})
}

// WithPoolMonitor appends pool monitors in the same order as arguments.
func WithPoolMonitor(monitors ...*event.PoolMonitor) Option {
	copied := append([]*event.PoolMonitor(nil), monitors...)
	return optionFunc(func(state *optionState) error {
		return appendValidatedOptions(&state.poolMonitors, copied, ErrNilPoolMonitor)
	})
}

// WithServerMonitor appends server monitors in the same order as arguments.
func WithServerMonitor(monitors ...*event.ServerMonitor) Option {
	copied := append([]*event.ServerMonitor(nil), monitors...)
	return optionFunc(func(state *optionState) error {
		return appendValidatedOptions(&state.serverMonitors, copied, ErrNilServerMonitor)
	})
}

// WithOpenTelemetry appends a command monitor backed by otelmongo tracing.
func WithOpenTelemetry(cfg otelmiddleware.Config) Option {
	copied := cfg
	return optionFunc(func(state *optionState) error {
		monitors, err := otelmiddleware.NewMonitors(copied)
		if err != nil {
			return err
		}
		if monitors.Command != nil {
			state.commandMonitors = append(state.commandMonitors, monitors.Command)
		}
		if monitors.Pool != nil {
			state.poolMonitors = append(state.poolMonitors, monitors.Pool)
		}
		return nil
	})
}

// WithLogger appends slog-based command, pool, and server monitors.
func WithLogger(cfg logmiddleware.Config) Option {
	copied := cfg
	return optionFunc(func(state *optionState) error {
		monitors := logmiddleware.New(copied)
		if monitors.Command != nil {
			state.commandMonitors = append(state.commandMonitors, monitors.Command)
		}
		if monitors.Pool != nil {
			state.poolMonitors = append(state.poolMonitors, monitors.Pool)
		}
		if monitors.Server != nil {
			state.serverMonitors = append(state.serverMonitors, monitors.Server)
		}
		return nil
	})
}

// WithHealthTracking enables lightweight health snapshot tracking.
func WithHealthTracking() Option {
	return optionFunc(func(state *optionState) error {
		if state.healthEnabled {
			return nil
		}
		if state.healthTracker == nil {
			state.healthTracker = newHealthTracker()
		}
		state.healthEnabled = true
		if monitor := state.healthTracker.poolMonitor(); monitor != nil {
			state.poolMonitors = append(state.poolMonitors, monitor)
		}
		if monitor := state.healthTracker.serverMonitor(); monitor != nil {
			state.serverMonitors = append(state.serverMonitors, monitor)
		}
		return nil
	})
}

func appendValidatedOptions[T any](dst *[]*T, values []*T, nilErr error) error {
	for idx, value := range values {
		if value == nil {
			return fmt.Errorf("%w at index %d", nilErr, idx)
		}
		*dst = append(*dst, value)
	}
	return nil
}
