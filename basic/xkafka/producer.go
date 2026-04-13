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
	"time"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/router"
	rtproducer "github.com/codesjoy/pkg/basic/xkafka/internal/runtime/producer"
	xsarama "github.com/codesjoy/pkg/basic/xkafka/internal/transport/sarama"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
	plogger "github.com/codesjoy/pkg/basic/xkafka/middleware/produce/logger"
	pretry "github.com/codesjoy/pkg/basic/xkafka/middleware/produce/retry"
)

var (
	// ErrProducerClosed indicates producer is already closed.
	ErrProducerClosed = errors.New("producer is closed")
	// ErrNilProducerMessage indicates produce message is nil.
	ErrNilProducerMessage = errors.New("producer message is nil")
	// ErrProducerTopicRequired indicates topic cannot be resolved.
	ErrProducerTopicRequired = errors.New("producer topic is required")
	// ErrProducerDropped indicates retry policy dropped one message.
	ErrProducerDropped = pretry.ErrMessageDropped
)

// Producer wraps one Sarama SyncProducer with sync/batch/async capabilities.
// Producer 封装了 Sarama SyncProducer，提供同步、批量、异步发送能力。
type Producer struct {
	// cfg 是生产者的完整配置。
	cfg ProducerConfig
	// sender 是底层 Sarama 同步生产者的发送适配器。
	sender *xsarama.SyncProducerSender
	// runtime 是异步消息分发运行时，用于 ProduceAsync。
	runtime *rtproducer.Runtime
	// closeOnce 保证 Close 操作只执行一次。
	closeOnce sync.Once
	// closeErr 保存关闭时遇到的错误。
	closeErr error
}

// NewProducer creates a configured producer wrapper.
// 根据配置创建 Producer 实例，包括验证配置、创建发送器、初始化异步运行时。
func NewProducer(cfg ProducerConfig) (*Producer, error) {
	// 验证配置完整性
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// 创建 Sarama 同步生产者发送器
	sender, err := xsarama.NewSyncProducerSender(xsarama.SyncProducerConfig{
		Brokers:  cfg.Brokers,
		Config:   cfg.SaramaConfig,
		Producer: cfg.SyncProducer,
	})
	if err != nil {
		return nil, err
	}

	producer := &Producer{cfg: cfg, sender: sender}

	// 创建异步消息分发运行时
	runtime, err := rtproducer.NewRuntime(rtproducer.Config{
		Mode:        toRuntimeDispatchMode(cfg.Dispatch.Mode),
		QueueSize:   cfg.Dispatch.QueueSize,
		ShardCount:  cfg.Dispatch.ShardCount,
		WorkerCount: cfg.Dispatch.WorkerCount,
		Execute:     producer.executePrepared,
		ClosedErr:   ErrProducerClosed,
	})
	if err != nil {
		// 创建运行时失败时回滚：关闭已创建的 sender
		_ = sender.Close()
		return nil, err
	}
	producer.runtime = runtime
	return producer, nil
}

// Produce sends one message synchronously.
// 同步发送一条消息，会经过中间件链处理。
func (p *Producer) Produce(ctx context.Context, msg *produce.Message) (*produce.Result, error) {
	if p == nil {
		return nil, errors.New("producer is nil")
	}
	// 规范化 context，nil 时替换为 Background
	ctx = normalizeContext(ctx)

	// 预处理消息：克隆、补填 topic
	prepared, err := p.prepareMessage(msg)
	if err != nil {
		return nil, err
	}

	// 执行中间件链并发送
	return p.executePrepared(ctx, prepared)
}

