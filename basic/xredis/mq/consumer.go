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
	"time"

	"github.com/redis/go-redis/v9"
)

// Consumer wraps one Redis Streams consumer group loop.
type Consumer struct {
	client redis.UniversalClient
	cfg    ConsumerConfig

	mu        sync.Mutex
	active    bool
	closed    bool
	closedCh  chan struct{}
	closeOnce sync.Once
}

// NewConsumer creates a configured consumer wrapper.
func NewConsumer(client redis.UniversalClient, cfg ConsumerConfig) (*Consumer, error) {
	if isNilClient(client) {
		return nil, ErrNilClient
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Consumer{
		client:   client,
		cfg:      cfg,
		closedCh: make(chan struct{}),
	}, nil
}

// Consume starts consuming until ctx is done, Close is called, or handler returns an error.
func (c *Consumer) Consume(ctx context.Context, handler HandlerFunc) error {
	if c == nil {
		return ErrNilConsumer
	}
	if handler == nil {
		return ErrNilHandlerFunc
	}
	ctx = normalizeContext(ctx)
	bindings := consumerBindings(c.cfg)
	bindingIndex := bindingByStream(bindings)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrConsumerClosed
	}
	if c.active {
		c.mu.Unlock()
		return ErrConsumerActive
	}
	c.active = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.active = false
		c.mu.Unlock()
	}()

	if c.cfg.AutoCreateGroup {
		if err := c.ensureGroups(ctx, bindings); err != nil {
			return err
		}
	}

	var runtime *consumeRuntime
	if queueIDs := c.runtimeQueueIDs(); len(queueIDs) > 1 || c.orderedMode() {
		runtime = newConsumeRuntime(ctx, c.client, queueIDs, c.cfg.ShardQueueSize, handler)
		defer runtime.shutdown()

		go func() {
			select {
			case <-c.closedCh:
				runtime.cancel()
			case <-runtime.ctx.Done():
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.closedCh:
			return ErrConsumerClosed
		case <-runtimeDone(runtime):
			if err := runtime.fatal(); err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if c.isClosed() {
				return ErrConsumerClosed
			}
			return runtime.ctx.Err()
		default:
		}

		claimed, err := c.claimPending(ctx, bindings)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return err
		}
		if len(claimed) > 0 {
			if err := c.handleMessages(ctx, handler, claimed, true, runtime); err != nil {
				return err
			}
			continue
		}

		readCtx := ctx
		if runtime != nil {
			readCtx = runtime.ctx
		}
		streams, err := c.client.XReadGroup(readCtx, &redis.XReadGroupArgs{
			Group:    c.cfg.Group,
			Consumer: c.cfg.Consumer,
			Streams:  readGroupStreams(bindings),
			Count:    c.cfg.Count,
			Block:    c.cfg.Block,
		}).Result()
		if err != nil {
			if runtime != nil && errors.Is(err, context.Canceled) {
				if fatalErr := runtime.fatal(); fatalErr != nil {
					return fatalErr
				}
				if c.isClosed() {
					return ErrConsumerClosed
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
			}
			if errors.Is(err, redis.Nil) {
				if err := sleepContext(ctx, c.cfg.IdleBackoff); err != nil {
					return err
				}
				continue
			}
			return err
		}

		if len(streams) == 0 {
			if err := sleepContext(ctx, c.cfg.IdleBackoff); err != nil {
				return err
			}
			continue
		}
		deliveries := make([]queuedDelivery, 0, len(streams))
		for _, stream := range streams {
			binding, ok := bindingIndex[stream.Stream]
			if !ok {
				continue
			}
			for _, msg := range stream.Messages {
				deliveries = append(deliveries, queuedDelivery{binding: binding, raw: msg})
			}
		}
		if len(deliveries) == 0 {
			if err := sleepContext(ctx, c.cfg.IdleBackoff); err != nil {
				return err
			}
			continue
		}
		if err := c.handleMessages(ctx, handler, deliveries, false, runtime); err != nil {
			return err
		}
	}
}

// Close marks the consumer as closed and stops future Consume calls.
func (c *Consumer) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		close(c.closedCh)
		c.mu.Unlock()
	})
	return nil
}

func (c *Consumer) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *Consumer) orderedMode() bool {
	return len(c.cfg.OwnedShards) > 0
}

