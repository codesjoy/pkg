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

func applyDefaultInt(value *int, defaultValue int) {
	if *value == 0 {
		*value = defaultValue
	}
}

func applyDefaultBool(value **bool, defaultValue bool) {
	if *value == nil {
		enabled := defaultValue
		*value = &enabled
	}
}

func ensureConsumeDependencies(
	keyExtractor *KeyExtractor,
	logger **slog.Logger,
	saramaConfig **sarama.Config,
) error {
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

func ensureSaramaConfig(
	logger **slog.Logger,
	saramaConfig **sarama.Config,
	invalidMessage string,
	configure func(*sarama.Config),
) error {
	if *logger == nil {
		*logger = slog.Default()
	}
	if *saramaConfig == nil {
		*saramaConfig = sarama.NewConfig()
	}

	configure(*saramaConfig)
	if (*saramaConfig).Version == sarama.MinVersion {
		(*saramaConfig).Version = sarama.V2_8_0_0
	}
	if err := (*saramaConfig).Validate(); err != nil {
		return fmt.Errorf("%s: %w", invalidMessage, err)
	}
	return nil
}

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

func normalizeConsumeRetryConfig(cfg *RetryConfig) error {
	if *cfg == (RetryConfig{}) {
		*cfg = cretry.DefaultConfig()
	}
	*cfg = cretry.NormalizeConfig(*cfg)
	return cretry.ValidateConfig(*cfg)
}

func normalizeProduceRetryConfig(cfg *RetryConfig) error {
	if *cfg == (RetryConfig{}) {
		*cfg = ppretry.DefaultConfig()
	}
	*cfg = ppretry.NormalizeConfig(*cfg)
	return ppretry.ValidateConfig(*cfg)
}

func normalizeChainMode(mode ChainMode) ChainMode {
	if mode == "" {
		return ChainModeAppend
	}
	return mode
}

func validateChainMode(topic string, mode ChainMode, messageFormat string) error {
	switch mode {
	case ChainModeAppend, ChainModeReplace:
		return nil
	default:
		return fmt.Errorf(messageFormat, topic, mode)
	}
}
