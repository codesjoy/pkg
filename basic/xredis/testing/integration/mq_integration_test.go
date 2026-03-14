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
	"hash/fnv"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xredis"
	"github.com/codesjoy/pkg/basic/xredis/mq"
)

func TestMQConsumerCloseStopsBlockedRead(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client, err := xredis.New(
		xredis.Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mustAddr(t)}}},
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	consumer, err := mq.NewConsumer(client, mq.ConsumerConfig{
		Stream:           "it:mq:blocked",
		Group:            "workers",
		Consumer:         "c1",
		AutoCreateGroup:  true,
		Block:            50 * time.Millisecond,
		IdleBackoff:      10 * time.Millisecond,
		AutoClaimMinIdle: time.Second,
	})
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Consume(ctx, func(context.Context, *mq.MessageContext) error {
			return nil
		})
	}()

	time.Sleep(30 * time.Millisecond)
	require.NoError(t, consumer.Close())
	require.ErrorIs(t, <-errCh, mq.ErrConsumerClosed)
}

func TestMQPublishFailAndAutoClaimAck(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client, err := xredis.New(
		xredis.Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mustAddr(t)}}},
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	publisher, err := mq.NewPublisher(client, mq.PublisherConfig{
		DefaultStream:     "it:mq:jobs",
		OrderedShardCount: 4,
	})
	require.NoError(t, err)
	_, err = publisher.Publish(ctx, &mq.Message{
		Payload: []byte("hello"),
		Headers: map[string]string{
			"kind":        "welcome",
			"x-order-key": "order-1",
		},
	})
	require.NoError(t, err)

	first, err := mq.NewConsumer(client, mq.ConsumerConfig{
		Stream:            "it:mq:jobs",
		Group:             "workers",
		Consumer:          "c1",
		AutoCreateGroup:   true,
		OrderedShardCount: 4,
		OwnedShards:       []int{shardFor("order-1", 4)},
		Block:             50 * time.Millisecond,
		IdleBackoff:       10 * time.Millisecond,
		AutoClaimMinIdle:  50 * time.Millisecond,
	})
	require.NoError(t, err)

	failOnce := errors.New("fail once")
	err = first.Consume(ctx, func(context.Context, *mq.MessageContext) error {
		return failOnce
	})
	require.ErrorIs(t, err, failOnce)

	time.Sleep(120 * time.Millisecond)

	second, err := mq.NewConsumer(client, mq.ConsumerConfig{
		Stream:            "it:mq:jobs",
		Group:             "workers",
		Consumer:          "c2",
		AutoCreateGroup:   true,
		OrderedShardCount: 4,
		OwnedShards:       []int{shardFor("order-1", 4)},
		Block:             50 * time.Millisecond,
		IdleBackoff:       10 * time.Millisecond,
		AutoClaimMinIdle:  50 * time.Millisecond,
	})
	require.NoError(t, err)

	var captured *mq.MessageContext
	err = second.Consume(ctx, func(_ context.Context, msg *mq.MessageContext) error {
		captured = msg
		return second.Close()
	})
	require.ErrorIs(t, err, mq.ErrConsumerClosed)
	require.NotNil(t, captured)
	require.True(t, captured.Claimed)
	require.Equal(t, "c2", captured.Consumer)
	require.Equal(t, "it:mq:jobs", captured.BaseStream)
	require.Equal(
		t,
		shardStreamNameForTest("it:mq:jobs", "", shardFor("order-1", 4)),
		captured.ShardStream,
	)
	require.Equal(t, "order-1", captured.LogicalKey)
	require.Equal(t, []byte("hello"), captured.Message.Payload)
	require.Equal(t, map[string]string{
		"kind":        "welcome",
		"x-order-key": "order-1",
	}, captured.Message.Headers)
	require.GreaterOrEqual(t, captured.DeliveryCount, int64(2))

	pending, err := client.XPending(
		ctx,
		shardStreamNameForTest("it:mq:jobs", "", shardFor("order-1", 4)),
		"workers",
	).Result()
	require.NoError(t, err)
	require.Equal(t, int64(0), pending.Count)
}

func TestMQConsumersOwnDifferentShards(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client, err := xredis.New(
		xredis.Config{UniversalOptions: redis.UniversalOptions{Addrs: []string{mustAddr(t)}}},
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	publisher, err := mq.NewPublisher(client, mq.PublisherConfig{
		DefaultStream:     "it:mq:parallel",
		OrderedShardCount: 2,
	})
	require.NoError(t, err)

	keyOne, keyTwo := distinctParallelKeys()
	for _, key := range []string{keyOne, keyTwo} {
		_, err = publisher.Publish(ctx, &mq.Message{
			Payload: []byte(key),
			Headers: map[string]string{"x-order-key": key},
		})
		require.NoError(t, err)
	}

	first, err := mq.NewConsumer(client, mq.ConsumerConfig{
		Stream:            "it:mq:parallel",
		Group:             "workers",
		Consumer:          "c1",
		AutoCreateGroup:   true,
		OrderedShardCount: 2,
		OwnedShards:       []int{shardFor(keyOne, 2)},
		Block:             50 * time.Millisecond,
		IdleBackoff:       10 * time.Millisecond,
		AutoClaimMinIdle:  time.Second,
	})
	require.NoError(t, err)

	var firstSeen []string
	err = first.Consume(ctx, func(_ context.Context, msg *mq.MessageContext) error {
		firstSeen = append(firstSeen, string(msg.Message.Payload))
		return first.Close()
	})
	require.ErrorIs(t, err, mq.ErrConsumerClosed)

	second, err := mq.NewConsumer(client, mq.ConsumerConfig{
		Stream:            "it:mq:parallel",
		Group:             "workers",
		Consumer:          "c2",
		AutoCreateGroup:   true,
		OrderedShardCount: 2,
		OwnedShards:       []int{shardFor(keyTwo, 2)},
		Block:             50 * time.Millisecond,
		IdleBackoff:       10 * time.Millisecond,
		AutoClaimMinIdle:  time.Second,
	})
	require.NoError(t, err)

	var secondSeen []string
	err = second.Consume(ctx, func(_ context.Context, msg *mq.MessageContext) error {
		secondSeen = append(secondSeen, string(msg.Message.Payload))
		return second.Close()
	})
	require.ErrorIs(t, err, mq.ErrConsumerClosed)
	require.Equal(t, []string{keyOne}, firstSeen)
	require.Equal(t, []string{keyTwo}, secondSeen)
}

func distinctParallelKeys() (string, string) {
	first := "parallel-a"
	for ch := 'b'; ch <= 'z'; ch++ {
		candidate := "parallel-" + string(ch)
		if shardFor(first, 2) != shardFor(candidate, 2) {
			return first, candidate
		}
	}
	return "parallel-a", "parallel-b"
}

func shardFor(key string, shardCount int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(shardCount))
}

func shardStreamNameForTest(baseStream, shardStreamPrefix string, shard int) string {
	if shardStreamPrefix != "" {
		return shardStreamPrefix + ":" + strconv.Itoa(shard)
	}
	return baseStream + ":shard:" + strconv.Itoa(shard)
}
