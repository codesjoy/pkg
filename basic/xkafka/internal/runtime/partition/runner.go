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
type Config struct {
	Topic     string
	Partition int32

	ShardCount     int
	ShardQueueSize int

	InitialOffset int64
	OffsetStore   OffsetStore

	ReconnectInitialBackoff time.Duration
	ReconnectMaxBackoff     time.Duration
	ReconnectMultiplier     float64

	ExtractLogicalKey router.ConsumeKeyExtractor
	BuildChain        BuildChainFunc
	Logger            *slog.Logger
}

// Runner manages one topic+partition consume loop with auto reconnect.
type Runner struct {
	consumer sarama.Consumer
	cfg      Config
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

	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := r.consumeOnce(ctx, business)
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

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

		if waitErr := backoff.Wait(ctx, wait); waitErr != nil {
			return waitErr
		}
	}
}

func (r *Runner) consumeOnce(ctx context.Context, business consume.HandlerFunc) error {
	startOffset, err := r.loadStartOffset(ctx)
	if err != nil {
		return err
	}

	pc, err := r.consumer.ConsumePartition(r.cfg.Topic, r.cfg.Partition, startOffset)
	if err != nil {
		return fmt.Errorf("consume partition: %w", err)
	}
	defer pc.AsyncClose()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	rt := newShardRuntime(
		runCtx,
		cancel,
		r.cfg.ShardCount,
		r.cfg.ShardQueueSize,
		r.cfg.BuildChain(r.cfg.Topic, business),
		newOffsetCommitter(r.cfg.Topic, r.cfg.Partition, r.cfg.OffsetStore),
	)
	defer rt.shutdown()

	errCh := pc.Errors()

	for {
		select {
		case <-runCtx.Done():
			return runtimeErrorOr(rt, runCtx.Err())
		case msg, ok := <-pc.Messages():
			if !ok {
				return runtimeErrorOr(rt, errors.New("partition consumer messages channel closed"))
			}

			rt.committer.observe(msg.Offset)

			logicalKey, err := r.cfg.ExtractLogicalKey(msg)
			if err != nil {
				rt.setFatal(fmt.Errorf("extract logical key: %w", err))
				cancel()
				continue
			}
			if logicalKey == "" {
				logicalKey = router.ConsumeFallbackKey(msg)
			}

			shard := router.ShardForKey(logicalKey, r.cfg.ShardCount)
			task := &queuedMessage{
				msgCtx: &consume.MessageContext{
					Message:    msg,
					LogicalKey: logicalKey,
					Shard:      shard,
					ReceivedAt: time.Now(),
				},
			}

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

func (r *Runner) loadStartOffset(ctx context.Context) (int64, error) {
	nextOffset, found, err := r.cfg.OffsetStore.Load(ctx, r.cfg.Topic, r.cfg.Partition)
	if err != nil {
		return 0, fmt.Errorf("load offset: %w", err)
	}
	if found {
		return nextOffset, nil
	}
	return r.cfg.InitialOffset, nil
}

type queuedMessage struct {
	msgCtx *consume.MessageContext
}

type shardRuntime struct {
	ctx    context.Context
	cancel context.CancelFunc

	chain     consume.HandlerFunc
	committer *offsetCommitter

	shards []chan *queuedMessage
	wg     sync.WaitGroup

	fatalMu  sync.RWMutex
	fatalErr error

	shutdownOnce sync.Once
}

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

	for shard := 0; shard < shardCount; shard++ {
		queue := make(chan *queuedMessage, shardQueueSize)
		rt.shards[shard] = queue
		rt.wg.Add(1)
		go rt.runShardWorker(queue)
	}

	return rt
}

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

func (r *shardRuntime) handleTask(task *queuedMessage) error {
	if err := r.chain(r.ctx, task.msgCtx); err != nil {
		if errors.Is(err, context.Canceled) && r.ctx.Err() != nil {
			return nil
		}
		return err
	}

	if err := r.committer.markDone(r.ctx, task.msgCtx.Message.Offset); err != nil {
		return err
	}
	return nil
}

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

func (r *shardRuntime) FatalErr() error {
	r.fatalMu.RLock()
	defer r.fatalMu.RUnlock()
	return r.fatalErr
}

func (r *shardRuntime) shutdown() {
	r.shutdownOnce.Do(func() {
		r.cancel()
		for _, queue := range r.shards {
			close(queue)
		}
		r.wg.Wait()
	})
}
