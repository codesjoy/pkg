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

	"github.com/redis/go-redis/v9"
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
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	base := redis.NewUniversalClient(&cfg.UniversalOptions)
	client := &Client{UniversalClient: base}

	for idx, option := range opts {
		if option == nil {
			continue
		}
		if err := option(client); err != nil {
			wrappedErr := fmt.Errorf("apply option #%d: %w", idx, err)
			return nil, closeClientOnError(client.UniversalClient, wrappedErr)
		}
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

func closeClientOnError(client redis.UniversalClient, err error) error {
	if client == nil {
		return err
	}
	if closeErr := client.Close(); closeErr != nil {
		return fmt.Errorf("%w; close client: %v", err, closeErr)
	}
	return err
}
