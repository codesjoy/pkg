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
// 描述特定 topic 的消费者中间件组合方式。
type ConsumeTopicHandlers struct {
	// Mode 决定 topic 处理器与全局处理器的组合模式（追加或替换）。
	Mode ChainMode
	// Handlers 是该 topic 专属的中间件处理器列表。
	Handlers []consume.Handler
}

// GroupConsumerConfig configures GroupConsumer.
// 消费者组的完整配置。
type GroupConsumerConfig struct {
	// Brokers 是 Kafka 集群地址列表。
	Brokers []string
	// GroupID 是消费者组 ID。
	GroupID string
	// Topics 是订阅的 topic 列表。
	Topics []string

	// SaramaConfig 是底层 Sarama 配置，nil 时使用默认值。
	SaramaConfig *sarama.Config

	// ShardCount 是分片数量，用于按键有序处理。
	ShardCount int
	// ShardQueueSize 是每个分片队列的缓冲区大小。
	ShardQueueSize int

	// GlobalHandlers 是所有 topic 共享的中间件处理器。
	GlobalHandlers []consume.Handler
	// TopicHandlers 是按 topic 名字索引的专属中间件配置。
	TopicHandlers map[string]ConsumeTopicHandlers

	// KeyExtractor 从消息中提取用于分片路由的逻辑键。
	KeyExtractor KeyExtractor

	// Logger 是结构化日志记录器。
	Logger *slog.Logger
	// LoggerHandlerEnabled 控制是否启用日志中间件，nil 表示启用。
	LoggerHandlerEnabled *bool

	// RetryConfig 控制重试行为。
	RetryConfig RetryConfig
	// ExhaustedPolicy 控制有限重试耗尽后的策略。
	ExhaustedPolicy ExhaustedPolicy
	// DLQ 是死信队列配置，仅在耗尽策略为 DLQ 时必需。
	DLQ *DLQConfig
	// FailureHook 是失败事件回调函数。
	FailureHook FailureHook
}

// defaultGroupConsumerConfig 返回带有合理默认值的消费者组配置。
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
// 规范化并验证消费者组配置，依次执行：补默认值、规范化输入、校验必填字段、
// 确保依赖、校验耗尽策略、校验重试配置、校验 DLQ、校验 topic 处理器。
func (cfg *GroupConsumerConfig) Validate() error {
	if cfg == nil {
		return errors.New("group consumer config is nil")
	}

	// 补填默认值
	cfg.applyDefaults()
	// 规范化输入
	cfg.normalizeInputs()

	// 校验必填字段
	if err := cfg.validateRequiredFields(); err != nil {
		return err
	}
	// 确保依赖项就绪
	if err := cfg.ensureDependencies(); err != nil {
		return err
	}
	// 校验耗尽策略
	if err := cfg.validateExhaustedPolicy(); err != nil {
		return err
	}
	// 校验重试配置
	if err := cfg.normalizeAndValidateRetryConfig(); err != nil {
		return err
	}
	// 校验 DLQ 配置
	if err := cfg.validateDLQ(); err != nil {
		return err
	}
	// 校验 topic 处理器
	return cfg.normalizeAndValidateTopicHandlers()
}

// applyDefaults 为 nil/零值字段填充默认值。
func (cfg *GroupConsumerConfig) applyDefaults() {
	applyDefaultInt(&cfg.ShardCount, DefaultShardCount)
	applyDefaultInt(&cfg.ShardQueueSize, DefaultShardQueueSize)
	if cfg.TopicHandlers == nil {
		cfg.TopicHandlers = make(map[string]ConsumeTopicHandlers)
	}
	applyDefaultBool(&cfg.LoggerHandlerEnabled, true)
}

// normalizeInputs 规范化输入字符串。
func (cfg *GroupConsumerConfig) normalizeInputs() {
	cfg.Brokers = normalizeStrings(cfg.Brokers)
	cfg.Topics = normalizeStrings(cfg.Topics)
	cfg.GroupID = strings.TrimSpace(cfg.GroupID)
}

// validateRequiredFields 校验必填字段是否已设置。
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

// ensureDependencies 确保依赖项（keyExtractor、logger、sarama config）已就绪。
func (cfg *GroupConsumerConfig) ensureDependencies() error {
	return ensureConsumeDependencies(&cfg.KeyExtractor, &cfg.Logger, &cfg.SaramaConfig)
}

// validateExhaustedPolicy 规范化并校验耗尽策略。
func (cfg *GroupConsumerConfig) validateExhaustedPolicy() error {
	return normalizeConsumeExhaustedPolicy(&cfg.ExhaustedPolicy)
}

// normalizeAndValidateRetryConfig 规范化并校验重试配置。
func (cfg *GroupConsumerConfig) normalizeAndValidateRetryConfig() error {
	return normalizeConsumeRetryConfig(&cfg.RetryConfig)
}

// validateDLQ 校验 DLQ 配置，仅在耗尽策略为 DLQCommit 时要求配置。
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

// normalizeAndValidateTopicHandlers 规范化并校验 topic 处理器配置。
func (cfg *GroupConsumerConfig) normalizeAndValidateTopicHandlers() error {
	for topic, handlers := range cfg.TopicHandlers {
		topicName := strings.TrimSpace(topic)
		if topicName == "" {
			return errors.New("topic handlers contain empty topic")
		}
		handlers.Mode = normalizeChainMode(handlers.Mode)
		if err := validateChainMode(
			topicName,
			handlers.Mode,
			"topic %q uses unsupported chain mode %q",
		); err != nil {
			return err
		}
		cfg.TopicHandlers[topic] = handlers
	}
	return nil
}
