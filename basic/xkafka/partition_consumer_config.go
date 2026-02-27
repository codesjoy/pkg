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
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/router"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
	cretry "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/retry"
)

// PartitionConsumerConfig configures PartitionConsumer.
type PartitionConsumerConfig struct {
	Brokers   []string
	Topic     string
	Partition int32

	SaramaConfig *sarama.Config

	ShardCount     int
	ShardQueueSize int

	GlobalHandlers []consume.Handler

	KeyExtractor KeyExtractor

	Logger               *slog.Logger
	LoggerHandlerEnabled *bool

	RetryConfig     RetryConfig
	ExhaustedPolicy ExhaustedPolicy
	DLQ             *DLQConfig
	FailureHook     FailureHook

	OffsetStore   OffsetStore
	InitialOffset int64
	Reconnect     BackoffConfig
}

func defaultPartitionConsumerConfig() PartitionConsumerConfig {
	enabled := true
	saramaCfg := sarama.NewConfig()
	saramaCfg.Consumer.Return.Errors = true

	return PartitionConsumerConfig{
		SaramaConfig:         saramaCfg,
		ShardCount:           DefaultShardCount,
		ShardQueueSize:       DefaultShardQueueSize,
		KeyExtractor:         KeyExtractor(router.DefaultConsumeKeyExtractor),
		Logger:               slog.Default(),
		LoggerHandlerEnabled: &enabled,
		RetryConfig:          cretry.DefaultConfig(),
		ExhaustedPolicy:      ExhaustedPolicyBlock,
		OffsetStore:          NewMemoryOffsetStore(),
		InitialOffset:        sarama.OffsetOldest,
		Reconnect:            defaultPartitionReconnectBackoff(),
	}
}

func defaultPartitionReconnectBackoff() BackoffConfig {
	return BackoffConfig{
		InitialBackoff: DefaultPartitionReconnectInitialBackoff,
		MaxBackoff:     DefaultPartitionReconnectMaxBackoff,
		Multiplier:     DefaultPartitionReconnectMultiplier,
	}
}

func normalizeBackoff(cfg BackoffConfig) BackoffConfig {
	normalized := cfg
	if normalized.InitialBackoff <= 0 {
		normalized.InitialBackoff = DefaultPartitionReconnectInitialBackoff
	}
	if normalized.MaxBackoff <= 0 {
		normalized.MaxBackoff = DefaultPartitionReconnectMaxBackoff
	}
	if normalized.MaxBackoff < normalized.InitialBackoff {
		normalized.MaxBackoff = normalized.InitialBackoff
	}
	if normalized.Multiplier <= 0 {
		normalized.Multiplier = DefaultPartitionReconnectMultiplier
	}
	if normalized.Multiplier < 1 {
		normalized.Multiplier = 1
	}
	return normalized
}

