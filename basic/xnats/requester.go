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

	"github.com/nats-io/nats.go"

	"github.com/codesjoy/pkg/basic/xnats/middleware/publish"
)

// Requester wraps NATS request/reply helpers.
type Requester struct {
	cfg RequesterConfig

	conn    *nats.Conn
	ownConn bool

	closeOnce sync.Once
	closeErr  error
}

// NewRequester creates a configured requester wrapper.
func NewRequester(cfg RequesterConfig) (*Requester, error) {
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

	return &Requester{cfg: cfg, conn: conn, ownConn: ownConn}, nil
}

// Request sends a logical request message and waits for one reply.
func (r *Requester) Request(ctx context.Context, msg *publish.Message) (*nats.Msg, error) {
	if r == nil {
		return nil, errors.New("requester is nil")
	}
	if r.conn == nil {
		return nil, ErrRequesterClosed
	}
	if msg == nil {
		return nil, ErrNilPublishMessage
	}
	if msg.Subject == "" {
		return nil, ErrPublishSubjectRequired
	}
	return r.RequestMsg(ctx, toNATSMessage(msg))
}

// RequestMsg sends a raw NATS message and waits for one reply.
func (r *Requester) RequestMsg(ctx context.Context, msg *nats.Msg) (*nats.Msg, error) {
	if r == nil {
		return nil, errors.New("requester is nil")
	}
	if r.conn == nil {
		return nil, ErrRequesterClosed
	}
	ctx = normalizeContext(ctx)
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.cfg.Timeout)
		defer cancel()
	}
	return r.conn.RequestMsgWithContext(ctx, msg)
}

// Close drains and closes owned resources.
func (r *Requester) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		if r.ownConn {
			r.closeErr = drainConnection(r.conn)
		}
		r.conn = nil
	})
	return r.closeErr
}
