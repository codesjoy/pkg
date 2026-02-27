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
	"fmt"
	"sync"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/router"
	rtpartition "github.com/codesjoy/pkg/basic/xkafka/internal/runtime/partition"
	xsarama "github.com/codesjoy/pkg/basic/xkafka/internal/transport/sarama"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
	clogger "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/logger"
	cretry "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/retry"
)

// PartitionConsumer wraps one Sarama partition consumer with ordered shard processing.
type PartitionConsumer struct {
	cfg PartitionConsumerConfig

	consumer sarama.Consumer
	dlq      *xsarama.DLQWriter

	closeOnce sync.Once
	closeErr  error
}

// NewPartitionConsumer creates a configured partition-mode Kafka consumer.
func NewPartitionConsumer(cfg PartitionConsumerConfig) (*PartitionConsumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	consumer, err := xsarama.NewConsumer(cfg.Brokers, cfg.SaramaConfig)
	if err != nil {
		return nil, err
	}

	dlq, err := newDLQWriter(cfg.Brokers, cfg.SaramaConfig, cfg.DLQ)
	if err != nil {
		_ = consumer.Close()
		return nil, err
	}

	return &PartitionConsumer{cfg: cfg, consumer: consumer, dlq: dlq}, nil
}

// Consume starts partition-mode consume loop with auto reconnect.
func (c *PartitionConsumer) Consume(ctx context.Context, business consume.HandlerFunc) error {
	if c == nil {
		return errors.New("partition consumer is nil")
	}
	if business == nil {
		return consume.ErrNilHandlerFunc
	}
	ctx = normalizeContext(ctx)

	runner := rtpartition.NewRunner(c.consumer, rtpartition.Config{
		Topic:                   c.cfg.Topic,
		Partition:               c.cfg.Partition,
		ShardCount:              c.cfg.ShardCount,
		ShardQueueSize:          c.cfg.ShardQueueSize,
		InitialOffset:           c.cfg.InitialOffset,
		OffsetStore:             c.cfg.OffsetStore,
		ReconnectInitialBackoff: c.cfg.Reconnect.InitialBackoff,
		ReconnectMaxBackoff:     c.cfg.Reconnect.MaxBackoff,
		ReconnectMultiplier:     c.cfg.Reconnect.Multiplier,
		ExtractLogicalKey:       c.extractLogicalKey,
		BuildChain:              c.buildConsumeChain,
		Logger:                  c.cfg.Logger,
	})
	return runner.Consume(ctx, business)
}

// Close releases partition consumer and owned DLQ producer.
func (c *PartitionConsumer) Close() error {
	if c == nil {
		return nil
	}

	c.closeOnce.Do(func() {
		var errs []error
		if c.consumer != nil {
			if err := c.consumer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		if c.dlq != nil {
			if err := c.dlq.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		c.closeErr = errors.Join(errs...)
	})

	return c.closeErr
}

func (c *PartitionConsumer) buildConsumeChain(
	topic string,
	business consume.HandlerFunc,
) consume.HandlerFunc {
	return consume.Compose(c.handlersForTopic(topic), business)
}

func (c *PartitionConsumer) handlersForTopic(_ string) []consume.Handler {
	handlers := make([]consume.Handler, 0, len(c.cfg.GlobalHandlers)+2)
	if boolValue(c.cfg.LoggerHandlerEnabled, true) {
		handlers = append(handlers, clogger.New(c.cfg.Logger))
	}
	handlers = append(handlers, cretry.New(
		c.cfg.RetryConfig,
		c.cfg.ExhaustedPolicy,
		c.cfg.FailureHook,
		c.cfg.Logger,
		c.dlq,
	))
	return append(handlers, c.cfg.GlobalHandlers...)
}

func (c *PartitionConsumer) extractLogicalKey(msg *sarama.ConsumerMessage) (string, error) {
	key, err := c.cfg.KeyExtractor(msg)
	if err != nil {
		return "", err
	}
	if key == "" {
		return router.ConsumeFallbackKey(msg), nil
	}
	return key, nil
}

func (c *PartitionConsumer) String() string {
	if c == nil {
		return "partition-consumer(nil)"
	}
	return fmt.Sprintf("partition-consumer(%s:%d)", c.cfg.Topic, c.cfg.Partition)
}