func validateBackoff(cfg BackoffConfig) error {
	if cfg.InitialBackoff <= 0 {
		return fmt.Errorf("initial reconnect backoff must be > 0, got %s", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff <= 0 {
		return fmt.Errorf("max reconnect backoff must be > 0, got %s", cfg.MaxBackoff)
	}
	if cfg.MaxBackoff < cfg.InitialBackoff {
		return fmt.Errorf(
			"max reconnect backoff (%s) must be >= initial reconnect backoff (%s)",
			cfg.MaxBackoff,
			cfg.InitialBackoff,
		)
	}
	if cfg.Multiplier < 1 {
		return fmt.Errorf("reconnect multiplier must be >= 1, got %f", cfg.Multiplier)
	}
	return nil
}

// Validate normalizes and validates partition consumer config.
func (cfg *PartitionConsumerConfig) Validate() error {
	if cfg == nil {
		return errors.New("partition consumer config is nil")
	}

	cfg.applyDefaults()
	cfg.normalizeInputs()

	if err := cfg.validateRequiredFields(); err != nil {
		return err
	}
	if err := cfg.ensureDependencies(); err != nil {
		return err
	}
	if err := cfg.validateExhaustedPolicy(); err != nil {
		return err
	}
	if err := cfg.normalizeAndValidateRetryConfig(); err != nil {
		return err
	}
	if err := cfg.validateDLQ(); err != nil {
		return err
	}
	return cfg.normalizeAndValidateReconnect()
}

func (cfg *PartitionConsumerConfig) applyDefaults() {
	if cfg.ShardCount == 0 {
		cfg.ShardCount = DefaultShardCount
	}
	if cfg.ShardQueueSize == 0 {
		cfg.ShardQueueSize = DefaultShardQueueSize
	}
	if cfg.InitialOffset == 0 {
		cfg.InitialOffset = sarama.OffsetOldest
	}
	if cfg.LoggerHandlerEnabled == nil {
		enabled := true
		cfg.LoggerHandlerEnabled = &enabled
	}
}

func (cfg *PartitionConsumerConfig) normalizeInputs() {
	cfg.Brokers = normalizeStrings(cfg.Brokers)
	cfg.Topic = strings.TrimSpace(cfg.Topic)
}

func (cfg *PartitionConsumerConfig) validateRequiredFields() error {
	if len(cfg.Brokers) == 0 {
		return errors.New("brokers are required")
	}
	if cfg.Topic == "" {
		return errors.New("topic is required")
	}
	if cfg.Partition < 0 {
		return fmt.Errorf("partition must be >= 0, got %d", cfg.Partition)
	}
	if cfg.ShardCount <= 0 {
		return fmt.Errorf("shard count must be > 0, got %d", cfg.ShardCount)
	}
	if cfg.ShardQueueSize <= 0 {
		return fmt.Errorf("shard queue size must be > 0, got %d", cfg.ShardQueueSize)
	}
	if cfg.InitialOffset < sarama.OffsetOldest {
		return fmt.Errorf(
			"initial offset must be >= %d (or %d/%d), got %d",
			sarama.OffsetOldest,
			sarama.OffsetOldest,
			sarama.OffsetNewest,
			cfg.InitialOffset,
		)
	}
	return nil
}

func (cfg *PartitionConsumerConfig) ensureDependencies() error {
	if cfg.KeyExtractor == nil {
		cfg.KeyExtractor = KeyExtractor(router.DefaultConsumeKeyExtractor)
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.OffsetStore == nil {
		cfg.OffsetStore = NewMemoryOffsetStore()
	}
	if cfg.SaramaConfig == nil {
		cfg.SaramaConfig = sarama.NewConfig()
	}
	cfg.SaramaConfig.Consumer.Return.Errors = true
	if cfg.SaramaConfig.Version == sarama.MinVersion {
		cfg.SaramaConfig.Version = sarama.V2_8_0_0
	}
	if err := cfg.SaramaConfig.Validate(); err != nil {
		return fmt.Errorf("invalid sarama config: %w", err)
	}
	return nil
}

func (cfg *PartitionConsumerConfig) validateExhaustedPolicy() error {
	switch cfg.ExhaustedPolicy {
	case "":
		cfg.ExhaustedPolicy = ExhaustedPolicyBlock
	case ExhaustedPolicyBlock, ExhaustedPolicyDLQCommit, ExhaustedPolicyStop:
	default:
		return fmt.Errorf("unsupported exhausted policy %q", cfg.ExhaustedPolicy)
	}
	return nil
}

func (cfg *PartitionConsumerConfig) normalizeAndValidateRetryConfig() error {
	if cfg.RetryConfig == (RetryConfig{}) {
		cfg.RetryConfig = cretry.DefaultConfig()
	}
	cfg.RetryConfig = cretry.NormalizeConfig(cfg.RetryConfig)
	if err := cretry.ValidateConfig(cfg.RetryConfig); err != nil {
		return err
	}
	return nil
}

func (cfg *PartitionConsumerConfig) validateDLQ() error {
	if cfg.ExhaustedPolicy != ExhaustedPolicyDLQCommit {
		return nil
	}
	if cfg.DLQ == nil {
		return errors.New("DLQ config is required when exhausted policy is dlq_commit")
	}
	cfg.DLQ.Topic = strings.TrimSpace(cfg.DLQ.Topic)
	if cfg.DLQ.Topic == "" {
		return errors.New("DLQ topic is required")
	}
	return nil
}

func (cfg *PartitionConsumerConfig) normalizeAndValidateReconnect() error {
	cfg.Reconnect = normalizeBackoff(cfg.Reconnect)
	if err := validateBackoff(cfg.Reconnect); err != nil {
		return err
	}
	return nil
}
