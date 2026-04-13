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
	"time"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/internal/primitives/router"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

// BuildChainFunc builds one topic consume chain around the business handler.
type BuildChainFunc func(topic string, business consume.HandlerFunc) consume.HandlerFunc

// ShardRouter maps one logical key to a shard index.
type ShardRouter func(logicalKey string) int

// Config describes session runtime dependencies.
// 消费者组会话运行时的配置。
type Config struct {
	// ShardCount 是分片数量。
	ShardCount int
	// ShardQueueSize 是每个分片队列的缓冲区大小。
	ShardQueueSize int

	// ExtractLogicalKey 从消息中提取逻辑键的函数。
	ExtractLogicalKey router.ConsumeKeyExtractor
	// ShardForKey 根据逻辑键计算分片索引的函数。
	ShardForKey ShardRouter
	// BuildChain 构建指定 topic 的中间件链。
	BuildChain BuildChainFunc
	// Business 是最终的业务处理函数。
	Business consume.HandlerFunc
}

func (cfg Config) normalize() Config {
	if cfg.ShardCount <= 0 {
		cfg.ShardCount = 1
	}
	if cfg.ShardQueueSize <= 0 {
		cfg.ShardQueueSize = 1
	}
	if cfg.ExtractLogicalKey == nil {
		cfg.ExtractLogicalKey = router.DefaultConsumeKeyExtractor
	}
	if cfg.ShardForKey == nil {
		cfg.ShardForKey = func(string) int { return 0 }
	}
	if cfg.BuildChain == nil {
		cfg.BuildChain = func(_ string, business consume.HandlerFunc) consume.HandlerFunc {
			return consume.Compose(nil, business)
		}
	}
	return cfg
}

// Handler implements Sarama ConsumerGroupHandler with sharded ordering.
// 实现 Sarama ConsumerGroupHandler 接口，使用分片队列保证消息顺序。
type Handler struct {
	// cfg 是运行时配置。
	cfg Config

	// runtimeMu 保护 runtime 字段的读写。
	runtimeMu sync.RWMutex
	// runtime 是当前会话的运行时实例。
	runtime *sessionRuntime
}

// NewHandler creates a runtime-backed consumer group handler.
func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg.normalize()}
}

// Setup initializes one consume session runtime.
// 在 Sarama rebalance 的 Setup 阶段创建新的会话运行时。
func (h *Handler) Setup(session sarama.ConsumerGroupSession) error {
	// 创建新的会话运行时
	rt := newSessionRuntime(h.cfg, session)

	h.runtimeMu.Lock()
	h.runtime = rt
	h.runtimeMu.Unlock()
	return nil
}

// Cleanup waits for runtime workers and returns fatal errors if present.
// 在 Sarama rebalance 的 Cleanup 阶段关闭运行时并检查致命错误。
func (h *Handler) Cleanup(_ sarama.ConsumerGroupSession) error {
	rt := h.getRuntime()
	if rt == nil {
		return nil
	}

	// 关闭运行时，等待工作协程退出
	rt.shutdown()
	// 检查是否有致命错误
	if err := rt.FatalErr(); err != nil {
		return err
	}
	return nil
}

// ConsumeClaim routes each message into shard workers.
// 消费消息循环：观察 offset、提取逻辑键、计算分片、构建上下文、入队。
func (h *Handler) ConsumeClaim(
	_ sarama.ConsumerGroupSession,
	claim sarama.ConsumerGroupClaim,
) error {
	rt := h.getRuntime()
	if rt == nil {
		return errors.New("session runtime is not initialized")
	}

	for {
		select {
		case <-rt.ctx.Done():
			// 运行时已关闭，检查致命错误
			if err := rt.FatalErr(); err != nil {
				return err
			}
			if errors.Is(rt.ctx.Err(), context.Canceled) {
				return nil
			}
			return rt.ctx.Err()
		case msg, ok := <-claim.Messages():
			if !ok {
				// 消息通道关闭，检查致命错误
				if err := rt.FatalErr(); err != nil {
					return err
				}
				return nil
			}

			// 观察 offset 用于连续前沿追踪
			tracker := rt.trackerFor(msg.Topic, msg.Partition)
			tracker.Observe(msg.Offset)

			// 提取逻辑键
			logicalKey, err := h.cfg.ExtractLogicalKey(msg)
			if err != nil {
				err = fmt.Errorf("extract logical key: %w", err)
				rt.setFatal(err)
				rt.cancel()
				return err
			}
			// 空值回退
			if logicalKey == "" {
				logicalKey = router.ConsumeFallbackKey(msg)
			}

			// 计算分片索引
			shard := h.cfg.ShardForKey(logicalKey)
			// 校验分片索引合法性
			if shard < 0 || shard >= len(rt.shards) {
				err = fmt.Errorf(
					"invalid shard index %d for key %q with shard count %d",
					shard,
					logicalKey,
					len(rt.shards),
				)
				rt.setFatal(err)
				rt.cancel()
				return err
			}

			// 构建 MessageContext
			msgCtx := &consume.MessageContext{
				Message:    msg,
				LogicalKey: logicalKey,
				Shard:      shard,
				ReceivedAt: time.Now(),
			}

			// 入队到分片 worker
			if err := rt.enqueue(&queuedMessage{tracker: tracker, msgCtx: msgCtx}); err != nil {
				if fatalErr := rt.FatalErr(); fatalErr != nil {
					return fatalErr
				}
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
		}
	}
}

// FatalErr returns fatal processing error in this consume session.
func (h *Handler) FatalErr() error {
	rt := h.getRuntime()
	if rt == nil {
		return nil
	}
	return rt.FatalErr()
}

func (h *Handler) getRuntime() *sessionRuntime {
	h.runtimeMu.RLock()
	defer h.runtimeMu.RUnlock()
	return h.runtime
}
