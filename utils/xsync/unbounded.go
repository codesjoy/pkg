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
	"errors"
	"sync"
)

// ErrQueueClosed indicates the queue cannot accept new items.
var ErrQueueClosed = errors.New("xsync: queue closed")

// Unbounded is a generic, goroutine-safe FIFO queue with unbounded capacity.
//
// It is suitable for producer/consumer style coordination where producers
// should not block on queue capacity.
type Unbounded[T any] struct {
	mu       sync.Mutex
	notEmpty *sync.Cond
	queue    []T
	closed   bool
}

// NewUnbounded returns a new, ready-to-use Unbounded queue.
func NewUnbounded[T any]() *Unbounded[T] {
	q := &Unbounded[T]{}
	q.notEmpty = sync.NewCond(&q.mu)
	return q
}

// Put appends v to the queue.
// It returns ErrQueueClosed when the queue has been closed.
func (q *Unbounded[T]) Put(v T) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrQueueClosed
	}

	q.queue = append(q.queue, v)
	q.notEmpty.Signal()
	return nil
}

// Get pops and returns the oldest queued value.
// It blocks while the queue is empty and open.
// It returns ok=false only when the queue is closed and drained.
func (q *Unbounded[T]) Get() (v T, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.queue) == 0 && !q.closed {
		q.notEmpty.Wait()
	}

	if len(q.queue) == 0 {
		var zero T
		return zero, false
	}

	return q.popFront(), true
}

// TryGet pops and returns the oldest queued value without blocking.
func (q *Unbounded[T]) TryGet() (v T, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) == 0 {
		var zero T
		return zero, false
	}
	return q.popFront(), true
}

// Close marks the queue as closed.
// Close is idempotent.
func (q *Unbounded[T]) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}
	q.closed = true
	q.notEmpty.Broadcast()
}

// Len returns the current number of queued items.
func (q *Unbounded[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}

func (q *Unbounded[T]) popFront() T {
	v := q.queue[0]
	var zero T
	q.queue[0] = zero
	q.queue = q.queue[1:]
	if len(q.queue) == 0 {
		q.queue = nil
	}
	return v
}
