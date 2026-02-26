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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka"
	"github.com/codesjoy/pkg/basic/xkafka/examples/internal/examplecfg"
	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume"
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

	store := xkafka.NewMemoryOffsetStore()
	consumer, err := xkafka.NewPartitionConsumer(xkafka.PartitionConsumerConfig{
		Brokers:       cfg.Brokers,
		Topic:         cfg.Topic,
		Partition:     cfg.Partition,
		OffsetStore:   store,
		InitialOffset: sarama.OffsetOldest,
		Logger:        logger,
	})
	if err != nil {
		fail(fmt.Errorf("create partition consumer: %w", err))
	}
	defer closeOrLog(logger, "partition consumer", consumer.Close)

	logger.Info(
		"partition consumer started",
		"brokers", cfg.Brokers,
		"topic", cfg.Topic,
		"partition", cfg.Partition,
	)

	err = consumer.Consume(ctx, func(_ context.Context, msg *consume.MessageContext) error {
		if msg == nil || msg.Message == nil {
			return nil
		}
		logger.Info(
			"message received",
			"topic", msg.Message.Topic,
			"partition", msg.Message.Partition,
			"offset", msg.Message.Offset,
			"key", string(msg.Message.Key),
			"value", string(msg.Message.Value),
		)
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) {
		fail(fmt.Errorf("consume partition: %w", err))
	}

	nextOffset, found, loadErr := store.Load(context.Background(), cfg.Topic, cfg.Partition)
	switch {
	case loadErr != nil:
		logger.Error("load checkpoint failed", "error", loadErr)
	case found:
		logger.Info("partition checkpoint", "next_offset", nextOffset)
	default:
		logger.Info("partition checkpoint not found")
	}

	logger.Info("partition consumer stopped")
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
