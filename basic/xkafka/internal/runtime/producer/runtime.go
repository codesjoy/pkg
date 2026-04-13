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

// queuedTask 是异步队列中的待处理任务。
type queuedTask struct {
	// message 是待发送的消息。
	message *produce.Message
	// future 是异步结果句柄。
	future *Future
}

// Future carries one async produce result.
// 异步发送结果的 Future 句柄。
type Future struct {
	// done 在结果就绪时关闭。
	done chan struct{}

	// once 保证只关闭一次。
	once sync.Once
	// res 是发送成功的结果。
	res *produce.Result
	// err 是发送失败的错误。
	err error
}

// NewFuture creates a pending future.
func NewFuture() *Future {
	return &Future{done: make(chan struct{})}
}

// Resolve closes future with one result.
// 使用 sync.Once 保证只关闭一次。
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
// 等待 Future 完成，支持 context 取消。
func (f *Future) Await(ctx context.Context) (*produce.Result, error) {
	if f == nil {
		return nil, context.Canceled
	}
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		// context 取消
		return nil, ctx.Err()
	case <-f.done:
		// 结果就绪
		return f.res, f.err
	}
}

// Done returns closed channel when future resolves.
// nil 安全处理：nil Future 返回已关闭的 channel。
func (f *Future) Done() <-chan struct{} {
	if f == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return f.done
}

// Runtime manages async produce workers and queueing.
// 异步生产者运行时，管理工作协程和消息队列。
type Runtime struct {
	// cfg 是运行时配置。
	cfg Config

	// ctx 是运行时 context。
	ctx context.Context
	// cancel 取消运行时 context。
	cancel context.CancelFunc

	// queues 是工作队列列表。
	queues []chan *queuedTask
	// wg 跟踪所有工作协程的退出。
	wg sync.WaitGroup

	// nextQueue 用于并行轮询模式的原子计数器。
	nextQueue atomic.Uint64

	// mu 保护 closed 字段的读写。
	mu sync.RWMutex
	// closed 标记运行时是否已关闭。
	closed bool

	// closeOnce 保证 Close 只执行一次。
	closeOnce sync.Once
}

// NewRuntime starts async workers.
// 根据配置创建异步运行时：参数校验、计算队列数、创建工作协程。
func NewRuntime(cfg Config) (*Runtime, error) {
	// 参数校验
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

	// 根据分发模式计算队列数量
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

	// 创建运行时和工作协程
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
// 提交一条消息到异步队列，返回 Future 句柄用于获取结果。
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

	// 检查运行时是否已关闭
	r.mu.RLock()
	closed := r.closed
	r.mu.RUnlock()
	if closed {
		return nil, r.cfg.ClosedErr
	}

	// 路由到对应队列
	queueIdx := r.routeQueue(msg)
	future := NewFuture()
	task := &queuedTask{message: msg, future: future}

	// 入队等待
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.ctx.Done():
		return nil, r.cfg.ClosedErr
	case r.queues[queueIdx] <- task:
		return future, nil
	}
}

// runWorker 运行一个工作协程，从队列取任务并执行。
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
			// 执行发送
			result, err := r.cfg.Execute(r.ctx, task.message)
			// 将 context.Canceled 映射为 closed 错误
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

// routeQueue 根据分发模式路由到对应队列：
// Serial → 队列 0，KeySharded → 按键哈希取模，Parallel → 原子轮询。
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
// 标记关闭、取消 context、排空队列中待处理任务、等待工作协程退出。
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}

	r.closeOnce.Do(func() {
		// 标记为关闭
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()

		// 取消 context
		r.cancel()
		// 排空队列中待处理任务
		for _, queue := range r.queues {
			drainQueue(queue, r.cfg.ClosedErr)
		}
		// 等待工作协程退出
		r.wg.Wait()
	})
	return nil
}

// drainQueue 排空队列中的待处理任务，将其 Future 以错误结果解决。
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
