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

package xsync

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"sync"
)

// ErrSerializerClosed indicates a serializer is closed or unavailable.
var ErrSerializerClosed = errors.New("xsync: serializer closed")

// Serializer executes submitted callbacks sequentially in FIFO order.
//
// Serializer is safe for concurrent use.
type Serializer struct {
	ctx   context.Context
	queue *Unbounded[func(context.Context)]
	done  chan struct{}

	closeOnce sync.Once
}

// NewSerializer creates a new Serializer.
// A nil context is treated as context.Background().
func NewSerializer(ctx context.Context) *Serializer {
	if ctx == nil {
		ctx = context.Background()
	}

	s := &Serializer{
		ctx:   ctx,
		queue: NewUnbounded[func(context.Context)](),
		done:  make(chan struct{}),
	}

	context.AfterFunc(ctx, s.Close)
	go s.run()
	return s
}

// Submit enqueues fn to be executed serially.
//
// It returns ErrSerializerClosed when the serializer is closed or the
// serializer context has already been canceled.
func (s *Serializer) Submit(fn func(context.Context)) error {
	if s.ctx.Err() != nil {
		return ErrSerializerClosed
	}
	if err := s.queue.Put(fn); err != nil {
		return ErrSerializerClosed
	}
	return nil
}

// Close stops accepting new callbacks and begins queue draining.
// Close is idempotent.
func (s *Serializer) Close() {
	s.closeOnce.Do(func() {
		s.queue.Close()
	})
}

// Done returns a channel closed when the serializer is fully shut down.
func (s *Serializer) Done() <-chan struct{} {
	return s.done
}

func (s *Serializer) run() {
	defer close(s.done)

	for {
		cb, ok := s.queue.Get()
		if !ok {
			return
		}
		s.invoke(cb)
	}
}

func (s *Serializer) invoke(cb func(context.Context)) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error(
				"xsync: serializer callback panic",
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
		}
	}()

	cb(s.ctx)
}
