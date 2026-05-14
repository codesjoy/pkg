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
	"testing"
	"time"

	ibmsarama "github.com/IBM/sarama"
	"github.com/stretchr/testify/require"

	"github.com/codesjoy/pkg/basic/xkafka/middleware/consume/retry"
)

func TestNewDLQWriterUsesInjectedProducer(t *testing.T) {
	producer := &fakeSyncProducer{}

	writer, err := NewDLQWriter(DLQWriterConfig{
		Topic:    "orders.dlq",
		Producer: producer,
	})
	require.NoError(t, err)
	require.Equal(t, "orders.dlq", writer.topic)
	require.Same(t, producer, writer.producer)
	require.False(t, writer.owned)
}

func TestNewDLQWriterRejectsMissingBrokers(t *testing.T) {
	writer, err := NewDLQWriter(DLQWriterConfig{Topic: "orders.dlq"})
	require.Nil(t, writer)
	require.EqualError(t, err, "brokers are required when producer is nil")
}

func TestDLQWriterPublishValidatesInputs(t *testing.T) {
	err := (*DLQWriter)(nil).Publish(context.Background(), retry.Event{})
	require.EqualError(t, err, "dlq producer is not configured")

	writer := &DLQWriter{producer: &fakeSyncProducer{}}
	err = writer.Publish(context.Background(), retry.Event{})
	require.EqualError(t, err, "dlq message is nil")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = writer.Publish(ctx, retry.Event{
		Message: &ibmsarama.ConsumerMessage{Topic: "orders"},
		Err:     errors.New("failed"),
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestDLQWriterPublish(t *testing.T) {
	failedAt := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	producer := &fakeSyncProducer{
		sendMessageFn: func(msg *ibmsarama.ProducerMessage) (int32, int64, error) {
			require.Equal(t, "orders.dlq", msg.Topic)
			key, err := msg.Key.Encode()
			require.NoError(t, err)
			require.Equal(t, []byte("order-1"), key)
			value, err := msg.Value.Encode()
			require.NoError(t, err)
			require.Equal(t, []byte("payload"), value)
			require.Equal(t, failedAt, msg.Timestamp)
			return 0, 0, nil
		},
	}
	writer := &DLQWriter{topic: "orders.dlq", producer: producer}

	err := writer.Publish(context.Background(), retry.Event{
		Message: &ibmsarama.ConsumerMessage{
			Topic:     "orders",
			Partition: 2,
			Offset:    99,
			Key:       []byte("order-1"),
			Value:     []byte("payload"),
			Headers: []*ibmsarama.RecordHeader{
				{Key: []byte("trace"), Value: []byte("abc")},
				nil,
			},
		},
		LogicalKey: "order-1",
		Attempt:    3,
		Err:        errors.New("handler failed"),
		Timestamp:  failedAt,
	})
	require.NoError(t, err)
	require.NotNil(t, producer.lastMessage)

	headers := recordHeadersByKey(producer.lastMessage.Headers)
	require.Equal(t, "abc", headers["trace"])
	require.Equal(t, "orders", headers["x-source-topic"])
	require.Equal(t, "2", headers["x-source-partition"])
	require.Equal(t, "99", headers["x-source-offset"])
	require.Equal(t, "order-1", headers["x-logical-key"])
	require.Equal(t, "3", headers["x-attempt"])
	require.Equal(t, "handler failed", headers["x-error"])
	require.Equal(t, failedAt.Format(time.RFC3339Nano), headers["x-failed-at"])
}

func TestDLQWriterPublishWrapsProducerError(t *testing.T) {
	writer := &DLQWriter{
		topic: "orders.dlq",
		producer: &fakeSyncProducer{
			sendMessageFn: func(*ibmsarama.ProducerMessage) (int32, int64, error) {
				return 0, 0, errors.New("broker down")
			},
		},
	}

	err := writer.Publish(context.Background(), retry.Event{
		Message:   &ibmsarama.ConsumerMessage{Topic: "orders"},
		Err:       errors.New("handler failed"),
		Timestamp: time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC),
	})
	require.EqualError(t, err, "send dlq message: broker down")
}

func TestDLQWriterCloseOnlyOwnedProducer(t *testing.T) {
	producer := &fakeSyncProducer{}
	require.NoError(t, (&DLQWriter{producer: producer}).Close())
	require.Equal(t, 0, producer.closeCalls)

	writer := &DLQWriter{producer: producer, owned: true}
	require.NoError(t, writer.Close())
	require.Equal(t, 1, producer.closeCalls)
}

func TestCloneRecordHeaders(t *testing.T) {
	headers := []*ibmsarama.RecordHeader{
		{Key: []byte("k1"), Value: []byte("v1")},
		nil,
		{Key: []byte("k2"), Value: []byte("v2")},
	}

	cloned := cloneRecordHeaders(headers)
	require.Len(t, cloned, 2)
	require.Equal(t, []byte("k1"), cloned[0].Key)
	require.Equal(t, []byte("v1"), cloned[0].Value)
	require.Equal(t, []byte("k2"), cloned[1].Key)
	require.Equal(t, []byte("v2"), cloned[1].Value)

	headers[0].Key[0] = 'K'
	headers[0].Value[0] = 'V'
	require.Equal(t, []byte("k1"), cloned[0].Key)
	require.Equal(t, []byte("v1"), cloned[0].Value)
	require.Nil(t, cloneRecordHeaders(nil))
}

func recordHeadersByKey(headers []ibmsarama.RecordHeader) map[string]string {
	out := make(map[string]string, len(headers))
	for _, header := range headers {
		out[string(header.Key)] = string(header.Value)
	}
	return out
}
