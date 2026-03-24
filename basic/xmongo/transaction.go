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

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SessionContext is a context carrying a MongoDB session.
type SessionContext = context.Context

// TransactionConfig controls session and transaction options for RunTransaction.
type TransactionConfig struct {
	SessionOptions     []options.Lister[options.SessionOptions]
	TransactionOptions []options.Lister[options.TransactionOptions]
}

// ErrNilTransactionFunc indicates RunTransaction was called with a nil callback.
var ErrNilTransactionFunc = errors.New("mongodb transaction callback is nil")

type transactionSessionRunner interface {
	EndSession(context.Context)
	WithTransaction(
		context.Context,
		func(context.Context) (any, error),
		...options.Lister[options.TransactionOptions],
	) (any, error)
}

type mongoTransactionSessionRunner struct {
	session *mongo.Session
}

// RunTransaction runs fn inside a MongoDB transaction.
func (c *Client) RunTransaction(
	ctx context.Context,
	cfg TransactionConfig,
	fn func(SessionContext) error,
) error {
	if c == nil || c.Client == nil {
		return mongo.ErrClientDisconnected
	}
	if fn == nil {
		return ErrNilTransactionFunc
	}
	if ctx == nil {
		ctx = context.Background()
	}

	runner, err := startSessionRunner(c.Client, cfg.SessionOptions...)
	if err != nil {
		return err
	}
	defer runner.EndSession(ctx)

	_, err = runner.WithTransaction(
		ctx,
		func(txCtx context.Context) (any, error) {
			return nil, fn(txCtx)
		},
		cfg.TransactionOptions...,
	)
	return err
}

func (r mongoTransactionSessionRunner) EndSession(ctx context.Context) {
	if r.session == nil {
		return
	}
	r.session.EndSession(ctx)
}

func (r mongoTransactionSessionRunner) WithTransaction(
	ctx context.Context,
	fn func(context.Context) (any, error),
	opts ...options.Lister[options.TransactionOptions],
) (any, error) {
	return r.session.WithTransaction(ctx, fn, opts...)
}

var startSessionRunner = func(
	client *mongo.Client,
	opts ...options.Lister[options.SessionOptions],
) (transactionSessionRunner, error) {
	if client == nil {
		return nil, mongo.ErrClientDisconnected
	}
	session, err := client.StartSession(opts...)
	if err != nil {
		return nil, err
	}
	return mongoTransactionSessionRunner{session: session}, nil
}
