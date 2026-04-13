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
	cretry "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/retry"
)

// GroupConsumer wraps a Sarama consumer group with ordered shard processing.
// GroupConsumer 封装 Sarama 消费者组，提供有序分片处理能力。
type GroupConsumer struct {
	// cfg 是消费者组的完整配置。
	cfg GroupConsumerConfig
	// group 是底层 Sarama 消费者组实例。
	group sarama.ConsumerGroup
	// dlq 是死信队列写入器，用于发送处理失败的消息。
	dlq *xsarama.DLQWriter
	// closeOnce 保证 Close 操作只执行一次。
	closeOnce sync.Once
	// closeErr 保存关闭时遇到的错误。
	closeErr error
}

// NewGroupConsumer creates a configured Sarama consumer-group wrapper.
// 根据配置创建 GroupConsumer 实例，包括验证配置、创建消费者组、创建 DLQ 写入器。
func NewGroupConsumer(cfg GroupConsumerConfig) (*GroupConsumer, error) {
	// 验证配置完整性
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// 创建 Sarama 消费者组
	group, err := xsarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, cfg.SaramaConfig)
	if err != nil {
		return nil, err
	}

	// 创建死信队列写入器
	dlq, err := newDLQWriter(cfg.Brokers, cfg.SaramaConfig, cfg.DLQ)
	if err != nil {
		// 创建 DLQ 失败时回滚：关闭已创建的消费者组
		_ = group.Close()
		return nil, err
	}

	return &GroupConsumer{cfg: cfg, group: group, dlq: dlq}, nil
}

// Consume starts consuming in a rebalance-safe loop.
// 在 rebalance 安全的循环中消费消息，每次 rebalance 创建新的 Handler。
func (c *GroupConsumer) Consume(ctx context.Context, business consume.HandlerFunc) error {
	var err error
	// 参数校验：nil receiver、nil handler、规范化 context
	if ctx, err = prepareConsumeCall(c == nil, "group consumer is nil", ctx, business); err != nil {
		return err
	}

	// rebalance 循环：每次 Consume 返回后检查 context 和致命错误
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// 为每次 rebalance 会话创建新的 Handler
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
			// 优先返回 context 错误
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("consume group: %w", err)
		}

		// 检查是否存在致命处理错误
		if fatalErr := h.FatalErr(); fatalErr != nil {
			return fatalErr
		}
	}
}

// Close releases consumer-group and owned DLQ producer.
// 依次关闭消费者组和 DLQ 写入器，使用 sync.Once 保证只关闭一次。
func (c *GroupConsumer) Close() error {
	if c == nil {
		return nil
	}

	c.closeOnce.Do(func() {
		var errs []error
		// 关闭消费者组
		if c.group != nil {
			if err := c.group.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		// 关闭 DLQ 写入器
		if c.dlq != nil {
			if err := c.dlq.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		c.closeErr = errors.Join(errs...)
	})

	return c.closeErr
}

// buildConsumeChain 为指定 topic 构建消费者中间件链。
func (c *GroupConsumer) buildConsumeChain(
	topic string,
	business consume.HandlerFunc,
) consume.HandlerFunc {
	return consume.Compose(c.handlersForTopic(topic), business)
}

// handlersForTopic 收集指定 topic 的消费者中间件处理器列表。
func (c *GroupConsumer) handlersForTopic(topic string) []consume.Handler {
	// 根据 topic 配置选择全局或 topic 特定处理器
	selected := c.cfg.GlobalHandlers
	if topicCfg, ok := c.cfg.TopicHandlers[topic]; ok {
		selected = selectTopicHandlers(selected, topicCfg.Mode, topicCfg.Handlers)
	}

	// 构建基础处理器：日志 + 重试
	handlers := baseConsumeHandlers(
		c.cfg.Logger,
		c.cfg.LoggerHandlerEnabled,
		cretry.New(
			c.cfg.RetryConfig,
			c.cfg.ExhaustedPolicy,
			c.cfg.FailureHook,
			c.cfg.Logger,
			c.dlq,
		),
		len(selected),
	)
	handlers = append(handlers, selected...)
	return handlers
}

// extractLogicalKey 从消息中提取用于分片路由的逻辑键。
func (c *GroupConsumer) extractLogicalKey(msg *sarama.ConsumerMessage) (string, error) {
	return extractConsumeLogicalKey(c.cfg.KeyExtractor, msg)
}

// routeShard 根据逻辑键计算分片索引。
func (c *GroupConsumer) routeShard(logicalKey string) int {
	return router.ShardForKey(logicalKey, c.cfg.ShardCount)
}

// newDLQWriter 根据 DLQ 配置创建死信队列写入器。
// 如果 DLQ 配置为 nil 则返回 nil。
func newDLQWriter(
	brokers []string,
	saramaCfg *sarama.Config,
	dlqCfg *DLQConfig,
) (*xsarama.DLQWriter, error) {
	// 未配置 DLQ 则不创建
	if dlqCfg == nil {
		return nil, nil
	}

	// 委托 transport 层创建 DLQ 写入器
	return xsarama.NewDLQWriter(xsarama.DLQWriterConfig{
		Topic:    dlqCfg.Topic,
		Producer: dlqCfg.Producer,
		Brokers:  brokers,
		Config:   saramaCfg,
	})
}
