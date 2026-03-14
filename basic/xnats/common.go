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

package xnats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	pretry "github.com/codesjoy/pkg/basic/xnats/internal/primitives/retry"
	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	cretry "github.com/codesjoy/pkg/basic/xnats/middleware/consume/retry"
	pretrymw "github.com/codesjoy/pkg/basic/xnats/middleware/publish/retry"
)

const (
	// DefaultRetryInitialBackoff is the first retry wait duration.
	DefaultRetryInitialBackoff = cretry.DefaultInitialBackoff
	// DefaultRetryMaxBackoff is the max retry wait duration.
	DefaultRetryMaxBackoff = cretry.DefaultMaxBackoff
	// DefaultRetryMultiplier is the exponential retry multiplier.
	DefaultRetryMultiplier = cretry.DefaultMultiplier
	// InfiniteRetries means retry forever.
	InfiniteRetries = pretry.InfiniteRetries

	// DefaultPullBatchSize is the default pull fetch size.
	DefaultPullBatchSize = 1
	// DefaultPullMaxWait is the default pull fetch max wait.
	DefaultPullMaxWait = 5 * time.Second
	// DefaultPullIdleBackoff is the default idle wait when no messages arrive.
	DefaultPullIdleBackoff = 200 * time.Millisecond
	// DefaultConsumeShardQueueSize is the default queue size per ordered consume shard.
	DefaultConsumeShardQueueSize = 1024
)

var (
	// ErrNilPublishMessage indicates no message was supplied for publish.
	ErrNilPublishMessage = errors.New("publish message is nil")
	// ErrPublishSubjectRequired indicates a subject is required before publish.
	ErrPublishSubjectRequired = errors.New("publish subject is required")
	// ErrRequesterClosed indicates requester has been closed.
	ErrRequesterClosed = errors.New("requester is closed")
	// ErrPublisherClosed indicates publisher has been closed.
	ErrPublisherClosed = errors.New("publisher is closed")
	// ErrJetStreamPublisherClosed indicates JetStream publisher has been closed.
	ErrJetStreamPublisherClosed = errors.New("jetstream publisher is closed")
	// ErrSubscriberClosed indicates subscriber has been closed.
	ErrSubscriberClosed = errors.New("subscriber is closed")
	// ErrJetStreamConsumerClosed indicates JetStream consumer has been closed.
	ErrJetStreamConsumerClosed = errors.New("jetstream consumer is closed")
	// ErrSubscriberActive indicates one subscriber Consume call is already running.
	ErrSubscriberActive = errors.New("subscriber consume is already running")
	// ErrJetStreamConsumerActive indicates one consumer loop is already running.
	ErrJetStreamConsumerActive = errors.New("jetstream consumer is already running")
	// ErrJetStreamRequired indicates JetStream context is required.
	ErrJetStreamRequired = errors.New("jetstream context is required")
	// ErrPushConsumerRequiresDeliverSubject indicates the bound consumer is not push based.
	ErrPushConsumerRequiresDeliverSubject = errors.New("push consumer requires deliver subject")
	// ErrPullConsumerRequiresNoDeliverSubject indicates the bound consumer is not pull based.
	ErrPullConsumerRequiresNoDeliverSubject = errors.New(
		"pull consumer requires no deliver subject",
	)
)

// ChainMode controls how subject handlers are combined with global handlers.
type ChainMode string

const (
	// ChainModeAppend appends subject handlers after global handlers.
	ChainModeAppend ChainMode = "append"
	// ChainModeReplace replaces global handlers with subject handlers.
	ChainModeReplace ChainMode = "replace"
)

// RetryConfig controls retry behavior.
type RetryConfig = pretry.Config

// PublishExhaustedPolicy controls publish action when finite retries are exhausted.
type PublishExhaustedPolicy = pretrymw.ExhaustedPolicy

const (
	// PublishExhaustedPolicyBlock keeps retrying forever.
	PublishExhaustedPolicyBlock = pretrymw.ExhaustedPolicyBlock
	// PublishExhaustedPolicyStop stops and returns the current error.
	PublishExhaustedPolicyStop = pretrymw.ExhaustedPolicyStop
	// PublishExhaustedPolicyDrop drops the message and returns a dropped error.
	PublishExhaustedPolicyDrop = pretrymw.ExhaustedPolicyDrop
)

// PublishFailureStage marks publish retry lifecycle stage.
type PublishFailureStage = pretrymw.FailureStage

