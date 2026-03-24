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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestNewAndMustNew(t *testing.T) {
	t.Parallel()

	cfg := Config{
		URI:             testMongoURI(),
		DefaultDatabase: " app ",
	}

	client, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotNil(t, client.Raw())
	require.Equal(t, "app", client.defaultDatabase)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	require.NoError(t, client.Disconnect(ctx))

	require.Panics(t, func() {
		_ = MustNew(Config{})
	})
}

func TestRawHandlesNilClient(t *testing.T) {
	t.Parallel()

	var client *Client
	require.Nil(t, client.Raw())
}

func TestDBUsesConfiguredDefaultDatabase(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, Config{
		URI:             testMongoURI(),
		DefaultDatabase: " app ",
	})

	db, err := client.DB()
	require.NoError(t, err)
	require.Equal(t, "app", db.Name())
}

func TestDBRequiresDefaultDatabase(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, Config{URI: testMongoURI()})
	_, err := client.DB()
	require.ErrorIs(t, err, ErrDefaultDatabaseRequired)
}

func TestCollectionUsesConfiguredDefaultDatabase(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, Config{
		URI:             testMongoURI(),
		DefaultDatabase: " app ",
	})

	collection, err := client.Collection(" widgets ")
	require.NoError(t, err)
	require.Equal(t, "widgets", collection.Name())
}

func TestCollectionRequiresDefaultDatabase(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, Config{URI: testMongoURI()})
	_, err := client.Collection("widgets")
	require.ErrorIs(t, err, ErrDefaultDatabaseRequired)
}

func TestCollectionRejectsEmptyName(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, Config{
		URI:             testMongoURI(),
		DefaultDatabase: "app",
	})

	_, err := client.Collection("   ")
	require.ErrorIs(t, err, ErrEmptyCollectionName)
}

func TestCloseHelpers(t *testing.T) {
	t.Parallel()

	client, err := New(Config{URI: testMongoURI()})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	require.NoError(t, client.Close(ctx))
	require.ErrorIs(t, client.CloseWithTimeout(time.Millisecond), mongo.ErrClientDisconnected)

	var nilClient *Client
	require.NoError(t, nilClient.Close(ctx))
	require.NoError(t, nilClient.CloseWithTimeout(time.Millisecond))
}

func newTestClient(t *testing.T, cfg Config) *Client {
	t.Helper()

	client, err := New(cfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		require.NoError(t, client.Disconnect(ctx))
	})

	return client
}

func testMongoURI() string {
	return "mongodb://127.0.0.1:1/?directConnection=true&serverSelectionTimeoutMS=5&connectTimeoutMS=5"
}
