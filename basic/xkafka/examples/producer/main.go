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

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/examples/internal/examplecfg"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

func main() {
	cfg, err := examplecfg.Load()
	if err != nil {
		fail(fmt.Errorf("load config: %w", err))
	}

	logger := examplecfg.NewLogger()
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := examplecfg.WithTimeout(rootCtx, cfg.Timeout)
	defer cancel()

	producer, err := xkafka.NewProducer(xkafka.ProducerConfig{
		Brokers:      cfg.Brokers,
		DefaultTopic: cfg.Topic,
		Logger:       logger,
		Dispatch: xkafka.ProducerDispatchConfig{
			Mode:       xkafka.ProducerDispatchModeKeySharded,
			ShardCount: xkafka.DefaultShardCount,
			QueueSize:  xkafka.DefaultShardQueueSize,
		},
	})
	if err != nil {
		fail(fmt.Errorf("create producer: %w", err))
	}
	defer closeOrLog(logger, "producer", producer.Close)

	singleResult, err := producer.Produce(ctx, &produce.Message{
		Key:   []byte("order-1"),
		Value: []byte("single message"),
	})
	if err != nil {
		fail(fmt.Errorf("produce single message: %w", err))
	}
	logResult(logger, "single", singleResult)

	batchResults, err := producer.ProduceBatch(ctx,
		&produce.Message{Key: []byte("order-1"), Value: []byte("batch message A")},
		&produce.Message{Key: []byte("order-2"), Value: []byte("batch message B")},
	)
	if err != nil {
		fail(fmt.Errorf("produce batch: %w", err))
	}
	for idx := range batchResults {
		logResult(logger, fmt.Sprintf("batch[%d]", idx), batchResults[idx])
	}

	future, err := producer.ProduceAsync(ctx, &produce.Message{
		Key:   []byte("order-3"),
		Value: []byte("async message"),
	})
	if err != nil {
		fail(fmt.Errorf("produce async enqueue: %w", err))
	}

	asyncResult, err := future.Await(ctx)
	if err != nil {
		fail(fmt.Errorf("await async result: %w", err))
	}
	logResult(logger, "async", asyncResult)

	logger.Info("producer example completed")
}

func logResult(logger *slog.Logger, kind string, result *produce.Result) {
	if result == nil {
		logger.Warn("produce result is nil", "kind", kind)
		return
	}

	logger.Info(
		"produce succeeded",
		"kind", kind,
		"topic", result.Topic,
		"partition", result.Partition,
		"offset", result.Offset,
		"attempt", result.Attempt,
	)
}

func closeOrLog(logger *slog.Logger, name string, closeFn func() error) {
	if closeFn == nil {
		return
	}
	if err := closeFn(); err != nil {
		logger.Error("close failed", "resource", name, "error", err)
	}
}

func fail(err error) {
	if err == nil {
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
