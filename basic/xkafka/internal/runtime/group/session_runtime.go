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

// queuedMessage 是待处理的队列消息，携带 offset 跟踪器和消息上下文。
type queuedMessage struct {
	// tracker 是该消息所属分区的 offset 跟踪器。
	tracker *offset.Tracker
	// msgCtx 是消息的中间件链上下文。
	msgCtx *consume.MessageContext
}

// sessionRuntime 管理一次消费者组会话的分片工作协程和资源。
type sessionRuntime struct {
	// cfg 是运行时配置。
	cfg Config
	// session 是 Sarama 消费者组会话，用于提交 offset。
	session sarama.ConsumerGroupSession

	// ctx 是会话级 context，随 session 生命周期结束。
	ctx context.Context
	// cancel 取消会话 context。
	cancel context.CancelFunc

	// shards 是分片队列列表，每个队列对应一个工作协程。
	shards []chan *queuedMessage
	// wg 跟踪所有工作协程的退出。
	wg sync.WaitGroup

	// trackerMu 保护 trackers 字段的读写。
	trackerMu sync.RWMutex
	// trackers 是按 "topic:partition" 索引的 offset 跟踪器。
	trackers map[string]*offset.Tracker

	// chainMu 保护 chains 字段的读写。
	chainMu sync.RWMutex
	// chains 是按 topic 索引的中间件链缓存。
	chains map[string]consume.HandlerFunc

	// fatalMu 保护 fatalErr 字段的读写。
	fatalMu sync.RWMutex
	// fatalErr 是首次致命错误。
	fatalErr error

	// shutdownOnce 保证 shutdown 只执行一次。
	shutdownOnce sync.Once
}

// newSessionRuntime 创建新的会话运行时，初始化分片队列和工作协程。
func newSessionRuntime(cfg Config, session sarama.ConsumerGroupSession) *sessionRuntime {
	// 创建会话级 context，绑定到 session 的 context
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

	// 为每个分片创建队列和工作协程
	for shard := 0; shard < cfg.ShardCount; shard++ {
		queue := make(chan *queuedMessage, cfg.ShardQueueSize)
		rt.shards[shard] = queue
		rt.wg.Add(1)
		go rt.runShardWorker(queue)
	}

	return rt
}

// runShardWorker 运行一个分片工作协程，从队列中取任务并处理。
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
			// 处理任务，遇到错误设置致命错误并退出
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

// handleTask 处理一条消息：获取 topic 链、执行处理、连续 offset 提交。
func (r *sessionRuntime) handleTask(task *queuedMessage) error {
	// 获取 topic 对应的中间件链
	chain := r.chainForTopic(task.msgCtx.Message.Topic)
	// 执行中间件链
	if err := chain(r.ctx, task.msgCtx); err != nil {
		if errors.Is(err, context.Canceled) && r.ctx.Err() != nil {
			return nil
		}
		return err
	}

	// 标记 offset 完成并尝试提交连续前沿
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

// enqueue 将消息路由到对应分片的队列中。
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

// trackerFor 获取指定 topic:partition 的 offset 跟踪器，使用双检锁懒初始化。
func (r *sessionRuntime) trackerFor(topic string, partition int32) *offset.Tracker {
	key := partitionKey(topic, partition)

	// 快速路径：读锁检查
	r.trackerMu.RLock()
	tracker, ok := r.trackers[key]
	r.trackerMu.RUnlock()
	if ok {
		return tracker
	}

	// 慢速路径：写锁创建
	r.trackerMu.Lock()
	defer r.trackerMu.Unlock()
	if tracker, ok = r.trackers[key]; ok {
		return tracker
	}
	tracker = offset.NewTracker()
	r.trackers[key] = tracker
	return tracker
}

// chainForTopic 获取指定 topic 的中间件链，使用双检锁懒初始化。
func (r *sessionRuntime) chainForTopic(topic string) consume.HandlerFunc {
	// 快速路径：读锁检查
	r.chainMu.RLock()
	chain, ok := r.chains[topic]
	r.chainMu.RUnlock()
	if ok {
		return chain
	}

	// 慢速路径：写锁创建
	r.chainMu.Lock()
	defer r.chainMu.Unlock()
	if chain, ok = r.chains[topic]; ok {
		return chain
	}
	chain = r.cfg.BuildChain(topic, r.cfg.Business)
	r.chains[topic] = chain
	return chain
}

// setFatal 设置首个致命错误，后续调用不会覆盖。
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
