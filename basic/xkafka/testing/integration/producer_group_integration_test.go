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
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

type consumedRecord struct {
	key string
	seq int
}

func TestProducerAndGroupConsumerEndToEnd(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	topic := uniqueKafkaName("producer_group")
	createTopic(t, topic, 3)

	consumer, err := xkafka.NewGroupConsumer(xkafka.GroupConsumerConfig{
		Brokers:      mustBrokers(t),
		GroupID:      uniqueKafkaName("group"),
		Topics:       []string{topic},
		SaramaConfig: newConsumerSaramaConfig(),
		ShardCount:   16,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, consumer.Close())
	})

	consumedCh := make(chan consumedRecord, 16)
	consumeErrCh := make(chan error, 1)
	var consumedCount atomic.Int32
	go func() {
		consumeErrCh <- consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
			seq, convErr := strconv.Atoi(string(msg.Message.Value))
			if convErr != nil {
				return convErr
			}
			consumedCh <- consumedRecord{key: msg.LogicalKey, seq: seq}
			if consumedCount.Add(1) == 4 {
				cancel()
			}
			return nil
		})
	}()

	producer, err := xkafka.NewProducer(xkafka.ProducerConfig{
		Brokers:      mustBrokers(t),
		DefaultTopic: topic,
		SaramaConfig: newProducerSaramaConfig(),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, producer.Close())
	})

	_, err = producer.Produce(ctx, &produce.Message{Key: []byte("key-1"), Value: []byte("1")})
	require.NoError(t, err)

	_, err = producer.ProduceBatch(ctx,
		&produce.Message{Key: []byte("key-1"), Value: []byte("2")},
		&produce.Message{Key: []byte("key-2"), Value: []byte("3")},
	)
	require.NoError(t, err)

	future, err := producer.ProduceAsync(
		ctx,
		&produce.Message{Key: []byte("key-1"), Value: []byte("4")},
	)
	require.NoError(t, err)
	_, err = future.Await(ctx)
	require.NoError(t, err)

	records := make([]consumedRecord, 0, 4)
	deadline := time.After(30 * time.Second)
	for len(records) < 4 {
		select {
		case record := <-consumedCh:
			records = append(records, record)
		case <-deadline:
			t.Fatalf("timed out waiting for consumed records: got %d", len(records))
		}
	}

	consumeErr := waitForError(t, consumeErrCh, 30*time.Second)
	require.ErrorIs(t, consumeErr, context.Canceled)

	var allSeq []int
	var keyOneSeq []int
	for _, record := range records {
		allSeq = append(allSeq, record.seq)
		if record.key == "key-1" {
			keyOneSeq = append(keyOneSeq, record.seq)
		}
	}
	require.ElementsMatch(t, []int{1, 2, 3, 4}, allSeq)
	require.Equal(t, []int{1, 2, 4}, keyOneSeq)
}
