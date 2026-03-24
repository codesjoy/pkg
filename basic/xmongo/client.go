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
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Client wraps mongo.Client and keeps the native driver API.
type Client struct {
	*mongo.Client
	defaultDatabase string
	healthTracker   *healthTracker
}

// Raw returns the underlying mongo.Client.
func (c *Client) Raw() *mongo.Client {
	if c == nil {
		return nil
	}
	return c.Client
}

// DB returns the configured default database.
func (c *Client) DB(
	opts ...options.Lister[options.DatabaseOptions],
) (*mongo.Database, error) {
	if c == nil || c.Client == nil {
		return nil, mongo.ErrClientDisconnected
	}
	if c.defaultDatabase == "" {
		return nil, ErrDefaultDatabaseRequired
	}
	return c.Database(c.defaultDatabase, opts...), nil
}

// Collection returns a collection from the configured default database.
func (c *Client) Collection(
	name string,
	opts ...options.Lister[options.CollectionOptions],
) (*mongo.Collection, error) {
	if c == nil || c.Client == nil {
		return nil, mongo.ErrClientDisconnected
	}

	normalizedName := strings.TrimSpace(name)
	if normalizedName == "" {
		return nil, ErrEmptyCollectionName
	}

	db, err := c.DB()
	if err != nil {
		return nil, err
	}
	return db.Collection(normalizedName, opts...), nil
}

// Close is an alias of Disconnect.
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.Client == nil {
		return nil
	}
	return c.Disconnect(ctx)
}

// CloseWithTimeout closes the client with a timeout-scoped background context.
func (c *Client) CloseWithTimeout(timeout time.Duration) error {
	if c == nil || c.Client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.Close(ctx)
}

// New builds a MongoDB client and applies options in the exact call order.
//
// New constructs and connects the driver client, but does not Ping. Callers can
// perform explicit readiness checks via client.PingPrimary(ctx) or the native
// client.Ping(ctx, readpref.Primary()).
func New(cfg Config, opts ...Option) (*Client, error) {
	normalized := cfg
	if err := normalized.Validate(); err != nil {
		return nil, err
	}

	state, err := buildOptionState(opts...)
	if err != nil {
		return nil, err
	}

	clientOptions, err := buildClientOptionsWithState(normalized, state)
	if err != nil {
		return nil, err
	}

	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return nil, err
	}
	return &Client{
		Client:          client,
		defaultDatabase: normalized.DefaultDatabase,
		healthTracker:   state.healthTracker,
	}, nil
}

// MustNew is like New but panics on error.
func MustNew(cfg Config, opts ...Option) *Client {
	client, err := New(cfg, opts...)
	if err != nil {
		panic(err)
	}
	return client
}
