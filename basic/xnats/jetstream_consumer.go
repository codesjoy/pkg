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
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/codesjoy/pkg/basic/xnats/internal/primitives/backoff"
	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	clogger "github.com/codesjoy/pkg/basic/xnats/middleware/consume/logger"
	cretry "github.com/codesjoy/pkg/basic/xnats/middleware/consume/retry"
)

// JetStreamConsumer wraps a bound JetStream consumer with middleware-aware handling.
type JetStreamConsumer struct {
	cfg JetStreamConsumerConfig

	conn    *nats.Conn
	js      jetstream.JetStream
	ownConn bool

	mu        sync.Mutex
	active    bool
	subs      []*nats.Subscription
	closed    bool
	closedCh  chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// NewJetStreamConsumer creates a configured JetStream consumer wrapper.
func NewJetStreamConsumer(cfg JetStreamConsumerConfig) (*JetStreamConsumer, error) {
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

	return &JetStreamConsumer{
		cfg:      cfg,
		conn:     conn,
		js:       js,
		ownConn:  ownConn,
		closedCh: make(chan struct{}),
	}, nil
}

// Consume starts consuming from the configured JetStream consumer until ctx is done or a fatal error occurs.
func (c *JetStreamConsumer) Consume(ctx context.Context, business consume.HandlerFunc) error {
	if c == nil {
		return errors.New("jetstream consumer is nil")
	}
	if business == nil {
		return consume.ErrNilHandlerFunc
	}
	ctx = normalizeContext(ctx)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrJetStreamConsumerClosed
	}
	if c.active {
		c.mu.Unlock()
		return ErrJetStreamConsumerActive
	}
	c.active = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.active = false
		c.subs = nil
		c.mu.Unlock()
	}()

	switch c.cfg.Mode {
	case JetStreamConsumerModePush:
		return c.consumePush(ctx, business)
	default:
		return c.consumePull(ctx, business)
	}
}

// Close drains active subscriptions and owned connection.
func (c *JetStreamConsumer) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		subs := append([]*nats.Subscription(nil), c.subs...)
		close(c.closedCh)
		c.mu.Unlock()

		var errs []error
		if err := drainSubscriptions(subs); err != nil {
			errs = append(errs, err)
		}
		if c.ownConn {
			if err := drainConnection(c.conn); err != nil {
				errs = append(errs, err)
			}
		}
		c.conn = nil
		c.js = nil
		c.closeErr = errors.Join(errs...)
	})
	return c.closeErr
}

func (c *JetStreamConsumer) consumePull(ctx context.Context, business consume.HandlerFunc) error {
	consumerRef, info, err := c.boundConsumer(ctx)
	if err != nil {
		return err
	}
	if info.Config.DeliverSubject != "" {
		return ErrPullConsumerRequiresNoDeliverSubject
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		jsMsg, err := consumerRef.Next(jetstream.FetchMaxWait(c.cfg.PullMaxWait))
		if err != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := backoff.Wait(ctx, c.cfg.IdleBackoff); err != nil {
				return err
			}
			continue
		}

		msgCtx := consumeContextFromJetStreamMessage(jsMsg)
		if err := c.buildConsumeChain(msgCtx.Subject, business)(ctx, msgCtx); err != nil {
			return err
		}
		if msgCtx.Acker != nil && !msgCtx.Acker.Handled() {
			if err := msgCtx.Acker.Ack(); err != nil {
				return err
			}
		}
	}
}

func (c *JetStreamConsumer) consumePush(ctx context.Context, business consume.HandlerFunc) error {
	if c.conn == nil {
		return ErrPushConsumerRequiresDeliverSubject
	}

	jsCtx, err := c.conn.JetStream()
	if err != nil {
		return err
	}
	info, err := jsCtx.ConsumerInfo(c.cfg.Stream, c.cfg.Consumer)
	if err != nil {
		return err
	}
	if info == nil || info.Config.DeliverSubject == "" {
		return ErrPushConsumerRequiresDeliverSubject
	}
	subject := info.Config.FilterSubject
	if subject == "" && len(info.Config.FilterSubjects) > 0 {
		subject = info.Config.FilterSubjects[0]
	}
	if subject == "" {
		streamInfo, err := jsCtx.StreamInfo(c.cfg.Stream)
		if err != nil {
			return err
		}
		if streamInfo == nil || streamInfo.Config.Subjects == nil ||
			len(streamInfo.Config.Subjects) != 1 {
			return ErrPushConsumerRequiresDeliverSubject
		}
		subject = streamInfo.Config.Subjects[0]
	}

	var sub *nats.Subscription
	bindOpt := nats.Bind(c.cfg.Stream, c.cfg.Consumer)
	if info.Config.DeliverGroup != "" {
		sub, err = jsCtx.QueueSubscribeSync(
			subject,
			info.Config.DeliverGroup,
			bindOpt,
		)
	} else {
		sub, err = jsCtx.SubscribeSync(subject, bindOpt)
	}
	if err != nil {
		return err
	}
	if err := c.conn.Flush(); err != nil {
		_ = sub.Drain()
		return err
	}
	c.mu.Lock()
	c.subs = []*nats.Subscription{sub}
	c.mu.Unlock()

	defer func() {
		_ = drainSubscriptions([]*nats.Subscription{sub})
	}()

	for {
		select {
		case <-c.closedCh:
			return ErrJetStreamConsumerClosed
		default:
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		msg, err := sub.NextMsg(c.cfg.PullMaxWait)
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				if err := backoff.Wait(ctx, c.cfg.IdleBackoff); err != nil {
					return err
				}
				continue
			}
			return err
		}

		msgCtx := consumeContextFromRawMessage(msg, consume.TransportJetStream, &rawAcker{msg: msg})
		if err := c.buildConsumeChain(msg.Subject, business)(ctx, msgCtx); err != nil {
			return err
		}
		if msgCtx.Acker != nil && !msgCtx.Acker.Handled() {
			if err := msgCtx.Acker.Ack(); err != nil {
				return err
			}
		}
	}
}

