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
	"fmt"
	"log/slog"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/router"
	cretry "github.com/codesjoy/pkg/basic/xkafka/middleware/consume/retry"
	ppretry "github.com/codesjoy/pkg/basic/xkafka/middleware/produce/retry"
)

// applyDefaultInt 为零值 int 指针设置默认值。
func applyDefaultInt(value *int, defaultValue int) {
	if *value == 0 {
		*value = defaultValue
	}
}

// applyDefaultBool 为 nil 的 *bool 指针设置默认值。
func applyDefaultBool(value **bool, defaultValue bool) {
	if *value == nil {
		enabled := defaultValue
		*value = &enabled
	}
}

// ensureConsumeDependencies 确保消费者所需的依赖项已就绪：
// keyExtractor、logger 和 sarama config。
func ensureConsumeDependencies(
	keyExtractor *KeyExtractor,
	logger **slog.Logger,
	saramaConfig **sarama.Config,
) error {
	// keyExtractor 为空时使用默认实现
	if *keyExtractor == nil {
		*keyExtractor = KeyExtractor(router.DefaultConsumeKeyExtractor)
	}
	return ensureSaramaConfig(
		logger,
		saramaConfig,
		"invalid sarama config",
		func(cfg *sarama.Config) {
			cfg.Consumer.Return.Errors = true
		},
	)
}

// ensureProducerDependencies 确保生产者所需的依赖项已就绪：
// logger 和 sarama config。
func ensureProducerDependencies(
	logger **slog.Logger,
	saramaConfig **sarama.Config,
) error {
	return ensureSaramaConfig(
		logger,
		saramaConfig,
		"invalid producer sarama config",
		func(cfg *sarama.Config) {
			cfg.Producer.Return.Successes = true
		},
	)
}

// ensureSaramaConfig 确保 Sarama 配置就绪：
// logger 为空时设置默认 logger，config 为空时创建默认配置，
// 设置默认 Kafka 版本，并执行配置校验。
func ensureSaramaConfig(
	logger **slog.Logger,
	saramaConfig **sarama.Config,
	invalidMessage string,
	configure func(*sarama.Config),
) error {
	// 默认 logger
	if *logger == nil {
		*logger = slog.Default()
	}
	// 默认 sarama config
	if *saramaConfig == nil {
		*saramaConfig = sarama.NewConfig()
	}

	// 应用特定配置（如 Return.Successes / Return.Errors）
	configure(*saramaConfig)
	// 设置默认 Kafka 版本
	if (*saramaConfig).Version == sarama.MinVersion {
		(*saramaConfig).Version = sarama.V2_8_0_0
	}
	// 校验 Sarama 配置
	if err := (*saramaConfig).Validate(); err != nil {
		return fmt.Errorf("%s: %w", invalidMessage, err)
	}
	return nil
}

// normalizeConsumeExhaustedPolicy 规范化消费者耗尽策略，空值默认为 Block。
func normalizeConsumeExhaustedPolicy(policy *ExhaustedPolicy) error {
	switch *policy {
	case "":
		*policy = ExhaustedPolicyBlock
	case ExhaustedPolicyBlock, ExhaustedPolicyDLQCommit, ExhaustedPolicyStop:
	default:
		return fmt.Errorf("unsupported exhausted policy %q", *policy)
	}
	return nil
}

// normalizeProducerExhaustedPolicy 规范化生产者耗尽策略，空值默认为 Block。
func normalizeProducerExhaustedPolicy(policy *ProducerExhaustedPolicy) error {
	switch *policy {
	case "":
		*policy = ProducerExhaustedPolicyBlock
	case ProducerExhaustedPolicyBlock,
		ProducerExhaustedPolicyStop,
		ProducerExhaustedPolicyDrop:
	default:
		return fmt.Errorf("unsupported producer exhausted policy %q", *policy)
	}
	return nil
}

// normalizeConsumeRetryConfig 规范化并校验消费者重试配置。
func normalizeConsumeRetryConfig(cfg *RetryConfig) error {
	if *cfg == (RetryConfig{}) {
		*cfg = cretry.DefaultConfig()
	}
	*cfg = cretry.NormalizeConfig(*cfg)
	return cretry.ValidateConfig(*cfg)
}

// normalizeProduceRetryConfig 规范化并校验生产者重试配置。
func normalizeProduceRetryConfig(cfg *RetryConfig) error {
	if *cfg == (RetryConfig{}) {
		*cfg = ppretry.DefaultConfig()
	}
	*cfg = ppretry.NormalizeConfig(*cfg)
	return ppretry.ValidateConfig(*cfg)
}

// normalizeChainMode 规范化链模式，空值默认为 Append。
func normalizeChainMode(mode ChainMode) ChainMode {
	if mode == "" {
		return ChainModeAppend
	}
	return mode
}

// validateChainMode 校验链模式是否为合法值。
func validateChainMode(topic string, mode ChainMode, messageFormat string) error {
	switch mode {
	case ChainModeAppend, ChainModeReplace:
		return nil
	default:
		return fmt.Errorf(messageFormat, topic, mode)
	}
}
