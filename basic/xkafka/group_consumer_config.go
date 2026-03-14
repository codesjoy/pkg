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

// ConsumeTopicHandlers describes topic-specific consume middleware composition.
type ConsumeTopicHandlers struct {
	Mode     ChainMode
	Handlers []consume.Handler
}

// GroupConsumerConfig configures GroupConsumer.
type GroupConsumerConfig struct {
	Brokers []string
	GroupID string
	Topics  []string

	SaramaConfig *sarama.Config

	ShardCount     int
	ShardQueueSize int

	GlobalHandlers []consume.Handler
	TopicHandlers  map[string]ConsumeTopicHandlers

	KeyExtractor KeyExtractor

	Logger               *slog.Logger
	LoggerHandlerEnabled *bool

	RetryConfig     RetryConfig
	ExhaustedPolicy ExhaustedPolicy
	DLQ             *DLQConfig
	FailureHook     FailureHook
}

func defaultGroupConsumerConfig() GroupConsumerConfig {
	enabled := true
	saramaCfg := sarama.NewConfig()
	saramaCfg.Consumer.Return.Errors = true
	saramaCfg.Consumer.Offsets.AutoCommit.Enable = true
	saramaCfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRange(),
	}

	return GroupConsumerConfig{
		SaramaConfig:         saramaCfg,
		ShardCount:           DefaultShardCount,
		ShardQueueSize:       DefaultShardQueueSize,
		TopicHandlers:        make(map[string]ConsumeTopicHandlers),
		KeyExtractor:         KeyExtractor(router.DefaultConsumeKeyExtractor),
		Logger:               slog.Default(),
		LoggerHandlerEnabled: &enabled,
		RetryConfig:          cretry.DefaultConfig(),
		ExhaustedPolicy:      ExhaustedPolicyBlock,
	}
}

// Validate normalizes and validates group consumer config.
func (cfg *GroupConsumerConfig) Validate() error {
	if cfg == nil {
		return errors.New("group consumer config is nil")
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
	return cfg.normalizeAndValidateTopicHandlers()
}

func (cfg *GroupConsumerConfig) applyDefaults() {
	applyDefaultInt(&cfg.ShardCount, DefaultShardCount)
	applyDefaultInt(&cfg.ShardQueueSize, DefaultShardQueueSize)
	if cfg.TopicHandlers == nil {
		cfg.TopicHandlers = make(map[string]ConsumeTopicHandlers)
	}
	applyDefaultBool(&cfg.LoggerHandlerEnabled, true)
}

func (cfg *GroupConsumerConfig) normalizeInputs() {
	cfg.Brokers = normalizeStrings(cfg.Brokers)
	cfg.Topics = normalizeStrings(cfg.Topics)
	cfg.GroupID = strings.TrimSpace(cfg.GroupID)
}

func (cfg *GroupConsumerConfig) validateRequiredFields() error {
	if len(cfg.Brokers) == 0 {
		return errors.New("brokers are required")
	}
	if cfg.GroupID == "" {
		return errors.New("group ID is required")
	}
	if len(cfg.Topics) == 0 {
		return errors.New("topics are required")
	}
	if cfg.ShardCount <= 0 {
		return fmt.Errorf("shard count must be > 0, got %d", cfg.ShardCount)
	}
	if cfg.ShardQueueSize <= 0 {
		return fmt.Errorf("shard queue size must be > 0, got %d", cfg.ShardQueueSize)
	}
	return nil
}

func (cfg *GroupConsumerConfig) ensureDependencies() error {
	return ensureConsumeDependencies(&cfg.KeyExtractor, &cfg.Logger, &cfg.SaramaConfig)
}

func (cfg *GroupConsumerConfig) validateExhaustedPolicy() error {
	return normalizeConsumeExhaustedPolicy(&cfg.ExhaustedPolicy)
}

func (cfg *GroupConsumerConfig) normalizeAndValidateRetryConfig() error {
	return normalizeConsumeRetryConfig(&cfg.RetryConfig)
}

func (cfg *GroupConsumerConfig) validateDLQ() error {
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

func (cfg *GroupConsumerConfig) normalizeAndValidateTopicHandlers() error {
	for topic, handlers := range cfg.TopicHandlers {
		topicName := strings.TrimSpace(topic)
		if topicName == "" {
			return errors.New("topic handlers contain empty topic")
		}
		handlers.Mode = normalizeChainMode(handlers.Mode)
		if err := validateChainMode(topicName, handlers.Mode, "topic %q uses unsupported chain mode %q"); err != nil {
			return err
		}
		cfg.TopicHandlers[topic] = handlers
	}
	return nil
}