// ProduceBatch sends messages sequentially and fails fast on first error.
// It is kept for compatibility and is not suitable for per-item acknowledgement
// flows such as xevent outbox relays.
// 按顺序逐条发送消息，遇到第一条错误立即返回（快速失败）。
func (p *Producer) ProduceBatch(
	ctx context.Context,
	msgs ...*produce.Message,
) ([]*produce.Result, error) {
	if p == nil {
		return nil, errors.New("producer is nil")
	}
	ctx = normalizeContext(ctx)
	if len(msgs) == 0 {
		return nil, nil
	}

	results := make([]*produce.Result, len(msgs))
	// 顺序发送，快速失败
	for i, msg := range msgs {
		result, err := p.Produce(ctx, msg)
		if err != nil {
			// 遇到错误立即返回，已成功的结果也会返回
			return results, fmt.Errorf("produce batch index %d: %w", i, err)
		}
		results[i] = result
	}
	return results, nil
}

// ProduceBatchReport sends messages and returns a per-item outcome vector.
// A top-level error is returned only for call-level failures such as a nil
// producer or a context that is already canceled before the call starts.
// 逐条发送消息并返回每条的独立结果，仅当调用级别出错时返回顶层错误。
func (p *Producer) ProduceBatchReport(
	ctx context.Context,
	msgs ...*produce.Message,
) ([]produce.BatchItemResult, error) {
	if p == nil {
		return nil, errors.New("producer is nil")
	}
	ctx = normalizeContext(ctx)
	// 调用开始前检查 context 是否已取消
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}

	results := make([]produce.BatchItemResult, len(msgs))
	for i, msg := range msgs {
		// 每次发送前检查 context 是否已取消
		if err := ctx.Err(); err != nil {
			// 剩余未发送的消息标记为 context 错误
			for j := i; j < len(msgs); j++ {
				results[j].Err = err
			}
			return results, nil
		}

		result, err := p.Produce(ctx, msg)
		results[i] = produce.BatchItemResult{
			Result: result,
			Err:    err,
		}
	}
	return results, nil
}

// ProduceAsync queues one message into async runtime.
// 异步发送一条消息，消息进入运行时队列后立即返回 Future。
func (p *Producer) ProduceAsync(ctx context.Context, msg *produce.Message) (produce.Future, error) {
	if p == nil {
		return nil, errors.New("producer is nil")
	}
	ctx = normalizeContext(ctx)

	// 预处理消息：克隆、补填 topic
	prepared, err := p.prepareMessage(msg)
	if err != nil {
		return nil, err
	}

	// 提交到异步运行时
	return p.runtime.Submit(ctx, prepared)
}

