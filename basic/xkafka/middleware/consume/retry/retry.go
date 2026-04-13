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

package retry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/backoff"
	pretry "github.com/codesjoy/pkg/basic/xkafka/internal/primitives/retry"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

const (
	// DefaultInitialBackoff is the first retry wait duration.
	DefaultInitialBackoff = 100 * time.Millisecond
	// DefaultMaxBackoff is the max retry wait duration.
	DefaultMaxBackoff = 10 * time.Second
	// DefaultMultiplier is the exponential retry multiplier.
	DefaultMultiplier = 2.0
	// InfiniteRetries means retry forever.
	InfiniteRetries = pretry.InfiniteRetries
)

// ExhaustedPolicy controls action when finite retries are exhausted.
type ExhaustedPolicy string

const (
	// ExhaustedPolicyBlock keeps retrying and blocks the shard.
	ExhaustedPolicyBlock ExhaustedPolicy = "block"
	// ExhaustedPolicyDLQCommit publishes to DLQ then allows commit.
	ExhaustedPolicyDLQCommit ExhaustedPolicy = "dlq_commit"
	// ExhaustedPolicyStop stops consumption and returns an error.
	ExhaustedPolicyStop ExhaustedPolicy = "stop"
)

// FailureStage marks current failure lifecycle stage.
type FailureStage string

const (
	// FailureStageRetry means a normal retry failure.
	FailureStageRetry FailureStage = "retry"
	// FailureStageExhausted means finite retries are exhausted.
	FailureStageExhausted FailureStage = "exhausted"
	// FailureStageDLQ means message is being or has been handled by DLQ flow.
	FailureStageDLQ FailureStage = "dlq"
	// FailureStageStop means consumer stops due to policy.
	FailureStageStop FailureStage = "stop"
)

// Config controls message retry behavior.
type Config = pretry.Config

// Event is emitted to FailureHook.
type Event struct {
	Message    *sarama.ConsumerMessage
	LogicalKey string
	Shard      int
	Attempt    int
	Err        error
	Stage      FailureStage
	Policy     ExhaustedPolicy
	WillRetry  bool
	Timestamp  time.Time
}

// FailureHook is called on retry/exhausted failure events.
type FailureHook func(context.Context, Event)

// DLQPublisher publishes exhausted messages to a dead-letter destination.
type DLQPublisher interface {
	Publish(context.Context, Event) error
}

// Middleware retries message handling and applies exhausted policies.
// 消费者重试中间件，处理消息失败时按策略重试或转发到死信队列。
type Middleware struct {
	// config 是重试配置参数。
	config Config
	// exhausted 是重试耗尽后的处理策略。
	exhausted ExhaustedPolicy
	// failureHook 是失败事件回调。
	failureHook FailureHook
	// logger 是结构化日志记录器。
	logger *slog.Logger
	// dlqPublisher 是死信队列发布器。
	dlqPublisher DLQPublisher
}

// New creates retry middleware.
func New(
	config Config,
	exhausted ExhaustedPolicy,
	failureHook FailureHook,
	logger *slog.Logger,
	dlqPublisher DLQPublisher,
) *Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	if exhausted == "" {
		exhausted = ExhaustedPolicyBlock
	}
	return &Middleware{
		config:       NormalizeConfig(config),
		exhausted:    exhausted,
		failureHook:  failureHook,
		logger:       logger,
		dlqPublisher: dlqPublisher,
	}
}

// Handle executes retries around downstream handlers.
// 执行重试循环：设置 attempt、调用下游、失败发射事件、耗尽时按策略处理、退避等待。
func (m *Middleware) Handle(
	ctx context.Context,
	msg *consume.MessageContext,
	next consume.Next,
) error {
	if m == nil {
		return next(ctx, msg)
	}

	cfg := NormalizeConfig(m.config)
	exhaustedNotified := false

	// 重试循环
	for attempt := 1; ; attempt++ {
		// 检查 context 取消
		if err := ctx.Err(); err != nil {
			return err
		}
		// 设置当前尝试次数
		if msg != nil {
			msg.Attempt = attempt
		}

		// 调用下游处理器
		err := next(ctx, msg)
		if err == nil {
			return nil
		}

		// 检查是否已耗尽有限重试次数
		exhausted := IsRetryExhausted(cfg, attempt)
		// 发射重试失败事件
		m.emitFailure(
			ctx,
			m.newEvent(
				msg,
				err,
				attempt,
				FailureStageRetry,
				!exhausted || m.exhausted != ExhaustedPolicyStop,
			),
		)

		// 首次耗尽时发射耗尽事件（仅触发一次）
		if exhausted && !exhaustedNotified {
			exhaustedNotified = true
			m.emitFailure(
				ctx,
				m.newEvent(
					msg,
					err,
					attempt,
					FailureStageExhausted,
					m.exhausted == ExhaustedPolicyBlock,
				),
			)
		}

		// 耗尽后按策略处理
		if exhausted {
			switch m.exhausted {
			case ExhaustedPolicyDLQCommit:
				// 尝试发送到死信队列
				dlqErr := m.publishDLQ(ctx, msg, err, attempt)
				if dlqErr == nil {
					return nil
				}
				err = dlqErr
				m.logger.ErrorContext(ctx, "xkafka dlq publish failed", logAttrs(msg, err, 0)...)
			case ExhaustedPolicyStop:
				// 停止消费并返回错误
				stopErr := fmt.Errorf("handle message exhausted retries: %w", err)
				m.emitFailure(ctx, m.newEvent(msg, stopErr, attempt, FailureStageStop, false))
				m.logger.ErrorContext(
					ctx,
					"xkafka stop consuming message",
					logAttrs(msg, stopErr, 0)...)
				return stopErr
			case ExhaustedPolicyBlock:
				// 继续无限重试
			}
		}

		// 计算退避等待时间
		wait := Backoff(cfg, attempt)
		m.logger.WarnContext(ctx, "xkafka retrying message", logAttrs(msg, err, wait)...)
		// 退避等待，支持 context 取消
		if err := backoff.Wait(ctx, wait); err != nil {
			return err
		}
	}
}

