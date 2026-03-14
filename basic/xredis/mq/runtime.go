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

package mq

import (
	"context"
	"errors"
	"sync"

	"github.com/redis/go-redis/v9"
)

type queuedMessage struct {
	msgCtx *MessageContext
}

type consumeRuntime struct {
	client  redis.UniversalClient
	callCtx context.Context
	handler HandlerFunc

	ctx    context.Context
	cancel context.CancelFunc

	queues map[int]chan *queuedMessage
	wg     sync.WaitGroup

	fatalMu  sync.RWMutex
	fatalErr error

	shutdownOnce sync.Once
}

func newConsumeRuntime(
	callCtx context.Context,
	client redis.UniversalClient,
	queueIDs []int,
	queueSize int,
	handler HandlerFunc,
) *consumeRuntime {
	rtCtx, cancel := context.WithCancel(callCtx)
	rt := &consumeRuntime{
		client:  client,
		callCtx: callCtx,
		handler: handler,
		ctx:     rtCtx,
		cancel:  cancel,
		queues:  make(map[int]chan *queuedMessage, len(queueIDs)),
	}

	for _, shard := range queueIDs {
		queue := make(chan *queuedMessage, queueSize)
		rt.queues[shard] = queue
		rt.wg.Add(1)
		go rt.runShardWorker(queue)
	}

	return rt
}

func (r *consumeRuntime) enqueue(task *queuedMessage) error {
	if task == nil || task.msgCtx == nil {
		return nil
	}

	shard := task.msgCtx.Shard
	queue, ok := r.queues[shard]
	if !ok {
		return ErrConsumerClosed
	}

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case queue <- task:
		return nil
	}
}

func (r *consumeRuntime) runShardWorker(queue <-chan *queuedMessage) {
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
			if err := r.handleTask(task); err != nil {
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

func (r *consumeRuntime) handleTask(task *queuedMessage) error {
	if err := r.handler(r.callCtx, task.msgCtx); err != nil {
		if errors.Is(err, context.Canceled) && r.callCtx.Err() != nil {
			return r.callCtx.Err()
		}
		return err
	}

	if err := r.client.XAck(
		r.callCtx,
		ackStream(task.msgCtx),
		task.msgCtx.Group,
		task.msgCtx.ID,
	).Err(); err != nil {
		if errors.Is(err, context.Canceled) && r.callCtx.Err() != nil {
			return r.callCtx.Err()
		}
		return err
	}
	return nil
}

func ackStream(msgCtx *MessageContext) string {
	if msgCtx == nil {
		return ""
	}
	if msgCtx.ShardStream != "" {
		return msgCtx.ShardStream
	}
	return msgCtx.Stream
}

func (r *consumeRuntime) setFatal(err error) {
	if err == nil {
		return
	}

	r.fatalMu.Lock()
	defer r.fatalMu.Unlock()
	if r.fatalErr == nil {
		r.fatalErr = err
	}
}

func (r *consumeRuntime) fatal() error {
	r.fatalMu.RLock()
	defer r.fatalMu.RUnlock()
	return r.fatalErr
}

func (r *consumeRuntime) shutdown() {
	r.shutdownOnce.Do(func() {
		r.cancel()
		for _, queue := range r.queues {
			close(queue)
		}
		r.wg.Wait()
	})
}
