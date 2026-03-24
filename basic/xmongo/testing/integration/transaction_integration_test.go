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

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/codesjoy/pkg/basic/xmongo"
)

func TestRunTransactionCommit(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client, err := xmongo.New(xmongo.Config{
		URI:             mustURI(t),
		DefaultDatabase: "xmongo_integration",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, client.Close(cleanupCtx))
	})

	collection, err := client.Collection("tx_commit_widgets")
	require.NoError(t, err)
	_, err = collection.DeleteMany(ctx, bson.D{})
	require.NoError(t, err)

	err = client.RunTransaction(
		ctx,
		xmongo.TransactionConfig{},
		func(txCtx xmongo.SessionContext) error {
			_, err := collection.InsertOne(txCtx, bson.D{{Key: "_id", Value: "tx-commit-1"}})
			return err
		},
	)
	require.NoError(t, err)

	count, err := collection.CountDocuments(ctx, bson.D{{Key: "_id", Value: "tx-commit-1"}})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}

func TestRunTransactionRollback(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client, err := xmongo.New(xmongo.Config{
		URI:             mustURI(t),
		DefaultDatabase: "xmongo_integration",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, client.Close(cleanupCtx))
	})

	collection, err := client.Collection("tx_rollback_widgets")
	require.NoError(t, err)
	_, err = collection.DeleteMany(ctx, bson.D{})
	require.NoError(t, err)

	sentinel := errors.New("rollback")
	err = client.RunTransaction(
		ctx,
		xmongo.TransactionConfig{},
		func(txCtx xmongo.SessionContext) error {
			_, err := collection.InsertOne(txCtx, bson.D{{Key: "_id", Value: "tx-rollback-1"}})
			require.NoError(t, err)
			return sentinel
		},
	)
	require.ErrorIs(t, err, sentinel)

	count, err := collection.CountDocuments(ctx, bson.D{{Key: "_id", Value: "tx-rollback-1"}})
	require.NoError(t, err)
	require.EqualValues(t, 0, count)
}
