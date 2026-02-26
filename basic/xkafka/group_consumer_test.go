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

package xkafka

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

func TestGroupConsumerConsumeLoop(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	group := &fakeConsumerGroup{
		consumeFn: func(ctx context.Context, _ []string, _ sarama.ConsumerGroupHandler) error {
			cancel()
			return nil
		},
	}

	cfg := defaultGroupConsumerConfig()
	cfg.Topics = []string{"orders"}
	consumer := &GroupConsumer{cfg: cfg, group: group}

	err := consumer.Consume(
		ctx,
		func(context.Context, *consume.MessageContext) error { return nil },
	)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, group.calls)
}

func TestGroupConsumerConsumeNilHandler(t *testing.T) {
	t.Parallel()

	cfg := defaultGroupConsumerConfig()
	cfg.Topics = []string{"orders"}
	consumer := &GroupConsumer{cfg: cfg, group: &fakeConsumerGroup{}}

	err := consumer.Consume(context.Background(), nil)
	require.ErrorIs(t, err, consume.ErrNilHandlerFunc)
}

func TestGroupConsumerCloseIdempotent(t *testing.T) {
	t.Parallel()

	group := &fakeConsumerGroup{closeErr: errors.New("close failed")}
	cfg := defaultGroupConsumerConfig()
	cfg.Topics = []string{"orders"}
	consumer := &GroupConsumer{cfg: cfg, group: group}

	err1 := consumer.Close()
	err2 := consumer.Close()
	require.EqualError(t, err1, "close failed")
	require.EqualError(t, err2, "close failed")
	require.Equal(t, 1, group.closeCalls)
}

func TestGroupConsumerStopPolicyNoCommit(t *testing.T) {
	t.Parallel()

	cfg := defaultGroupConsumerConfig()
	cfg.Topics = []string{"orders"}
	enabled := false
	cfg.LoggerHandlerEnabled = &enabled
	cfg.RetryConfig = RetryConfig{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}
	cfg.ExhaustedPolicy = ExhaustedPolicyStop

	sessionCtx := context.Background()
	session := newFakeSession(sessionCtx)
	claim := &fakeClaim{
		topic:     "orders",
		partition: 0,
		messages:  make(chan *sarama.ConsumerMessage, 1),
	}
	claim.messages <- &sarama.ConsumerMessage{Topic: "orders", Partition: 0, Offset: 0, Key: []byte("a")}

	group := &fakeConsumerGroup{
		consumeFn: func(_ context.Context, _ []string, handler sarama.ConsumerGroupHandler) error {
			if err := handler.Setup(session); err != nil {
				return err
			}
			if err := handler.ConsumeClaim(session, claim); err != nil {
				_ = handler.Cleanup(session)
				return err
			}
			return handler.Cleanup(session)
		},
	}

	consumer := &GroupConsumer{cfg: cfg, group: group}

	err := consumer.Consume(
		context.Background(),
		func(context.Context, *consume.MessageContext) error {
			return errors.New("failed permanently")
		},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exhausted retries")
	require.Equal(t, int64(-1), session.highest("orders", 0))
	require.Equal(t, 1, group.calls)
}

type fakeSession struct {
	ctx context.Context

	mu    sync.Mutex
	marks map[string][]int64
}

func newFakeSession(ctx context.Context) *fakeSession {
	return &fakeSession{ctx: ctx, marks: make(map[string][]int64)}
}

func (s *fakeSession) Claims() map[string][]int32 {
	return map[string][]int32{"orders": {0, 1}}
}

func (s *fakeSession) MemberID() string {
	return "member-1"
}

func (s *fakeSession) GenerationID() int32 {
	return 1
}

func (s *fakeSession) MarkOffset(topic string, partition int32, offset int64, _ string) {
	s.mu.Lock()
	s.marks[partitionKeyForTest(topic, partition)] = append(
		s.marks[partitionKeyForTest(topic, partition)],
		offset,
	)
	s.mu.Unlock()
}

func (s *fakeSession) ResetOffset(topic string, partition int32, offset int64, _ string) {
	s.MarkOffset(topic, partition, offset, "")
}

func (s *fakeSession) MarkMessage(msg *sarama.ConsumerMessage, _ string) {
	s.MarkOffset(msg.Topic, msg.Partition, msg.Offset+1, "")
}

func (s *fakeSession) Context() context.Context {
	return s.ctx
}

func (s *fakeSession) Commit() {}

func (s *fakeSession) highest(topic string, partition int32) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := partitionKeyForTest(topic, partition)
	items := s.marks[key]
	if len(items) == 0 {
		return -1
	}
	return items[len(items)-1]
}

type fakeClaim struct {
	topic     string
	partition int32
	messages  chan *sarama.ConsumerMessage
}

func (c *fakeClaim) Topic() string {
	return c.topic
}

func (c *fakeClaim) Partition() int32 {
	return c.partition
}

func (c *fakeClaim) InitialOffset() int64 {
	return 0
}

func (c *fakeClaim) HighWaterMarkOffset() int64 {
	return 0
}

func (c *fakeClaim) Messages() <-chan *sarama.ConsumerMessage {
	return c.messages
}

type fakeConsumerGroup struct {
	calls      int
	closeCalls int
	closeErr   error
	consumeFn  func(context.Context, []string, sarama.ConsumerGroupHandler) error
}

func (g *fakeConsumerGroup) Consume(
	ctx context.Context,
	topics []string,
	handler sarama.ConsumerGroupHandler,
) error {
	g.calls++
	if g.consumeFn == nil {
		return nil
	}
	return g.consumeFn(ctx, topics, handler)
}

func (g *fakeConsumerGroup) Errors() <-chan error { return nil }

func (g *fakeConsumerGroup) Pause(map[string][]int32) {}

func (g *fakeConsumerGroup) Resume(map[string][]int32) {}

func (g *fakeConsumerGroup) PauseAll() {}

func (g *fakeConsumerGroup) ResumeAll() {}

func (g *fakeConsumerGroup) Close() error {
	g.closeCalls++
	return g.closeErr
}

func partitionKeyForTest(topic string, partition int32) string {
	return fmt.Sprintf("%s:%d", topic, partition)
}
