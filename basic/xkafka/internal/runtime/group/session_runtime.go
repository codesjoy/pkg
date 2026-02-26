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

package group

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/offset"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

type queuedMessage struct {
	tracker *offset.Tracker
	msgCtx  *consume.MessageContext
}

type sessionRuntime struct {
	cfg     Config
	session sarama.ConsumerGroupSession

	ctx    context.Context
	cancel context.CancelFunc

	shards []chan *queuedMessage
	wg     sync.WaitGroup

	trackerMu sync.RWMutex
	trackers  map[string]*offset.Tracker

	chainMu sync.RWMutex
	chains  map[string]consume.HandlerFunc

	fatalMu  sync.RWMutex
	fatalErr error

	shutdownOnce sync.Once
}

func newSessionRuntime(cfg Config, session sarama.ConsumerGroupSession) *sessionRuntime {
	ctx, cancel := context.WithCancel(session.Context())
	rt := &sessionRuntime{
		cfg:      cfg,
		session:  session,
		ctx:      ctx,
		cancel:   cancel,
		shards:   make([]chan *queuedMessage, cfg.ShardCount),
		trackers: make(map[string]*offset.Tracker),
		chains:   make(map[string]consume.HandlerFunc),
	}

	for shard := 0; shard < cfg.ShardCount; shard++ {
		queue := make(chan *queuedMessage, cfg.ShardQueueSize)
		rt.shards[shard] = queue
		rt.wg.Add(1)
		go rt.runShardWorker(queue)
	}

	return rt
}

func (r *sessionRuntime) runShardWorker(queue <-chan *queuedMessage) {
	defer r.wg.Done()

	for {
		select {
		case <-r.ctx.Done():
			return
		case task, ok := <-queue:
			if !ok {
				return
			}
			if task == nil {
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

func (r *sessionRuntime) handleTask(task *queuedMessage) error {
	chain := r.chainForTopic(task.msgCtx.Message.Topic)
	if err := chain(r.ctx, task.msgCtx); err != nil {
		if errors.Is(err, context.Canceled) && r.ctx.Err() != nil {
			return nil
		}
		return err
	}

	nextOffset, advanced := task.tracker.MarkDone(task.msgCtx.Message.Offset)
	if advanced {
		r.session.MarkOffset(
			task.msgCtx.Message.Topic,
			task.msgCtx.Message.Partition,
			nextOffset,
			"",
		)
	}
	return nil
}

func (r *sessionRuntime) enqueue(task *queuedMessage) error {
	if task == nil {
		return nil
	}
	shardQueue := r.shards[task.msgCtx.Shard]

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case shardQueue <- task:
		return nil
	}
}

func (r *sessionRuntime) trackerFor(topic string, partition int32) *offset.Tracker {
	key := partitionKey(topic, partition)

	r.trackerMu.RLock()
	tracker, ok := r.trackers[key]
	r.trackerMu.RUnlock()
	if ok {
		return tracker
	}

	r.trackerMu.Lock()
	defer r.trackerMu.Unlock()
	if tracker, ok = r.trackers[key]; ok {
		return tracker
	}
	tracker = offset.NewTracker()
	r.trackers[key] = tracker
	return tracker
}

func (r *sessionRuntime) chainForTopic(topic string) consume.HandlerFunc {
	r.chainMu.RLock()
	chain, ok := r.chains[topic]
	r.chainMu.RUnlock()
	if ok {
		return chain
	}

	r.chainMu.Lock()
	defer r.chainMu.Unlock()
	if chain, ok = r.chains[topic]; ok {
		return chain
	}
	chain = r.cfg.BuildChain(topic, r.cfg.Business)
	r.chains[topic] = chain
	return chain
}

func (r *sessionRuntime) setFatal(err error) {
	if err == nil {
		return
	}

	r.fatalMu.Lock()
	defer r.fatalMu.Unlock()
	if r.fatalErr == nil {
		r.fatalErr = err
	}
}

// FatalErr returns fatal consume error, if any.
func (r *sessionRuntime) FatalErr() error {
	r.fatalMu.RLock()
	defer r.fatalMu.RUnlock()
	return r.fatalErr
}

func (r *sessionRuntime) shutdown() {
	r.shutdownOnce.Do(func() {
		r.cancel()
		for _, queue := range r.shards {
			close(queue)
		}
		r.wg.Wait()
	})
}

func partitionKey(topic string, partition int32) string {
	return fmt.Sprintf("%s:%d", topic, partition)
}
