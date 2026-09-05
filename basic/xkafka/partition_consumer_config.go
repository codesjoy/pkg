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
// 分区消费者的完整配置。
type PartitionConsumerConfig struct {
	// Brokers 是 Kafka 集群地址列表。
	Brokers []string
	// Topic 是消费的目标 topic。
	Topic string
	// Partition 是消费的目标分区号。
	Partition int32

	// SaramaConfig 是底层 Sarama 配置，nil 时使用默认值。
	SaramaConfig *sarama.Config

	// ShardCount 是分片数量，用于按键有序处理。
	ShardCount int
	// ShardQueueSize 是每个分片队列的缓冲区大小。
	ShardQueueSize int

	// GlobalHandlers 是所有消息共享的中间件处理器。
	GlobalHandlers []consume.Handler

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
	// DLQ 是死信队列配置。
	DLQ *DLQConfig
	// FailureHook 是失败事件回调函数。
	FailureHook FailureHook

	// OffsetStore 是分区 offset 持久化存储。
	OffsetStore OffsetStore
	// InitialOffset 是首次消费时的起始 offset。
	InitialOffset int64
	// Reconnect 控制重连退避策略。
	Reconnect BackoffConfig
}

// defaultPartitionConsumerConfig 返回带有合理默认值的分区消费者配置。
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

// defaultPartitionReconnectBackoff 返回默认的重连退避配置。
func defaultPartitionReconnectBackoff() BackoffConfig {
	return BackoffConfig{
		InitialBackoff: DefaultPartitionReconnectInitialBackoff,
		MaxBackoff:     DefaultPartitionReconnectMaxBackoff,
		Multiplier:     DefaultPartitionReconnectMultiplier,
	}
}

// normalizeBackoff 将退避配置的零值字段替换为默认值并校正不合理参数。
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

// validateBackoff 校验退避配置参数的合法性。
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
// 规范化并验证分区消费者配置，依次执行：补默认值、规范化输入、校验必填字段、
// 确保依赖、校验耗尽策略、校验重试配置、校验 DLQ、校验重连配置。
func (cfg *PartitionConsumerConfig) Validate() error {
	if cfg == nil {
		return errors.New("partition consumer config is nil")
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
	// 规范化并校验重连退避配置
	return cfg.normalizeAndValidateReconnect()
}

// applyDefaults 为 nil/零值字段填充默认值。
func (cfg *PartitionConsumerConfig) applyDefaults() {
	applyDefaultInt(&cfg.ShardCount, DefaultShardCount)
	applyDefaultInt(&cfg.ShardQueueSize, DefaultShardQueueSize)
	if cfg.InitialOffset == 0 {
		cfg.InitialOffset = sarama.OffsetOldest
	}
	applyDefaultBool(&cfg.LoggerHandlerEnabled, true)
}

// normalizeInputs 规范化输入字符串。
func (cfg *PartitionConsumerConfig) normalizeInputs() {
	cfg.Brokers = normalizeStrings(cfg.Brokers)
	cfg.Topic = strings.TrimSpace(cfg.Topic)
}

// validateRequiredFields 校验必填字段是否已设置。
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

// ensureDependencies 确保依赖项（keyExtractor、logger、sarama config、offset store）已就绪。
func (cfg *PartitionConsumerConfig) ensureDependencies() error {
	if err := ensureConsumeDependencies(
		&cfg.KeyExtractor,
		&cfg.Logger,
		&cfg.SaramaConfig,
	); err != nil {
		return err
	}
	if cfg.OffsetStore == nil {
		cfg.OffsetStore = NewMemoryOffsetStore()
	}
	return nil
}

// validateExhaustedPolicy 规范化并校验耗尽策略。
func (cfg *PartitionConsumerConfig) validateExhaustedPolicy() error {
	return normalizeConsumeExhaustedPolicy(&cfg.ExhaustedPolicy)
}

// normalizeAndValidateRetryConfig 规范化并校验重试配置。
func (cfg *PartitionConsumerConfig) normalizeAndValidateRetryConfig() error {
	return normalizeConsumeRetryConfig(&cfg.RetryConfig)
}

// validateDLQ 校验 DLQ 配置，仅在耗尽策略为 DLQCommit 时要求配置。
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

// normalizeAndValidateReconnect 规范化并校验重连退避配置。
func (cfg *PartitionConsumerConfig) normalizeAndValidateReconnect() error {
	cfg.Reconnect = normalizeBackoff(cfg.Reconnect)
	if err := validateBackoff(cfg.Reconnect); err != nil {
		return err
	}
	return nil
}