func (c *Consumer) runtimeQueueIDs() []int {
	if c.orderedMode() {
		return append([]int(nil), c.cfg.OwnedShards...)
	}
	queueIDs := make([]int, c.cfg.ShardCount)
	for shard := range queueIDs {
		queueIDs[shard] = shard
	}
	return queueIDs
}

func (c *Consumer) ensureGroups(ctx context.Context, bindings []streamBinding) error {
	for _, binding := range bindings {
		err := c.client.XGroupCreateMkStream(
			ctx,
			binding.ShardStream,
			c.cfg.Group,
			c.cfg.GroupStartID,
		).Err()
		if err == nil || isBusyGroupError(err) {
			continue
		}
		return err
	}
	return nil
}

type queuedDelivery struct {
	binding streamBinding
	raw     redis.XMessage
}

func (c *Consumer) claimPending(ctx context.Context, bindings []streamBinding) ([]queuedDelivery, error) {
	deliveries := make([]queuedDelivery, 0)
	for _, binding := range bindings {
		result, _, err := c.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   binding.ShardStream,
			Group:    c.cfg.Group,
			Consumer: c.cfg.Consumer,
			MinIdle:  c.cfg.AutoClaimMinIdle,
			Start:    "0-0",
			Count:    c.cfg.AutoClaimCount,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			return nil, err
		}
		for _, msg := range result {
			deliveries = append(deliveries, queuedDelivery{binding: binding, raw: msg})
		}
	}
	return deliveries, nil
}

func (c *Consumer) handleMessages(
	ctx context.Context,
	handler HandlerFunc,
	deliveries []queuedDelivery,
	claimed bool,
	runtime *consumeRuntime,
) error {
	for _, delivery := range deliveries {
		msgCtx, err := c.buildMessageContext(ctx, delivery.binding, delivery.raw, claimed)
		if err != nil {
			return err
		}
		if runtime == nil {
			if err := handler(ctx, msgCtx); err != nil {
				return err
			}
			if err := c.client.XAck(
				ctx,
				delivery.binding.ShardStream,
				c.cfg.Group,
				delivery.raw.ID,
			).Err(); err != nil {
				return err
			}
			continue
		}
		if err := runtime.enqueue(&queuedMessage{msgCtx: msgCtx}); err != nil {
			if fatalErr := runtime.fatal(); fatalErr != nil {
				return fatalErr
			}
			if c.isClosed() {
				return ErrConsumerClosed
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
	}
	return nil
}

func (c *Consumer) buildMessageContext(
	ctx context.Context,
	binding streamBinding,
	raw redis.XMessage,
	claimed bool,
) (*MessageContext, error) {
	deliveryCount, err := c.deliveryCount(ctx, binding.ShardStream, raw.ID)
	if err != nil {
		return nil, err
	}

	message := decodeMessage(binding.BaseStream, c.cfg.HeaderPrefix, c.cfg.PayloadField, raw)
	logicalKey := c.logicalKey(message, binding.BaseStream)
	shard := binding.Shard
	if !c.orderedMode() {
		shard = shardForKey(logicalKey, c.cfg.ShardCount)
	}

	return &MessageContext{
		Message:       message,
		BaseStream:    binding.BaseStream,
		ShardStream:   binding.ShardStream,
		Stream:        binding.BaseStream,
		ID:            raw.ID,
		Group:         c.cfg.Group,
		Consumer:      c.cfg.Consumer,
		LogicalKey:    logicalKey,
		Shard:         shard,
		DeliveryCount: deliveryCount,
		Claimed:       claimed,
		ReceivedAt:    time.Now(),
	}, nil
}

func (c *Consumer) logicalKey(message *Message, fallback string) string {
	return resolveLogicalKey(messageHeaders(message), c.cfg.OrderKeyHeader, fallback)
}

func runtimeDone(runtime *consumeRuntime) <-chan struct{} {
	if runtime == nil {
		return nil
	}
	return runtime.ctx.Done()
}

func (c *Consumer) deliveryCount(ctx context.Context, stream, id string) (int64, error) {
	pending, err := c.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: stream,
		Group:  c.cfg.Group,
		Start:  id,
		End:    id,
		Count:  1,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}
	return pending[0].RetryCount, nil
}
