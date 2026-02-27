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

package xredis

import (
	"fmt"
	"reflect"

	"github.com/redis/go-redis/v9"

	logmiddleware "github.com/codesjoy/pkg/basic/xredis/middleware/logger"
	otelmiddleware "github.com/codesjoy/pkg/basic/xredis/middleware/otel"
)

// Option customizes a client after it is constructed.
//
// Options are applied in the exact order they are passed to New.
type Option func(*Client) error

// WithHook appends hooks in the same order as arguments.
func WithHook(hooks ...redis.Hook) Option {
	return func(client *Client) error {
		if client == nil || client.UniversalClient == nil {
			return nil
		}
		for idx, hook := range hooks {
			if isNilHook(hook) {
				return fmt.Errorf("%w at index %d", ErrNilHook, idx)
			}
			client.AddHook(hook)
		}
		return nil
	}
}

// WithLogger appends slog logger middleware hook.
func WithLogger(cfg logmiddleware.Config) Option {
	copied := cfg
	return func(client *Client) error {
		if client == nil || client.UniversalClient == nil {
			return nil
		}
		client.AddHook(logmiddleware.New(copied))
		return nil
	}
}

// WithOpenTelemetry appends OpenTelemetry middleware.
func WithOpenTelemetry(cfg otelmiddleware.Config) Option {
	copied := cfg
	return func(client *Client) error {
		if client == nil || client.UniversalClient == nil {
			return nil
		}
		return otelmiddleware.Apply(client.UniversalClient, copied)
	}
}

func isNilHook(hook redis.Hook) bool {
	if hook == nil {
		return true
	}

	value := reflect.ValueOf(hook)
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
