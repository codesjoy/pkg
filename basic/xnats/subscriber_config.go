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

	"github.com/nats-io/nats.go"

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	cretry "github.com/codesjoy/pkg/basic/xnats/middleware/consume/retry"
)

// ConsumeSubjectHandlers describes subject-specific consume middleware composition.
type ConsumeSubjectHandlers struct {
	Mode     ChainMode
	Handlers []consume.Handler
}

// SubscriberConfig configures Subscriber.
type SubscriberConfig struct {
	URLs           []string
	Conn           *nats.Conn
	ConnectOptions []nats.Option
	Subjects       []string
	QueueGroup     string

	GlobalHandlers  []consume.Handler
	SubjectHandlers map[string]ConsumeSubjectHandlers

	Logger               *slog.Logger
	LoggerHandlerEnabled *bool

	RetryConfig     RetryConfig
	ExhaustedPolicy ConsumeExhaustedPolicy
	FailureHook     ConsumeFailureHook
}

func defaultSubscriberConfig() SubscriberConfig {
	return SubscriberConfig{
		SubjectHandlers:      make(map[string]ConsumeSubjectHandlers),
		Logger:               slog.Default(),
		LoggerHandlerEnabled: boolPtr(true),
		RetryConfig:          cretry.DefaultConfig(),
		ExhaustedPolicy:      ConsumeExhaustedPolicyBlock,
	}
}

// Validate normalizes and validates subscriber config.
func (cfg *SubscriberConfig) Validate() error {
	if cfg == nil {
		return errors.New("subscriber config is nil")
	}

	cfg.applyDefaults()
	cfg.normalizeInputs()
	if err := cfg.validateRequiredFields(); err != nil {
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
	return cfg.normalizeAndValidateSubjectHandlers()
}

func (cfg *SubscriberConfig) applyDefaults() {
	if cfg.SubjectHandlers == nil {
		cfg.SubjectHandlers = make(map[string]ConsumeSubjectHandlers)
	}
	ensureLoggerHandlerEnabled(&cfg.LoggerHandlerEnabled)
}

func (cfg *SubscriberConfig) normalizeInputs() {
	cfg.URLs = normalizeStrings(cfg.URLs)
	cfg.Subjects = normalizeStrings(cfg.Subjects)
	cfg.QueueGroup = strings.TrimSpace(cfg.QueueGroup)
}

func (cfg *SubscriberConfig) validateRequiredFields() error {
	if len(cfg.URLs) == 0 && cfg.Conn == nil {
		return errors.New("subscriber URLs are required")
	}
	if len(cfg.Subjects) == 0 {
		return errors.New("subscriber subjects are required")
	}
	return nil
}

func (cfg *SubscriberConfig) ensureDependencies() error {
	ensureLogger(&cfg.Logger)
	return nil
}

func (cfg *SubscriberConfig) normalizeAndValidateRetryConfig() error {
	if cfg.RetryConfig == (RetryConfig{}) {
		cfg.RetryConfig = cretry.DefaultConfig()
	}
	cfg.RetryConfig = cretry.NormalizeConfig(cfg.RetryConfig)
	return cretry.ValidateConfig(cfg.RetryConfig)
}

func (cfg *SubscriberConfig) validateExhaustedPolicy() error {
	switch cfg.ExhaustedPolicy {
	case "":
		cfg.ExhaustedPolicy = ConsumeExhaustedPolicyBlock
	case ConsumeExhaustedPolicyBlock, ConsumeExhaustedPolicyStop, ConsumeExhaustedPolicyDrop:
	default:
		return fmt.Errorf("unsupported consume exhausted policy %q", cfg.ExhaustedPolicy)
	}
	return nil
}

func (cfg *SubscriberConfig) normalizeAndValidateSubjectHandlers() error {
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
