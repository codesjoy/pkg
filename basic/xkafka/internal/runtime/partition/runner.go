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

package partition

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/backoff"
	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/router"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

// BuildChainFunc builds one topic consume chain around the business handler.
type BuildChainFunc func(topic string, business consume.HandlerFunc) consume.HandlerFunc

// Config controls partition-mode consume runtime.
// 分区消费运行时的配置。
type Config struct {
	// Topic 是消费的目标 topic。
	Topic string
	// Partition 是消费的目标分区号。
	Partition int32

	// ShardCount 是分片数量。
	ShardCount int
	// ShardQueueSize 是每个分片队列的缓冲区大小。
	ShardQueueSize int

	// InitialOffset 是首次消费时的起始 offset。
	InitialOffset int64
	// OffsetStore 是 offset 持久化存储。
	OffsetStore OffsetStore

	// ReconnectInitialBackoff 是首次重连等待时长。
	ReconnectInitialBackoff time.Duration
	// ReconnectMaxBackoff 是重连等待的最大时长。
	ReconnectMaxBackoff time.Duration
	// ReconnectMultiplier 是重连指数退避的乘数因子。
	ReconnectMultiplier float64

	// ExtractLogicalKey 从消息中提取逻辑键。
	ExtractLogicalKey router.ConsumeKeyExtractor
	// BuildChain 构建中间件链。
	BuildChain BuildChainFunc
	// Logger 是日志记录器。
	Logger *slog.Logger
}

// Runner manages one topic+partition consume loop with auto reconnect.
// 分区消费运行器，管理单个 topic+partition 的消费循环，支持自动重连。
type Runner struct {
	// consumer 是底层 Sarama 消费者。
	consumer sarama.Consumer
	// cfg 是运行时配置。
	cfg Config
}

// NewRunner creates a partition-mode runner.
func NewRunner(consumer sarama.Consumer, cfg Config) *Runner {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.ExtractLogicalKey == nil {
		cfg.ExtractLogicalKey = router.DefaultConsumeKeyExtractor
	}
	if cfg.BuildChain == nil {
		cfg.BuildChain = func(_ string, business consume.HandlerFunc) consume.HandlerFunc {
			return consume.Compose(nil, business)
		}
	}
	return &Runner{consumer: consumer, cfg: cfg}
}

// Consume starts one partition consume loop and auto reconnects on failures.
// 启动无限重连循环：每次失败后计算退避等待，然后重试。
func (r *Runner) Consume(ctx context.Context, business consume.HandlerFunc) error {
	if r == nil {
		return errors.New("partition runner is nil")
	}
	if business == nil {
		return consume.ErrNilHandlerFunc
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// 无限重连循环
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		// 执行一次消费
		err := r.consumeOnce(ctx, business)
		if err == nil {
			return nil
		}
		// context 取消时优先返回
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		// 计算退避等待时间
		wait := backoff.Exponential(
			r.cfg.ReconnectInitialBackoff,
			r.cfg.ReconnectMaxBackoff,
			r.cfg.ReconnectMultiplier,
			attempt,
		)
		r.cfg.Logger.WarnContext(ctx,
			"xkafka partition consume retrying",
			slog.String("topic", r.cfg.Topic),
			slog.Int64("partition", int64(r.cfg.Partition)),
			slog.Int("attempt", attempt),
			slog.Duration("backoff", wait),
			slog.String("error", err.Error()),
		)

		// 退避等待
		if waitErr := backoff.Wait(ctx, wait); waitErr != nil {
			return waitErr
		}
	}
}

// consumeOnce 执行一次分区消费，从加载 offset 到消息循环。
func (r *Runner) consumeOnce(ctx context.Context, business consume.HandlerFunc) error {
	// 加载起始 offset
	startOffset, err := r.loadStartOffset(ctx)
	if err != nil {
		return err
	}

	// 创建分区消费者
	pc, err := r.consumer.ConsumePartition(r.cfg.Topic, r.cfg.Partition, startOffset)
	if err != nil {
		return fmt.Errorf("consume partition: %w", err)
	}
	defer pc.AsyncClose()

	// 创建运行时 context
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 初始化 shard runtime
	rt := newShardRuntime(
		runCtx,
		cancel,
		r.cfg.ShardCount,
		r.cfg.ShardQueueSize,
		r.cfg.BuildChain(r.cfg.Topic, business),
		newOffsetCommitter(r.cfg.Topic, r.cfg.Partition, r.cfg.OffsetStore),
	)
	defer rt.shutdown()

	// 消费者错误通道
	errCh := pc.Errors()

	// 消息循环
	for {
		select {
		case <-runCtx.Done():
			return runtimeErrorOr(rt, runCtx.Err())
		case msg, ok := <-pc.Messages():
			if !ok {
				return runtimeErrorOr(rt, errors.New("partition consumer messages channel closed"))
			}

			// 观察 offset
			rt.committer.observe(msg.Offset)

			// 提取逻辑键
			logicalKey, err := r.cfg.ExtractLogicalKey(msg)
			if err != nil {
				rt.setFatal(fmt.Errorf("extract logical key: %w", err))
				cancel()
				continue
			}
			// 空值回退
			if logicalKey == "" {
				logicalKey = router.ConsumeFallbackKey(msg)
			}

			// 计算分片索引
			shard := router.ShardForKey(logicalKey, r.cfg.ShardCount)
			task := &queuedMessage{
				msgCtx: &consume.MessageContext{
					Message:    msg,
					LogicalKey: logicalKey,
					Shard:      shard,
					ReceivedAt: time.Now(),
				},
			}

			// 入队到分片 worker
			if err := rt.enqueue(task); err != nil {
				return runtimeErrorOr(rt, err)
			}
		case consumeErr, ok := <-errCh:
			if !ok || consumeErr == nil {
				continue
			}
			return fmt.Errorf("partition consumer error: %w", consumeErr.Err)
		}
	}
}