// DefaultConfig returns recommended retry defaults.
func DefaultConfig() Config {
	return Config{
		MaxRetries:     InfiniteRetries,
		InitialBackoff: DefaultInitialBackoff,
		MaxBackoff:     DefaultMaxBackoff,
		Multiplier:     DefaultMultiplier,
	}
}

// NormalizeConfig fills zero-values with defaults.
func NormalizeConfig(cfg Config) Config {
	return pretry.NormalizeConfig(cfg, DefaultConfig())
}

// ValidateConfig validates retry settings.
func ValidateConfig(cfg Config) error {
	return pretry.ValidateConfig(cfg)
}

// IsRetryExhausted reports whether the current attempt reaches finite retry limits.
func IsRetryExhausted(cfg Config, attempt int) bool {
	return pretry.IsExhausted(cfg, attempt)
}

// Backoff returns retry backoff duration for the attempt.
func Backoff(cfg Config, attempt int) time.Duration {
	return pretry.Backoff(cfg, attempt)
}

// publishDLQ 将消息发送到死信队列，成功时发射 DLQ 事件。
func (m *Middleware) publishDLQ(
	ctx context.Context,
	msg *consume.MessageContext,
	handleErr error,
	attempt int,
) error {
	event := m.newEvent(msg, handleErr, attempt, FailureStageDLQ, false)
	// 检查 DLQ 发布器是否可用
	if m.dlqPublisher == nil {
		err := fmt.Errorf("dlq producer is not configured")
		m.emitFailure(ctx, m.newEvent(msg, err, attempt, FailureStageDLQ, true))
		return err
	}

	// 发布到 DLQ
	if err := m.dlqPublisher.Publish(ctx, event); err != nil {
		wrapped := fmt.Errorf("publish dlq message: %w", err)
		m.emitFailure(ctx, m.newEvent(msg, wrapped, attempt, FailureStageDLQ, true))
		return wrapped
	}

	// 发布成功，发射 DLQ 事件
	m.emitFailure(ctx, event)
	return nil
}

// emitFailure 调用失败事件回调（如果已配置）。
func (m *Middleware) emitFailure(ctx context.Context, event Event) {
	if m.failureHook == nil {
		return
	}
	m.failureHook(ctx, event)
}

// newEvent 构建一个失败事件。
func (m *Middleware) newEvent(
	msg *consume.MessageContext,
	err error,
	attempt int,
	stage FailureStage,
	willRetry bool,
) Event {
	event := Event{
		Attempt:   attempt,
		Err:       err,
		Stage:     stage,
		Policy:    m.exhausted,
		WillRetry: willRetry,
		Timestamp: time.Now(),
	}
	if msg == nil {
		return event
	}

	event.Message = msg.Message
	event.LogicalKey = msg.LogicalKey
	event.Shard = msg.Shard
	return event
}

// logAttrs 构建消费者重试日志的结构化属性列表。
func logAttrs(msg *consume.MessageContext, err error, wait time.Duration) []any {
	attrs := make([]any, 0, 9)
	if msg != nil && msg.Message != nil {
		attrs = append(attrs,
			slog.String("topic", msg.Message.Topic),
			slog.Int64("partition", int64(msg.Message.Partition)),
			slog.Int64("offset", msg.Message.Offset),
			slog.String("logical_key", msg.LogicalKey),
			slog.Int("shard", msg.Shard),
			slog.Int("attempt", msg.Attempt),
		)
	}
	if wait > 0 {
		attrs = append(attrs, slog.Duration("backoff", wait))
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	return attrs
}
