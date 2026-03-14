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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestConsumerAutoCreateGroupAndAck(t *testing.T) {
	t.Parallel()

	client, _ := newTestRedis(t)
	publisher, err := NewPublisher(client, PublisherConfig{DefaultStream: "jobs"})
	require.NoError(t, err)
	_, err = publisher.Publish(context.Background(), &Message{
		Payload: []byte("hello"),
		Headers: map[string]string{"kind": "email"},
	})
	require.NoError(t, err)

	consumer, err := NewConsumer(client, ConsumerConfig{
		Stream:           "jobs",
		Group:            "workers",
		Consumer:         "c1",
		AutoCreateGroup:  true,
		Block:            10 * time.Millisecond,
		IdleBackoff:      5 * time.Millisecond,
		AutoClaimMinIdle: time.Second,
	})
	require.NoError(t, err)

	var captured *MessageContext
	err = consumer.Consume(context.Background(), func(_ context.Context, msg *MessageContext) error {
		captured = msg
		return consumer.Close()
	})
	require.ErrorIs(t, err, ErrConsumerClosed)
	require.NotNil(t, captured)
	require.Equal(t, "jobs", captured.BaseStream)
	require.Equal(t, "jobs", captured.ShardStream)
	require.Equal(t, "jobs", captured.Stream)
	require.Equal(t, "workers", captured.Group)
	require.Equal(t, "c1", captured.Consumer)
	require.Equal(t, "jobs", captured.LogicalKey)
	require.Equal(t, 0, captured.Shard)
	require.Equal(t, []byte("hello"), captured.Message.Payload)
	require.Equal(t, map[string]string{"kind": "email"}, captured.Message.Headers)
	require.False(t, captured.Claimed)
	require.GreaterOrEqual(t, captured.DeliveryCount, int64(1))

	pending, err := client.XPending(context.Background(), "jobs", "workers").Result()
	require.NoError(t, err)
	require.Equal(t, int64(0), pending.Count)
}

func TestConsumerFailureLeavesPending(t *testing.T) {
	t.Parallel()

	client, _ := newTestRedis(t)
	publisher, err := NewPublisher(client, PublisherConfig{DefaultStream: "jobs"})
	require.NoError(t, err)
	_, err = publisher.Publish(context.Background(), &Message{Payload: []byte("hello")})
	require.NoError(t, err)

	consumer, err := NewConsumer(client, ConsumerConfig{
		Stream:           "jobs",
		Group:            "workers",
		Consumer:         "c1",
		AutoCreateGroup:  true,
		Block:            10 * time.Millisecond,
		IdleBackoff:      5 * time.Millisecond,
		AutoClaimMinIdle: time.Second,
	})
	require.NoError(t, err)

	boom := errors.New("boom")
	err = consumer.Consume(context.Background(), func(_ context.Context, _ *MessageContext) error {
		return boom
	})
	require.ErrorIs(t, err, boom)

	pending, err := client.XPending(context.Background(), "jobs", "workers").Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), pending.Count)
	require.Equal(t, int64(1), pending.Consumers["c1"])
}

