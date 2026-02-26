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

	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

func TestProducerHandlersForTopicModes(t *testing.T) {
	t.Parallel()

	newRecorder := func(name string, log *[]string) produce.Handler {
		return produce.Func(
			func(ctx context.Context, msg *produce.MessageContext, next produce.Next) (*produce.Result, error) {
				*log = append(*log, name)
				return next(ctx, msg)
			},
		)
	}

	cfg := defaultProducerConfig()
	enabled := false
	cfg.LoggerHandlerEnabled = &enabled
	cfg.RetryConfig = RetryConfig{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}

	producerInstance := &Producer{cfg: cfg}

	var appendOrder []string
	producerInstance.cfg.GlobalHandlers = []produce.Handler{newRecorder("global", &appendOrder)}
	producerInstance.cfg.TopicHandlers["append-topic"] = ProduceTopicHandlers{
		Mode:     ChainModeAppend,
		Handlers: []produce.Handler{newRecorder("topic-append", &appendOrder)},
	}
	appendChain := producerInstance.buildProduceChain(
		"append-topic",
		func(context.Context, *produce.MessageContext) (*produce.Result, error) {
			appendOrder = append(appendOrder, "business")
			return &produce.Result{}, nil
		},
	)
	_, err := appendChain(
		context.Background(),
		&produce.MessageContext{Message: &produce.Message{Topic: "append-topic"}},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"global", "topic-append", "business"}, appendOrder)

	var replaceOrder []string
	producerInstance.cfg.GlobalHandlers = []produce.Handler{newRecorder("global", &replaceOrder)}
	producerInstance.cfg.TopicHandlers["replace-topic"] = ProduceTopicHandlers{
		Mode:     ChainModeReplace,
		Handlers: []produce.Handler{newRecorder("topic-replace", &replaceOrder)},
	}
	replaceChain := producerInstance.buildProduceChain(
		"replace-topic",
		func(context.Context, *produce.MessageContext) (*produce.Result, error) {
			replaceOrder = append(replaceOrder, "business")
			return &produce.Result{}, nil
		},
	)
	_, err = replaceChain(
		context.Background(),
		&produce.MessageContext{Message: &produce.Message{Topic: "replace-topic"}},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"topic-replace", "business"}, replaceOrder)
}

func TestProducerBuiltInRetryHandlerSetsAttempt(t *testing.T) {
	t.Parallel()

	cfg := defaultProducerConfig()
	enabled := false
	cfg.LoggerHandlerEnabled = &enabled
	cfg.RetryConfig = RetryConfig{
		MaxRetries:     InfiniteRetries,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1,
	}

	producerInstance := &Producer{cfg: cfg}
	attempts := 0
	chain := producerInstance.buildProduceChain(
		"orders",
		func(context.Context, *produce.MessageContext) (*produce.Result, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("retry once")
			}
			return &produce.Result{Topic: "orders", Partition: 0, Offset: 1}, nil
		},
	)

	msg := &produce.MessageContext{Message: &produce.Message{Topic: "orders"}}
	result, err := chain(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 2, attempts)
	require.Equal(t, 2, msg.Attempt)
	require.Equal(t, 2, result.Attempt)
}