const (
	// PublishFailureStageRetry means a normal retry failure.
	PublishFailureStageRetry = pretrymw.FailureStageRetry
	// PublishFailureStageExhausted means finite retries are exhausted.
	PublishFailureStageExhausted = pretrymw.FailureStageExhausted
	// PublishFailureStageStop means current publish stops due to policy.
	PublishFailureStageStop = pretrymw.FailureStageStop
	// PublishFailureStageDrop means current publish is dropped due to policy.
	PublishFailureStageDrop = pretrymw.FailureStageDrop
)

// PublishFailureEvent is emitted to PublishFailureHook.
type PublishFailureEvent = pretrymw.Event

// PublishFailureHook is called on publish retry/exhausted failure events.
type PublishFailureHook = pretrymw.FailureHook

// ConsumeExhaustedPolicy controls consume action when finite retries are exhausted.
type ConsumeExhaustedPolicy = cretry.ExhaustedPolicy

const (
	// ConsumeExhaustedPolicyBlock keeps retrying forever.
	ConsumeExhaustedPolicyBlock = cretry.ExhaustedPolicyBlock
	// ConsumeExhaustedPolicyStop stops consumption and returns an error.
	ConsumeExhaustedPolicyStop = cretry.ExhaustedPolicyStop
	// ConsumeExhaustedPolicyDrop swallows the business error after transport-specific handling.
	ConsumeExhaustedPolicyDrop = cretry.ExhaustedPolicyDrop
)

// ConsumeFailureStage marks consume retry lifecycle stage.
type ConsumeFailureStage = cretry.FailureStage

const (
	// ConsumeFailureStageRetry means a normal retry failure.
	ConsumeFailureStageRetry = cretry.FailureStageRetry
	// ConsumeFailureStageExhausted means finite retries are exhausted.
	ConsumeFailureStageExhausted = cretry.FailureStageExhausted
	// ConsumeFailureStageStop means consumption stops due to policy.
	ConsumeFailureStageStop = cretry.FailureStageStop
	// ConsumeFailureStageDrop means the message is dropped due to policy.
	ConsumeFailureStageDrop = cretry.FailureStageDrop
)

// ConsumeFailureEvent is emitted to ConsumeFailureHook.
type ConsumeFailureEvent = cretry.Event

// ConsumeFailureHook is called on consume retry/exhausted failure events.
type ConsumeFailureHook = cretry.FailureHook

// ConsumeKeyExtractor derives logical key for ordered consume shard routing.
type ConsumeKeyExtractor func(*consume.MessageContext) (string, error)

// JetStreamConsumerMode controls how a bound JetStream consumer is consumed.
type JetStreamConsumerMode string

const (
	// JetStreamConsumerModePull consumes with pull fetch requests.
	JetStreamConsumerModePull JetStreamConsumerMode = "pull"
	// JetStreamConsumerModePush consumes by subscribing to the consumer deliver subject.
	JetStreamConsumerModePush JetStreamConsumerMode = "push"
)

// PublishOption configures native NATS connection creation.
type PublishOption = jetstream.PublishOpt

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func boolValue(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func boolPtr(value bool) *bool {
	return &value
}

func ensureLoggerHandlerEnabled(flag **bool) {
	if *flag == nil {
		enabled := true
		*flag = &enabled
	}
}

func ensureLogger(logger **slog.Logger) {
	if *logger == nil {
		*logger = slog.Default()
	}
}

func normalizeChainMode(mode ChainMode) ChainMode {
	if mode == "" {
		return ChainModeAppend
	}
	return mode
}

func validateChainMode(kind string, subject string, mode ChainMode) error {
	switch mode {
	case ChainModeAppend, ChainModeReplace:
		return nil
	default:
		return fmt.Errorf("%s subject %q uses unsupported chain mode %q", kind, subject, mode)
	}
}

func connect(urls []string, opts []nats.Option) (*nats.Conn, error) {
	normalized := normalizeStrings(urls)
	if len(normalized) == 0 {
		return nil, errors.New("nats URLs are required")
	}
	return nats.Connect(strings.Join(normalized, ","), opts...)
}

func newJetStream(conn *nats.Conn) (jetstream.JetStream, error) {
	if conn == nil {
		return nil, ErrJetStreamRequired
	}
	return jetstream.New(conn)
}

func drainConnection(conn *nats.Conn) error {
	if conn == nil {
		return nil
	}
	err := conn.Drain()
	conn.Close()
	return err
}

func drainSubscriptions(subs []*nats.Subscription) error {
	var errs []error
	for _, sub := range subs {
		if sub == nil {
			continue
		}
		if err := sub.Drain(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
