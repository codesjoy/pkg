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
	rtgroup "github.com/codesjoy/pkg/basic/xkafka/internal/runtime/group"
	xsarama "github.com/codesjoy/pkg/basic/xkafka/internal/transport/sarama"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
	clogger "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/logger"
	cretry "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/retry"
)

// GroupConsumer wraps a Sarama consumer group with ordered shard processing.
type GroupConsumer struct {
	cfg GroupConsumerConfig

	group sarama.ConsumerGroup
	dlq   *xsarama.DLQWriter

	closeOnce sync.Once
	closeErr  error
}

// NewGroupConsumer creates a configured Sarama consumer-group wrapper.
func NewGroupConsumer(cfg GroupConsumerConfig) (*GroupConsumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	group, err := xsarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, cfg.SaramaConfig)
	if err != nil {
		return nil, err
	}

	dlq, err := newDLQWriter(cfg.Brokers, cfg.SaramaConfig, cfg.DLQ)
	if err != nil {
		_ = group.Close()
		return nil, err
	}

	return &GroupConsumer{cfg: cfg, group: group, dlq: dlq}, nil
}

// Consume starts consuming in a rebalance-safe loop.
func (c *GroupConsumer) Consume(ctx context.Context, business consume.HandlerFunc) error {
	if c == nil {
		return errors.New("group consumer is nil")
	}
	if business == nil {
		return consume.ErrNilHandlerFunc
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		h := rtgroup.NewHandler(rtgroup.Config{
			ShardCount:        c.cfg.ShardCount,
			ShardQueueSize:    c.cfg.ShardQueueSize,
			ExtractLogicalKey: c.extractLogicalKey,
			ShardForKey:       c.routeShard,
			BuildChain:        c.buildConsumeChain,
			Business:          business,
		})
		err := c.group.Consume(ctx, c.cfg.Topics, h)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("consume group: %w", err)
		}

		if fatalErr := h.FatalErr(); fatalErr != nil {
			return fatalErr
		}
	}
}

// Close releases consumer-group and owned DLQ producer.
func (c *GroupConsumer) Close() error {
	if c == nil {
		return nil
	}

	c.closeOnce.Do(func() {
		var errs []error
		if c.group != nil {
			if err := c.group.Close(); err != nil {
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

func (c *GroupConsumer) buildConsumeChain(
	topic string,
	business consume.HandlerFunc,
) consume.HandlerFunc {
	return consume.Compose(c.handlersForTopic(topic), business)
}

func (c *GroupConsumer) handlersForTopic(topic string) []consume.Handler {
	handlers := make([]consume.Handler, 0, len(c.cfg.GlobalHandlers)+3)
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

	selected := c.cfg.GlobalHandlers
	if topicCfg, ok := c.cfg.TopicHandlers[topic]; ok {
		if topicCfg.Mode == ChainModeReplace {
			selected = topicCfg.Handlers
		} else {
			selected = append(append([]consume.Handler(nil), selected...), topicCfg.Handlers...)
		}
	}

	handlers = append(handlers, selected...)
	return handlers
}

func (c *GroupConsumer) extractLogicalKey(msg *sarama.ConsumerMessage) (string, error) {
	key, err := c.cfg.KeyExtractor(msg)
	if err != nil {
		return "", err
	}
	if key == "" {
		return router.ConsumeFallbackKey(msg), nil
	}
	return key, nil
}

func (c *GroupConsumer) routeShard(logicalKey string) int {
	return router.ShardForKey(logicalKey, c.cfg.ShardCount)
}

func newDLQWriter(
	brokers []string,
	saramaCfg *sarama.Config,
	dlqCfg *DLQConfig,
) (*xsarama.DLQWriter, error) {
	if dlqCfg == nil {
		return nil, nil
	}

	return xsarama.NewDLQWriter(xsarama.DLQWriterConfig{
		Topic:    dlqCfg.Topic,
		Producer: dlqCfg.Producer,
		Brokers:  brokers,
		Config:   saramaCfg,
	})
}
