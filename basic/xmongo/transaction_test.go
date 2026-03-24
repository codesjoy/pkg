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
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
)

type transactionTestKey struct{}

func TestRunTransactionRequiresClient(t *testing.T) {
	t.Parallel()

	var client *Client
	err := client.RunTransaction(
		context.Background(),
		TransactionConfig{},
		func(SessionContext) error { return nil },
	)
	require.ErrorIs(t, err, mongo.ErrClientDisconnected)
}

func TestRunTransactionRequiresCallback(t *testing.T) {
	t.Parallel()

	client := &Client{Client: &mongo.Client{}}
	err := client.RunTransaction(context.Background(), TransactionConfig{}, nil)
	require.ErrorIs(t, err, ErrNilTransactionFunc)
}

func TestRunTransactionPassesOptionsAndCallbackContext(t *testing.T) {
	original := startSessionRunner
	defer func() {
		startSessionRunner = original
	}()

	sessionOpt := options.Session().SetDefaultTransactionOptions(options.Transaction())
	txOpt := options.Transaction().SetReadConcern(readconcern.Majority())

	runner := &fakeTransactionRunner{}
	var gotSessionOptions []options.Lister[options.SessionOptions]
	startSessionRunner = func(
		_ *mongo.Client,
		opts ...options.Lister[options.SessionOptions],
	) (transactionSessionRunner, error) {
		gotSessionOptions = append([]options.Lister[options.SessionOptions](nil), opts...)
		return runner, nil
	}

	client := &Client{Client: &mongo.Client{}}
	ctx := context.Background()
	err := client.RunTransaction(
		ctx,
		TransactionConfig{
			SessionOptions:     []options.Lister[options.SessionOptions]{sessionOpt},
			TransactionOptions: []options.Lister[options.TransactionOptions]{txOpt},
		},
		func(txCtx SessionContext) error {
			require.Equal(t, "tx", txCtx.Value(transactionTestKey{}))
			return nil
		},
	)
	require.NoError(t, err)
	require.Len(t, gotSessionOptions, 1)
	require.Equal(
		t,
		reflect.ValueOf(sessionOpt).Pointer(),
		reflect.ValueOf(gotSessionOptions[0]).Pointer(),
	)
	require.Len(t, runner.txOptions, 1)
	require.Equal(
		t,
		reflect.ValueOf(txOpt).Pointer(),
		reflect.ValueOf(runner.txOptions[0]).Pointer(),
	)
	require.Equal(t, ctx, runner.endCtx)
}

func TestRunTransactionReturnsCallbackError(t *testing.T) {
	original := startSessionRunner
	defer func() {
		startSessionRunner = original
	}()

	boom := errors.New("boom")
	runner := &fakeTransactionRunner{}
	startSessionRunner = func(
		_ *mongo.Client,
		_ ...options.Lister[options.SessionOptions],
	) (transactionSessionRunner, error) {
		return runner, nil
	}

	client := &Client{Client: &mongo.Client{}}
	err := client.RunTransaction(
		context.Background(),
		TransactionConfig{},
		func(SessionContext) error {
			return boom
		},
	)
	require.ErrorIs(t, err, boom)
	require.NotNil(t, runner.endCtx)
}

type fakeTransactionRunner struct {
	endCtx    context.Context
	txOptions []options.Lister[options.TransactionOptions]
}

func (r *fakeTransactionRunner) EndSession(ctx context.Context) {
	r.endCtx = ctx
}

func (r *fakeTransactionRunner) WithTransaction(
	ctx context.Context,
	fn func(context.Context) (any, error),
	opts ...options.Lister[options.TransactionOptions],
) (any, error) {
	r.txOptions = append([]options.Lister[options.TransactionOptions](nil), opts...)
	return fn(context.WithValue(ctx, transactionTestKey{}, "tx"))
}
