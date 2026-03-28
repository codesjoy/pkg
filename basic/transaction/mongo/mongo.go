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

package mongotx

import (
	"context"
	"errors"

	"github.com/codesjoy/pkg/basic/transaction"
	"github.com/codesjoy/pkg/basic/transaction/internal/scope"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Config controls default session and transaction options for the runner.
type Config struct {
	SessionOptions     []options.Lister[options.SessionOptions]
	TransactionOptions []options.Lister[options.TransactionOptions]
}

// Option customizes the Mongo transaction runner.
type Option func(*Runner)

type transactionMarkerKey struct{}

type transactionMarker struct {
	client *mongo.Client
}

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

// Runner executes REQUIRED transactions over the MongoDB driver.
type Runner struct {
	client *mongo.Client
	cfg    Config
}

// New constructs a REQUIRED transaction runner over a MongoDB client.
func New(client *mongo.Client, opts ...Option) *Runner {
	runner := &Runner{client: client}
	for _, opt := range opts {
		if opt != nil {
			opt(runner)
		}
	}
	return runner
}

// WithDefaultConfig overrides the runner's default session and transaction options.
func WithDefaultConfig(cfg Config) Option {
	cloned := cloneConfig(cfg)
	return func(r *Runner) {
		r.cfg = cloned
	}
}

// Within reuses an existing transaction in ctx or starts a new one.
func (r *Runner) Within(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if marker, ok := markerFromContext(ctx); ok && marker.client == r.client &&
		mongo.SessionFromContext(ctx) != nil {
		return fn(ctx)
	}
	if r == nil || r.client == nil {
		return mongo.ErrClientDisconnected
	}

	cfg := cloneConfig(r.cfg)
	txScope := scope.New()
	scopeCtx := scope.IntoContext(ctx, txScope)

	runner, err := startSessionRunner(r.client, cfg.SessionOptions...)
	if err != nil {
		return err
	}
	defer runner.EndSession(ctx)

	_, err = runner.WithTransaction(
		ctx,
		func(txCtx context.Context) (any, error) {
			txCtx = scope.IntoContext(txCtx, txScope)
			txCtx = withMarker(txCtx, r.client)
			return nil, fn(txCtx)
		},
		cfg.TransactionOptions...,
	)
	if err != nil {
		return err
	}

	if err := scope.Run(scopeCtx, ctx); err != nil {
		return errors.Join(transaction.ErrAfterCommitFailed, err)
	}
	return nil
}

func cloneConfig(cfg Config) Config {
	cloned := Config{}
	if len(cfg.SessionOptions) > 0 {
		cloned.SessionOptions = append(
			[]options.Lister[options.SessionOptions](nil),
			cfg.SessionOptions...)
	}
	if len(cfg.TransactionOptions) > 0 {
		cloned.TransactionOptions = append(
			[]options.Lister[options.TransactionOptions](nil),
			cfg.TransactionOptions...,
		)
	}
	return cloned
}

func withMarker(ctx context.Context, client *mongo.Client) context.Context {
	return context.WithValue(ctx, transactionMarkerKey{}, transactionMarker{client: client})
}

func markerFromContext(ctx context.Context) (transactionMarker, bool) {
	if ctx == nil {
		return transactionMarker{}, false
	}
	marker, ok := ctx.Value(transactionMarkerKey{}).(transactionMarker)
	return marker, ok
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
