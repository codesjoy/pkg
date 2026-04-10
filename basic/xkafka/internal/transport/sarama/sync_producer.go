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
	"errors"
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

// SendBatchReport sends multiple messages using Sarama's batch API and returns
// one per-item outcome. It is an internal fast path and does not run higher
// level xkafka middleware or retry chains.
func (s *SyncProducerSender) SendBatchReport(
	ctx context.Context,
	msgs []*produce.Message,
) ([]produce.BatchItemResult, error) {
	if s == nil || s.producer == nil {
		return nil, fmt.Errorf("sync producer is not configured")
	}
	if len(msgs) == 0 {
		return nil, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	results := make([]produce.BatchItemResult, len(msgs))
	producerMsgs := make([]*ibmsarama.ProducerMessage, 0, len(msgs))
	for i, msg := range msgs {
		if msg == nil {
			results[i].Err = fmt.Errorf("producer message is nil")
			continue
		}

		producerMsgs = append(producerMsgs, &ibmsarama.ProducerMessage{
			Topic:     msg.Topic,
			Key:       ibmsarama.ByteEncoder(msg.Key),
			Value:     ibmsarama.ByteEncoder(msg.Value),
			Timestamp: msg.Timestamp,
			Headers:   msg.Headers,
			Metadata:  i,
		})
	}
	if len(producerMsgs) == 0 {
		return results, nil
	}

	if err := s.producer.SendMessages(producerMsgs); err != nil {
		var producerErrors ibmsarama.ProducerErrors
		if !errors.As(err, &producerErrors) {
			return nil, fmt.Errorf("send messages: %w", err)
		}
		for _, producerErr := range producerErrors {
			if producerErr == nil || producerErr.Msg == nil {
				return nil, fmt.Errorf("send messages: producer error missing message")
			}
			index, ok := producerErr.Msg.Metadata.(int)
			if !ok || index < 0 || index >= len(results) {
				return nil, fmt.Errorf("send messages: producer error missing index metadata")
			}
			results[index].Err = fmt.Errorf("send message: %w", producerErr.Err)
		}
	}

	for _, producerMsg := range producerMsgs {
		index, ok := producerMsg.Metadata.(int)
		if !ok || index < 0 || index >= len(results) {
			return nil, fmt.Errorf("send messages: producer message missing index metadata")
		}
		if results[index].Err != nil {
			continue
		}

		timestamp := producerMsg.Timestamp
		if timestamp.IsZero() {
			timestamp = time.Now()
		}
		results[index].Result = &produce.Result{
			Topic:     msgs[index].Topic,
			Partition: producerMsg.Partition,
			Offset:    producerMsg.Offset,
			Timestamp: timestamp,
		}
	}

	return results, nil
}
