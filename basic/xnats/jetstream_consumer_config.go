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
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	cretry "github.com/codesjoy/pkg/basic/xnats/middleware/consume/retry"
)

// JetStreamConsumerConfig configures JetStreamConsumer.
type JetStreamConsumerConfig struct {
	URLs           []string
	Conn           *nats.Conn
	JetStream      jetstream.JetStream
	ConnectOptions []nats.Option

	Stream   string
	Consumer string
	Mode     JetStreamConsumerMode

	PullBatchSize  int
	PullMaxWait    time.Duration
	IdleBackoff    time.Duration
	ShardCount     int
	ShardQueueSize int
	KeyExtractor   ConsumeKeyExtractor

	GlobalHandlers  []consume.Handler
	SubjectHandlers map[string]ConsumeSubjectHandlers

	Logger               *slog.Logger
	LoggerHandlerEnabled *bool

	RetryConfig     RetryConfig
	ExhaustedPolicy ConsumeExhaustedPolicy
	FailureHook     ConsumeFailureHook
}

// Validate normalizes and validates JetStream consumer config.
func (cfg *JetStreamConsumerConfig) Validate() error {
	if cfg == nil {
		return errors.New("jetstream consumer config is nil")
	}

	if cfg.SubjectHandlers == nil {
		cfg.SubjectHandlers = make(map[string]ConsumeSubjectHandlers)
	}
	ensureLoggerHandlerEnabled(&cfg.LoggerHandlerEnabled)
	cfg.URLs = normalizeStrings(cfg.URLs)
	cfg.Stream = strings.TrimSpace(cfg.Stream)
	cfg.Consumer = strings.TrimSpace(cfg.Consumer)
	if cfg.Mode == "" {
		cfg.Mode = JetStreamConsumerModePull
	}
	if cfg.PullBatchSize <= 0 {
		cfg.PullBatchSize = DefaultPullBatchSize
	}
	if cfg.PullMaxWait <= 0 {
		cfg.PullMaxWait = DefaultPullMaxWait
	}
	if cfg.IdleBackoff <= 0 {
		cfg.IdleBackoff = DefaultPullIdleBackoff
	}
	if cfg.ShardCount == 0 {
		cfg.ShardCount = 1
	}
	if cfg.ShardQueueSize == 0 {
		cfg.ShardQueueSize = DefaultConsumeShardQueueSize
	}
	ensureLogger(&cfg.Logger)
	if len(cfg.URLs) == 0 && cfg.Conn == nil && cfg.JetStream == nil {
		return errors.New("jetstream consumer URLs or injected JetStream are required")
	}
	if cfg.Stream == "" {
		return errors.New("jetstream consumer stream is required")
	}
	if cfg.Consumer == "" {
		return errors.New("jetstream consumer name is required")
	}
	switch cfg.Mode {
	case JetStreamConsumerModePull, JetStreamConsumerModePush:
	default:
		return fmt.Errorf("unsupported jetstream consumer mode %q", cfg.Mode)
	}
	if cfg.PullBatchSize <= 0 {
		return fmt.Errorf("pull batch size must be > 0, got %d", cfg.PullBatchSize)
	}
	if cfg.PullMaxWait <= 0 {
		return fmt.Errorf("pull max wait must be > 0, got %s", cfg.PullMaxWait)
	}
	if cfg.IdleBackoff <= 0 {
		return fmt.Errorf("idle backoff must be > 0, got %s", cfg.IdleBackoff)
	}
	if cfg.ShardCount <= 0 {
		return fmt.Errorf("consume shard count must be > 0, got %d", cfg.ShardCount)
	}
	if cfg.ShardQueueSize <= 0 {
		return fmt.Errorf("consume shard queue size must be > 0, got %d", cfg.ShardQueueSize)
	}
	if cfg.ShardCount > 1 && cfg.KeyExtractor == nil {
		return errors.New("ordered consume requires key extractor when shard count > 1")
	}
	if cfg.RetryConfig == (RetryConfig{}) {
		cfg.RetryConfig = cretry.DefaultConfig()
	}
	cfg.RetryConfig = cretry.NormalizeConfig(cfg.RetryConfig)
	if err := cretry.ValidateConfig(cfg.RetryConfig); err != nil {
		return err
	}
	switch cfg.ExhaustedPolicy {
	case "":
		cfg.ExhaustedPolicy = ConsumeExhaustedPolicyBlock
	case ConsumeExhaustedPolicyBlock, ConsumeExhaustedPolicyStop, ConsumeExhaustedPolicyDrop:
	default:
		return fmt.Errorf("unsupported consume exhausted policy %q", cfg.ExhaustedPolicy)
	}
	for subject, handlers := range cfg.SubjectHandlers {
		name := strings.TrimSpace(subject)
		if name == "" {
			return errors.New("consume subject handlers contain empty subject")
		}
		handlers.Mode = normalizeChainMode(handlers.Mode)
		cfg.SubjectHandlers[subject] = handlers
		if err := validateChainMode("consume", name, handlers.Mode); err != nil {
			return err
		}
	}
	return nil
}
