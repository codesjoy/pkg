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
	"strconv"
	"time"

	ibmsarama "github.com/IBM/sarama"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume/retry"
)

// DLQWriterConfig controls DLQ writer construction.
// 死信队列写入器的构造配置。
type DLQWriterConfig struct {
	// Topic 是死信队列的目标 topic。
	Topic string
	// Producer 是外部传入的同步生产者，nil 时自动创建。
	Producer ibmsarama.SyncProducer
	// Brokers 是 Kafka 集群地址列表。
	Brokers []string
	// Config 是底层 Sarama 配置。
	Config *ibmsarama.Config
}

// DLQWriter publishes exhausted messages to a dead-letter topic.
// 死信队列写入器，将处理耗尽的消息发送到指定的 DLQ topic。
type DLQWriter struct {
	// topic 是死信队列的目标 topic。
	topic string
	// producer 是底层 Sarama 同步生产者。
	producer ibmsarama.SyncProducer
	// owned 标记是否由本模块创建（需要自行关闭）。
	owned bool
}

// NewDLQWriter creates one DLQ writer.
// 创建死信队列写入器：如果外部提供了 producer 则直接使用，否则自动创建。
func NewDLQWriter(cfg DLQWriterConfig) (*DLQWriter, error) {
	writer := &DLQWriter{topic: cfg.Topic}
	// 外部传入 producer，直接使用
	if cfg.Producer != nil {
		writer.producer = cfg.Producer
		return writer, nil
	}

	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("brokers are required when producer is nil")
	}

	// 创建新的同步生产者，使用高可靠配置
	producerCfg := ibmsarama.NewConfig()
	if cfg.Config != nil {
		producerCfg.Version = cfg.Config.Version
		producerCfg.ClientID = cfg.Config.ClientID
	}
	producerCfg.Producer.Return.Successes = true
	producerCfg.Producer.RequiredAcks = ibmsarama.WaitForAll
	producerCfg.Producer.Retry.Max = 3

	syncProducer, err := ibmsarama.NewSyncProducer(cfg.Brokers, producerCfg)
	if err != nil {
		return nil, fmt.Errorf("create dlq producer: %w", err)
	}

	writer.producer = syncProducer
	writer.owned = true
	return writer, nil
}

// Close closes owned producer resources.
func (w *DLQWriter) Close() error {
	if w == nil || !w.owned || w.producer == nil {
		return nil
	}
	return w.producer.Close()
}

// Publish sends one exhausted message into DLQ topic.
// 克隆原始消息头、追加诊断头信息、发送到 DLQ topic。
func (w *DLQWriter) Publish(ctx context.Context, event retry.Event) error {
	if w == nil || w.producer == nil {
		return fmt.Errorf("dlq producer is not configured")
	}
	if event.Message == nil {
		return fmt.Errorf("dlq message is nil")
	}

	// context 取消检查
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// 克隆原始消息头并追加诊断信息
	headers := cloneRecordHeaders(event.Message.Headers)
	headers = append(
		headers,
		ibmsarama.RecordHeader{Key: []byte("x-source-topic"), Value: []byte(event.Message.Topic)},
		ibmsarama.RecordHeader{
			Key:   []byte("x-source-partition"),
			Value: []byte(strconv.FormatInt(int64(event.Message.Partition), 10)),
		},
		ibmsarama.RecordHeader{
			Key:   []byte("x-source-offset"),
			Value: []byte(strconv.FormatInt(event.Message.Offset, 10)),
		},
		ibmsarama.RecordHeader{Key: []byte("x-logical-key"), Value: []byte(event.LogicalKey)},
		ibmsarama.RecordHeader{
			Key:   []byte("x-attempt"),
			Value: []byte(strconv.Itoa(event.Attempt)),
		},
		ibmsarama.RecordHeader{Key: []byte("x-error"), Value: []byte(event.Err.Error())},
		ibmsarama.RecordHeader{
			Key:   []byte("x-failed-at"),
			Value: []byte(event.Timestamp.UTC().Format(time.RFC3339Nano)),
		},
	)

	// 构建并发送 DLQ 消息
	producerMsg := &ibmsarama.ProducerMessage{
		Topic:     w.topic,
		Key:       ibmsarama.ByteEncoder(event.Message.Key),
		Value:     ibmsarama.ByteEncoder(event.Message.Value),
		Headers:   headers,
		Timestamp: event.Timestamp,
	}

	if _, _, err := w.producer.SendMessage(producerMsg); err != nil {
		return fmt.Errorf("send dlq message: %w", err)
	}
	return nil
}

// cloneRecordHeaders 深拷贝 Sarama RecordHeader 列表。
func cloneRecordHeaders(in []*ibmsarama.RecordHeader) []ibmsarama.RecordHeader {
	if len(in) == 0 {
		return nil
	}
	out := make([]ibmsarama.RecordHeader, 0, len(in))
	for _, header := range in {
		if header == nil {
			continue
		}
		out = append(out, ibmsarama.RecordHeader{
			Key:   append([]byte(nil), header.Key...),
			Value: append([]byte(nil), header.Value...),
		})
	}
	return out
}
