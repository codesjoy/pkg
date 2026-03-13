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

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
	clogger "github.com/codesjoy/pkg/basic/xnats/middleware/consume/logger"
	cretry "github.com/codesjoy/pkg/basic/xnats/middleware/consume/retry"
)

// Subscriber wraps NATS subscriptions with middleware-aware business handling.
type Subscriber struct {
	cfg SubscriberConfig

	conn    *nats.Conn
	ownConn bool

	mu        sync.Mutex
	active    bool
	subs      []*nats.Subscription
	closed    bool
	closedCh  chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// NewSubscriber creates a configured subscriber wrapper.
func NewSubscriber(cfg SubscriberConfig) (*Subscriber, error) {
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

	return &Subscriber{
		cfg:      cfg,
		conn:     conn,
		ownConn:  ownConn,
		closedCh: make(chan struct{}),
	}, nil
}

// Consume starts consuming subscribed subjects until ctx is done or a fatal error occurs.
func (s *Subscriber) Consume(ctx context.Context, business consume.HandlerFunc) error {
	if s == nil {
		return errors.New("subscriber is nil")
	}
	if business == nil {
		return consume.ErrNilHandlerFunc
	}
	ctx = normalizeContext(ctx)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrSubscriberClosed
	}
	if s.active {
		s.mu.Unlock()
		return ErrSubscriberActive
	}
	s.active = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.active = false
		s.subs = nil
		s.mu.Unlock()
	}()

	errCh := make(chan error, 1)
	subs := make([]*nats.Subscription, 0, len(s.cfg.Subjects))
	for _, subject := range s.cfg.Subjects {
		subjectName := subject
		handler := func(msg *nats.Msg) {
			msgCtx := &consume.MessageContext{
				Message:    msg,
				Transport:  consume.TransportCore,
				Subject:    msg.Subject,
				Reply:      msg.Reply,
				ReceivedAt: time.Now(),
			}
			if err := s.buildConsumeChain(msg.Subject, business)(ctx, msgCtx); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}

		var (
			sub *nats.Subscription
			err error
		)
		if s.cfg.QueueGroup == "" {
			sub, err = s.conn.Subscribe(subjectName, handler)
		} else {
			sub, err = s.conn.QueueSubscribe(subjectName, s.cfg.QueueGroup, handler)
		}
		if err != nil {
			_ = drainSubscriptions(subs)
			return err
		}
		subs = append(subs, sub)
	}
	s.mu.Lock()
	s.subs = subs
	s.mu.Unlock()

	defer func() {
		_ = drainSubscriptions(subs)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closedCh:
		return ErrSubscriberClosed
	}
}

// Close drains active subscriptions and closes owned resources.
func (s *Subscriber) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		subs := append([]*nats.Subscription(nil), s.subs...)
		close(s.closedCh)
		s.mu.Unlock()

		var errs []error
		if err := drainSubscriptions(subs); err != nil {
			errs = append(errs, err)
		}
		if s.ownConn {
			if err := drainConnection(s.conn); err != nil {
				errs = append(errs, err)
			}
		}
		s.conn = nil
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}

func (s *Subscriber) buildConsumeChain(
	subject string,
	business consume.HandlerFunc,
) consume.HandlerFunc {
	return consume.Compose(s.handlersForSubject(subject), business)
}

func (s *Subscriber) handlersForSubject(subject string) []consume.Handler {
	handlers := make([]consume.Handler, 0, len(s.cfg.GlobalHandlers)+2)
	if boolValue(s.cfg.LoggerHandlerEnabled, true) {
		handlers = append(handlers, clogger.New(s.cfg.Logger))
	}
	handlers = append(handlers, cretry.New(
		s.cfg.RetryConfig,
		s.cfg.ExhaustedPolicy,
		s.cfg.FailureHook,
		s.cfg.Logger,
	))

	selected := s.cfg.GlobalHandlers
	if subjectCfg, ok := s.cfg.SubjectHandlers[subject]; ok {
		if subjectCfg.Mode == ChainModeReplace {
			selected = subjectCfg.Handlers
		} else {
			selected = append(append([]consume.Handler(nil), selected...), subjectCfg.Handlers...)
		}
	}
	handlers = append(handlers, selected...)
	return handlers
}
