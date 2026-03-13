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
	"github.com/nats-io/nats.go/jetstream"

	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
	pretry "github.com/codesjoy/pkg/basic/xnats/middleware/publish/retry"
)

// JetStreamPublisherConfig configures JetStreamPublisher.
type JetStreamPublisherConfig struct {
	URLs           []string
	Conn           *nats.Conn
	JetStream      jetstream.JetStream
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

// Validate normalizes and validates JetStream publisher config.
func (cfg *JetStreamPublisherConfig) Validate() error {
	if cfg == nil {
		return errors.New("jetstream publisher config is nil")
	}

	defaults := defaultPublisherConfig()
	if cfg.SubjectHandlers == nil {
		cfg.SubjectHandlers = make(map[string]PublishSubjectHandlers)
	}
	if cfg.LoggerHandlerEnabled == nil {
		cfg.LoggerHandlerEnabled = defaults.LoggerHandlerEnabled
	}
	cfg.URLs = normalizeStrings(cfg.URLs)
	cfg.DefaultSubject = strings.TrimSpace(cfg.DefaultSubject)
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if len(cfg.URLs) == 0 && cfg.Conn == nil && cfg.JetStream == nil {
		return errors.New("jetstream publisher URLs or injected JetStream are required")
	}
	if cfg.RetryConfig == (RetryConfig{}) {
		cfg.RetryConfig = pretry.DefaultConfig()
	}
	cfg.RetryConfig = pretry.NormalizeConfig(cfg.RetryConfig)
	if err := pretry.ValidateConfig(cfg.RetryConfig); err != nil {
		return err
	}
	switch cfg.ExhaustedPolicy {
	case "":
		cfg.ExhaustedPolicy = PublishExhaustedPolicyBlock
	case PublishExhaustedPolicyBlock, PublishExhaustedPolicyStop, PublishExhaustedPolicyDrop:
	default:
		return fmt.Errorf("unsupported publish exhausted policy %q", cfg.ExhaustedPolicy)
	}
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