// Close stops runtime and closes owned producer.
// 关闭异步运行时和发送器，使用 sync.Once 保证只关闭一次。
func (p *Producer) Close() error {
	if p == nil {
		return nil
	}

	p.closeOnce.Do(func() {
		var errs []error
		// 先关闭异步运行时，停止工作协程
		if p.runtime != nil {
			if err := p.runtime.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		// 再关闭底层发送器
		if p.sender != nil {
			if err := p.sender.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		p.closeErr = errors.Join(errs...)
	})

	return p.closeErr
}

// executePrepared 构建中间件链并执行已预处理的消息。
func (p *Producer) executePrepared(
	ctx context.Context,
	msg *produce.Message,
) (*produce.Result, error) {
	// 构建 MessageContext，包含消息、分发键、接收时间
	msgCtx := &produce.MessageContext{
		Message:     msg,
		DispatchKey: router.ProduceDispatchKey(msg),
		ReceivedAt:  time.Now(),
	}

	// 构建 topic 对应的中间件链并以 send 作为最终处理器
	chain := p.buildProduceChain(msg.Topic, p.send)
	result, err := chain(ctx, msgCtx)
	if err != nil {
		return nil, err
	}
	// 回填 attempt：如果中间件链（如 retry）设置了 attempt 但 result 尚未更新
	if result != nil && result.Attempt == 0 {
		result.Attempt = msgCtx.Attempt
	}
	return result, nil
}

// prepareMessage 预处理消息：nil 检查、深克隆、补填默认 topic。
func (p *Producer) prepareMessage(msg *produce.Message) (*produce.Message, error) {
	// nil 消息检查
	if msg == nil {
		return nil, ErrNilProducerMessage
	}

	// 深克隆消息以避免外部修改
	prepared := cloneProducerMessage(msg)
	// 如果消息未指定 topic，使用默认 topic
	if prepared.Topic == "" {
		prepared.Topic = p.cfg.DefaultTopic
	}
	// topic 仍为空则报错
	if prepared.Topic == "" {
		return nil, ErrProducerTopicRequired
	}
	return prepared, nil
}

// buildProduceChain 为指定 topic 构建生产者中间件链。
func (p *Producer) buildProduceChain(
	topic string,
	business produce.HandlerFunc,
) produce.HandlerFunc {
	return produce.Compose(p.handlersForTopic(topic), business)
}

// handlersForTopic 收集指定 topic 的中间件处理器列表。
// 包含日志中间件、重试中间件以及全局/topic 特定处理器。
func (p *Producer) handlersForTopic(topic string) []produce.Handler {
	handlers := make([]produce.Handler, 0, len(p.cfg.GlobalHandlers)+2)
	// 按需添加日志中间件
	if boolValue(p.cfg.LoggerHandlerEnabled, true) {
		handlers = append(handlers, plogger.New(p.cfg.Logger))
	}
	// 添加重试中间件
	handlers = append(handlers, pretry.New(
		p.cfg.RetryConfig,
		p.cfg.ExhaustedPolicy,
		p.cfg.FailureHook,
		p.cfg.Logger,
	))

	// 根据 topic 配置选择全局处理器或 topic 特定处理器
	selected := p.cfg.GlobalHandlers
	if topicCfg, ok := p.cfg.TopicHandlers[topic]; ok {
		selected = selectTopicHandlers(selected, topicCfg.Mode, topicCfg.Handlers)
	}

	handlers = append(handlers, selected...)
	return handlers
}

// send 是中间件链的最终处理器，委托底层 sender 发送消息。
func (p *Producer) send(
	ctx context.Context,
	msgCtx *produce.MessageContext,
) (*produce.Result, error) {
	if msgCtx == nil {
		return nil, ErrNilProducerMessage
	}
	return p.sender.Send(ctx, msgCtx.Message)
}

// toRuntimeDispatchMode 将公开的 DispatchMode 转换为运行时内部的 DispatchMode。
func toRuntimeDispatchMode(mode ProducerDispatchMode) rtproducer.DispatchMode {
	switch mode {
	case ProducerDispatchModeSerial:
		return rtproducer.DispatchModeSerial
	case ProducerDispatchModeParallel:
		return rtproducer.DispatchModeParallel
	default:
		return rtproducer.DispatchModeKeySharded
	}
}

// cloneProducerMessage 深克隆一条生产者消息。
// 浅克隆结构体后对 Key、Value、Headers 进行深拷贝，防止外部修改影响内部状态。
func cloneProducerMessage(msg *produce.Message) *produce.Message {
	if msg == nil {
		return nil
	}

	// 浅克隆结构体
	cloned := *msg
	// 深拷贝 Key
	if len(msg.Key) > 0 {
		cloned.Key = append([]byte(nil), msg.Key...)
	}
	// 深拷贝 Value
	if len(msg.Value) > 0 {
		cloned.Value = append([]byte(nil), msg.Value...)
	}
	// 深拷贝 Headers
	if len(msg.Headers) > 0 {
		cloned.Headers = make([]sarama.RecordHeader, 0, len(msg.Headers))
		for _, header := range msg.Headers {
			item := sarama.RecordHeader{}
			if len(header.Key) > 0 {
				item.Key = append([]byte(nil), header.Key...)
			}
			if len(header.Value) > 0 {
				item.Value = append([]byte(nil), header.Value...)
			}
			cloned.Headers = append(cloned.Headers, item)
		}
	}
	return &cloned
}
