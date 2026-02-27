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
	"sync"
)

// Event represents a one-time signal which may be fired in the future.
type Event struct {
	once sync.Once
	done chan struct{}
}

// NewEvent returns a new, ready-to-use Event.
func NewEvent() *Event {
	return &Event{done: make(chan struct{})}
}

// Fire signals the event.
// It returns true only for the first successful signal.
func (e *Event) Fire() bool {
	fired := false
	e.once.Do(func() {
		close(e.done)
		fired = true
	})
	return fired
}

// Done returns a channel that is closed when Fire is called.
func (e *Event) Done() <-chan struct{} {
	return e.done
}

// HasFired reports whether Fire has been called.
func (e *Event) HasFired() bool {
	select {
	case <-e.done:
		return true
	default:
		return false
	}
}

// Wait blocks until the event fires or the context is canceled.
// A nil context is treated as context.Background().
func (e *Event) Wait(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-e.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
