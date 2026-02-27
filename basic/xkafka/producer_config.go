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

	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
	ppretry "github.com/codesjoy/pkg/basic/xkafka/middleware/produce/retry"
)

// ProduceTopicHandlers describes topic-specific producer middleware composition.
type ProduceTopicHandlers struct {
	Mode     ChainMode
	Handlers []produce.Handler
}

// ProducerConfig configures Producer.
type ProducerConfig struct {
	Brokers      []string
	DefaultTopic string

	SaramaConfig *sarama.Config
	SyncProducer sarama.SyncProducer

	Dispatch ProducerDispatchConfig

	GlobalHandlers []produce.Handler
	TopicHandlers  map[string]ProduceTopicHandlers

	Logger               *slog.Logger
	LoggerHandlerEnabled *bool

	RetryConfig     RetryConfig
	ExhaustedPolicy ProducerExhaustedPolicy
	FailureHook     ProducerFailureHook
}

func defaultProducerDispatchConfig() ProducerDispatchConfig {
	return ProducerDispatchConfig{
		Mode:        DefaultProducerDispatchMode,
		ShardCount:  DefaultShardCount,
		WorkerCount: DefaultProducerWorkerCount,
		QueueSize:   DefaultShardQueueSize,
	}
}

func defaultProducerConfig() ProducerConfig {
	enabled := true
	saramaCfg := sarama.NewConfig()
	saramaCfg.Producer.Return.Successes = true

	return ProducerConfig{
		SaramaConfig:         saramaCfg,
		Dispatch:             defaultProducerDispatchConfig(),
		TopicHandlers:        make(map[string]ProduceTopicHandlers),
		Logger:               slog.Default(),
		LoggerHandlerEnabled: &enabled,
		RetryConfig:          ppretry.DefaultConfig(),
		ExhaustedPolicy:      ProducerExhaustedPolicyBlock,
	}
}

func normalizeProducerDispatchConfig(cfg ProducerDispatchConfig) ProducerDispatchConfig {
	normalized := cfg
	if normalized.Mode == "" {
		normalized.Mode = DefaultProducerDispatchMode
	}
	if normalized.ShardCount <= 0 {
		normalized.ShardCount = DefaultShardCount
	}
	if normalized.WorkerCount <= 0 {
		normalized.WorkerCount = DefaultProducerWorkerCount
	}
	if normalized.QueueSize <= 0 {
		normalized.QueueSize = DefaultShardQueueSize
	}
	return normalized
}

// Validate normalizes and validates producer config.
func (cfg *ProducerConfig) Validate() error {
	if cfg == nil {
		return errors.New("producer config is nil")
	}

	cfg.applyDefaults()
	cfg.normalizeInputs()

	if err := cfg.validateRequiredFields(); err != nil {
		return err
	}
	if err := cfg.normalizeAndValidateDispatch(); err != nil {
		return err
	}
	if err := cfg.ensureDependencies(); err != nil {
		return err
	}
	if err := cfg.normalizeAndValidateRetryConfig(); err != nil {
		return err
	}
	if err := cfg.validateExhaustedPolicy(); err != nil {
		return err
	}
	return cfg.normalizeAndValidateTopicHandlers()
}

func (cfg *ProducerConfig) applyDefaults() {
	if cfg.TopicHandlers == nil {
		cfg.TopicHandlers = make(map[string]ProduceTopicHandlers)
	}
	if cfg.LoggerHandlerEnabled == nil {
		enabled := true
		cfg.LoggerHandlerEnabled = &enabled
	}
}

func (cfg *ProducerConfig) normalizeInputs() {
	cfg.Brokers = normalizeStrings(cfg.Brokers)
	cfg.DefaultTopic = strings.TrimSpace(cfg.DefaultTopic)
}

func (cfg *ProducerConfig) validateRequiredFields() error {
	if len(cfg.Brokers) == 0 && cfg.SyncProducer == nil {
		return errors.New("producer brokers are required")
	}
	return nil
}

func (cfg *ProducerConfig) normalizeAndValidateDispatch() error {
	cfg.Dispatch = normalizeProducerDispatchConfig(cfg.Dispatch)
	if cfg.Dispatch.QueueSize <= 0 {
		return fmt.Errorf("producer queue size must be > 0, got %d", cfg.Dispatch.QueueSize)
	}

	switch cfg.Dispatch.Mode {
	case ProducerDispatchModeSerial:
		return nil
	case ProducerDispatchModeKeySharded:
		if cfg.Dispatch.ShardCount <= 0 {
			return fmt.Errorf("producer shard count must be > 0, got %d", cfg.Dispatch.ShardCount)
		}
		return nil
	case ProducerDispatchModeParallel:
		if cfg.Dispatch.WorkerCount <= 0 {
			return fmt.Errorf("producer worker count must be > 0, got %d", cfg.Dispatch.WorkerCount)
		}
		return nil
	default:
		return fmt.Errorf("unsupported producer dispatch mode %q", cfg.Dispatch.Mode)
	}
}

func (cfg *ProducerConfig) ensureDependencies() error {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.SaramaConfig == nil {
		cfg.SaramaConfig = sarama.NewConfig()
	}
	cfg.SaramaConfig.Producer.Return.Successes = true
	if cfg.SaramaConfig.Version == sarama.MinVersion {
		cfg.SaramaConfig.Version = sarama.V2_8_0_0
	}
	if err := cfg.SaramaConfig.Validate(); err != nil {
		return fmt.Errorf("invalid producer sarama config: %w", err)
	}
	return nil
}

func (cfg *ProducerConfig) normalizeAndValidateRetryConfig() error {
	if cfg.RetryConfig == (RetryConfig{}) {
		cfg.RetryConfig = ppretry.DefaultConfig()
	}
	cfg.RetryConfig = ppretry.NormalizeConfig(cfg.RetryConfig)
	if err := ppretry.ValidateConfig(cfg.RetryConfig); err != nil {
		return err
	}
	return nil
}

func (cfg *ProducerConfig) validateExhaustedPolicy() error {
	switch cfg.ExhaustedPolicy {
	case "":
		cfg.ExhaustedPolicy = ProducerExhaustedPolicyBlock
	case ProducerExhaustedPolicyBlock,
		ProducerExhaustedPolicyStop,
		ProducerExhaustedPolicyDrop:
	default:
		return fmt.Errorf("unsupported producer exhausted policy %q", cfg.ExhaustedPolicy)
	}
	return nil
}

func (cfg *ProducerConfig) normalizeAndValidateTopicHandlers() error {
	for topic, handlers := range cfg.TopicHandlers {
		topicName := strings.TrimSpace(topic)
		if topicName == "" {
			return errors.New("producer topic handlers contain empty topic")
		}
		if handlers.Mode == "" {
			handlers.Mode = ChainModeAppend
			cfg.TopicHandlers[topic] = handlers
		}
		switch handlers.Mode {
		case ChainModeAppend, ChainModeReplace:
		default:
			return fmt.Errorf(
				"producer topic %q uses unsupported chain mode %q",
				topicName,
				handlers.Mode,
			)
		}
	}
	return nil
}
