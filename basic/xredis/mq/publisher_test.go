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

package mq

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestPublishUsesDefaultAndOverrideStream(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	publisher, err := NewPublisher(client, PublisherConfig{DefaultStream: "jobs"})
	require.NoError(t, err)

	first, err := publisher.Publish(context.Background(), &Message{
		Payload: []byte("one"),
		Headers: map[string]string{"kind": "email"},
	})
	require.NoError(t, err)
	require.Equal(t, "jobs", first.BaseStream)
	require.Equal(t, "jobs", first.Stream)
	require.Equal(t, 0, first.Shard)

	second, err := publisher.Publish(context.Background(), &Message{
		Stream:  "jobs:priority",
		Payload: []byte("two"),
	})
	require.NoError(t, err)
	require.Equal(t, "jobs:priority", second.BaseStream)
	require.Equal(t, "jobs:priority", second.Stream)
	require.Equal(t, 0, second.Shard)

	raw, err := client.XRange(context.Background(), "jobs", "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, raw, 1)
	require.Equal(t, "one", raw[0].Values[defaultPayloadField])
	require.Equal(t, "email", raw[0].Values[defaultHeaderPrefix+"kind"])

	override, err := client.XRange(context.Background(), "jobs:priority", "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, override, 1)
	require.Equal(t, "two", override[0].Values[defaultPayloadField])
}

func TestPublishOrderedShardRouting(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	publisher, err := NewPublisher(client, PublisherConfig{
		DefaultStream:     "jobs",
		OrderedShardCount: 4,
	})
	require.NoError(t, err)

	result, err := publisher.Publish(context.Background(), &Message{
		Payload: []byte("one"),
		Headers: map[string]string{defaultOrderKeyHeader: "order-1"},
	})
	require.NoError(t, err)

	expectedShard := shardForKey("order-1", 4)
	expectedStream := shardStreamName("jobs", "", expectedShard)
	require.Equal(t, "jobs", result.BaseStream)
	require.Equal(t, expectedStream, result.Stream)
	require.Equal(t, expectedShard, result.Shard)

	raw, err := client.XRange(context.Background(), expectedStream, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, raw, 1)
	require.Equal(t, "one", raw[0].Values[defaultPayloadField])
}

func TestPublishOrderedShardFallbackToBaseStreamKey(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	publisher, err := NewPublisher(client, PublisherConfig{
		DefaultStream:     "jobs",
		OrderedShardCount: 4,
	})
	require.NoError(t, err)

	result, err := publisher.Publish(context.Background(), &Message{Payload: []byte("one")})
	require.NoError(t, err)

	expectedShard := shardForKey("jobs", 4)
	require.Equal(t, expectedShard, result.Shard)
	require.Equal(t, shardStreamName("jobs", "", expectedShard), result.Stream)
}

func TestPublishBatchFailsFast(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	publisher, err := NewPublisher(client, PublisherConfig{DefaultStream: "jobs"})
	require.NoError(t, err)

	results, err := publisher.PublishBatch(
		context.Background(),
		&Message{Payload: []byte("one")},
		nil,
		&Message{Payload: []byte("three")},
	)
	require.Error(t, err)
	require.Len(t, results, 3)
	require.NotNil(t, results[0])
	require.Nil(t, results[1])
	require.Nil(t, results[2])
	require.Contains(t, err.Error(), "publish batch index 1")
}

func TestPublishRequiresStreamWithoutDefault(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	publisher, err := NewPublisher(client, PublisherConfig{})
	require.NoError(t, err)

	_, err = publisher.Publish(context.Background(), &Message{Payload: []byte("one")})
	require.ErrorIs(t, err, ErrMessageStreamRequired)
}

func newTestClient(t *testing.T) redis.UniversalClient {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client
}
