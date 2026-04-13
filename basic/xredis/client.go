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

// Client wraps redis.UniversalClient and keeps native command style.
type Client struct {
	redis.UniversalClient
}

// Raw returns the underlying redis.UniversalClient.
func (c *Client) Raw() redis.UniversalClient {
	if c == nil {
		return nil
	}
	return c.UniversalClient
}

// New builds a redis client and applies options in the exact call order.
func New(cfg Config, opts ...Option) (*Client, error) {
	// Validate config before creating the underlying client.
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Construct the underlying go-redis universal client.
	base := redis.NewUniversalClient(&cfg.UniversalOptions)
	client := &Client{UniversalClient: base}

	// Apply all options; close the client if any option fails.
	if err := applyOptions(client, opts); err != nil {
		return nil, closeClientOnError(client.UniversalClient, err)
	}

	return client, nil
}

// MustNew is like New but panics on error.
func MustNew(cfg Config, opts ...Option) *Client {
	client, err := New(cfg, opts...)
	if err != nil {
		panic(err)
	}
	return client
}

// Option customizes a client after it is constructed.
//
// Options are applied in the exact order they are passed to New.
type Option func(*Client) error

// WithHook appends hooks in the same order as arguments.
func WithHook(hooks ...redis.Hook) Option {
	return func(client *Client) error {
		if !clientReady(client) {
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
		if !clientReady(client) {
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
		if !clientReady(client) {
			return nil
		}
		return otelmiddleware.Apply(client.UniversalClient, copied)
	}
}

// closeClientOnError closes the client and returns the original error,
// appending the close error if one occurs.
func closeClientOnError(client redis.UniversalClient, err error) error {
	if client == nil {
		return err
	}
	if closeErr := client.Close(); closeErr != nil {
		return fmt.Errorf("%w; close client: %v", err, closeErr)
	}
	return err
}

// applyOptions applies each Option in order; returns on the first error.
func applyOptions(client *Client, opts []Option) error {
	for idx, option := range opts {
		if option == nil {
			continue
		}
		if err := option(client); err != nil {
			return fmt.Errorf("apply option #%d: %w", idx, err)
		}
	}
	return nil
}

// clientReady returns true when both the Client wrapper and its underlying
// UniversalClient are non-nil.
func clientReady(client *Client) bool {
	return client != nil && client.UniversalClient != nil
}

// isNilHook checks whether a redis.Hook value is nil, handling
// wrapped pointer/interface types that require reflection.
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
