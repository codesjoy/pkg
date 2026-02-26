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
type Middleware struct {
	config       Config
	exhausted    ExhaustedPolicy
	failureHook  FailureHook
	logger       *slog.Logger
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

	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if msg != nil {
			msg.Attempt = attempt
		}

		err := next(ctx, msg)
		if err == nil {
			return nil
		}

		exhausted := IsRetryExhausted(cfg, attempt)
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

		if exhausted {
			switch m.exhausted {
			case ExhaustedPolicyDLQCommit:
				dlqErr := m.publishDLQ(ctx, msg, err, attempt)
				if dlqErr == nil {
					return nil
				}
				err = dlqErr
				m.logger.ErrorContext(ctx, "xkafka dlq publish failed", logAttrs(msg, err, 0)...)
			case ExhaustedPolicyStop:
				stopErr := fmt.Errorf("handle message exhausted retries: %w", err)
				m.emitFailure(ctx, m.newEvent(msg, stopErr, attempt, FailureStageStop, false))
				m.logger.ErrorContext(
					ctx,
					"xkafka stop consuming message",
					logAttrs(msg, stopErr, 0)...)
				return stopErr
			case ExhaustedPolicyBlock:
				// Keep retrying forever after exhaustion.
			}
		}

		wait := Backoff(cfg, attempt)
		m.logger.WarnContext(ctx, "xkafka retrying message", logAttrs(msg, err, wait)...)
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

func (m *Middleware) publishDLQ(
	ctx context.Context,
	msg *consume.MessageContext,
	handleErr error,
	attempt int,
) error {
	event := m.newEvent(msg, handleErr, attempt, FailureStageDLQ, false)
	if m.dlqPublisher == nil {
		err := fmt.Errorf("dlq producer is not configured")
		m.emitFailure(ctx, m.newEvent(msg, err, attempt, FailureStageDLQ, true))
		return err
	}

	if err := m.dlqPublisher.Publish(ctx, event); err != nil {
		wrapped := fmt.Errorf("publish dlq message: %w", err)
		m.emitFailure(ctx, m.newEvent(msg, wrapped, attempt, FailureStageDLQ, true))
		return wrapped
	}

	m.emitFailure(ctx, event)
	return nil
}

func (m *Middleware) emitFailure(ctx context.Context, event Event) {
	if m.failureHook == nil {
		return
	}
	m.failureHook(ctx, event)
}

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
