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

package xkafka

import (
	"context"
	"errors"
	"testing"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

func TestNewPartitionConsumerValidate(t *testing.T) {
	t.Parallel()

	_, err := NewPartitionConsumer(PartitionConsumerConfig{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "brokers are required")

	_, err = NewPartitionConsumer(PartitionConsumerConfig{Brokers: []string{"127.0.0.1:9092"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "topic is required")

	_, err = NewPartitionConsumer(PartitionConsumerConfig{
		Brokers:   []string{"127.0.0.1:9092"},
		Topic:     "orders",
		Partition: -1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "partition must be >= 0")
}

func TestPartitionConsumerConsumeNilHandler(t *testing.T) {
	t.Parallel()

	cfg := defaultPartitionConsumerConfig()
	cfg.Brokers = []string{"127.0.0.1:9092"}
	cfg.Topic = "orders"
	cfg.Partition = 0
	pc := &PartitionConsumer{cfg: cfg, consumer: &fakeSaramaConsumer{}}

	err := pc.Consume(context.Background(), nil)
	require.ErrorIs(t, err, consume.ErrNilHandlerFunc)
}

func TestPartitionConsumerCloseIdempotent(t *testing.T) {
	t.Parallel()

	consumer := &fakeSaramaConsumer{closeErr: errors.New("close failed")}
	pc := &PartitionConsumer{cfg: defaultPartitionConsumerConfig(), consumer: consumer}

	err1 := pc.Close()
	err2 := pc.Close()
	require.EqualError(t, err1, "close failed")
	require.EqualError(t, err2, "close failed")
	require.Equal(t, 1, consumer.closeCalls)
}

func TestPartitionConsumerNilReceiver(t *testing.T) {
	t.Parallel()

	var consumer *PartitionConsumer

	err := consumer.Consume(
		context.Background(),
		func(context.Context, *consume.MessageContext) error { return nil },
	)
	require.EqualError(t, err, "partition consumer is nil")
	require.NoError(t, consumer.Close())
	require.Equal(t, "partition-consumer(nil)", consumer.String())
}

func TestPartitionConsumerExtractLogicalKey(t *testing.T) {
	t.Parallel()

	msg := &sarama.ConsumerMessage{Topic: "orders", Partition: 1}

	t.Run("propagates extractor error", func(t *testing.T) {
		consumer := &PartitionConsumer{cfg: PartitionConsumerConfig{
			KeyExtractor: func(*sarama.ConsumerMessage) (string, error) {
				return "", errors.New("boom")
			},
		}}

		key, err := consumer.extractLogicalKey(msg)
		require.Error(t, err)
		require.Empty(t, key)
	})

	t.Run("falls back when extractor returns empty key", func(t *testing.T) {
		consumer := &PartitionConsumer{cfg: PartitionConsumerConfig{
			KeyExtractor: func(*sarama.ConsumerMessage) (string, error) {
				return "", nil
			},
		}}

		key, err := consumer.extractLogicalKey(msg)
		require.NoError(t, err)
		require.Equal(t, "orders:1", key)
	})

	t.Run("uses explicit key", func(t *testing.T) {
		consumer := &PartitionConsumer{cfg: PartitionConsumerConfig{
			KeyExtractor: func(*sarama.ConsumerMessage) (string, error) {
				return "order-1", nil
			},
		}}

		key, err := consumer.extractLogicalKey(msg)
		require.NoError(t, err)
		require.Equal(t, "order-1", key)
	})
}

func TestPartitionConsumerBuildConsumeChain(t *testing.T) {
	t.Parallel()

	marks := make([]string, 0, 2)
	enabled := false
	cfg := defaultPartitionConsumerConfig()
	cfg.LoggerHandlerEnabled = &enabled
	cfg.GlobalHandlers = []consume.Handler{
		consumeMarkerHandler("global", &marks),
	}

	consumer := &PartitionConsumer{cfg: cfg}
	require.Len(t, consumer.handlersForTopic("orders"), 2)

	chain := consumer.buildConsumeChain(
		"orders",
		func(context.Context, *consume.MessageContext) error {
			marks = append(marks, "business")
			return nil
		},
	)

	require.NoError(t, chain(context.Background(), &consume.MessageContext{}))
	require.Equal(t, []string{"global", "business"}, marks)
}

type fakeSaramaConsumer struct {
	closeCalls int
	closeErr   error
}

func (c *fakeSaramaConsumer) Topics() ([]string, error) {
	return nil, nil
}

func (c *fakeSaramaConsumer) Partitions(string) ([]int32, error) {
	return nil, nil
}

func (c *fakeSaramaConsumer) ConsumePartition(
	string,
	int32,
	int64,
) (sarama.PartitionConsumer, error) {
	return nil, errors.New("not implemented")
}

func (c *fakeSaramaConsumer) HighWaterMarks() map[string]map[int32]int64 {
	return nil
}

func (c *fakeSaramaConsumer) Close() error {
	c.closeCalls++
	return c.closeErr
}

func (c *fakeSaramaConsumer) Pause(map[string][]int32) {}

func (c *fakeSaramaConsumer) Resume(map[string][]int32) {}

func (c *fakeSaramaConsumer) PauseAll() {}

func (c *fakeSaramaConsumer) ResumeAll() {}
