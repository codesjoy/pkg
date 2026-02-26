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

package sarama

import (
	"context"
	"fmt"
	"time"

	ibmsarama "github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/produce"
)

// SyncProducerConfig controls producer sender construction.
type SyncProducerConfig struct {
	Brokers  []string
	Config   *ibmsarama.Config
	Producer ibmsarama.SyncProducer
}

// SyncProducerSender is the transport adapter for Sarama SyncProducer.
type SyncProducerSender struct {
	producer ibmsarama.SyncProducer
	owned    bool
}

// NewSyncProducerSender creates one sender.
func NewSyncProducerSender(cfg SyncProducerConfig) (*SyncProducerSender, error) {
	if cfg.Producer != nil {
		return &SyncProducerSender{producer: cfg.Producer, owned: false}, nil
	}
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("brokers are required when sync producer is nil")
	}

	saramaCfg := ibmsarama.NewConfig()
	if cfg.Config != nil {
		saramaCfg = cfg.Config
	}
	saramaCfg.Producer.Return.Successes = true
	if err := saramaCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid sarama producer config: %w", err)
	}

	syncProducer, err := ibmsarama.NewSyncProducer(cfg.Brokers, saramaCfg)
	if err != nil {
		return nil, fmt.Errorf("create producer: %w", err)
	}
	return &SyncProducerSender{producer: syncProducer, owned: true}, nil
}

// Close closes owned producer resources.
func (s *SyncProducerSender) Close() error {
	if s == nil || !s.owned || s.producer == nil {
		return nil
	}
	return s.producer.Close()
}

// Send sends one message and returns broker placement result.
func (s *SyncProducerSender) Send(
	ctx context.Context,
	msg *produce.Message,
) (*produce.Result, error) {
	if s == nil || s.producer == nil {
		return nil, fmt.Errorf("sync producer is not configured")
	}
	if msg == nil {
		return nil, fmt.Errorf("producer message is nil")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	producerMsg := &ibmsarama.ProducerMessage{
		Topic:     msg.Topic,
		Key:       ibmsarama.ByteEncoder(msg.Key),
		Value:     ibmsarama.ByteEncoder(msg.Value),
		Timestamp: msg.Timestamp,
		Headers:   msg.Headers,
	}
	partition, offset, err := s.producer.SendMessage(producerMsg)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	timestamp := producerMsg.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	return &produce.Result{
		Topic:     msg.Topic,
		Partition: partition,
		Offset:    offset,
		Timestamp: timestamp,
	}, nil
}
