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
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
	plogger "github.com/codesjoy/pkg/basic/xnats/middleware/publish/logger"
	pretry "github.com/codesjoy/pkg/basic/xnats/middleware/publish/retry"
)

// Publisher wraps a NATS connection with middleware-aware publish helpers.
type Publisher struct {
	cfg PublisherConfig

	conn    *nats.Conn
	ownConn bool

	closeOnce sync.Once
	closeErr  error
}

// NewPublisher creates a configured publisher wrapper.
func NewPublisher(cfg PublisherConfig) (*Publisher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	conn := cfg.Conn
	ownConn := false
	if conn == nil {
		var err error
		conn, err = connect(cfg.URLs, cfg.ConnectOptions)
		if err != nil {
			return nil, err
		}
		ownConn = true
	}

	return &Publisher{cfg: cfg, conn: conn, ownConn: ownConn}, nil
}

// Publish sends one message synchronously.
func (p *Publisher) Publish(ctx context.Context, msg *publish.Message) (*publish.Result, error) {
	if p == nil {
		return nil, errors.New("publisher is nil")
	}
	if p.conn == nil {
		return nil, ErrPublisherClosed
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
func (p *Publisher) PublishBatch(
	ctx context.Context,
	msgs ...*publish.Message,
) ([]*publish.Result, error) {
	if p == nil {
		return nil, errors.New("publisher is nil")
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
func (p *Publisher) PublishBatchReport(
	ctx context.Context,
	msgs ...*publish.Message,
) ([]publish.BatchItemResult, error) {
	if p == nil {
		return nil, errors.New("publisher is nil")
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
func (p *Publisher) Close() error {
	if p == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		if p.ownConn {
			p.closeErr = drainConnection(p.conn)
		}
		p.conn = nil
	})
	return p.closeErr
}

func (p *Publisher) prepareMessage(msg *publish.Message) (*publish.Message, error) {
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

func (p *Publisher) buildPublishChain(
	subject string,
	business publish.HandlerFunc,
) publish.HandlerFunc {
	return publish.Compose(p.handlersForSubject(subject), business)
}

func (p *Publisher) handlersForSubject(subject string) []publish.Handler {
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

func (p *Publisher) send(
	ctx context.Context,
	msgCtx *publish.MessageContext,
) (*publish.Result, error) {
	if msgCtx == nil || msgCtx.Message == nil {
		return nil, ErrNilPublishMessage
	}
	raw := toNATSMessage(msgCtx.Message)
	if err := p.conn.PublishMsg(raw); err != nil {
		return nil, err
	}
	if _, ok := ctx.Deadline(); ok {
		if err := p.conn.FlushWithContext(ctx); err != nil {
			return nil, err
		}
	} else {
		if err := p.conn.Flush(); err != nil {
			return nil, err
		}
	}
	return &publish.Result{
		Subject:   msgCtx.Message.Subject,
		Published: time.Now(),
	}, nil
}

func clonePublishMessage(msg *publish.Message) *publish.Message {
	if msg == nil {
		return nil
	}
	cloned := &publish.Message{
		Subject: msg.Subject,
		Reply:   msg.Reply,
	}
	if len(msg.Data) > 0 {
		cloned.Data = append([]byte(nil), msg.Data...)
	}
	if msg.Header != nil {
		cloned.Header = cloneHeader(msg.Header)
	}
	return cloned
}

func toNATSMessage(msg *publish.Message) *nats.Msg {
	raw := &nats.Msg{
		Subject: msg.Subject,
		Reply:   msg.Reply,
		Data:    append([]byte(nil), msg.Data...),
	}
	if msg.Header != nil {
		raw.Header = cloneHeader(msg.Header)
	}
	return raw
}

func cloneHeader(header nats.Header) nats.Header {
	if header == nil {
		return nil
	}
	cloned := make(nats.Header, len(header))
	for key, values := range header {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}
