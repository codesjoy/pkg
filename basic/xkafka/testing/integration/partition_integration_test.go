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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

func TestPartitionConsumerCheckpointResume(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	topic := uniqueKafkaName("partition_resume")
	createTopic(t, topic, 1)

	produceSequenceRange(t, ctx, topic, 0, 3)

	store := xkafka.NewMemoryOffsetStore()

	firstOffsets := consumePartitionUntilCount(t, topic, store, 3)
	require.Equal(t, []int64{0, 1, 2}, firstOffsets)

	nextOffset, found, err := store.Load(context.Background(), topic, 0)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(3), nextOffset)

	produceSequenceRange(t, ctx, topic, 3, 3)

	secondOffsets := consumePartitionUntilCount(t, topic, store, 3)
	require.Equal(t, []int64{3, 4, 5}, secondOffsets)

	nextOffset, found, err = store.Load(context.Background(), topic, 0)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(6), nextOffset)
}

func produceSequenceRange(t *testing.T, ctx context.Context, topic string, start, count int) {
	t.Helper()

	producer, err := xkafka.NewProducer(xkafka.ProducerConfig{
		Brokers:      mustBrokers(t),
		DefaultTopic: topic,
		SaramaConfig: newProducerSaramaConfig(),
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, producer.Close())
	}()

	for i := range count {
		sequence := start + i
		_, err = producer.Produce(ctx, &produce.Message{
			Key:   []byte("partition-key"),
			Value: []byte(fmt.Sprintf("payload-%d", sequence)),
		})
		require.NoError(t, err)
	}
}

func consumePartitionUntilCount(
	t *testing.T,
	topic string,
	store xkafka.OffsetStore,
	target int,
) []int64 {
	t.Helper()

	ctx, cancel := integrationContext(t)
	defer cancel()

	consumer, err := xkafka.NewPartitionConsumer(xkafka.PartitionConsumerConfig{
		Brokers:       mustBrokers(t),
		Topic:         topic,
		Partition:     0,
		SaramaConfig:  newConsumerSaramaConfig(),
		OffsetStore:   store,
		InitialOffset: sarama.OffsetOldest,
		ShardCount:    8,
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, consumer.Close())
	}()

	consumeErrCh := make(chan error, 1)
	reachedTargetCh := make(chan struct{}, 1)

	var mu sync.Mutex
	offsets := make([]int64, 0, target)
	go func() {
		consumeErrCh <- consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			mu.Lock()
			offsets = append(offsets, msg.Message.Offset)
			if len(offsets) >= target {
				select {
				case reachedTargetCh <- struct{}{}:
				default:
				}
			}
			mu.Unlock()
			return nil
		})
	}()

	select {
	case <-reachedTargetCh:
		cancel()
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %d partition messages", target)
	}

	err = waitForError(t, consumeErrCh, 30*time.Second)
	require.ErrorIs(t, err, context.Canceled)

	mu.Lock()
	result := make([]int64, len(offsets))
	copy(result, offsets)
	mu.Unlock()

	if len(result) > target {
		result = result[:target]
	}
	return result
}
