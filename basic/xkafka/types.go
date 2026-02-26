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
	"time"

	"github.com/IBM/sarama"

	pretry "github.com/codesjoy/pkg/basic/xkafka/internal/primitives/retry"
	cconsume "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/retry"
	pproduce "github.com/codesjoy/pkg/basic/xkafka/middleware/produce/retry"
)

const (
	// DefaultShardCount is the default count of key-hash shards.
	DefaultShardCount = 32
	// DefaultShardQueueSize is the default queue size of one shard worker.
	DefaultShardQueueSize = 1024

	// DefaultRetryInitialBackoff is the first retry wait duration.
	DefaultRetryInitialBackoff = cconsume.DefaultInitialBackoff
	// DefaultRetryMaxBackoff is the max retry wait duration.
	DefaultRetryMaxBackoff = cconsume.DefaultMaxBackoff
	// DefaultRetryMultiplier is the exponential retry multiplier.
	DefaultRetryMultiplier = cconsume.DefaultMultiplier
	// InfiniteRetries means retry forever.
	InfiniteRetries = pretry.InfiniteRetries

	// DefaultPartitionReconnectInitialBackoff is the first reconnect wait duration.
	DefaultPartitionReconnectInitialBackoff = 200 * time.Millisecond
	// DefaultPartitionReconnectMaxBackoff is the max reconnect wait duration.
	DefaultPartitionReconnectMaxBackoff = 5 * time.Second
	// DefaultPartitionReconnectMultiplier is reconnect exponential multiplier.
	DefaultPartitionReconnectMultiplier = 2.0

	// DefaultProducerWorkerCount is default worker count for parallel dispatch.
	DefaultProducerWorkerCount = 4
)

// KeyExtractor derives logical key for shard routing.
type KeyExtractor func(*sarama.ConsumerMessage) (string, error)

// ChainMode controls how topic handlers are combined with global handlers.
type ChainMode string

const (
	// ChainModeAppend appends topic handlers after global handlers.
	ChainModeAppend ChainMode = "append"
	// ChainModeReplace replaces global handlers with topic handlers.
	ChainModeReplace ChainMode = "replace"
)

// RetryConfig controls message retry behavior.
type RetryConfig = pretry.Config

// ExhaustedPolicy controls action when finite retries are exhausted.
type ExhaustedPolicy = cconsume.ExhaustedPolicy

const (
	// ExhaustedPolicyBlock keeps retrying and blocks the shard.
	ExhaustedPolicyBlock = cconsume.ExhaustedPolicyBlock
	// ExhaustedPolicyDLQCommit publishes to DLQ then marks offset as done.
	ExhaustedPolicyDLQCommit = cconsume.ExhaustedPolicyDLQCommit
	// ExhaustedPolicyStop stops consumption and returns an error.
	ExhaustedPolicyStop = cconsume.ExhaustedPolicyStop
)

// FailureStage marks current failure lifecycle stage.
type FailureStage = cconsume.FailureStage

const (
	// FailureStageRetry means a normal retry failure.
	FailureStageRetry = cconsume.FailureStageRetry
	// FailureStageExhausted means finite retries are exhausted.
	FailureStageExhausted = cconsume.FailureStageExhausted
	// FailureStageDLQ means message is being or has been handled by DLQ flow.
	FailureStageDLQ = cconsume.FailureStageDLQ
	// FailureStageStop means consumer stops due to policy.
	FailureStageStop = cconsume.FailureStageStop
)

// FailureEvent is emitted to FailureHook.
type FailureEvent = cconsume.Event

// FailureHook is called on retry/exhausted failure events.
type FailureHook = cconsume.FailureHook

// DLQConfig configures dead-letter publishing when retries are exhausted.
type DLQConfig struct {
	// Topic is the destination topic for dead-letter messages.
	Topic string
	// Producer is optional. If nil, xkafka creates and owns a SyncProducer.
	Producer sarama.SyncProducer
}

// BackoffConfig controls partition reconnect backoff strategy.
type BackoffConfig struct {
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
}

// OffsetStore persists per-partition next offsets for partition mode.
type OffsetStore interface {
	Load(
		ctx context.Context,
		topic string,
		partition int32,
	) (nextOffset int64, found bool, err error)
	Save(ctx context.Context, topic string, partition int32, nextOffset int64) error
}

// ProducerExhaustedPolicy controls action when finite retries are exhausted.
type ProducerExhaustedPolicy = pproduce.ExhaustedPolicy

const (
	// ProducerExhaustedPolicyBlock keeps retrying and blocks the pipeline.
	ProducerExhaustedPolicyBlock = pproduce.ExhaustedPolicyBlock
	// ProducerExhaustedPolicyStop returns error and stops current call.
	ProducerExhaustedPolicyStop = pproduce.ExhaustedPolicyStop
	// ProducerExhaustedPolicyDrop drops message and returns dropped error.
	ProducerExhaustedPolicyDrop = pproduce.ExhaustedPolicyDrop
)

// ProducerFailureStage marks producer failure lifecycle stage.
type ProducerFailureStage = pproduce.FailureStage

const (
	// ProducerFailureStageRetry means a normal retry failure.
	ProducerFailureStageRetry = pproduce.FailureStageRetry
	// ProducerFailureStageExhausted means finite retries are exhausted.
	ProducerFailureStageExhausted = pproduce.FailureStageExhausted
	// ProducerFailureStageStop means call stops due to policy.
	ProducerFailureStageStop = pproduce.FailureStageStop
	// ProducerFailureStageDrop means message is dropped due to policy.
	ProducerFailureStageDrop = pproduce.FailureStageDrop
)

// ProducerFailureEvent is emitted to ProducerFailureHook.
type ProducerFailureEvent = pproduce.Event

// ProducerFailureHook is called on producer retry/exhausted failure events.
type ProducerFailureHook = pproduce.FailureHook

// ProducerDispatchMode controls async dispatch behavior.
type ProducerDispatchMode string

const (
	// ProducerDispatchModeSerial routes all messages to one worker.
	ProducerDispatchModeSerial ProducerDispatchMode = "serial"
	// ProducerDispatchModeKeySharded routes by key hash modulo shard count.
	ProducerDispatchModeKeySharded ProducerDispatchMode = "key_sharded"
	// ProducerDispatchModeParallel routes by round-robin across workers.
	ProducerDispatchModeParallel ProducerDispatchMode = "parallel"

	// DefaultProducerDispatchMode is default async dispatch mode.
	DefaultProducerDispatchMode = ProducerDispatchModeKeySharded
)

// ProducerDispatchConfig controls async runtime queueing and routing.
type ProducerDispatchConfig struct {
	Mode        ProducerDispatchMode
	ShardCount  int
	WorkerCount int
	QueueSize   int
}
