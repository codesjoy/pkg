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

package producer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/router"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

// DispatchMode controls async routing behavior.
type DispatchMode string

const (
	// DispatchModeSerial routes all messages to one worker.
	DispatchModeSerial DispatchMode = "serial"
	// DispatchModeKeySharded routes by key hash modulo shard count.
	DispatchModeKeySharded DispatchMode = "key_sharded"
	// DispatchModeParallel routes by round-robin across workers.
	DispatchModeParallel DispatchMode = "parallel"
)

// ExecuteFunc executes one produce call.
type ExecuteFunc func(context.Context, *produce.Message) (*produce.Result, error)

// Config controls async runtime lifecycle.
type Config struct {
	Mode        DispatchMode
	QueueSize   int
	ShardCount  int
	WorkerCount int
	Execute     ExecuteFunc
	ClosedErr   error
}

type queuedTask struct {
	message *produce.Message
	future  *Future
}

// Future carries one async produce result.
type Future struct {
	done chan struct{}

	once sync.Once
	res  *produce.Result
	err  error
}

// NewFuture creates a pending future.
func NewFuture() *Future {
	return &Future{done: make(chan struct{})}
}

// Resolve closes future with one result.
func (f *Future) Resolve(res *produce.Result, err error) {
	if f == nil {
		return
	}
	f.once.Do(func() {
		f.res = res
		f.err = err
		close(f.done)
	})
}

// Await waits for future completion or context cancellation.
func (f *Future) Await(ctx context.Context) (*produce.Result, error) {
	if f == nil {
		return nil, context.Canceled
	}
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.done:
		return f.res, f.err
	}
}

// Done returns closed channel when future resolves.
func (f *Future) Done() <-chan struct{} {
	if f == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return f.done
}

// Runtime manages async produce workers and queueing.
type Runtime struct {
	cfg Config

	ctx    context.Context
	cancel context.CancelFunc

	queues []chan *queuedTask
	wg     sync.WaitGroup

	nextQueue atomic.Uint64

	mu     sync.RWMutex
	closed bool

	closeOnce sync.Once
}

// NewRuntime starts async workers.
func NewRuntime(cfg Config) (*Runtime, error) {
	if cfg.Execute == nil {
		return nil, fmt.Errorf("execute is required")
	}
	if cfg.QueueSize <= 0 {
		return nil, fmt.Errorf("queue size must be > 0, got %d", cfg.QueueSize)
	}
	if cfg.ClosedErr == nil {
		cfg.ClosedErr = errors.New("producer runtime closed")
	}
	if cfg.Mode == "" {
		cfg.Mode = DispatchModeKeySharded
	}

	queueCount := 0
	switch cfg.Mode {
	case DispatchModeSerial:
		queueCount = 1
	case DispatchModeKeySharded:
		if cfg.ShardCount <= 0 {
			return nil, fmt.Errorf("shard count must be > 0, got %d", cfg.ShardCount)
		}
		queueCount = cfg.ShardCount
	case DispatchModeParallel:
		if cfg.WorkerCount <= 0 {
			return nil, fmt.Errorf("worker count must be > 0, got %d", cfg.WorkerCount)
		}
		queueCount = cfg.WorkerCount
	default:
		return nil, fmt.Errorf("unsupported dispatch mode %q", cfg.Mode)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rt := &Runtime{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		queues: make([]chan *queuedTask, queueCount),
	}
	for i := 0; i < queueCount; i++ {
		queue := make(chan *queuedTask, cfg.QueueSize)
		rt.queues[i] = queue
		rt.wg.Add(1)
		go rt.runWorker(queue)
	}
	return rt, nil
}

// Submit queues one async message and returns its future.
func (r *Runtime) Submit(ctx context.Context, msg *produce.Message) (*Future, error) {
	if r == nil {
		return nil, errors.New("producer runtime is nil")
	}
	if msg == nil {
		return nil, fmt.Errorf("producer message is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.RLock()
	closed := r.closed
	r.mu.RUnlock()
	if closed {
		return nil, r.cfg.ClosedErr
	}

	queueIdx := r.routeQueue(msg)
	future := NewFuture()
	task := &queuedTask{message: msg, future: future}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.ctx.Done():
		return nil, r.cfg.ClosedErr
	case r.queues[queueIdx] <- task:
		return future, nil
	}
}

func (r *Runtime) runWorker(queue <-chan *queuedTask) {
	defer r.wg.Done()

	for {
		select {
		case <-r.ctx.Done():
			return
		case task, ok := <-queue:
			if !ok {
				return
			}
			if task == nil || task.future == nil {
				continue
			}
			result, err := r.cfg.Execute(r.ctx, task.message)
			if err != nil && errors.Is(err, context.Canceled) {
				r.mu.RLock()
				isClosed := r.closed
				r.mu.RUnlock()
				if isClosed {
					err = r.cfg.ClosedErr
				}
			}
			task.future.Resolve(result, err)
		}
	}
}

func (r *Runtime) routeQueue(msg *produce.Message) int {
	switch r.cfg.Mode {
	case DispatchModeSerial:
		return 0
	case DispatchModeKeySharded:
		return router.ShardForKey(router.ProduceDispatchKey(msg), len(r.queues))
	case DispatchModeParallel:
		next := r.nextQueue.Add(1)
		return int(next % uint64(len(r.queues)))
	default:
		return 0
	}
}

// Close stops workers and resolves pending futures with closed error.
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}

	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()

		r.cancel()
		for _, queue := range r.queues {
			drainQueue(queue, r.cfg.ClosedErr)
		}
		r.wg.Wait()
	})
	return nil
}

func drainQueue(queue chan *queuedTask, err error) {
	for {
		select {
		case task := <-queue:
			if task == nil || task.future == nil {
				continue
			}
			task.future.Resolve(nil, err)
		default:
			return
		}
	}
}
