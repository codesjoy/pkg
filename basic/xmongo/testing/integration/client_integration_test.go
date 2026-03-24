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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/codesjoy/pkg/basic/xmongo"
)

func TestInsertFindAndDisconnect(t *testing.T) {
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
		require.NoError(t, client.Disconnect(cleanupCtx))
	})

	require.NoError(t, client.PingPrimary(ctx))

	collection, err := client.Collection("widgets")
	require.NoError(t, err)

	_, err = collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "widget-1"},
		{Key: "name", Value: "gear"},
		{Key: "qty", Value: 12},
	})
	require.NoError(t, err)

	var doc bson.M
	err = collection.FindOne(ctx, bson.D{{Key: "_id", Value: "widget-1"}}).Decode(&doc)
	require.NoError(t, err)
	require.Equal(t, "gear", doc["name"])
	require.EqualValues(t, 12, doc["qty"])
}
