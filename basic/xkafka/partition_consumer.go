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

	rtpartition "github.com/codesjoy/pkg/basic/xkafka/internal/runtime/partition"
	xsarama "github.com/codesjoy/pkg/basic/xkafka/internal/transport/sarama"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
	cretry "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/retry"
)

// PartitionConsumer wraps one Sarama partition consumer with ordered shard processing.
// PartitionConsumer 封装 Sarama 分区消费者，提供有序分片处理和自动重连能力。
type PartitionConsumer struct {
	// cfg 是分区消费者的完整配置。
	cfg PartitionConsumerConfig
	// consumer 是底层 Sarama 分区消费者实例。
	consumer sarama.Consumer
	// dlq 是死信队列写入器。
	dlq *xsarama.DLQWriter
	// closeOnce 保证 Close 操作只执行一次。
	closeOnce sync.Once
	// closeErr 保存关闭时遇到的错误。
	closeErr error
}

// NewPartitionConsumer creates a configured partition-mode Kafka consumer.
// 根据配置创建 PartitionConsumer 实例，包括验证配置、创建消费者、创建 DLQ 写入器。
func NewPartitionConsumer(cfg PartitionConsumerConfig) (*PartitionConsumer, error) {
	// 验证配置完整性
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// 创建 Sarama 分区消费者
	consumer, err := xsarama.NewConsumer(cfg.Brokers, cfg.SaramaConfig)
	if err != nil {
		return nil, err
	}

	// 创建死信队列写入器
	dlq, err := newDLQWriter(cfg.Brokers, cfg.SaramaConfig, cfg.DLQ)
	if err != nil {
		// 创建 DLQ 失败时回滚：关闭已创建的消费者
		_ = consumer.Close()
		return nil, err
	}

	return &PartitionConsumer{cfg: cfg, consumer: consumer, dlq: dlq}, nil
}

// Consume starts partition-mode consume loop with auto reconnect.
// 启动分区消费循环，支持自动重连。委托 Runner 处理实际消费逻辑。
func (c *PartitionConsumer) Consume(ctx context.Context, business consume.HandlerFunc) error {
	var err error
	// 参数校验：nil receiver、nil handler、规范化 context
	if ctx, err = prepareConsumeCall(c == nil, "partition consumer is nil", ctx, business); err != nil {
		return err
	}

	// 创建 Runner 并委托执行消费循环
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
// 依次关闭分区消费者和 DLQ 写入器，使用 sync.Once 保证只关闭一次。
func (c *PartitionConsumer) Close() error {
	if c == nil {
		return nil
	}

	c.closeOnce.Do(func() {
		var errs []error
		// 关闭分区消费者
		if c.consumer != nil {
			if err := c.consumer.Close(); err != nil {
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
func (c *PartitionConsumer) buildConsumeChain(
	topic string,
	business consume.HandlerFunc,
) consume.HandlerFunc {
	return consume.Compose(c.handlersForTopic(topic), business)
}

// handlersForTopic 收集指定 topic 的消费者中间件处理器列表。
// 注意：分区模式不支持按 topic 切换处理器，始终使用全局处理器。
func (c *PartitionConsumer) handlersForTopic(_ string) []consume.Handler {
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
		len(c.cfg.GlobalHandlers),
	)
	return append(handlers, c.cfg.GlobalHandlers...)
}

// extractLogicalKey 从消息中提取用于分片路由的逻辑键。
func (c *PartitionConsumer) extractLogicalKey(msg *sarama.ConsumerMessage) (string, error) {
	return extractConsumeLogicalKey(c.cfg.KeyExtractor, msg)
}

// String 返回分区消费者的可读标识，格式为 "partition-consumer(topic:partition)"。
func (c *PartitionConsumer) String() string {
	if c == nil {
		return "partition-consumer(nil)"
	}
	return fmt.Sprintf("partition-consumer(%s:%d)", c.cfg.Topic, c.cfg.Partition)
}
