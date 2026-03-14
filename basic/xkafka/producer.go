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
type Producer struct {
	cfg ProducerConfig

	sender  *xsarama.SyncProducerSender
	runtime *rtproducer.Runtime

	closeOnce sync.Once
	closeErr  error
}

// NewProducer creates a configured producer wrapper.
func NewProducer(cfg ProducerConfig) (*Producer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	sender, err := xsarama.NewSyncProducerSender(xsarama.SyncProducerConfig{
		Brokers:  cfg.Brokers,
		Config:   cfg.SaramaConfig,
		Producer: cfg.SyncProducer,
	})
	if err != nil {
		return nil, err
	}

	producer := &Producer{cfg: cfg, sender: sender}
	runtime, err := rtproducer.NewRuntime(rtproducer.Config{
		Mode:        toRuntimeDispatchMode(cfg.Dispatch.Mode),
		QueueSize:   cfg.Dispatch.QueueSize,
		ShardCount:  cfg.Dispatch.ShardCount,
		WorkerCount: cfg.Dispatch.WorkerCount,
		Execute:     producer.executePrepared,
		ClosedErr:   ErrProducerClosed,
	})
	if err != nil {
		_ = sender.Close()
		return nil, err
	}
	producer.runtime = runtime
	return producer, nil
}

// Produce sends one message synchronously.
func (p *Producer) Produce(ctx context.Context, msg *produce.Message) (*produce.Result, error) {
	if p == nil {
		return nil, errors.New("producer is nil")
	}
	ctx = normalizeContext(ctx)

	prepared, err := p.prepareMessage(msg)
	if err != nil {
		return nil, err
	}

	return p.executePrepared(ctx, prepared)
}

// ProduceBatch sends messages sequentially and fails fast on first error.
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
	for i, msg := range msgs {
		result, err := p.Produce(ctx, msg)
		if err != nil {
			return results, fmt.Errorf("produce batch index %d: %w", i, err)
		}
		results[i] = result
	}
	return results, nil
}

// ProduceAsync queues one message into async runtime.
func (p *Producer) ProduceAsync(ctx context.Context, msg *produce.Message) (produce.Future, error) {
	if p == nil {
		return nil, errors.New("producer is nil")
	}
	ctx = normalizeContext(ctx)

	prepared, err := p.prepareMessage(msg)
	if err != nil {
		return nil, err
	}

	return p.runtime.Submit(ctx, prepared)
}

// Close stops runtime and closes owned producer.
func (p *Producer) Close() error {
	if p == nil {
		return nil
	}

	p.closeOnce.Do(func() {
		var errs []error
		if p.runtime != nil {
			if err := p.runtime.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		if p.sender != nil {
			if err := p.sender.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		p.closeErr = errors.Join(errs...)
	})

	return p.closeErr
}

func (p *Producer) executePrepared(
	ctx context.Context,
	msg *produce.Message,
) (*produce.Result, error) {
	msgCtx := &produce.MessageContext{
		Message:     msg,
		DispatchKey: router.ProduceDispatchKey(msg),
		ReceivedAt:  time.Now(),
	}

	chain := p.buildProduceChain(msg.Topic, p.send)
	result, err := chain(ctx, msgCtx)
	if err != nil {
		return nil, err
	}
	if result != nil && result.Attempt == 0 {
		result.Attempt = msgCtx.Attempt
	}
	return result, nil
}

func (p *Producer) prepareMessage(msg *produce.Message) (*produce.Message, error) {
	if msg == nil {
		return nil, ErrNilProducerMessage
	}

	prepared := cloneProducerMessage(msg)
	if prepared.Topic == "" {
		prepared.Topic = p.cfg.DefaultTopic
	}
	if prepared.Topic == "" {
		return nil, ErrProducerTopicRequired
	}
	return prepared, nil
}

func (p *Producer) buildProduceChain(
	topic string,
	business produce.HandlerFunc,
) produce.HandlerFunc {
	return produce.Compose(p.handlersForTopic(topic), business)
}

func (p *Producer) handlersForTopic(topic string) []produce.Handler {
	handlers := make([]produce.Handler, 0, len(p.cfg.GlobalHandlers)+2)
	if boolValue(p.cfg.LoggerHandlerEnabled, true) {
		handlers = append(handlers, plogger.New(p.cfg.Logger))
	}
	handlers = append(handlers, pretry.New(
		p.cfg.RetryConfig,
		p.cfg.ExhaustedPolicy,
		p.cfg.FailureHook,
		p.cfg.Logger,
	))

	selected := p.cfg.GlobalHandlers
	if topicCfg, ok := p.cfg.TopicHandlers[topic]; ok {
		selected = selectTopicHandlers(selected, topicCfg.Mode, topicCfg.Handlers)
	}

	handlers = append(handlers, selected...)
	return handlers
}

func (p *Producer) send(
	ctx context.Context,
	msgCtx *produce.MessageContext,
) (*produce.Result, error) {
	if msgCtx == nil {
		return nil, ErrNilProducerMessage
	}
	return p.sender.Send(ctx, msgCtx.Message)
}

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

func cloneProducerMessage(msg *produce.Message) *produce.Message {
	if msg == nil {
		return nil
	}

	cloned := *msg
	if len(msg.Key) > 0 {
		cloned.Key = append([]byte(nil), msg.Key...)
	}
	if len(msg.Value) > 0 {
		cloned.Value = append([]byte(nil), msg.Value...)
	}
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