func (c *JetStreamConsumer) boundConsumer(
	ctx context.Context,
) (jetstream.Consumer, *jetstream.ConsumerInfo, error) {
	consumerRef, err := c.js.Consumer(ctx, c.cfg.Stream, c.cfg.Consumer)
	if err != nil {
		return nil, nil, err
	}
	info, err := consumerRef.Info(ctx)
	if err != nil {
		return nil, nil, err
	}
	return consumerRef, info, nil
}

func (c *JetStreamConsumer) buildConsumeChain(
	subject string,
	business consume.HandlerFunc,
) consume.HandlerFunc {
	return consume.Compose(c.handlersForSubject(subject), business)
}

func (c *JetStreamConsumer) handlersForSubject(subject string) []consume.Handler {
	handlers := make([]consume.Handler, 0, len(c.cfg.GlobalHandlers)+2)
	if boolValue(c.cfg.LoggerHandlerEnabled, true) {
		handlers = append(handlers, clogger.New(c.cfg.Logger))
	}
	handlers = append(handlers, cretry.New(
		c.cfg.RetryConfig,
		c.cfg.ExhaustedPolicy,
		c.cfg.FailureHook,
		c.cfg.Logger,
	))

	selected := c.cfg.GlobalHandlers
	if subjectCfg, ok := c.cfg.SubjectHandlers[subject]; ok {
		if subjectCfg.Mode == ChainModeReplace {
			selected = subjectCfg.Handlers
		} else {
			selected = append(append([]consume.Handler(nil), selected...), subjectCfg.Handlers...)
		}
	}
	handlers = append(handlers, selected...)
	return handlers
}

func consumeContextFromRawMessage(
	msg *nats.Msg,
	transport consume.Transport,
	acker consume.Acknowledger,
) *consume.MessageContext {
	ctx := &consume.MessageContext{
		Message:    msg,
		Transport:  transport,
		Attempt:    0,
		ReceivedAt: time.Now(),
		Acker:      acker,
	}
	if msg != nil {
		ctx.Subject = msg.Subject
		ctx.Reply = msg.Reply
		if meta, err := msg.Metadata(); err == nil {
			ctx.JetStream = &consume.JetStreamMetadata{
				Stream:           meta.Stream,
				Consumer:         meta.Consumer,
				Domain:           meta.Domain,
				NumDelivered:     meta.NumDelivered,
				NumPending:       meta.NumPending,
				StreamSequence:   meta.Sequence.Stream,
				ConsumerSequence: meta.Sequence.Consumer,
				Timestamp:        meta.Timestamp,
			}
		}
	}
	return ctx
}

func consumeContextFromJetStreamMessage(msg jetstream.Msg) *consume.MessageContext {
	if msg == nil {
		return &consume.MessageContext{
			Transport:  consume.TransportJetStream,
			ReceivedAt: time.Now(),
		}
	}

	raw := &nats.Msg{
		Subject: msg.Subject(),
		Reply:   msg.Reply(),
		Data:    append([]byte(nil), msg.Data()...),
	}
	if headers := msg.Headers(); headers != nil {
		raw.Header = cloneHeader(headers)
	}

	ctx := &consume.MessageContext{
		Message:    raw,
		Transport:  consume.TransportJetStream,
		Subject:    raw.Subject,
		Reply:      raw.Reply,
		ReceivedAt: time.Now(),
		Acker:      &jetStreamAcker{msg: msg},
	}
	if meta, err := msg.Metadata(); err == nil {
		ctx.JetStream = &consume.JetStreamMetadata{
			Stream:           meta.Stream,
			Consumer:         meta.Consumer,
			Domain:           meta.Domain,
			NumDelivered:     meta.NumDelivered,
			NumPending:       meta.NumPending,
			StreamSequence:   meta.Sequence.Stream,
			ConsumerSequence: meta.Sequence.Consumer,
			Timestamp:        meta.Timestamp,
		}
	}
	return ctx
}

type rawAcker struct {
	msg     *nats.Msg
	handled bool
}

func (a *rawAcker) Ack() error {
	if a.msg == nil {
		return nil
	}
	if err := a.msg.Ack(); err != nil {
		return err
	}
	a.handled = true
	return nil
}

func (a *rawAcker) Nak() error {
	if a.msg == nil {
		return nil
	}
	if err := a.msg.Nak(); err != nil {
		return err
	}
	a.handled = true
	return nil
}

func (a *rawAcker) Handled() bool {
	if a == nil {
		return false
	}
	return a.handled
}

type jetStreamAcker struct {
	msg     jetstream.Msg
	handled bool
}

func (a *jetStreamAcker) Ack() error {
	if a == nil || a.msg == nil {
		return nil
	}
	if err := a.msg.Ack(); err != nil {
		return err
	}
	a.handled = true
	return nil
}

func (a *jetStreamAcker) Nak() error {
	if a == nil || a.msg == nil {
		return nil
	}
	if err := a.msg.Nak(); err != nil {
		return err
	}
	a.handled = true
	return nil
}

func (a *jetStreamAcker) Handled() bool {
	if a == nil {
		return false
	}
	return a.handled
}
