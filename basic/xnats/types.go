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
	"time"

	"github.com/nats-io/nats.go/jetstream"

	pretry "github.com/codesjoy/pkg/basic/xnats/internal/primitives/retry"
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
