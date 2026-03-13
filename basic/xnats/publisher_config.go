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

	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
	pretry "github.com/codesjoy/pkg/basic/xnats/middleware/publish/retry"
)

// PublishSubjectHandlers describes subject-specific publish middleware composition.
type PublishSubjectHandlers struct {
	Mode     ChainMode
	Handlers []publish.Handler
}

// PublisherConfig configures Publisher.
type PublisherConfig struct {
	URLs           []string
	Conn           *nats.Conn
	ConnectOptions []nats.Option
	DefaultSubject string

	GlobalHandlers  []publish.Handler
	SubjectHandlers map[string]PublishSubjectHandlers

	Logger               *slog.Logger
	LoggerHandlerEnabled *bool

	RetryConfig     RetryConfig
	ExhaustedPolicy PublishExhaustedPolicy
	FailureHook     PublishFailureHook
}

func defaultPublisherConfig() PublisherConfig {
	enabled := true
	return PublisherConfig{
		SubjectHandlers:      make(map[string]PublishSubjectHandlers),
		Logger:               slog.Default(),
		LoggerHandlerEnabled: &enabled,
		RetryConfig:          pretry.DefaultConfig(),
		ExhaustedPolicy:      PublishExhaustedPolicyBlock,
	}
}

// Validate normalizes and validates publisher config.
func (cfg *PublisherConfig) Validate() error {
	if cfg == nil {
		return errors.New("publisher config is nil")
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

func (cfg *PublisherConfig) applyDefaults() {
	if cfg.SubjectHandlers == nil {
		cfg.SubjectHandlers = make(map[string]PublishSubjectHandlers)
	}
	if cfg.LoggerHandlerEnabled == nil {
		enabled := true
		cfg.LoggerHandlerEnabled = &enabled
	}
}

func (cfg *PublisherConfig) normalizeInputs() {
	cfg.URLs = normalizeStrings(cfg.URLs)
	cfg.DefaultSubject = strings.TrimSpace(cfg.DefaultSubject)
}

func (cfg *PublisherConfig) validateRequiredFields() error {
	if len(cfg.URLs) == 0 && cfg.Conn == nil {
		return errors.New("publisher URLs are required")
	}
	return nil
}

func (cfg *PublisherConfig) ensureDependencies() error {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return nil
}

func (cfg *PublisherConfig) normalizeAndValidateRetryConfig() error {
	if cfg.RetryConfig == (RetryConfig{}) {
		cfg.RetryConfig = pretry.DefaultConfig()
	}
	cfg.RetryConfig = pretry.NormalizeConfig(cfg.RetryConfig)
	return pretry.ValidateConfig(cfg.RetryConfig)
}

func (cfg *PublisherConfig) validateExhaustedPolicy() error {
	switch cfg.ExhaustedPolicy {
	case "":
		cfg.ExhaustedPolicy = PublishExhaustedPolicyBlock
	case PublishExhaustedPolicyBlock, PublishExhaustedPolicyStop, PublishExhaustedPolicyDrop:
	default:
		return fmt.Errorf("unsupported publish exhausted policy %q", cfg.ExhaustedPolicy)
	}
	return nil
}

func (cfg *PublisherConfig) normalizeAndValidateSubjectHandlers() error {
	for subject, handlers := range cfg.SubjectHandlers {
		name := strings.TrimSpace(subject)
		if name == "" {
			return errors.New("publish subject handlers contain empty subject")
		}
		if handlers.Mode == "" {
			handlers.Mode = ChainModeAppend
			cfg.SubjectHandlers[subject] = handlers
		}
		switch handlers.Mode {
		case ChainModeAppend, ChainModeReplace:
		default:
			return fmt.Errorf(
				"publish subject %q uses unsupported chain mode %q",
				name,
				handlers.Mode,
			)
		}
	}
	return nil
}
