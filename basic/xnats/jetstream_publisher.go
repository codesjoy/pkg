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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
	plogger "github.com/codesjoy/pkg/basic/xnats/middleware/publish/logger"
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

	if cfg.SubjectHandlers == nil {
		cfg.SubjectHandlers = make(map[string]PublishSubjectHandlers)
	}
	ensureLoggerHandlerEnabled(&cfg.LoggerHandlerEnabled)
	cfg.URLs = normalizeStrings(cfg.URLs)
	cfg.DefaultSubject = strings.TrimSpace(cfg.DefaultSubject)
	ensureLogger(&cfg.Logger)
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
		handlers.Mode = normalizeChainMode(handlers.Mode)
		cfg.SubjectHandlers[subject] = handlers
		if err := validateChainMode("publish", name, handlers.Mode); err != nil {
			return err
		}
	}
	return nil
}

// JetStreamPublisher wraps JetStream publish with middleware-aware retry helpers.
type JetStreamPublisher struct {
	cfg JetStreamPublisherConfig

	conn    *nats.Conn
	js      jetstream.JetStream
	ownConn bool

	closeOnce sync.Once
	closeErr  error
}

// NewJetStreamPublisher creates a configured JetStream publisher wrapper.
func NewJetStreamPublisher(cfg JetStreamPublisherConfig) (*JetStreamPublisher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	conn := cfg.Conn
	ownConn := false
	if conn == nil && len(cfg.URLs) > 0 {
		var err error
		conn, err = connect(cfg.URLs, cfg.ConnectOptions)
		if err != nil {
			return nil, err
		}
		ownConn = true
	}

	js := cfg.JetStream
	if js == nil {
		if conn == nil {
			return nil, ErrJetStreamRequired
		}
		var err error
		js, err = newJetStream(conn)
		if err != nil {
			if ownConn {
				_ = drainConnection(conn)
			}
			return nil, err
		}
	}

	return &JetStreamPublisher{cfg: cfg, conn: conn, js: js, ownConn: ownConn}, nil
}

// Publish sends one message synchronously to JetStream.
func (p *JetStreamPublisher) Publish(
	ctx context.Context,
	msg *publish.Message,
) (*publish.Result, error) {
	if p == nil {
		return nil, errors.New("jetstream publisher is nil")
	}
	if p.js == nil {
		return nil, ErrJetStreamPublisherClosed
	}
	ctx = normalizeContext(ctx)

	prepared, err := p.prepareMessage(msg)
	if err != nil {
		return nil, err
	}
	msgCtx := &publish.MessageContext{Message: prepared, ReceivedAt: time.Now()}
	return p.buildPublishChain(prepared.Subject, p.send)(ctx, msgCtx)
}

// PublishBatch sends messages sequentially and fails fast on the first error.
// It is kept for compatibility and is not suitable for per-item acknowledgement
// flows such as xevent outbox relays.
func (p *JetStreamPublisher) PublishBatch(
	ctx context.Context,
	msgs ...*publish.Message,
) ([]*publish.Result, error) {
	if p == nil {
		return nil, errors.New("jetstream publisher is nil")
	}
	ctx = normalizeContext(ctx)
	if len(msgs) == 0 {
		return nil, nil
	}
	results := make([]*publish.Result, len(msgs))
	for i, msg := range msgs {
		result, err := p.Publish(ctx, msg)
		if err != nil {
			return results, fmt.Errorf("publish batch index %d: %w", i, err)
		}
		results[i] = result
	}
	return results, nil
}

// PublishBatchReport sends messages and returns a per-item outcome vector.
// A top-level error is returned only for call-level failures such as a nil
// publisher or a context that is already canceled before the call starts.
func (p *JetStreamPublisher) PublishBatchReport(
	ctx context.Context,
	msgs ...*publish.Message,
) ([]publish.BatchItemResult, error) {
	if p == nil {
		return nil, errors.New("jetstream publisher is nil")
	}
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}

	results := make([]publish.BatchItemResult, len(msgs))
	for i, msg := range msgs {
		if err := ctx.Err(); err != nil {
			for j := i; j < len(msgs); j++ {
				results[j].Err = err
			}
			return results, nil
		}

		result, err := p.Publish(ctx, msg)
		results[i] = publish.BatchItemResult{
			Result: result,
			Err:    err,
		}
	}
	return results, nil
}

// Close drains and closes owned resources.
func (p *JetStreamPublisher) Close() error {
	if p == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		if p.ownConn {
			p.closeErr = drainConnection(p.conn)
		}
		p.conn = nil
		p.js = nil
	})
	return p.closeErr
}

func (p *JetStreamPublisher) prepareMessage(msg *publish.Message) (*publish.Message, error) {
	if msg == nil {
		return nil, ErrNilPublishMessage
	}
	prepared := clonePublishMessage(msg)
	if prepared.Subject == "" {
		prepared.Subject = p.cfg.DefaultSubject
	}
	if prepared.Subject == "" {
		return nil, ErrPublishSubjectRequired
	}
	return prepared, nil
}

func (p *JetStreamPublisher) buildPublishChain(
	subject string,
	business publish.HandlerFunc,
) publish.HandlerFunc {
	return publish.Compose(p.handlersForSubject(subject), business)
}

func (p *JetStreamPublisher) handlersForSubject(subject string) []publish.Handler {
	handlers := make([]publish.Handler, 0, len(p.cfg.GlobalHandlers)+2)
	if boolValue(p.cfg.LoggerHandlerEnabled, true) {
		handlers = append(handlers, plogger.New(p.cfg.Logger))
	}
	handlers = append(handlers, pretry.New(
		p.cfg.RetryConfig,
		p.cfg.ExhaustedPolicy,
		p.cfg.FailureHook,
		p.cfg.Logger,
	))

	selected := p.cfg.GlobalHandlers
	if subjectCfg, ok := p.cfg.SubjectHandlers[subject]; ok {
		if subjectCfg.Mode == ChainModeReplace {
			selected = subjectCfg.Handlers
		} else {
			selected = append(append([]publish.Handler(nil), selected...), subjectCfg.Handlers...)
		}
	}
	handlers = append(handlers, selected...)
	return handlers
}

func (p *JetStreamPublisher) send(
	ctx context.Context,
	msgCtx *publish.MessageContext,
) (*publish.Result, error) {
	if msgCtx == nil || msgCtx.Message == nil {
		return nil, ErrNilPublishMessage
	}
	ack, err := p.js.PublishMsg(ctx, toNATSMessage(msgCtx.Message))
	if err != nil {
		return nil, err
	}
	return &publish.Result{
		Subject:   msgCtx.Message.Subject,
		Published: time.Now(),
		Stream:    ack.Stream,
		Sequence:  ack.Sequence,
		Duplicate: ack.Duplicate,
	}, nil
}
