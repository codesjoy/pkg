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

	// Mark the consumer as active; prevents concurrent Consume calls.
	if err := c.activate(); err != nil {
		return err
	}
	defer c.deactivate()

	// Auto-create the consumer group on each shard stream if configured.
	if c.cfg.AutoCreateGroup {
		if err := c.ensureGroups(ctx, bindings); err != nil {
			return err
		}
	}

	// Create the shard-queue runtime when multiple shards are in use.
	runtime := c.newRuntime(ctx, handler)
	if runtime != nil {
		defer runtime.shutdown()
	}

	// Main consume loop: alternate between claiming pending messages and
	// reading new deliveries.
	for {
		// Check for cancellation, close, or runtime errors before each iteration.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.closedCh:
			return ErrConsumerClosed
		case <-runtimeDone(runtime):
			return c.runtimeDoneErr(ctx, runtime)
		default:
		}

		// First priority: claim pending (idle) messages from other consumers.
		claimed, err := c.claimPending(ctx, bindings)
		if err != nil {
			return err
		}
		if len(claimed) > 0 {
			if err := c.handleMessages(ctx, handler, claimed, true, runtime); err != nil {
				return err
			}
			continue
		}

		// Second priority: read new deliveries via XREADGROUP.
		deliveries, err := c.readDeliveries(ctx, bindings, bindingIndex, runtime)
		if err != nil {
			return err
		}
		if len(deliveries) == 0 {
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

// isClosed returns whether the consumer has been closed.
func (c *Consumer) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// orderedMode returns true when the consumer is using ordered shard ownership.
func (c *Consumer) orderedMode() bool {
	return len(c.cfg.OwnedShards) > 0
}

// runtimeQueueIDs returns the shard IDs that should back runtime queues.
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

// ensureGroups creates consumer groups on each shard stream, tolerating
// the BUSYGROUP error that indicates the group already exists.
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

// activate marks the consumer as active; returns an error if closed or already active.
func (c *Consumer) activate() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrConsumerClosed
	}
	if c.active {
		return ErrConsumerActive
	}

	c.active = true
	return nil
}

// deactivate clears the active flag when Consume returns.
func (c *Consumer) deactivate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = false
}

// newRuntime creates a consumeRuntime for multi-shard parallel processing.
// Returns nil when only a single shard is used (no queue needed).
func (c *Consumer) newRuntime(ctx context.Context, handler HandlerFunc) *consumeRuntime {
	queueIDs := c.runtimeQueueIDs()
	if len(queueIDs) <= 1 && !c.orderedMode() {
		return nil
	}

	runtime := newConsumeRuntime(ctx, c.client, queueIDs, c.cfg.ShardQueueSize, handler)
	go func() {
		select {
		case <-c.closedCh:
			runtime.cancel()
		case <-runtime.ctx.Done():
		}
	}()
	return runtime
}

// runtimeDoneErr determines the cause of a runtime shutdown.
func (c *Consumer) runtimeDoneErr(ctx context.Context, runtime *consumeRuntime) error {
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
}

// queuedDelivery pairs a raw Redis message with its stream binding.
type queuedDelivery struct {
	binding streamBinding
	raw     redis.XMessage
}

// claimPending uses XAUTOCLAIM to take over idle messages across all bindings.
func (c *Consumer) claimPending(
	ctx context.Context,
	bindings []streamBinding,
) ([]queuedDelivery, error) {
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

// readDeliveries reads new messages via XREADGROUP and maps them to their
// stream bindings.
func (c *Consumer) readDeliveries(
	ctx context.Context,
	bindings []streamBinding,
	bindingIndex map[string]streamBinding,
	runtime *consumeRuntime,
) ([]queuedDelivery, error) {
	streams, err := c.client.XReadGroup(c.readContext(ctx, runtime), &redis.XReadGroupArgs{
		Group:    c.cfg.Group,
		Consumer: c.cfg.Consumer,
		Streams:  readGroupStreams(bindings),
		Count:    c.cfg.Count,
		Block:    c.cfg.Block,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, c.waitForMore(ctx)
		}
		if runtime != nil && errors.Is(err, context.Canceled) {
			if runtimeErr := c.runtimeDoneErr(ctx, runtime); runtimeErr != nil {
				return nil, runtimeErr
			}
		}
		return nil, err
	}
	if len(streams) == 0 {
		return nil, c.waitForMore(ctx)
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
		return nil, c.waitForMore(ctx)
	}
	return deliveries, nil
}

// handleMessages dispatches deliveries to the handler either directly or
// through the shard-queue runtime.
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
			if err := c.ackDelivery(ctx, delivery); err != nil {
				return err
			}
			continue
		}
		if err := runtime.enqueue(&queuedMessage{msgCtx: msgCtx}); err != nil {
			return c.runtimeEnqueueErr(ctx, runtime, err)
		}
	}
	return nil
}

// buildMessageContext constructs a MessageContext from a raw delivery.
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

// logicalKey resolves the ordering key for a message.
func (c *Consumer) logicalKey(message *Message, fallback string) string {
	return resolveLogicalKey(messageHeaders(message), c.cfg.OrderKeyHeader, fallback)
}

// ackDelivery sends XACK for a single delivery.
func (c *Consumer) ackDelivery(ctx context.Context, delivery queuedDelivery) error {
	return c.client.XAck(
		ctx,
		delivery.binding.ShardStream,
		c.cfg.Group,
		delivery.raw.ID,
	).Err()
}

// readContext returns the runtime context when available, enabling graceful
// cancellation during XREADGROUP blocking waits.
func (c *Consumer) readContext(ctx context.Context, runtime *consumeRuntime) context.Context {
	if runtime == nil {
		return ctx
	}
	return runtime.ctx
}

// waitForMore sleeps briefly when no messages are available to avoid busy-looping.
func (c *Consumer) waitForMore(ctx context.Context) error {
	return sleepContext(ctx, c.cfg.IdleBackoff)
}

// runtimeEnqueueErr determines the root cause of an enqueue failure.
func (c *Consumer) runtimeEnqueueErr(
	ctx context.Context,
	runtime *consumeRuntime,
	err error,
) error {
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

// runtimeDone returns a channel that is closed when the runtime context ends.
func runtimeDone(runtime *consumeRuntime) <-chan struct{} {
	if runtime == nil {
		return nil
	}
	return runtime.ctx.Done()
}

// deliveryCount queries the pending entries list to get the retry count for
// a specific message.
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