func runtimeErrorOr(rt *shardRuntime, fallback error) error {
	if fatalErr := rt.FatalErr(); fatalErr != nil {
		return fatalErr
	}
	return fallback
}

// loadStartOffset 从 offset store 加载起始 offset，未找到时使用配置的默认值。
func (r *Runner) loadStartOffset(ctx context.Context) (int64, error) {
	nextOffset, found, err := r.cfg.OffsetStore.Load(ctx, r.cfg.Topic, r.cfg.Partition)
	if err != nil {
		return 0, fmt.Errorf("load offset: %w", err)
	}
	if found {
		return nextOffset, nil
	}
	// 未找到已保存的 offset，使用配置的初始值
	return r.cfg.InitialOffset, nil
}

// queuedMessage 是分区模式下待处理的队列消息。
type queuedMessage struct {
	// msgCtx 是消息的中间件链上下文。
	msgCtx *consume.MessageContext
}

// shardRuntime 管理分区模式下的分片工作协程和资源。
type shardRuntime struct {
	// ctx 是运行时 context。
	ctx context.Context
	// cancel 取消运行时 context。
	cancel context.CancelFunc

	// chain 是该 topic 的中间件链。
	chain consume.HandlerFunc
	// committer 是 offset 提交器。
	committer *offsetCommitter

	// shards 是分片队列列表。
	shards []chan *queuedMessage
	// wg 跟踪所有工作协程的退出。
	wg sync.WaitGroup

	// fatalMu 保护 fatalErr 字段的读写。
	fatalMu sync.RWMutex
	// fatalErr 是首次致命错误。
	fatalErr error

	// shutdownOnce 保证 shutdown 只执行一次。
	shutdownOnce sync.Once
}

// newShardRuntime 创建 shard runtime，初始化分片队列和工作协程。
func newShardRuntime(
	ctx context.Context,
	cancel context.CancelFunc,
	shardCount int,
	shardQueueSize int,
	chain consume.HandlerFunc,
	committer *offsetCommitter,
) *shardRuntime {
	rt := &shardRuntime{
		ctx:       ctx,
		cancel:    cancel,
		chain:     chain,
		committer: committer,
		shards:    make([]chan *queuedMessage, shardCount),
	}

	// 为每个分片创建队列和工作协程
	for shard := 0; shard < shardCount; shard++ {
		queue := make(chan *queuedMessage, shardQueueSize)
		rt.shards[shard] = queue
		rt.wg.Add(1)
		go rt.runShardWorker(queue)
	}

	return rt
}

// runShardWorker 运行一个分片工作协程，从队列中取任务并处理。
func (r *shardRuntime) runShardWorker(queue <-chan *queuedMessage) {
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

// handleTask 处理一条消息：执行中间件链、标记 offset 完成。
func (r *shardRuntime) handleTask(task *queuedMessage) error {
	// 执行中间件链
	if err := r.chain(r.ctx, task.msgCtx); err != nil {
		if errors.Is(err, context.Canceled) && r.ctx.Err() != nil {
			return nil
		}
		return err
	}

	// 标记 offset 完成并尝试持久化
	if err := r.committer.markDone(r.ctx, task.msgCtx.Message.Offset); err != nil {
		return err
	}
	return nil
}

// enqueue 将消息路由到对应分片的队列中。
func (r *shardRuntime) enqueue(task *queuedMessage) error {
	if task == nil {
		return nil
	}

	shard := task.msgCtx.Shard
	if shard < 0 || shard >= len(r.shards) {
		return fmt.Errorf("invalid shard %d", shard)
	}

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.shards[shard] <- task:
		return nil
	}
}

// setFatal 设置首个致命错误，后续调用不会覆盖。
func (r *shardRuntime) setFatal(err error) {
	if err == nil {
		return
	}

	r.fatalMu.Lock()
	if r.fatalErr == nil {
		r.fatalErr = err
	}
	r.fatalMu.Unlock()
}

// FatalErr 返回致命错误（如果有）。
func (r *shardRuntime) FatalErr() error {
	r.fatalMu.RLock()
	defer r.fatalMu.RUnlock()
	return r.fatalErr
}

// shutdown 优雅关闭：取消 context、关闭所有分片队列、等待工作协程退出。
func (r *shardRuntime) shutdown() {
	r.shutdownOnce.Do(func() {
		r.cancel()
		for _, queue := range r.shards {
			close(queue)
		}
		r.wg.Wait()
	})
}
