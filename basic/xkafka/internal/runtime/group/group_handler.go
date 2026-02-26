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
type Config struct {
	ShardCount     int
	ShardQueueSize int

	ExtractLogicalKey router.ConsumeKeyExtractor
	ShardForKey       ShardRouter
	BuildChain        BuildChainFunc
	Business          consume.HandlerFunc
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
type Handler struct {
	cfg Config

	runtimeMu sync.RWMutex
	runtime   *sessionRuntime
}

// NewHandler creates a runtime-backed consumer group handler.
func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg.normalize()}
}

// Setup initializes one consume session runtime.
func (h *Handler) Setup(session sarama.ConsumerGroupSession) error {
	rt := newSessionRuntime(h.cfg, session)

	h.runtimeMu.Lock()
	h.runtime = rt
	h.runtimeMu.Unlock()
	return nil
}

// Cleanup waits for runtime workers and returns fatal errors if present.
func (h *Handler) Cleanup(_ sarama.ConsumerGroupSession) error {
	rt := h.getRuntime()
	if rt == nil {
		return nil
	}

	rt.shutdown()
	if err := rt.FatalErr(); err != nil {
		return err
	}
	return nil
}

// ConsumeClaim routes each message into shard workers.
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
			if err := rt.FatalErr(); err != nil {
				return err
			}
			if errors.Is(rt.ctx.Err(), context.Canceled) {
				return nil
			}
			return rt.ctx.Err()
		case msg, ok := <-claim.Messages():
			if !ok {
				if err := rt.FatalErr(); err != nil {
					return err
				}
				return nil
			}

			tracker := rt.trackerFor(msg.Topic, msg.Partition)
			tracker.Observe(msg.Offset)

			logicalKey, err := h.cfg.ExtractLogicalKey(msg)
			if err != nil {
				err = fmt.Errorf("extract logical key: %w", err)
				rt.setFatal(err)
				rt.cancel()
				return err
			}
			if logicalKey == "" {
				logicalKey = router.ConsumeFallbackKey(msg)
			}

			shard := h.cfg.ShardForKey(logicalKey)
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

			msgCtx := &consume.MessageContext{
				Message:    msg,
				LogicalKey: logicalKey,
				Shard:      shard,
				ReceivedAt: time.Now(),
			}

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