func TestConsumerAutoClaimReclaimsPending(t *testing.T) {
	t.Parallel()

	client, mr := newTestRedis(t)
	publisher, err := NewPublisher(client, PublisherConfig{})
	require.NoError(t, err)
	shard := shardForKey("order-1", 4)
	_, err = publisher.Publish(context.Background(), &Message{
		Stream:  shardStreamName("jobs", "", shard),
		Payload: []byte("hello"),
		Headers: map[string]string{defaultOrderKeyHeader: "order-1"},
	})
	require.NoError(t, err)

	first, err := NewConsumer(client, ConsumerConfig{
		Stream:            "jobs",
		Group:             "workers",
		Consumer:          "c1",
		AutoCreateGroup:   true,
		Block:             10 * time.Millisecond,
		IdleBackoff:       5 * time.Millisecond,
		OrderedShardCount: 4,
		OwnedShards:       []int{shard},
		AutoClaimMinIdle:  time.Second,
	})
	require.NoError(t, err)

	failOnce := errors.New("fail once")
	err = first.Consume(context.Background(), func(_ context.Context, _ *MessageContext) error {
		return failOnce
	})
	require.ErrorIs(t, err, failOnce)

	mr.FastForward(2 * time.Second)

	second, err := NewConsumer(client, ConsumerConfig{
		Stream:            "jobs",
		Group:             "workers",
		Consumer:          "c2",
		AutoCreateGroup:   true,
		Block:             10 * time.Millisecond,
		IdleBackoff:       5 * time.Millisecond,
		OrderedShardCount: 4,
		OwnedShards:       []int{shard},
		AutoClaimMinIdle:  time.Second,
	})
	require.NoError(t, err)

	var captured *MessageContext
	err = second.Consume(context.Background(), func(_ context.Context, msg *MessageContext) error {
		captured = msg
		return second.Close()
	})
	require.ErrorIs(t, err, ErrConsumerClosed)
	require.NotNil(t, captured)
	require.True(t, captured.Claimed)
	require.Equal(t, "jobs", captured.BaseStream)
	require.Equal(t, shardStreamName("jobs", "", shard), captured.ShardStream)
	require.Equal(t, "c2", captured.Consumer)
	require.Equal(t, "order-1", captured.LogicalKey)
	require.Equal(t, shard, captured.Shard)
	require.GreaterOrEqual(t, captured.DeliveryCount, int64(2))

	pending, err := client.XPending(context.Background(), shardStreamName("jobs", "", shard), "workers").Result()
	require.NoError(t, err)
	require.Equal(t, int64(0), pending.Count)
}

func TestConsumerRejectsConcurrentConsume(t *testing.T) {
	t.Parallel()

	client, _ := newTestRedis(t)
	publisher, err := NewPublisher(client, PublisherConfig{DefaultStream: "jobs"})
	require.NoError(t, err)
	_, err = publisher.Publish(context.Background(), &Message{Payload: []byte("hello")})
	require.NoError(t, err)

	consumer, err := NewConsumer(client, ConsumerConfig{
		Stream:           "jobs",
		Group:            "workers",
		Consumer:         "c1",
		AutoCreateGroup:  true,
		Block:            10 * time.Millisecond,
		IdleBackoff:      5 * time.Millisecond,
		AutoClaimMinIdle: time.Second,
	})
	require.NoError(t, err)

	started := make(chan struct{})
	release := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Consume(context.Background(), func(_ context.Context, _ *MessageContext) error {
			close(started)
			<-release
			return consumer.Close()
		})
	}()

	<-started
	require.ErrorIs(t, consumer.Consume(context.Background(), func(context.Context, *MessageContext) error {
		return nil
	}), ErrConsumerActive)

	close(release)
	require.ErrorIs(t, <-errCh, ErrConsumerClosed)
}

func TestConsumerStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	client, _ := newTestRedis(t)
	consumer, err := NewConsumer(client, ConsumerConfig{
		Stream:           "jobs",
		Group:            "workers",
		Consumer:         "c1",
		AutoCreateGroup:  true,
		Block:            10 * time.Millisecond,
		IdleBackoff:      5 * time.Millisecond,
		AutoClaimMinIdle: time.Second,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err = consumer.Consume(ctx, func(context.Context, *MessageContext) error {
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestConsumerStopsOnClose(t *testing.T) {
	t.Parallel()

	client, _ := newTestRedis(t)
	consumer, err := NewConsumer(client, ConsumerConfig{
		Stream:           "jobs",
		Group:            "workers",
		Consumer:         "c1",
		AutoCreateGroup:  true,
		Block:            10 * time.Millisecond,
		IdleBackoff:      5 * time.Millisecond,
		AutoClaimMinIdle: time.Second,
	})
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Consume(context.Background(), func(context.Context, *MessageContext) error {
			return nil
		})
	}()

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, consumer.Close())
	require.ErrorIs(t, <-errCh, ErrConsumerClosed)
}

func TestConsumerShardCountOneKeepsSerialBehavior(t *testing.T) {
	t.Parallel()

	client, _ := newTestRedis(t)
	publisher, err := NewPublisher(client, PublisherConfig{DefaultStream: "jobs"})
	require.NoError(t, err)

	_, err = publisher.Publish(context.Background(), &Message{
		Payload: []byte("first"),
		Headers: map[string]string{defaultOrderKeyHeader: "order-1"},
	})
	require.NoError(t, err)
	_, err = publisher.Publish(context.Background(), &Message{
		Payload: []byte("second"),
		Headers: map[string]string{defaultOrderKeyHeader: "order-1"},
	})
	require.NoError(t, err)

	consumer, err := NewConsumer(client, ConsumerConfig{
		Stream:           "jobs",
		Group:            "workers",
		Consumer:         "c1",
		Block:            10 * time.Millisecond,
		IdleBackoff:      5 * time.Millisecond,
		ShardCount:       1,
		AutoClaimMinIdle: time.Second,
	})
	require.NoError(t, err)

	var payloads []string
	err = consumer.Consume(context.Background(), func(_ context.Context, msg *MessageContext) error {
		payloads = append(payloads, string(msg.Message.Payload))
		if len(payloads) == 2 {
			return consumer.Close()
		}
		return nil
	})
	require.ErrorIs(t, err, ErrConsumerClosed)
	require.Equal(t, []string{"first", "second"}, payloads)
}

func TestConsumerSameKeyPreservesOrderAcrossShards(t *testing.T) {
	t.Parallel()

	client, _ := newTestRedis(t)
	publisher, err := NewPublisher(client, PublisherConfig{})
	require.NoError(t, err)

	orderShard := shardForKey("order-1", 4)
	orderTwoShard := shardForKey("order-2", 4)
	for _, payload := range []string{"a1", "b1", "a2", "a3"} {
		key := "order-1"
		stream := shardStreamName("jobs", "", orderShard)
		if payload == "b1" {
			key = "order-2"
			stream = shardStreamName("jobs", "", orderTwoShard)
		}
		_, err = publisher.Publish(context.Background(), &Message{
			Stream:  stream,
			Payload: []byte(payload),
			Headers: map[string]string{defaultOrderKeyHeader: key},
		})
		require.NoError(t, err)
	}

	consumer, err := NewConsumer(client, ConsumerConfig{
		Stream:            "jobs",
		Group:             "workers",
		Consumer:          "c1",
		Block:             10 * time.Millisecond,
		IdleBackoff:       5 * time.Millisecond,
		OrderedShardCount: 4,
		OwnedShards:       []int{orderShard},
		AutoClaimMinIdle:  time.Second,
	})
	require.NoError(t, err)

	var mu sync.Mutex
	var orderOne []string
	seen := 0
	err = consumer.Consume(context.Background(), func(_ context.Context, msg *MessageContext) error {
		mu.Lock()
		defer mu.Unlock()
		orderOne = append(orderOne, string(msg.Message.Payload))
		seen++
		if seen == 3 {
			return consumer.Close()
		}
		return nil
	})
	require.ErrorIs(t, err, ErrConsumerClosed)
	require.Equal(t, []string{"a1", "a2", "a3"}, orderOne)
}

func TestConsumerOwnedShardsOnlyConsumeAssignedStreams(t *testing.T) {
	t.Parallel()

	client, _ := newTestRedis(t)
	publisher, err := NewPublisher(client, PublisherConfig{})
	require.NoError(t, err)

	keyOne, keyTwo := distinctKeysForShards(2)
	for _, key := range []string{keyOne, keyTwo} {
		_, err = publisher.Publish(context.Background(), &Message{
			Stream:  shardStreamName("jobs", "", shardForKey(key, 2)),
			Payload: []byte(key),
			Headers: map[string]string{defaultOrderKeyHeader: key},
		})
		require.NoError(t, err)
	}

	consumer, err := NewConsumer(client, ConsumerConfig{
		Stream:            "jobs",
		Group:             "workers",
		Consumer:          "c1",
		Block:             10 * time.Millisecond,
		IdleBackoff:       5 * time.Millisecond,
		OrderedShardCount: 2,
		OwnedShards:       []int{shardForKey(keyOne, 2)},
		AutoClaimMinIdle:  time.Second,
	})
	require.NoError(t, err)

	var seen []string
	err = consumer.Consume(context.Background(), func(_ context.Context, msg *MessageContext) error {
		seen = append(seen, string(msg.Message.Payload))
		if len(seen) == 1 {
			return consumer.Close()
		}
		return nil
	})
	require.ErrorIs(t, err, ErrConsumerClosed)
	require.Equal(t, []string{keyOne}, seen)
}

func TestConsumerDifferentConsumersOwnDifferentShards(t *testing.T) {
	t.Parallel()

	client, _ := newTestRedis(t)
	publisher, err := NewPublisher(client, PublisherConfig{})
	require.NoError(t, err)

	keyOne, keyTwo := distinctKeysForShards(2)
	_, err = publisher.Publish(context.Background(), &Message{
		Stream:  shardStreamName("jobs", "", shardForKey(keyOne, 2)),
		Payload: []byte("one"),
		Headers: map[string]string{defaultOrderKeyHeader: keyOne},
	})
	require.NoError(t, err)
	_, err = publisher.Publish(context.Background(), &Message{
		Stream:  shardStreamName("jobs", "", shardForKey(keyTwo, 2)),
		Payload: []byte("two"),
		Headers: map[string]string{defaultOrderKeyHeader: keyTwo},
	})
	require.NoError(t, err)

	first, err := NewConsumer(client, ConsumerConfig{
		Stream:            "jobs",
		Group:             "workers",
		Consumer:          "c1",
		Block:             10 * time.Millisecond,
		IdleBackoff:       5 * time.Millisecond,
		OrderedShardCount: 2,
		OwnedShards:       []int{shardForKey(keyOne, 2)},
		AutoClaimMinIdle:  time.Second,
	})
	require.NoError(t, err)

	var firstSeen []string
	err = first.Consume(context.Background(), func(_ context.Context, msg *MessageContext) error {
		firstSeen = append(firstSeen, string(msg.Message.Payload))
		return first.Close()
	})
	require.ErrorIs(t, err, ErrConsumerClosed)

	second, err := NewConsumer(client, ConsumerConfig{
		Stream:            "jobs",
		Group:             "workers",
		Consumer:          "c2",
		Block:             10 * time.Millisecond,
		IdleBackoff:       5 * time.Millisecond,
		OrderedShardCount: 2,
		OwnedShards:       []int{shardForKey(keyTwo, 2)},
		AutoClaimMinIdle:  time.Second,
	})
	require.NoError(t, err)

	var secondSeen []string
	err = second.Consume(context.Background(), func(_ context.Context, msg *MessageContext) error {
		secondSeen = append(secondSeen, string(msg.Message.Payload))
		return second.Close()
	})
	require.ErrorIs(t, err, ErrConsumerClosed)
	require.Equal(t, []string{"one"}, firstSeen)
	require.Equal(t, []string{"two"}, secondSeen)
}

func TestConsumerMissingOrderHeaderFallsBackToBaseStreamShard(t *testing.T) {
	t.Parallel()

	client, _ := newTestRedis(t)
	publisher, err := NewPublisher(client, PublisherConfig{
		DefaultStream:     "jobs",
		OrderedShardCount: 4,
	})
	require.NoError(t, err)

	_, err = publisher.Publish(context.Background(), &Message{Payload: []byte("one")})
	require.NoError(t, err)
	_, err = publisher.Publish(context.Background(), &Message{Payload: []byte("two")})
	require.NoError(t, err)

	expectedShard := shardForKey("jobs", 4)
	consumer, err := NewConsumer(client, ConsumerConfig{
		Stream:            "jobs",
		Group:             "workers",
		Consumer:          "c1",
		Block:             10 * time.Millisecond,
		IdleBackoff:       5 * time.Millisecond,
		OrderedShardCount: 4,
		OwnedShards:       []int{expectedShard},
		AutoClaimMinIdle:  time.Second,
	})
	require.NoError(t, err)

	var logicalKeys []string
	var shardStreams []string
	err = consumer.Consume(context.Background(), func(_ context.Context, msg *MessageContext) error {
		logicalKeys = append(logicalKeys, msg.LogicalKey)
		shardStreams = append(shardStreams, msg.ShardStream)
		if len(logicalKeys) == 2 {
			return consumer.Close()
		}
		return nil
	})
	require.ErrorIs(t, err, ErrConsumerClosed)
	require.Equal(t, []string{"jobs", "jobs"}, logicalKeys)
	require.Equal(
		t,
		[]string{shardStreamName("jobs", "", expectedShard), shardStreamName("jobs", "", expectedShard)},
		shardStreams,
	)
}

func distinctKeysForShards(shardCount int) (string, string) {
	first := "key-0"
	for i := 1; i < 128; i++ {
		candidate := "key-" + string(rune('a'+i))
		if shardForKey(first, shardCount) != shardForKey(candidate, shardCount) {
			return first, candidate
		}
	}
	return "key-a", "key-b"
}

func newTestRedis(t *testing.T) (redis.UniversalClient, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client, mr
}
