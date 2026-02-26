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
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
)

func TestGroupConsumerHandlersForTopicModes(t *testing.T) {
	t.Parallel()

	newRecorder := func(name string, log *[]string) consume.Handler {
		return consume.Func(
			func(ctx context.Context, msg *consume.MessageContext, next consume.Next) error {
				*log = append(*log, name)
				return next(ctx, msg)
			},
		)
	}

	cfg := defaultGroupConsumerConfig()
	enabled := false
	cfg.LoggerHandlerEnabled = &enabled
	cfg.RetryConfig = RetryConfig{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}

	consumerInstance := &GroupConsumer{cfg: cfg}

	var appendOrder []string
	consumerInstance.cfg.GlobalHandlers = []consume.Handler{newRecorder("global", &appendOrder)}
	consumerInstance.cfg.TopicHandlers["append-topic"] = ConsumeTopicHandlers{
		Mode:     ChainModeAppend,
		Handlers: []consume.Handler{newRecorder("topic-append", &appendOrder)},
	}
	appendChain := consumerInstance.buildConsumeChain(
		"append-topic",
		func(context.Context, *consume.MessageContext) error {
			appendOrder = append(appendOrder, "business")
			return nil
		},
	)
	require.NoError(t, appendChain(context.Background(), &consume.MessageContext{}))
	require.Equal(t, []string{"global", "topic-append", "business"}, appendOrder)

	var replaceOrder []string
	consumerInstance.cfg.GlobalHandlers = []consume.Handler{newRecorder("global", &replaceOrder)}
	consumerInstance.cfg.TopicHandlers["replace-topic"] = ConsumeTopicHandlers{
		Mode:     ChainModeReplace,
		Handlers: []consume.Handler{newRecorder("topic-replace", &replaceOrder)},
	}
	replaceChain := consumerInstance.buildConsumeChain(
		"replace-topic",
		func(context.Context, *consume.MessageContext) error {
			replaceOrder = append(replaceOrder, "business")
			return nil
		},
	)
	require.NoError(t, replaceChain(context.Background(), &consume.MessageContext{}))
	require.Equal(t, []string{"topic-replace", "business"}, replaceOrder)
}

func TestGroupConsumerBuiltInRetryHandlerSetsAttempt(t *testing.T) {
	t.Parallel()

	cfg := defaultGroupConsumerConfig()
	enabled := false
	cfg.LoggerHandlerEnabled = &enabled
	cfg.RetryConfig = RetryConfig{
		MaxRetries:     InfiniteRetries,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}

	consumerInstance := &GroupConsumer{cfg: cfg}
	attempts := 0
	chain := consumerInstance.buildConsumeChain(
		"orders",
		func(context.Context, *consume.MessageContext) error {
			attempts++
			if attempts == 1 {
				return errors.New("retry once")
			}
			return nil
		},
	)

	msg := &consume.MessageContext{
		Message: &sarama.ConsumerMessage{Topic: "orders", Partition: 0, Offset: 1},
	}
	require.NoError(t, chain(context.Background(), msg))
	require.Equal(t, 2, attempts)
	require.Equal(t, 2, msg.Attempt)
}
