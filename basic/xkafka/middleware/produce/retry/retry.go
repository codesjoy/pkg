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
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/backoff"
	pretry "github.com/codesjoy/pkg/basic/xkafka/internal/primitives/retry"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
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
	// ExhaustedPolicyBlock keeps retrying and blocks the pipeline.
	ExhaustedPolicyBlock ExhaustedPolicy = "block"
	// ExhaustedPolicyStop returns error and stops the current call.
	ExhaustedPolicyStop ExhaustedPolicy = "stop"
	// ExhaustedPolicyDrop drops message and returns dropped error.
	ExhaustedPolicyDrop ExhaustedPolicy = "drop"
)

// FailureStage marks current failure lifecycle stage.
type FailureStage string

const (
	// FailureStageRetry means a normal retry failure.
	FailureStageRetry FailureStage = "retry"
	// FailureStageExhausted means finite retries are exhausted.
	FailureStageExhausted FailureStage = "exhausted"
	// FailureStageStop means call stops due to policy.
	FailureStageStop FailureStage = "stop"
	// FailureStageDrop means message is dropped due to policy.
	FailureStageDrop FailureStage = "drop"
)

// Config controls message retry behavior.
type Config = pretry.Config

// Event is emitted to FailureHook.
type Event struct {
	Message   *produce.Message
	Attempt   int
	Err       error
	Stage     FailureStage
	Policy    ExhaustedPolicy
	WillRetry bool
	Timestamp time.Time
}

// FailureHook is called on retry/exhausted failure events.
type FailureHook func(context.Context, Event)

// ErrMessageDropped is returned when exhausted policy is drop.
var ErrMessageDropped = errors.New("producer message dropped after retries exhausted")

// Middleware retries message handling and applies exhausted policies.
// 生产者重试中间件，处理发送失败时按策略重试或丢弃消息。
type Middleware struct {
	// config 是重试配置参数。
	config Config
	// exhausted 是重试耗尽后的处理策略。
	exhausted ExhaustedPolicy
	// failureHook 是失败事件回调。
	failureHook FailureHook
	// logger 是结构化日志记录器。
	logger *slog.Logger
}

// New creates retry middleware.
func New(
	config Config,
	exhausted ExhaustedPolicy,
	failureHook FailureHook,
	logger *slog.Logger,
) *Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	if exhausted == "" {
		exhausted = ExhaustedPolicyBlock
	}
	return &Middleware{
		config:      NormalizeConfig(config),
		exhausted:   exhausted,
		failureHook: failureHook,
		logger:      logger,
	}
}

// Handle executes retries around downstream handlers.
// 执行重试循环：设置 attempt、调用下游、失败发射事件、耗尽时按策略处理、退避等待。
func (m *Middleware) Handle(
	ctx context.Context,
	msg *produce.MessageContext,
	next produce.Next,
) (*produce.Result, error) {
	if m == nil {
		return next(ctx, msg)
	}

	cfg := NormalizeConfig(m.config)
	exhaustedNotified := false

	// 重试循环
	for attempt := 1; ; attempt++ {
		// 检查 context 取消
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// 设置当前尝试次数
		if msg != nil {
			msg.Attempt = attempt
		}

		// 调用下游处理器
		result, err := next(ctx, msg)
		if err == nil {
			// 回填 attempt 到 result
			if result != nil && result.Attempt == 0 {
				result.Attempt = attempt
			}
			return result, nil
		}

		// 检查是否已耗尽有限重试次数
		exhausted := IsRetryExhausted(cfg, attempt)
		willRetry := !exhausted || m.exhausted == ExhaustedPolicyBlock
		// 发射重试失败事件
		m.emitFailure(ctx, m.newEvent(msg, err, attempt, FailureStageRetry, willRetry))

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
			case ExhaustedPolicyStop:
				// 停止并返回错误
				stopErr := fmt.Errorf("produce message exhausted retries: %w", err)
				m.emitFailure(ctx, m.newEvent(msg, stopErr, attempt, FailureStageStop, false))
				m.logger.ErrorContext(ctx, "xkafka producer stop", logAttrs(msg, stopErr, 0)...)
				return nil, stopErr
			case ExhaustedPolicyDrop:
				// 丢弃消息并返回错误
				dropErr := fmt.Errorf("%w: %v", ErrMessageDropped, err)
				m.emitFailure(ctx, m.newEvent(msg, dropErr, attempt, FailureStageDrop, false))
				m.logger.ErrorContext(ctx, "xkafka producer drop", logAttrs(msg, dropErr, 0)...)
				return nil, dropErr
			case ExhaustedPolicyBlock:
				// 继续无限重试
			}
		}

		// 计算退避等待时间
		wait := Backoff(cfg, attempt)
		m.logger.WarnContext(ctx, "xkafka producer retrying", logAttrs(msg, err, wait)...)
		// 退避等待，支持 context 取消
		if err := backoff.Wait(ctx, wait); err != nil {
			return nil, err
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

// IsRetryExhausted reports whether current attempt reaches finite retry limit.
func IsRetryExhausted(cfg Config, attempt int) bool {
	return pretry.IsExhausted(cfg, attempt)
}

// Backoff returns retry backoff duration for the attempt.
func Backoff(cfg Config, attempt int) time.Duration {
	return pretry.Backoff(cfg, attempt)
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
	msg *produce.MessageContext,
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
	return event
}

// logAttrs 构建生产者重试日志的结构化属性列表。
func logAttrs(msg *produce.MessageContext, err error, wait time.Duration) []any {
	attrs := make([]any, 0, 8)
	if msg != nil {
		attrs = append(attrs,
			slog.Int("attempt", msg.Attempt),
			slog.String("dispatch_key", msg.DispatchKey),
			slog.Int("worker", msg.Worker),
		)
		if msg.Message != nil {
			attrs = append(attrs,
				slog.String("topic", msg.Message.Topic),
				slog.String("key", string(msg.Message.Key)),
			)
		}
	}
	if wait > 0 {
		attrs = append(attrs, slog.Duration("backoff", wait))
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	return attrs
}
