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
// 描述特定 topic 的生产者中间件组合方式。
type ProduceTopicHandlers struct {
	// Mode 决定 topic 处理器与全局处理器的组合模式（追加或替换）。
	Mode ChainMode
	// Handlers 是该 topic 专属的中间件处理器列表。
	Handlers []produce.Handler
}

// ProducerConfig configures Producer.
// Producer 的完整配置，涵盖 broker、发送器、分发、中间件、重试等。
type ProducerConfig struct {
	// Brokers 是 Kafka 集群地址列表。
	Brokers []string
	// DefaultTopic 是消息未指定 topic 时的默认目标 topic。
	DefaultTopic string

	// SaramaConfig 是底层 Sarama 配置，nil 时使用默认值。
	SaramaConfig *sarama.Config
	// SyncProducer 是外部传入的同步生产者，nil 时自动创建。
	SyncProducer sarama.SyncProducer

	// Dispatch 控制异步消息分发的路由策略。
	Dispatch ProducerDispatchConfig

	// GlobalHandlers 是所有 topic 共享的中间件处理器。
	GlobalHandlers []produce.Handler
	// TopicHandlers 是按 topic 名字索引的专属中间件配置。
	TopicHandlers map[string]ProduceTopicHandlers

	// Logger 是结构化日志记录器。
	Logger *slog.Logger
	// LoggerHandlerEnabled 控制是否启用日志中间件，nil 表示启用。
	LoggerHandlerEnabled *bool

	// RetryConfig 控制重试行为。
	RetryConfig RetryConfig
	// ExhaustedPolicy 控制有限重试耗尽后的策略。
	ExhaustedPolicy ProducerExhaustedPolicy
	// FailureHook 是失败事件回调函数。
	FailureHook ProducerFailureHook
}

// defaultProducerDispatchConfig 返回带有合理默认值的生产者分发配置。
func defaultProducerDispatchConfig() ProducerDispatchConfig {
	return ProducerDispatchConfig{
		Mode:        DefaultProducerDispatchMode,
		ShardCount:  DefaultShardCount,
		WorkerCount: DefaultProducerWorkerCount,
		QueueSize:   DefaultShardQueueSize,
	}
}

// defaultProducerConfig 返回带有合理默认值的生产者配置。
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

// normalizeProducerDispatchConfig 将零值字段替换为默认值。
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
// 规范化并验证生产者配置，依次执行：补默认值、规范化输入、校验必填字段、
// 校验分发配置、确保依赖、校验重试配置、校验耗尽策略、校验 topic 处理器。
func (cfg *ProducerConfig) Validate() error {
	if cfg == nil {
		return errors.New("producer config is nil")
	}

	// 补填默认值
	cfg.applyDefaults()
	// 规范化输入（去空白、去重等）
	cfg.normalizeInputs()

	// 校验必填字段
	if err := cfg.validateRequiredFields(); err != nil {
		return err
	}
	// 规范化并校验分发配置
	if err := cfg.normalizeAndValidateDispatch(); err != nil {
		return err
	}
	// 确保依赖项（logger、sarama config）就绪
	if err := cfg.ensureDependencies(); err != nil {
		return err
	}
	// 规范化并校验重试配置
	if err := cfg.normalizeAndValidateRetryConfig(); err != nil {
		return err
	}
	// 校验耗尽策略
	if err := cfg.validateExhaustedPolicy(); err != nil {
		return err
	}
	// 规范化并校验 topic 处理器
	return cfg.normalizeAndValidateTopicHandlers()
}

// applyDefaults 为 nil/零值字段填充默认值。
func (cfg *ProducerConfig) applyDefaults() {
	if cfg.TopicHandlers == nil {
		cfg.TopicHandlers = make(map[string]ProduceTopicHandlers)
	}
	applyDefaultBool(&cfg.LoggerHandlerEnabled, true)
}

// normalizeInputs 规范化输入字符串（去空白、去重等）。
func (cfg *ProducerConfig) normalizeInputs() {
	cfg.Brokers = normalizeStrings(cfg.Brokers)
	cfg.DefaultTopic = strings.TrimSpace(cfg.DefaultTopic)
}

// validateRequiredFields 校验必填字段是否已设置。
func (cfg *ProducerConfig) validateRequiredFields() error {
	if len(cfg.Brokers) == 0 && cfg.SyncProducer == nil {
		return errors.New("producer brokers are required")
	}
	return nil
}

// normalizeAndValidateDispatch 规范化分发配置并校验参数合法性。
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

// ensureDependencies 确保依赖项（logger、sarama config）已就绪。
func (cfg *ProducerConfig) ensureDependencies() error {
	return ensureProducerDependencies(&cfg.Logger, &cfg.SaramaConfig)
}

// normalizeAndValidateRetryConfig 规范化并校验重试配置。
func (cfg *ProducerConfig) normalizeAndValidateRetryConfig() error {
	return normalizeProduceRetryConfig(&cfg.RetryConfig)
}

// validateExhaustedPolicy 规范化并校验耗尽策略。
func (cfg *ProducerConfig) validateExhaustedPolicy() error {
	return normalizeProducerExhaustedPolicy(&cfg.ExhaustedPolicy)
}

// normalizeAndValidateTopicHandlers 规范化并校验 topic 处理器配置。
func (cfg *ProducerConfig) normalizeAndValidateTopicHandlers() error {
	for topic, handlers := range cfg.TopicHandlers {
		topicName := strings.TrimSpace(topic)
		if topicName == "" {
			return errors.New("producer topic handlers contain empty topic")
		}
		handlers.Mode = normalizeChainMode(handlers.Mode)
		if err := validateChainMode(
			topicName,
			handlers.Mode,
			"producer topic %q uses unsupported chain mode %q",
		); err != nil {
			return err
		}
		cfg.TopicHandlers[topic] = handlers
	}
	return nil
}
