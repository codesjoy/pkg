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

package jetstream

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"

	"github.com/codesjoy/pkg/basic/xnats/middleware/consume"
)

// HandleFunc handles one routed JetStream message.
type HandleFunc func(context.Context, *consume.MessageContext) error

type task struct {
	msgCtx *consume.MessageContext
}

// Runtime executes shard-routed consume callbacks with in-shard ordering.
type Runtime struct {
	ctx    context.Context
	cancel context.CancelFunc

	handle HandleFunc

	shards []chan *task
	wg     sync.WaitGroup

	fatalMu  sync.RWMutex
	fatalErr error

	shutdownOnce sync.Once
}

// New creates a new ordered consume runtime.
func New(
	callCtx context.Context,
	shardCount int,
	shardQueueSize int,
	handle HandleFunc,
) *Runtime {
	rtCtx, cancel := context.WithCancel(callCtx)
	rt := &Runtime{
		ctx:    rtCtx,
		cancel: cancel,
		handle: handle,
		shards: make([]chan *task, shardCount),
	}

	for shard := 0; shard < shardCount; shard++ {
		queue := make(chan *task, shardQueueSize)
		rt.shards[shard] = queue
		rt.wg.Add(1)
		go rt.runShardWorker(queue)
	}

	return rt
}

func (r *Runtime) runShardWorker(queue <-chan *task) {
	defer r.wg.Done()

	for {
		select {
		case <-r.ctx.Done():
			return
		case task, ok := <-queue:
			if !ok {
				return
			}
			if task == nil || task.msgCtx == nil {
				continue
			}
			if err := r.handle(r.ctx, task.msgCtx); err != nil {
				if errors.Is(err, context.Canceled) && r.ctx.Err() != nil {
					return
				}
				r.setFatal(err)
				r.cancel()
				return
			}
		}
	}
}

// Enqueue submits one message context to its precomputed shard.
func (r *Runtime) Enqueue(msgCtx *consume.MessageContext) error {
	if msgCtx == nil {
		return nil
	}

	shard := msgCtx.Shard
	if shard < 0 || shard >= len(r.shards) {
		return fmt.Errorf("invalid shard %d", shard)
	}

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.shards[shard] <- &task{msgCtx: msgCtx}:
		return nil
	}
}

func (r *Runtime) setFatal(err error) {
	if err == nil {
		return
	}

	r.fatalMu.Lock()
	if r.fatalErr == nil {
		r.fatalErr = err
	}
	r.fatalMu.Unlock()
}

// FatalErr returns the first fatal worker error, if any.
func (r *Runtime) FatalErr() error {
	r.fatalMu.RLock()
	defer r.fatalMu.RUnlock()
	return r.fatalErr
}

// Shutdown stops the runtime and waits for worker exit.
func (r *Runtime) Shutdown() {
	r.shutdownOnce.Do(func() {
		r.cancel()
		for _, queue := range r.shards {
			close(queue)
		}
		r.wg.Wait()
	})
}

// ErrorOr returns runtime fatal error when present, otherwise fallback.
func ErrorOr(r *Runtime, fallback error) error {
	if r != nil {
		if fatalErr := r.FatalErr(); fatalErr != nil {
			return fatalErr
		}
	}
	return fallback
}

// ShardForKey hashes key and maps it into [0, shardCount).
func ShardForKey(key string, shardCount int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(shardCount))
}
